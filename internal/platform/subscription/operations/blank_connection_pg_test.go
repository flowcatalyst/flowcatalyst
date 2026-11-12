//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// A blank connectionId (the SPA's edit sends "" for an empty field) leaves
// the connection as it was, as an omitted one does (bug 12 in
// docs/ui/README.md; docs/platform-bug-fixes.md); it stored "" before.
func TestUpdateSubscription_BlankConnectionIDLeavesItAlone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := subscription.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "subupd-blankconn", "Blank")
	// Naming a connection checks it exists, so the one the test binds is real.
	testpg.SeedServiceAccount(t, "sva_blankconn001", testpg.ServiceAccountSpec{})
	insertRawConnection(t, testpg.Pool(t), "con_blankconn01", "blankconn-01", "sva_blankconn001", nil, nil)

	_, err := runAuthorized(uow, operations.UpdateSubscription(repo, credRepos(t)), operations.UpdateCommand{ID: seeded.SubscriptionID, ConnectionID: new("  ")})
	require.NoError(t, err)
	got, err := repo.FindByID(ctx, seeded.SubscriptionID)
	require.NoError(t, err)
	assert.Nil(t, got.ConnectionID, "no connection, and not an empty id")

	_, err = runAuthorized(uow, operations.UpdateSubscription(repo, credRepos(t)), operations.UpdateCommand{ID: seeded.SubscriptionID, ConnectionID: new("con_blankconn01")})
	require.NoError(t, err)
	_, err = runAuthorized(uow, operations.UpdateSubscription(repo, credRepos(t)), operations.UpdateCommand{ID: seeded.SubscriptionID, ConnectionID: new("")})
	require.NoError(t, err)
	got, err = repo.FindByID(ctx, seeded.SubscriptionID)
	require.NoError(t, err)
	require.NotNil(t, got.ConnectionID)
	assert.Equal(t, "con_blankconn01", *got.ConnectionID)
}
