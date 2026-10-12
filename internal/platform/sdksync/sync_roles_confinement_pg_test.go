//go:build integration

package sdksync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// Confinement (F12) on POST /api/applications/{appCode}/roles/sync: an
// application's roles hold only its own permissions. Before the fix the route
// stored whatever the body carried, so an application could put another
// application's permissions, or a platform permission its caller holds, on its
// roles. The SDK sync never gets the super-admin exception. Only what the sync
// adds is checked: a role that already holds a foreign permission re-syncs.
func TestSyncRoles_PermissionsConfinedToTheApplication(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	roles := role.NewRepository(pool)

	const appCode = "confinesync"
	app, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(application.NewRepository(pool)),
		appops.CreateCommand{Code: appCode, Name: "Confinement role sync"}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{Apps: application.NewRepository(pool), Roles: roles, UoW: uow}
	syncAs := func(perms []string, rs ...syncRoleInputRequest) error {
		c := auth.WithContext(ctx, &auth.AuthContext{
			PrincipalID: "prn_confinesync01", Scope: auth.ScopeAnchor,
			Applications: []string{app.ApplicationID},
			Permissions:  perms,
		})
		_, err := s.syncRoles(c, &syncRolesInput{AppCode: appCode, Body: syncRolesRequest{Roles: rs}})
		return err
	}
	sdk := []string{"platform:application-service:role:create", "platform:application-service:role:update"}
	refused := func(label string, err error, permission string) {
		t.Helper()
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "%s: want a refusal, got %v", label, err)
		assert.Equal(t, usecase.KindValidation, ue.Kind, label)
		assert.Equal(t, "PERMISSION_OUTSIDE_APPLICATION", ue.Code, label)
		assert.Equal(t, "Permission '"+permission+"' does not belong to application '"+appCode+
			"'; a role may only hold its own application's permissions", ue.Message, label)
	}
	permsOf := func(name string) []string {
		t.Helper()
		r, err := roles.FindByName(ctx, appCode+":"+name)
		require.NoError(t, err)
		if r == nil {
			return nil
		}
		return r.Permissions
	}

	// The legitimate flow: the application's own permissions.
	require.NoError(t, syncAs(sdk, syncRoleInputRequest{Name: "clerk", Permissions: []string{appCode + ":order:view"}}))

	// Another application's permission, a platform permission (held by the
	// caller, so the ceiling passes it) and the wildcard: refused on a new
	// role and an existing one, nothing written; a super-admin caller too.
	refused("new role, another application's", syncAs(sdk,
		syncRoleInputRequest{Name: "foreign", Permissions: []string{appCode + ":order:view", "other:order:view"}}),
		"other:order:view")
	refused("new role, held platform permission", syncAs(append(sdk, "platform:iam:user:view"),
		syncRoleInputRequest{Name: "platformish", Permissions: []string{"platform:iam:user:view"}}),
		"platform:iam:user:view")
	refused("existing role, wildcard by a super-admin", syncAs([]string{"platform:*:*:*"},
		syncRoleInputRequest{Name: "clerk", Permissions: []string{appCode + ":order:view", "platform:*:*:*"}}),
		"platform:*:*:*")
	assert.Nil(t, permsOf("foreign"))
	assert.Nil(t, permsOf("platformish"))
	assert.Equal(t, []string{appCode + ":order:view"}, permsOf("clerk"))

	// Existing data: a super-admin put another application's permission on
	// the role through the admin API. The SDK re-syncs it with the same list,
	// with none (which keeps the stored list), with an own permission added,
	// and may drop the foreign one.
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, roleops.GrantPermission(roles),
		roleops.GrantPermissionCommand{RoleName: appCode + ":clerk", Permission: "other:order:view"}, testpg.TestEC())
	require.NoError(t, err)
	require.NoError(t, syncAs(sdk, syncRoleInputRequest{
		Name:        "clerk",
		Permissions: []string{appCode + ":order:view", "other:order:view"},
	}))
	require.NoError(t, syncAs(sdk, syncRoleInputRequest{Name: "clerk"}))
	require.NoError(t, syncAs(sdk, syncRoleInputRequest{
		Name:        "clerk",
		Permissions: []string{appCode + ":order:view", appCode + ":order:create", "other:order:view"},
	}))
	assert.ElementsMatch(t, []string{appCode + ":order:view", appCode + ":order:create", "other:order:view"}, permsOf("clerk"))
	require.NoError(t, syncAs(sdk, syncRoleInputRequest{Name: "clerk", Permissions: []string{appCode + ":order:view"}}))
	assert.Equal(t, []string{appCode + ":order:view"}, permsOf("clerk"))
}
