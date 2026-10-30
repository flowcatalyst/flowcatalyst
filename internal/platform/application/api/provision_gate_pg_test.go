//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// ensureApplicationServiceRole installs the built-in role the provisioning
// grants, with the seed's permissions, when the test database lacks it.
func ensureApplicationServiceRole(t *testing.T, roles *role.Repository, uow *usecasepgx.UnitOfWork) {
	t.Helper()
	existing, err := roles.FindByName(context.Background(), "platform:application-service")
	require.NoError(t, err)
	if existing != nil {
		return
	}
	testpg.EnsureApplication(t, "platform")
	for _, r := range seed.PlatformRoles() {
		if r.Name != "platform:application-service" {
			continue
		}
		_, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
			roleops.CreateCommand{
				ApplicationCode: "platform", RoleName: "application-service",
				DisplayName: r.DisplayName, Permissions: r.Permissions,
			}, testpg.TestEC())
		require.NoError(t, err)
		return
	}
	t.Fatal("seed has no platform:application-service role")
}

// Provisioning creates a service account holding platform:application-service
// and hands out its client secret. It was anchor-only: any anchor principal,
// an application's own service account included, could provision an account
// for any application. It now needs service-account:create and
// application:update (Java, Rust) and, by the role ceiling (F8), the
// application-service role's platform permissions. fcdev init runs the
// operation as the system, with no caller, and is not bounded.
func TestProvisionServiceAccount_GateAndRoleCeiling(t *testing.T) {
	pool := testpg.Pool(t)
	repo := application.NewRepository(pool)
	roles := role.NewRepository(pool)
	uow := testpg.NewUoW(t)
	ensureApplicationServiceRole(t, roles, uow)
	s := &State{
		Repo: repo, Roles: roles, Principals: principal.NewRepository(pool),
		ServiceAccounts: serviceaccount.NewRepository(pool),
		OAuthClients:    platformauth.NewRepository(pool).OAuthClients, UoW: uow,
	}
	mkApp := func(code string) string {
		t.Helper()
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateApplication(repo),
			operations.CreateCommand{Code: code, Name: code}, testpg.TestEC())
		require.NoError(t, err)
		return ev.ApplicationID
	}
	caller := func(perms ...string) context.Context {
		return auth.WithContext(context.Background(), &auth.AuthContext{
			PrincipalID: "prn_provgate00001", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: perms,
		})
	}
	refusedWith := func(t *testing.T, err error, code string) {
		t.Helper()
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "want %s, got %v", code, err)
		assert.Equal(t, usecase.KindAuthorization, ue.Kind)
		assert.Equal(t, code, ue.Code)
	}
	appID := mkApp("provgate-app")
	in := &apicommon.IDInput{ID: appID}

	// Anchor reach alone (a read-only staff role, an application's own
	// service account) no longer provisions.
	_, err := s.provisionServiceAccount(caller("platform:admin:application:view"), in)
	refusedWith(t, err, "PERMISSION_REQUIRED")
	_, err = s.provisionServiceAccount(caller("platform:iam:service-account:create"), in)
	refusedWith(t, err, "PERMISSION_REQUIRED")

	// Both permissions but not the role's: the ceiling refuses.
	_, err = s.provisionServiceAccount(caller(
		"platform:iam:service-account:create", "platform:admin:application:update"), in)
	refusedWith(t, err, "ROLE_ABOVE_CALLER")
	app, err := repo.FindByID(context.Background(), appID)
	require.NoError(t, err)
	assert.Nil(t, app.ServiceAccountID, "nothing was provisioned")

	// A super-admin provisions.
	out, err := s.provisionServiceAccount(caller("platform:*:*:*"), in)
	require.NoError(t, err)
	assert.NotEmpty(t, out.Body.ServiceAccount.OAuthClient.ClientSecret)

	// The system path (fcdev init) runs the operation with no caller.
	other := mkApp("provgate-system")
	res, err := usecaseop.RunTx(context.Background(), uow,
		operations.ProvisionServiceAccount(repo, s.ServiceAccounts, s.Principals, s.OAuthClients),
		operations.ProvisionServiceAccountCommand{ApplicationID: other}, usecase.NewExecutionContext("prn_bootstrap001"))
	require.NoError(t, err)
	assert.NotEmpty(t, res.OAuthClientSecret)
}
