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
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// The permission ceiling (F10) on POST /api/applications/{appCode}/roles/sync:
// an application's own permissions sync for any caller, but a synced role
// cannot gain or lose a platform permission the caller lacks. The route wrote
// whatever permissions the body carried, so an application's service account
// could put platform:*:*:* on a role its users hold.
func TestSyncRoles_PermissionsBoundedByCallersPermissions(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	roles := role.NewRepository(pool)

	const appCode = "ceilrolesync"
	app, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(application.NewRepository(pool)),
		appops.CreateCommand{Code: appCode, Name: "Ceiling role sync"}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{Apps: application.NewRepository(pool), Roles: roles, UoW: uow}
	syncAs := func(perms []string, removeUnlisted bool, rs ...syncRoleInputRequest) error {
		c := auth.WithContext(ctx, &auth.AuthContext{
			PrincipalID: "prn_ceilrolesync1", Scope: auth.ScopeAnchor,
			Applications: []string{app.ApplicationID},
			Permissions:  perms,
		})
		_, err := s.syncRoles(c, &syncRolesInput{AppCode: appCode, RemoveUnlisted: removeUnlisted, Body: syncRolesRequest{Roles: rs}})
		return err
	}
	sdk := []string{"platform:application-service:role:create", "platform:application-service:role:update"}
	refused := func(label string, err error, named string) {
		t.Helper()
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "%s: want a refusal, got %v", label, err)
		assert.Equal(t, usecase.KindAuthorization, ue.Kind, label)
		assert.Equal(t, "PERMISSION_ABOVE_CALLER", ue.Code, label)
		assert.Contains(t, ue.Message, named, label)
	}

	// The application's own permissions: synced for any caller.
	require.NoError(t, syncAs(sdk, false, syncRoleInputRequest{Name: "clerk", Permissions: []string{appCode + ":order:view"}}))

	// Adding a platform permission: since confinement (F12) an application's
	// role may not gain one at all (400 before the ceiling), on create and
	// update, nothing written.
	outside := func(label string, err error) {
		t.Helper()
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "%s: want a refusal, got %v", label, err)
		assert.Equal(t, "PERMISSION_OUTSIDE_APPLICATION", ue.Code, label)
	}
	outside("new role", syncAs(sdk, false,
		syncRoleInputRequest{Name: "escalated", Permissions: []string{"platform:*:*:*"}}))
	outside("existing role", syncAs(sdk, false,
		syncRoleInputRequest{Name: "clerk", Permissions: []string{appCode + ":order:view", "platform:*:*:*"}}))
	none, err := roles.FindByName(ctx, appCode+":escalated")
	require.NoError(t, err)
	assert.Nil(t, none)
	clerk, err := roles.FindByName(ctx, appCode+":clerk")
	require.NoError(t, err)
	require.NotNil(t, clerk)
	assert.Equal(t, []string{appCode + ":order:view"}, clerk.Permissions)

	// A super-admin put a platform permission on the role through the admin
	// API (the one way left to do it); removal counts, by a replacing list or
	// by removeUnlisted deleting the role.
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, roleops.GrantPermission(roles),
		roleops.GrantPermissionCommand{RoleName: appCode + ":clerk", Permission: "platform:iam:user:view"}, testpg.TestEC())
	require.NoError(t, err)
	refused("removing", syncAs(sdk, false,
		syncRoleInputRequest{Name: "clerk", Permissions: []string{appCode + ":order:view"}}), "platform:iam:user:view")
	refused("removeUnlisted", syncAs(sdk, true,
		syncRoleInputRequest{Name: "other", Permissions: []string{appCode + ":order:view"}}), "platform:iam:user:view")
	clerk, err = roles.FindByName(ctx, appCode+":clerk")
	require.NoError(t, err)
	require.NotNil(t, clerk)
	assert.ElementsMatch(t, []string{appCode + ":order:view", "platform:iam:user:view"}, clerk.Permissions)

	// Unchanged permissions are not checked: a re-sync of the same list, or of
	// none (which keeps the stored list), passes.
	require.NoError(t, syncAs(sdk, false,
		syncRoleInputRequest{Name: "clerk", Permissions: []string{"platform:iam:user:view", appCode + ":order:view"}}))
	require.NoError(t, syncAs(sdk, false, syncRoleInputRequest{Name: "clerk"}))
}

