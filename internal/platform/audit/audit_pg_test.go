//go:build integration

package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

func mkLog(id string) *audit.Log {
	return &audit.Log{
		ID: id, EntityType: "BatchTest", EntityID: "ent-1", Operation: "CREATE",
		OperationJSON: []byte(`{"k":"v"}`), PerformedAt: time.Now().UTC(),
	}
}

// TestInsertBatch_SingleRoundTripAndAllOrNothing pins the batched audit
// insert: rows land together, and a mid-batch failure (duplicate PK) rolls
// the whole batch back — previously each row auto-committed individually,
// so a failure left a partial prefix behind.
func TestInsertBatch_SingleRoundTripAndAllOrNothing(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := audit.NewRepository(pool)

	require.NoError(t, repo.InsertBatch(ctx, []*audit.Log{
		mkLog("audbatchtest00001"), mkLog("audbatchtest00002"),
	}))

	count := func() int {
		var n int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM aud_logs WHERE entity_type = 'BatchTest'`).Scan(&n))
		return n
	}
	require.Equal(t, 2, count())

	// Duplicate PK mid-batch: nothing from this batch may land.
	err := repo.InsertBatch(ctx, []*audit.Log{
		mkLog("audbatchtest00003"),
		mkLog("audbatchtest00001"), // PK collision
	})
	require.Error(t, err)
	assert.Equal(t, 2, count(), "failed batch must be all-or-nothing")

	// Empty batch is a no-op.
	require.NoError(t, repo.InsertBatch(ctx, nil))
}
