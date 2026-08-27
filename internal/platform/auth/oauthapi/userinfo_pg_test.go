//go:build integration

package oauthapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	roledomain "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestUserinfoAnswersFromThePrincipal covers the endpoint's whole point: an
// interactive login holds an IDENTITY-ONLY access token, so echoing that
// token's claims back made userinfo assert "this user holds nothing" on every
// call. It now recomputes from the principal, confined to the calling client's
// applications via the token's azp — the same confinement the id_token got.
func TestUserinfoAnswersFromThePrincipal(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	principals := principal.NewRepository(pool)
	roles := roledomain.NewRepository(pool)
	uow := testpg.NewUoW(t)

	mine, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
		roleops.CreateCommand{ApplicationCode: "uinfo", RoleName: "insider", DisplayName: "Insider"}, testpg.TestEC())
	require.NoError(t, err)
	theirs, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
		roleops.CreateCommand{ApplicationCode: "elsewhere", RoleName: "outsider", DisplayName: "Outsider"}, testpg.TestEC())
	require.NoError(t, err)

	userEv, err := usecaseop.Run(testpg.AnchorCtx(), uow, principalops.CreateUser(principals),
		principalops.CreateCommand{Email: "userinfo@token.test", Scope: "ANCHOR"}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, principalops.AssignRoles(principals, roles),
		principalops.AssignRolesCommand{UserID: userEv.UserID, Roles: []string{mine.Name, theirs.Name}}, testpg.TestEC())
	require.NoError(t, err)

	client := &auth.OAuthClient{
		ClientID: "uinfo-rp", Active: true,
		ApplicationIDs: []string{"app_uinfo"},
	}
	s := &State{
		OAuthClients: fakeClientFinder{client: client},
		Principals:   principals,
		Auth:         testAuthService(t),
		FilterRolesForApplications: func(_ context.Context, names, _ []string) ([]string, error) {
			out := make([]string, 0, 1)
			for _, n := range names {
				if n == mine.Name {
					out = append(out, n)
				}
			}
			return out, nil
		},
	}

	p, err := principals.FindByID(ctx, userEv.UserID)
	require.NoError(t, err)

	call := func(t *testing.T, token string) userInfoResponse {
		t.Helper()
		r := chi.NewRouter()
		s.RegisterUserinfoRoutes(r)
		req := httptest.NewRequest("GET", "/oauth/userinfo", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, 200, rec.Code, rec.Body.String())
		var resp userInfoResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return resp
	}

	t.Run("identity token gets real roles, confined to the client's app", func(t *testing.T) {
		tok, err := s.Auth.GenerateIdentityAccessTokenFor(p, client.ClientID)
		require.NoError(t, err)
		resp := call(t, tok)

		assert.Equal(t, []string{mine.Name}, resp.Roles,
			"the other application's role must not be disclosed")
		assert.Equal(t, p.ID, resp.Sub)
		assert.Empty(t, resp.Scope, "an identity token grants no scope; that is not recomputed")
	})

	t.Run("no azp falls back to the token's own claims", func(t *testing.T) {
		// A session token or client_credentials token has no relying party to
		// confine to, so there is nothing to key confinement on.
		tok, err := s.Auth.GenerateIdentityAccessToken(p)
		require.NoError(t, err)
		resp := call(t, tok)
		assert.ElementsMatch(t, []string{mine.Name, theirs.Name}, resp.Roles,
			"unconfined: the full role list, recomputed from the principal")
	})
}
