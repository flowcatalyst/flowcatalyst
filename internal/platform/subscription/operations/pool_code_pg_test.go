//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	poolops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Create and update store the dispatch pool's code beside its id, as the SDK
// sync does (bug 11 in docs/ui/README.md; docs/platform-bug-fixes.md): the
// SPA's Pool column was empty for every subscription created in a UI or
// through POST /api/subscriptions.
func TestCreateAndUpdateStoreTheDispatchPoolCode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := subscription.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)
	uow := testpg.NewUoW(t)

	first, err := runAuthorized(uow, poolops.CreateDispatchPool(poolRepo), poolops.CreateCommand{Code: "subpc-first", Name: "First"})
	require.NoError(t, err)
	second, err := runAuthorized(uow, poolops.CreateDispatchPool(poolRepo), poolops.CreateCommand{Code: "subpc-second", Name: "Second"})
	require.NoError(t, err)

	ev, err := runAuthorized(uow, operations.CreateSubscription(repo), operations.CreateCommand{
		Code: "subpc", Name: "Pool code", Endpoint: "https://hooks.example.com/pc",
		EventTypes:     []subscription.EventTypeBinding{subscription.NewEventTypeBinding("subpc:orders:order:created")},
		DispatchPoolID: &first.PoolID,
	})
	require.NoError(t, err)
	got, err := repo.FindByID(ctx, ev.SubscriptionID)
	require.NoError(t, err)
	require.NotNil(t, got.DispatchPoolCode)
	assert.Equal(t, "subpc-first", *got.DispatchPoolCode)

	_, err = runAuthorized(uow, operations.UpdateSubscription(repo), operations.UpdateCommand{ID: ev.SubscriptionID, DispatchPoolID: &second.PoolID})
	require.NoError(t, err)
	got, err = repo.FindByID(ctx, ev.SubscriptionID)
	require.NoError(t, err)
	require.NotNil(t, got.DispatchPoolCode)
	assert.Equal(t, "subpc-second", *got.DispatchPoolCode, "re-pointing the pool re-reads its code")

	unknown := "dpl_doesnotexist"
	_, err = runAuthorized(uow, operations.UpdateSubscription(repo), operations.UpdateCommand{ID: ev.SubscriptionID, DispatchPoolID: &unknown})
	require.NoError(t, err)
	got, err = repo.FindByID(ctx, ev.SubscriptionID)
	require.NoError(t, err)
	assert.Nil(t, got.DispatchPoolCode, "no stale code for an id that names no pool")
}