// F15 (owner ruling 2026-10-13): the SDK role sync applies the permission
// ceiling as the role-edit routes do, to additions as well as removals. On an
// ordinary application confinement (F12) stops a platform permission first, so
// the addition case is reachable on the `platform` application only (seeded in
// every deployment): its SDK roles may hold platform permissions, but only
// ones the caller holds. Only the change is checked, the whole sync is refused
// (it is all-or-nothing, as its other refusals) and nothing is written.
func TestSyncRoles_PlatformApplicationAdditionsBoundedByCaller(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	roles := role.NewRepository(pool)
	apps := application.NewRepository(pool)

	platformApp, err := apps.FindByCode(ctx, "platform")
	require.NoError(t, err)
	var appID string
	if platformApp != nil {
		appID = string(platformApp.ID)
	} else {
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(apps),
			appops.CreateCommand{Code: "platform", Name: "FlowCatalyst Platform"}, testpg.TestEC())
		require.NoError(t, err)
		appID = ev.ApplicationID
	}

	s := &State{Apps: apps, Roles: roles, UoW: uow}
	sdk := []string{"platform:application-service:role:create", "platform:application-service:role:update"}
	syncAs := func(perms []string, rs ...syncRoleInputRequest) error {
		c := auth.WithContext(ctx, &auth.AuthContext{
			PrincipalID: "prn_ceilplatsync1", Scope: auth.ScopeAnchor,
			Applications: []string{appID},
			Permissions:  perms,
		})
		_, err := s.syncRoles(c, &syncRolesInput{AppCode: "platform", Body: syncRolesRequest{Roles: rs}})
		return err
	}
	refused := func(label string, err error, named string) {
		t.Helper()
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "%s: want a refusal, got %v", label, err)
		assert.Equal(t, usecase.KindAuthorization, ue.Kind, label)
		assert.Equal(t, "PERMISSION_ABOVE_CALLER", ue.Code, label)
		assert.Equal(t, "You may only add or remove permissions you hold yourself: "+named, ue.Message, label)
	}
	permsOf := func(name string) []string {
		t.Helper()
		r, err := roles.FindByName(ctx, name)
		require.NoError(t, err)
		if r == nil {
			return nil
		}
		return r.Permissions
	}
	const viewer = "platform:ceilplat-viewer"
	held := append([]string{"platform:iam:user:view"}, sdk...)

	// A held platform permission: synced.
	require.NoError(t, syncAs(held, syncRoleInputRequest{Name: "ceilplat-viewer", Permissions: []string{"platform:iam:user:view"}}))
	assert.Equal(t, []string{"platform:iam:user:view"}, permsOf(viewer))

	// One the caller lacks, on a new role and added to an existing one:
	// refused, the whole sync with it, nothing written.
	refused("new role", syncAs(held,
		syncRoleInputRequest{Name: "ceilplat-other", Permissions: []string{"platform:iam:user:view"}},
		syncRoleInputRequest{Name: "ceilplat-escalated", Permissions: []string{"platform:*:*:*"}}), "platform:*:*:*")
	assert.Nil(t, permsOf("platform:ceilplat-other"))
	assert.Nil(t, permsOf("platform:ceilplat-escalated"))
	refused("existing role", syncAs(held, syncRoleInputRequest{
		Name:        "ceilplat-viewer",
		Permissions: []string{"platform:iam:user:view", "platform:iam:user:delete"},
	}), "platform:iam:user:delete")
	assert.Equal(t, []string{"platform:iam:user:view"}, permsOf(viewer))

	// Only the change: a caller lacking the kept permission re-syncs the same
	// list, but cannot remove it.
	require.NoError(t, syncAs(sdk, syncRoleInputRequest{Name: "ceilplat-viewer", Permissions: []string{"platform:iam:user:view"}}))
	refused("removal", syncAs(sdk, syncRoleInputRequest{
		Name:        "ceilplat-viewer",
		Permissions: []string{"platform:application-service:role:create"},
	}), "platform:iam:user:view")
	assert.Equal(t, []string{"platform:iam:user:view"}, permsOf(viewer))

	// A holder of the permission adds it.
	require.NoError(t, syncAs(append([]string{"platform:iam:user:delete"}, held...), syncRoleInputRequest{
		Name:        "ceilplat-viewer",
		Permissions: []string{"platform:iam:user:view", "platform:iam:user:delete"},
	}))
	assert.ElementsMatch(t, []string{"platform:iam:user:view", "platform:iam:user:delete"}, permsOf(viewer))
}
