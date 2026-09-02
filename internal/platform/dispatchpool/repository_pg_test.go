//go:build integration

package dispatchpool_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestFindByID_CorruptStatusFailsLoudly is the X-06 read boundary: a row
// whose status column holds a value that isn't one of the known Status
// constants (junk written before write-boundary validation existed, or a
// hand-edited row) must fail the read with a distinct error, never
// round-trip as that literal string and never silently coerce to ACTIVE.
func TestFindByID_CorruptStatusFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := dispatchpool.NewRepository(pool)

	const id = "dpl_corrupt_test1"
	testpg.WithConstraintDropped(t, pool, "msg_dispatch_pools", "chk_msg_dispatch_pools_status", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_dispatch_pools (id, code, name, status)
		 VALUES ($1, 'corrupt-code', 'Corrupt', 'NOT_A_REAL_STATUS')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_dispatch_pools WHERE id = $1`, id)
		})
	})

	p, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt status must fail the read, not round-trip it")
	assert.Nil(t, p)
	assert.Contains(t, err.Error(), "CORRUPT_DISPATCH_POOL_STATUS")
}

// TestFindWithFilters_CorruptStatusFailsTheWholeList pins the ruling's list
// semantics explicitly: "a list containing the row fails too" — one bad row
// must not be silently skipped or coerced while the rest of the list is
// returned successfully.
func TestFindWithFilters_CorruptStatusFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := dispatchpool.NewRepository(pool)

	const goodID = "dpl_cl_good012345"
	const badID = "dpl_cl_bad0012345"
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_dispatch_pools (id, code, name, status)
		 VALUES ($1, 'corruptlist-good', 'Good', 'ACTIVE')`, goodID)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "msg_dispatch_pools", "chk_msg_dispatch_pools_status", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO msg_dispatch_pools (id, code, name, status)
		 VALUES ($1, 'corruptlist-bad', 'Bad', 'NOT_A_REAL_STATUS')`, badID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_dispatch_pools WHERE id = $1`, badID)
		})
	})

	rows, err := repo.FindWithFilters(ctx, nil, nil)
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_DISPATCH_POOL_STATUS")
}
