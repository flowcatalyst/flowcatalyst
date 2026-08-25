//go:build integration

package oauthapi

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	roledomain "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestAPIAccessInteractiveTokens pins the per-client apiAccess opt-in:
// a flagged client's authorization_code redemption mints an
// authority-bearing (token_use=api) access token narrowed to the client's
// applications — roles filtered, applications intersected,
// all_applications forced off, scope derived from the narrowed roles —
// while an unflagged client keeps identity-only tokens. Refresh re-mints
// through the same rule.
func TestAPIAccessInteractiveTokens(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	principals := principal.NewRepository(pool)
	roles := roledomain.NewRepository(pool)
	uow := testpg.NewUoW(t)

	// Two roles; the filter keeps only roleA (simulating "roleA belongs to
	// the client's app, roleB doesn't").
	roleAEv, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
		roleops.CreateCommand{ApplicationCode: "apiacc", RoleName: "role-a", DisplayName: "Role A"}, testpg.TestEC())
	require.NoError(t, err)
	roleBEv, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
		roleops.CreateCommand{ApplicationCode: "apiacc", RoleName: "role-b", DisplayName: "Role B"}, testpg.TestEC())
	require.NoError(t, err)

	userEv, err := usecaseop.Run(testpg.AnchorCtx(), uow, principalops.CreateUser(principals),
		principalops.CreateCommand{Email: "api-access@token.test", Scope: "ANCHOR"}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, principalops.AssignRoles(principals, roles),
		principalops.AssignRolesCommand{UserID: userEv.UserID, Roles: []string{roleAEv.Name, roleBEv.Name}}, testpg.TestEC())
	require.NoError(t, err)

	apiClient := &auth.OAuthClient{
		ClientID: "apiacc-app", Active: true, APIAccess: true,
		ApplicationIDs: []string{"app_apiacc"},
		RedirectURIs:   []string{"https://apiacc.test/cb"},
		GrantTypes:     []string{"authorization_code", "refresh_token"},
	}
	authCodes := grantstore.NewAuthorizationCodeRepository(pool)
	s := &State{
		OAuthClients:  fakeClientFinder{client: apiClient},
		Principals:    principals,
		AuthCodes:     authCodes,
		RefreshTokens: grantstore.NewRefreshTokenRepository(pool),
		Auth:          testAuthService(t),
		FilterRolesForApplications: func(_ context.Context, names, appIDs []string) ([]string, error) {
			require.Equal(t, []string{"app_apiacc"}, appIDs)
			out := make([]string, 0, 1)
			for _, n := range names {
				if n == roleAEv.Name {
					out = append(out, n)
				}
			}
			return out, nil
		},
		FlattenPermissions: func(_ context.Context, names []string) ([]string, error) {
			// The ceiling must be computed from the NARROWED role set.
			require.Equal(t, []string{roleAEv.Name}, names)
			return []string{"apiacc:widget:view"}, nil
		},
	}

	mint := func(scope string) string {
		raw := "apiacc-" + strings.ReplaceAll(scope, " ", "-") + "-code-xxxxxxxxxxxxxxxx"
		code := grantstore.NewAuthorizationCode(raw, "apiacc-app", userEv.UserID, "https://apiacc.test/cb")
		code.Scope = &scope
		require.NoError(t, authCodes.Insert(ctx, code))
		return raw
	}

	// apiAccess client → token_use=api, narrowed everywhere.
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {mint("openid offline_access")},
		"redirect_uri": {"https://apiacc.test/cb"},
		"client_id":    {"apiacc-app"},
	}
	rr := doTokenRequest(t, s, form)
	require.Equal(t, 200, rr.Code, rr.Body.String())
	var resp struct {
		AccessToken  string  `json:"access_token"`
		RefreshToken *string `json:"refresh_token"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	claims := decodeClaims(t, resp.AccessToken)
	assert.Equal(t, "api", claims["token_use"])
	assert.Equal(t, []any{roleAEv.Name}, claims["roles"], "roleB is filtered out")
	assert.Equal(t, "apiacc:widget:view", claims["scope"])
	assert.Equal(t, []any{"app_apiacc"}, claims["applications"])
	assert.Equal(t, false, claims["all_applications"])

	// Refresh re-mints through the same narrowing.
	require.NotNil(t, resp.RefreshToken)
	rr = doTokenRequest(t, s, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {*resp.RefreshToken},
		"client_id":     {"apiacc-app"},
	})
	require.Equal(t, 200, rr.Code, rr.Body.String())
	var refreshed struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &refreshed))
	rclaims := decodeClaims(t, refreshed.AccessToken)
	assert.Equal(t, "api", rclaims["token_use"])
	assert.Equal(t, []any{roleAEv.Name}, rclaims["roles"])

	// The same user through an UNFLAGGED client stays identity-only.
	plainClient := &auth.OAuthClient{
		ClientID: "apiacc-app", Active: true,
		RedirectURIs: []string{"https://apiacc.test/cb"},
		GrantTypes:   []string{"authorization_code"},
	}
	s2 := *s
	s2.OAuthClients = fakeClientFinder{client: plainClient}
	form.Set("code", mint("openid plain"))
	rr = doTokenRequest(t, &s2, form)
	require.Equal(t, 200, rr.Code, rr.Body.String())
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	claims = decodeClaims(t, resp.AccessToken)
	assert.Equal(t, "identity", claims["token_use"])
	assert.Equal(t, []any{}, claims["roles"])
}

// TestAPIAccessPortalConflict pins the plane rule: a portal client can never
// opt into authority-bearing tokens.
func TestAPIAccessPortalConflict(t *testing.T) {
	pool := testpg.Pool(t)
	authRepo := auth.NewRepository(pool)
	uow := testpg.NewUoW(t)

	owner := "clt_apiacc_owner"
	yes := true
	_, err := usecaseop.Run(testpg.AnchorCtx(), uow, authops.CreateOAuthClient(authRepo.OAuthClients),
		authops.CreateOAuthClientCommand{
			ClientName: "Conflicted", ClientType: "PUBLIC",
			PortalClientID: &owner, APIAccess: &yes,
		}, testpg.TestEC())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PORTAL_API_ACCESS_CONFLICT")
}
