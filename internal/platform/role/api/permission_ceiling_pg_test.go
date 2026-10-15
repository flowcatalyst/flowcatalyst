//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

const (
	permRoleCreate = "platform:iam:role:create"
	permRoleUpdate = "platform:iam:role:update"
	permRoleDelete = "platform:iam:role:delete"
	permUserView   = "platform:iam:user:view"
	permUserCreate = "platform:iam:user:create"
	superAdminPerm = "platform:*:*:*"
)

func callerHolding(roles []string, perms ...string) context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_permceil_c01",
		Scope:       auth.ScopeAnchor,
		Roles:       roles,
		Permissions: perms,
	})
}

func requirePermissionAboveCaller(t *testing.T, err error, label string, named ...string) {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "%s: want a refusal, got %v", label, err)
	assert.Equal(t, usecase.KindAuthorization, ue.Kind, label)
	assert.Equal(t, "PERMISSION_ABOVE_CALLER", ue.Code, label)
	assert.Contains(t, ue.Message, "You may only add or remove permissions you hold yourself: ", label)
	for _, n := range named {
		assert.Contains(t, ue.Message, n, label)
	}
}

func permissionsOf(t *testing.T, repo *role.Repository, name string) []string {
	t.Helper()
	r, err := repo.FindByName(context.Background(), name)
	require.NoError(t, err)
	require.NotNil(t, r, "role %s", name)
	return r.Permissions
}

