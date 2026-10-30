//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// F30: a role sync that changes nothing reports nothing updated and writes
// nothing (the same rule event types follow, ledger item 6).
func TestSyncRoles_UnchangedRolesAreNotUpdated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	appID := testpg.EnsureApplication(t, "f30roles")

	desc := "Handles things"
	mk := func(mutate func(r *operations.SyncRoleInput)) operations.SyncRolesCommand {
		agent := operations.SyncRoleInput{
			Name: "agent", DisplayName: new("Agent"), Description: &desc,
			Permissions: []string{"f30roles:ticket:view", "f30roles:ticket:edit"},
		}
		if mutate != nil {
			mutate(&agent)
		}
		return operations.SyncRolesCommand{
			ApplicationCode: "f30roles", ApplicationID: appID,
			Roles: []operations.SyncRoleInput{agent, {Name: "lead"}},
		}
	}
	run := func(cmd operations.SyncRolesCommand) operations.RolesSynced {
		t.Helper()
		out, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncRoles(repo), cmd, testpg.TestEC())
		require.NoError(t, err)
		return out
	}

	first := run(mk(nil))
	assert.Equal(t, uint32(2), first.Created)
	before, err := repo.FindByName(ctx, "f30roles:agent")
	require.NoError(t, err)
	require.NotNil(t, before)

	again := run(mk(nil))
	assert.Equal(t, uint32(0), again.Created)
	assert.Equal(t, uint32(0), again.Updated, "nothing changed")
	after, err := repo.FindByName(ctx, "f30roles:agent")
	require.NoError(t, err)
	assert.Equal(t, before.UpdatedAt, after.UpdatedAt, "an unchanged role is not rewritten")

	// The same permissions in another order are the same set.
	reordered := run(mk(func(r *operations.SyncRoleInput) {
		r.Permissions = []string{"f30roles:ticket:edit", "f30roles:ticket:view"}
	}))
	assert.Equal(t, uint32(0), reordered.Updated)

	// Omitted permissions keep the stored ones (unchanged), they do not wipe.
	omitted := run(mk(func(r *operations.SyncRoleInput) { r.Permissions = nil }))
	assert.Equal(t, uint32(0), omitted.Updated)

	for name, mutate := range map[string]func(r *operations.SyncRoleInput){
		"display name": func(r *operations.SyncRoleInput) { r.DisplayName = new("Agent v2") },
		"description":  func(r *operations.SyncRoleInput) { r.Description = new("changed") },
		"permissions":  func(r *operations.SyncRoleInput) { r.Permissions = []string{"f30roles:ticket:view"} },
		"managed":      func(r *operations.SyncRoleInput) { r.ClientManaged = true },
	} {
		changed := run(mk(mutate))
		assert.Equal(t, uint32(1), changed.Updated, "a changed %s updates its role", name)
		// restore, so each case differs from the stored state by one field
		run(mk(nil))
	}
}

// F30: the platform catalogue sync leaves an unchanged built-in role alone.
// SERIAL (no t.Parallel): the sync sweeps every CODE role absent from the
// catalogue, like the other platform-sync tests.
func TestSyncPlatformRoles_UnchangedRolesAreNotUpdated(t *testing.T) {
	ctx := context.Background()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	catalogue := append([]role.Role{}, seed.PlatformRoles()...)

	run := func(cat []role.Role) operations.RolesSynced {
		t.Helper()
		out, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.SyncPlatformRoles(repo, cat),
			operations.SyncPlatformRolesCommand{}, testpg.TestEC())
		require.NoError(t, err)
		return out
	}
	run(catalogue)
	before, err := repo.FindByName(ctx, "platform:admin")
	require.NoError(t, err)
	require.NotNil(t, before)

	again := run(catalogue)
	assert.Equal(t, uint32(0), again.Created)
	assert.Equal(t, uint32(0), again.Updated, "nothing changed")
	after, err := repo.FindByName(ctx, "platform:admin")
	require.NoError(t, err)
	assert.Equal(t, before.UpdatedAt, after.UpdatedAt)

	drifted := append([]role.Role{}, catalogue...)
	for i := range drifted {
		if drifted[i].Name == "platform:admin" {
			drifted[i].DisplayName = "Admin (renamed)"
		}
	}
	third := run(drifted)
	assert.Equal(t, uint32(1), third.Updated, "only the role whose catalogue entry changed")
	run(catalogue)
}
