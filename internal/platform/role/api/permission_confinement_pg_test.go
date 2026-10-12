//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

func requireOutsideApplication(t *testing.T, err error, label, permission, app string) {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "%s: want a refusal, got %v", label, err)
	assert.Equal(t, usecase.KindValidation, ue.Kind, label)
	assert.Equal(t, "PERMISSION_OUTSIDE_APPLICATION", ue.Code, label)
	assert.Equal(t, "Permission '"+permission+"' does not belong to application '"+app+
		"'; a role may only hold its own application's permissions", ue.Message, label)
}

// Confinement (F12): a role may hold only its own application's permissions.
// Before the fix a holder of role:create or role:update could put any
// permission on any role: another application's, or a platform permission it
// holds onto an application role (which the F10 ceiling lets through, since
// the caller holds it). A super-admin may still cross applications through the
// admin API. Only what a write adds is checked, so a role already holding
// another application's permissions keeps working.
func TestRolePermissions_ConfinedToTheirApplication(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := role.NewRepository(pool)
	s := &State{Repo: repo, UoW: testpg.NewUoW(t)}
	superAdmin := callerHolding(nil, superAdminPerm)
	admin := callerHolding(nil, permRoleCreate, permRoleUpdate, permRoleDelete, "platform:iam:role:view", permUserView)

	create := func(c context.Context, app, name string, perms ...string) error {
		_, err := s.create(c, &apicommon.In[CreateRoleRequest]{Body: CreateRoleRequest{
			ApplicationCode: app, RoleName: name, DisplayName: name, Permissions: perms,
		}})
		return err
	}
	absent := func(name string) {
		t.Helper()
		r, err := repo.FindByName(ctx, name)
		require.NoError(t, err)
		assert.Nil(t, r, "a refused create writes nothing: %s", name)
	}

	// The legitimate flows: an application role with its own permissions, a
	// platform role with platform permissions the caller holds.
	require.NoError(t, create(admin, "confine", "clerk", "confine:order:view"))
	require.NoError(t, create(admin, "platform", "confine-viewer", permUserView))
	const clerk, viewer = "confine:clerk", "platform:confine-viewer"

	// Create: another application's permission, or a platform permission the
	// caller holds, on an application role.
	requireOutsideApplication(t, create(admin, "confine", "foreign", "confine:order:view", "other:order:view"),
		"create with another application's permission", "other:order:view", "confine")
	absent("confine:foreign")
	requireOutsideApplication(t, create(admin, "confine", "platformish", permUserView),
		"create with a held platform permission", permUserView, "confine")
	absent("confine:platformish")
	requireOutsideApplication(t, create(admin, "platform", "confine-apps", "confine:order:view"),
		"an application permission on a platform role", "confine:order:view", "platform")
	absent("platform:confine-apps")

	// Update and both grants.
	clerkRow, err := repo.FindByName(ctx, clerk)
	require.NoError(t, err)
	_, err = s.update(admin, &updateInput{ID: clerkRow.ID, Body: UpdateRoleRequest{
		Permissions: []string{"confine:order:view", permUserView},
	}})
	requireOutsideApplication(t, err, "update adding a platform permission", permUserView, "confine")
	_, err = s.grantPermission(admin, &rolePermissionGrantInput{RoleName: clerk, Permission: "other:order:view"})
	requireOutsideApplication(t, err, "grant (path)", "other:order:view", "confine")
	_, err = s.grantPermissionByBody(admin, &rolePermissionGrantBodyInput{
		RoleName: clerk, Body: GrantPermissionRequest{Permission: "*:*:*:*"},
	})
	requireOutsideApplication(t, err, "grant (body) of a wildcard application", "*:*:*:*", "confine")
	_, err = s.grantPermission(admin, &rolePermissionGrantInput{RoleName: viewer, Permission: "confine:order:view"})
	requireOutsideApplication(t, err, "grant onto a platform role", "confine:order:view", "platform")
	assert.Equal(t, []string{"confine:order:view"}, permissionsOf(t, repo, clerk), "nothing written")

	// Own-application changes still pass.
	_, err = s.grantPermission(admin, &rolePermissionGrantInput{RoleName: clerk, Permission: "confine:order:create"})
	require.NoError(t, err)
	_, err = s.update(admin, &updateInput{ID: clerkRow.ID, Body: UpdateRoleRequest{
		Permissions: []string{"confine:order:view", "confine:order:create", "confine:order:delete"},
	}})
	require.NoError(t, err)

	// A super-admin may cross applications through the admin API.
	require.NoError(t, create(superAdmin, "confine", "legacy", "confine:order:view", "other:order:view", permUserView))
	const legacy = "confine:legacy"
	_, err = s.grantPermission(superAdmin, &rolePermissionGrantInput{RoleName: clerk, Permission: "other:order:create"})
	require.NoError(t, err)

	// Existing data: a role already holding foreign permissions reads, keeps
	// them through edits that do not add them, and can lose them.
	legacyRow, err := repo.FindByName(ctx, legacy)
	require.NoError(t, err)
	got, err := s.byCode(admin, &byCodeInput{Code: legacy})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"confine:order:view", "other:order:view", permUserView}, got.Body.Permissions)
	_, err = s.listRolePermissions(admin, &roleNameInput{RoleName: legacy})
	require.NoError(t, err)
	renamed := "Legacy (renamed)"
	_, err = s.update(admin, &updateInput{ID: legacyRow.ID, Body: UpdateRoleRequest{DisplayName: &renamed}})
	require.NoError(t, err)
	_, err = s.update(admin, &updateInput{ID: legacyRow.ID, Body: UpdateRoleRequest{
		Permissions: []string{"confine:order:view", "confine:order:create", "other:order:view", permUserView},
	}})
	require.NoError(t, err, "the foreign permissions are kept, an own one added")
	_, err = s.grantPermission(admin, &rolePermissionGrantInput{RoleName: legacy, Permission: "other:order:view"})
	require.NoError(t, err, "granting a held permission changes nothing")
	_, err = s.revokePermission(admin, &rolePermissionGrantInput{RoleName: legacy, Permission: "other:order:view"})
	require.NoError(t, err, "a foreign permission may always be removed")
	_, err = s.update(admin, &updateInput{ID: legacyRow.ID, Body: UpdateRoleRequest{
		Permissions: []string{"confine:order:view"},
	}})
	require.NoError(t, err, "removing a held platform permission by update")
	assert.Equal(t, []string{"confine:order:view"}, permissionsOf(t, repo, legacy))
	_, err = s.delete(admin, &apicommon.IDInput{ID: legacyRow.ID})
	require.NoError(t, err)
}