// The permission ceiling (F10): a holder of role:create/update/delete adds or
// removes only platform permissions it holds itself, on every route that
// writes a role's permissions. Before the fix a holder of role:update could
// add platform:*:*:* to a role it holds and become super-admin.
func TestRolePermissions_BoundedByCallersPermissions(t *testing.T) {
	pool := testpg.Pool(t)
	repo := role.NewRepository(pool)
	s := &State{Repo: repo, UoW: testpg.NewUoW(t)}
	superAdmin := callerHolding(nil, superAdminPerm)
	testpg.EnsureApplication(t, "platform")
	testpg.EnsureApplication(t, "permceil")

	// The caller's own role: what it holds is exactly this role's permissions.
	// The roles holding platform permissions are platform roles: confinement
	// (F12) refuses a platform permission on another application's role before
	// the ceiling is reached.
	ownPerms := []string{permRoleCreate, permRoleUpdate, permRoleDelete, permUserView}
	_, err := s.create(superAdmin, &apicommon.In[CreateRoleRequest]{Body: CreateRoleRequest{
		ApplicationCode: "platform", RoleName: "permceil-role-admin", DisplayName: "Role Admin", Permissions: ownPerms,
	}})
	require.NoError(t, err)
	const own = "platform:permceil-role-admin"
	ownRow, err := repo.FindByName(context.Background(), own)
	require.NoError(t, err)
	require.NotNil(t, ownRow)
	admin := callerHolding([]string{own}, ownPerms...)

	// Self-escalation: the wildcard onto its own role, by every route.
	_, err = s.update(admin, &updateInput{ID: ownRow.ID, Body: UpdateRoleRequest{
		Permissions: append(append([]string{}, ownPerms...), superAdminPerm),
	}})
	requirePermissionAboveCaller(t, err, "update own role", superAdminPerm)
	_, err = s.grantPermission(admin, &rolePermissionGrantInput{RoleName: own, Permission: superAdminPerm})
	requirePermissionAboveCaller(t, err, "grant (path) on own role", superAdminPerm)
	_, err = s.grantPermissionByBody(admin, &rolePermissionGrantBodyInput{
		RoleName: own, Body: GrantPermissionRequest{Permission: superAdminPerm},
	})
	requirePermissionAboveCaller(t, err, "grant (body) on own role", superAdminPerm)
	assert.ElementsMatch(t, ownPerms, permissionsOf(t, repo, own), "own role unchanged")

	// Create: only permissions it holds.
	_, err = s.create(admin, &apicommon.In[CreateRoleRequest]{Body: CreateRoleRequest{
		ApplicationCode: "platform", RoleName: "permceil-escalated", DisplayName: "Escalated",
		Permissions: []string{permUserView, permUserCreate},
	}})
	requirePermissionAboveCaller(t, err, "create", permUserCreate)
	gone, err := repo.FindByName(context.Background(), "platform:permceil-escalated")
	require.NoError(t, err)
	assert.Nil(t, gone, "a refused create writes nothing")
	_, err = s.create(admin, &apicommon.In[CreateRoleRequest]{Body: CreateRoleRequest{
		ApplicationCode: "platform", RoleName: "permceil-clerk", DisplayName: "Clerk",
		Permissions: []string{permUserView},
	}})
	require.NoError(t, err)
	const clerk = "platform:permceil-clerk"

	// Adding a held permission: allowed. Adding one it lacks: refused by
	// every route.
	_, err = s.grantPermission(admin, &rolePermissionGrantInput{RoleName: clerk, Permission: permRoleUpdate})
	require.NoError(t, err)
	_, err = s.grantPermission(admin, &rolePermissionGrantInput{RoleName: clerk, Permission: permUserCreate})
	requirePermissionAboveCaller(t, err, "grant lacking", permUserCreate)
	clerkRow, err := repo.FindByName(context.Background(), clerk)
	require.NoError(t, err)
	_, err = s.update(admin, &updateInput{ID: clerkRow.ID, Body: UpdateRoleRequest{
		Permissions: append(append([]string{}, clerkRow.Permissions...), permUserCreate),
	}})
	requirePermissionAboveCaller(t, err, "update adding lacking", permUserCreate)
	assert.ElementsMatch(t, []string{permUserView, permRoleUpdate}, permissionsOf(t, repo, clerk))

	// A held wildcard covers what it names.
	wide := callerHolding(nil, permRoleUpdate, "platform:iam:user:*")
	_, err = s.grantPermission(wide, &rolePermissionGrantInput{RoleName: clerk, Permission: permUserCreate})
	require.NoError(t, err)

	// Removal counts: the lacking permission cannot be taken off by update,
	// revoke or deleting the role; a kept one is not checked.
	clerkRow, err = repo.FindByName(context.Background(), clerk)
	require.NoError(t, err)
	_, err = s.update(admin, &updateInput{ID: clerkRow.ID, Body: UpdateRoleRequest{
		Permissions: []string{permUserView},
	}})
	requirePermissionAboveCaller(t, err, "update removing lacking", permUserCreate)
	_, err = s.revokePermission(admin, &rolePermissionGrantInput{RoleName: clerk, Permission: permUserCreate})
	requirePermissionAboveCaller(t, err, "revoke lacking", permUserCreate)
	_, err = s.delete(admin, &apicommon.IDInput{ID: clerkRow.ID})
	requirePermissionAboveCaller(t, err, "delete a role holding a lacking permission", permUserCreate)
	assert.Contains(t, permissionsOf(t, repo, clerk), permUserCreate)

	newName := "Clerk (renamed)"
	_, err = s.update(admin, &updateInput{ID: clerkRow.ID, Body: UpdateRoleRequest{DisplayName: &newName}})
	require.NoError(t, err, "no permission change, nothing to check")
	_, err = s.update(admin, &updateInput{ID: clerkRow.ID, Body: UpdateRoleRequest{
		Permissions: []string{permUserCreate, permUserView, permRoleUpdate},
	}})
	require.NoError(t, err, "the lacking permission is kept, so not checked")
	_, err = s.revokePermission(admin, &rolePermissionGrantInput{RoleName: clerk, Permission: permRoleUpdate})
	require.NoError(t, err, "a held permission may be revoked")
	_, err = s.revokePermission(admin, &rolePermissionGrantInput{RoleName: clerk, Permission: "platform:never:held:x"})
	require.NoError(t, err, "revoking a permission the role lacks changes nothing")
	assert.ElementsMatch(t, []string{permUserCreate, permUserView}, permissionsOf(t, repo, clerk))

	// Application permissions are outside the ceiling, even for a caller
	// holding none of them.
	_, err = s.create(admin, &apicommon.In[CreateRoleRequest]{Body: CreateRoleRequest{
		ApplicationCode: "permceil", RoleName: "app-only", DisplayName: "App only",
		Permissions: []string{"permceil:order:view", "permceil:order:delete"},
	}})
	require.NoError(t, err)
	appOnly, err := repo.FindByName(context.Background(), "permceil:app-only")
	require.NoError(t, err)
	_, err = s.revokePermission(admin, &rolePermissionGrantInput{RoleName: "permceil:app-only", Permission: "permceil:order:delete"})
	require.NoError(t, err)
	_, err = s.delete(admin, &apicommon.IDInput{ID: appOnly.ID})
	require.NoError(t, err)

	// The wildcard holder may add, remove and delete anything.
	_, err = s.grantPermission(superAdmin, &rolePermissionGrantInput{RoleName: own, Permission: superAdminPerm})
	require.NoError(t, err)
	_, err = s.revokePermission(superAdmin, &rolePermissionGrantInput{RoleName: own, Permission: superAdminPerm})
	require.NoError(t, err)
	_, err = s.delete(superAdmin, &apicommon.IDInput{ID: clerkRow.ID})
	require.NoError(t, err)
}
