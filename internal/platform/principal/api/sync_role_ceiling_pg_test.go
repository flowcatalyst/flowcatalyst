//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The role ceiling (F8) on POST /api/principals/sync: it is gated on any of
// the user permissions, and before the fix a caller holding only user:create
// could sync any email, its own included, with platform:super-admin.
func TestUserSync_BoundedByCallersPermissions(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)
	s := &State{Repo: repo, Roles: role.NewRepository(pool), UoW: uow}

	power := mustRoleWith(t, uow, "ceilsync", "power", "platform:iam:user:create", "platform:iam:user:view")
	appOnly := mustRoleWith(t, uow, "ceilsync", "app-only", "ceilsync:thing:view")

	sync := func(c context.Context, email string, roles ...string) error {
		_, err := s.syncUsers(c, &apicommon.In[SyncUsersRequest]{Body: SyncUsersRequest{
			Principals: []SyncUserInput{{Email: email, Name: email, Roles: roles}},
		}})
		return err
	}
	createOnly := anchorHolding("USER", permUserCreateOnly)

	// Lacking the role's permissions: refused, nothing written.
	requireRoleAboveCaller(t, sync(createOnly, "escalate@ceilsync.test", power), "sync power", power)
	got, err := repo.FindByEmail(ctx, "escalate@ceilsync.test")
	require.NoError(t, err)
	assert.Nil(t, got, "a refused sync writes nothing")

	// Application roles pass for any caller allowed to sync.
	require.NoError(t, sync(createOnly, "clerk@ceilsync.test", appOnly))

	// A caller holding the permissions, or the wildcard, may.
	require.NoError(t, sync(anchorHolding("USER", permUserCreateOnly, "platform:iam:user:view"),
		"subset@ceilsync.test", power))
	require.NoError(t, sync(anchorHolding("USER", "platform:*:*:*"), "super@ceilsync.test", power))

	// Replacing an existing user's synced role counts as removing it.
	requireRoleAboveCaller(t, sync(createOnly, "subset@ceilsync.test", appOnly), "strip power", power)
	sub, err := repo.FindByEmail(ctx, "subset@ceilsync.test")
	require.NoError(t, err)
	assert.Equal(t, []string{power}, roleNamesFrom(sub.Roles))
}
