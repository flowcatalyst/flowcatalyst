//go:build integration

package sdksync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// Application confinement (F12) on POST /api/applications/{appCode}/principals/sync:
// the route may hand out only the application's own roles, whoever the caller,
// 403 ROLE_APP_FORBIDDEN otherwise (Java requireSyncableRoles, Rust
// require_syncable_roles). Before the fix it accepted any role name: another
// application's roles, and platform roles the F8 ceiling let through (one
// holding no platform permission, or any for a caller holding them).
func TestSyncPrincipals_AppRouteOnlyTheApplicationsRoles(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	roles := role.NewRepository(pool)
	principals := principal.NewRepository(pool)

	const appCode, otherCode = "rolefbsdk", "rolefbother"
	apps := application.NewRepository(pool)
	app, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(apps),
		appops.CreateCommand{Code: appCode, Name: "Role app SDK"}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(apps),
		appops.CreateCommand{Code: otherCode, Name: "Role app other"}, testpg.TestEC())
	require.NoError(t, err)
	mkRole := func(app, name string, perms ...string) string {
		t.Helper()
		testpg.EnsureApplication(t, app)
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
			roleops.CreateCommand{ApplicationCode: app, RoleName: name, DisplayName: name, Permissions: perms},
			testpg.TestEC())
		require.NoError(t, err)
		return ev.Name
	}
	clerk := mkRole(appCode, "clerk", appCode+":order:view")
	foreign := mkRole(otherCode, "clerk", otherCode+":order:view")
	platformRole := mkRole("platform", "rolefb-empty") // no permissions: below every ceiling

	s := &State{Apps: apps, Principals: principals, Roles: roles, UoW: uow}
	as := func(perms ...string) context.Context {
		return auth.WithContext(ctx, &auth.AuthContext{
			PrincipalID: "prn_rolefbsdk_c01", Scope: auth.ScopeAnchor,
			Applications: []string{app.ApplicationID},
			Permissions:  perms,
		})
	}
	caller := as("platform:iam:user:update")
	sync := func(c context.Context, email string, rs ...string) error {
		_, err := s.syncPrincipals(c, &syncPrincipalsInput{AppCode: appCode, Body: syncPrincipalsRequest{
			Principals: []syncPrincipalInputRequest{{Email: email, Name: email, Roles: rs}},
		}})
		return err
	}
	forbidden := func(label string, err error, roleName string) {
		t.Helper()
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "%s: want a refusal, got %v", label, err)
		assert.Equal(t, usecase.KindAuthorization, ue.Kind, label)
		assert.Equal(t, "ROLE_APP_FORBIDDEN", ue.Code, label)
		assert.Equal(t, "role '"+roleName+"' does not belong to application '"+appCode+"'", ue.Message, label)
	}
	absent := func(email string) {
		t.Helper()
		p, err := principals.FindByEmail(ctx, email)
		require.NoError(t, err)
		assert.Nil(t, p, "a refused sync writes nothing: %s", email)
	}

	forbidden("another application's role", sync(caller, "a@rolefbsdk.test", clerk, foreign), foreign)
	absent("a@rolefbsdk.test")
	forbidden("a platform role", sync(caller, "b@rolefbsdk.test", platformRole), platformRole)
	absent("b@rolefbsdk.test")
	// A name that is not a role, even one prefixed for another application, is
	// skipped instead since F15 (sync_principals_unknown_roles_pg_test.go).
	forbidden("a super-admin caller too", sync(as("platform:*:*:*"), "d@rolefbsdk.test", foreign), foreign)
	absent("d@rolefbsdk.test")
	forbidden("mixed case is lowered first", sync(caller, "e@rolefbsdk.test", "RoleFbOther:Clerk"), foreign)

	// The legitimate flow: the application's own roles; names that are not
	// roles are not refused, and since F15 not assigned either.
	require.NoError(t, sync(caller, "ok@rolefbsdk.test", clerk, appCode+":not-yet-synced", "unprefixed"))
	p, err := principals.FindByEmail(ctx, "ok@rolefbsdk.test")
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, []string{clerk}, roleNamesOfAssignments(p))
}
