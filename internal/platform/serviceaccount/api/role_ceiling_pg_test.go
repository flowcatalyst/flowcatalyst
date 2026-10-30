//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// The role ceiling (F8): a holder of service-account:update may add or remove
// only roles whose platform permissions it holds itself, and may mint a token
// only for an account whose roles it could have assigned. Before the fix any
// holder of the update permission could give an account super-admin and mint
// its token.
func TestServiceAccountRoles_BoundedByCallersPermissions(t *testing.T) {
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	roles := role.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)
	s := &State{
		Repo: saRepo, Principals: principals, Roles: roles, OAuthClients: oauthRepo,
		Clients: client.NewRepository(pool), Grants: principal.NewClientAccessGrantRepo(pool), UoW: uow,
		Auth: authservice.NewWithSecret(authservice.Config{
			SecretKey: "role-ceiling-test-secret-role-ceiling-test",
			Issuer:    "https://fc.test", Audience: "https://fc.test",
		}),
	}

	mkRole := func(name string, perms ...string) string {
		t.Helper()
		testpg.EnsureApplication(t, "ceilsa")
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
			roleops.CreateCommand{ApplicationCode: "ceilsa", RoleName: name, DisplayName: name, Permissions: perms},
			testpg.TestEC())
		require.NoError(t, err)
		return ev.Name
	}
	power := mkRole("power", "platform:iam:user:create", "platform:iam:user:view")
	viewer := mkRole("viewer", "platform:iam:user:view")
	appOnly := mkRole("app-only", "ceilsa:thing:view")
	iamWild := mkRole("iam-wild", "platform:iam:*:*")

	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo, s.Clients, s.Grants),
		operations.CreateCommand{Code: "role-ceiling-sa", Name: "Role Ceiling SA"}, testpg.TestEC())
	require.NoError(t, err)
	saID := res.ServiceAccount.ID

	callerWith := func(id string, perms ...string) context.Context {
		return auth.WithContext(context.Background(), &auth.AuthContext{
			PrincipalID: id, Scope: auth.ScopeAnchor, AllApplications: true,
			Permissions: append([]string{"platform:iam:service-account:update"}, perms...),
		})
	}
	viewerOnly := callerWith("prn_ceilviewer01", "platform:iam:user:view")
	subset := callerWith("prn_ceilsubset01", "platform:iam:user:view", "platform:iam:user:create")
	superAdmin := callerWith("prn_ceilsuper001", "platform:*:*:*")
	iamAdmin := callerWith("prn_ceiliamwild1", "platform:iam:*:*")

	assign := func(ctx context.Context, rs ...string) error {
		_, err := s.assignRoles(ctx, &assignRolesInput{ID: saID, Body: AssignRolesRequest{Roles: rs}})
		return err
	}
	aboveCaller := func(t *testing.T, err error, named string) {
		t.Helper()
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "expected a refusal, got %v", err)
		assert.Equal(t, usecase.KindAuthorization, ue.Kind)
		assert.Equal(t, "ROLE_ABOVE_CALLER", ue.Code)
		assert.Contains(t, ue.Message, named)
	}
	held := func() []string {
		t.Helper()
		p, err := principals.FindByServiceAccount(context.Background(), saID)
		require.NoError(t, err)
		out := []string{}
		for _, ra := range p.Roles {
			out = append(out, ra.Role.String())
		}
		return out
	}

	// A caller lacking a permission cannot grant a role holding it.
	aboveCaller(t, assign(viewerOnly, power), power)
	aboveCaller(t, assign(viewerOnly, viewer, power), power)
	assert.Empty(t, held(), "a refused assignment writes nothing")

	// A subset caller can; an application's own permissions are outside the
	// ceiling.
	require.NoError(t, assign(viewerOnly, viewer, appOnly))
	assert.ElementsMatch(t, []string{viewer, appOnly}, held())

	// A wildcard caller can, a role's own wildcard included when the held one
	// is as wide.
	require.NoError(t, assign(iamAdmin, viewer, iamWild))
	require.NoError(t, assign(superAdmin, viewer, power))
	assert.ElementsMatch(t, []string{viewer, power}, held())

	// Removal counts: a lesser caller cannot strip a role above it; keeping it
	// and changing others is fine.
	aboveCaller(t, assign(viewerOnly, viewer), power)
	require.NoError(t, assign(viewerOnly, power, viewer, appOnly))
	require.NoError(t, assign(subset, power, viewer))

	// Mint: the token carries the account's authority, so the caller must hold it.
	_, err = s.mintToken(viewerOnly, &apicommon.IDInput{ID: saID})
	aboveCaller(t, err, power)
	_, err = s.mintToken(subset, &apicommon.IDInput{ID: saID})
	require.NoError(t, err)
	_, err = s.mintToken(superAdmin, &apicommon.IDInput{ID: saID})
	require.NoError(t, err)
}
