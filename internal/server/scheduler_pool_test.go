package server

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
)

// The scheduler's planner settings go on the scheduler's own pool and on no other:
// the shared pool's connection config must not pick them up.
func TestSchedulerPool_GetsThePlannerSettingsAndTheSharedPoolDoesNot(t *testing.T) {
	ctx := context.Background()
	// Lazy pools: nothing connects until a connection is acquired.
	shared, err := pgxpool.New(ctx, "postgres://u:p@127.0.0.1:1/db?sslmode=disable")
	require.NoError(t, err)
	defer shared.Close()

	sp, err := newSchedulerPool(ctx, shared, 5)
	require.NoError(t, err)
	defer sp.Close()

	got := sp.Config().ConnConfig.RuntimeParams
	for k, v := range scheduler.PoolRuntimeParams {
		assert.Equal(t, v, got[k], "scheduler pool: %s", k)
		_, onShared := shared.Config().ConnConfig.RuntimeParams[k]
		assert.False(t, onShared, "the shared pool must not carry %s", k)
	}
	assert.Equal(t, "force_custom_plan", got["plan_cache_mode"])
	assert.Equal(t, "off", got["enable_sort"])
	// Opening a second scheduler pool does not leak into the shared one either.
	sp2, err := newSchedulerPool(ctx, shared, 5)
	require.NoError(t, err)
	defer sp2.Close()
	assert.NotContains(t, shared.Config().ConnConfig.RuntimeParams, "enable_sort")
}
