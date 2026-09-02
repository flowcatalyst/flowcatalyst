//go:build integration

package subscription_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
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
	repo := subscription.NewRepository(pool)

	const id = "sub_corrupt_test1"
	testpg.WithConstraintDropped(t, pool, "msg_subscriptions", "chk_msg_subscriptions_status", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_subscriptions (id, code, name, target, status)
		 VALUES ($1, 'corrupt-code-1', 'Corrupt', 'https://example.com/hook', 'NOT_A_REAL_STATUS')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_subscriptions WHERE id = $1`, id)
		})
	})

	s, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt status must fail the read, not round-trip it")
	assert.Nil(t, s)
	assert.Contains(t, err.Error(), "CORRUPT_SUBSCRIPTION_STATUS")
}

// TestFindByID_CorruptSourceFailsLoudly mirrors the above for the source
// column.
func TestFindByID_CorruptSourceFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := subscription.NewRepository(pool)

	const id = "sub_corrupt_test2"
	testpg.WithConstraintDropped(t, pool, "msg_subscriptions", "chk_msg_subscriptions_source", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_subscriptions (id, code, name, target, source)
		 VALUES ($1, 'corrupt-code-2', 'Corrupt', 'https://example.com/hook', 'NOT_A_REAL_SOURCE')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_subscriptions WHERE id = $1`, id)
		})
	})

	s, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt source must fail the read, not round-trip it")
	assert.Nil(t, s)
	assert.Contains(t, err.Error(), "CORRUPT_SUBSCRIPTION_SOURCE")
}

// TestFindAll_CorruptStatusFailsTheWholeList pins the ruling's list
// semantics explicitly: "a list containing the row fails too" — one bad row
// must not be silently skipped or coerced while the rest of the list is
// returned successfully.
func TestFindAll_CorruptStatusFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := subscription.NewRepository(pool)

	const goodID = "sub_cl_good012345"
	const badID = "sub_cl_bad0012345"
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_subscriptions (id, code, name, target, status)
		 VALUES ($1, 'corruptlist-good-sub', 'Good', 'https://example.com/hook', 'ACTIVE')`, goodID)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "msg_subscriptions", "chk_msg_subscriptions_status", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO msg_subscriptions (id, code, name, target, status)
		 VALUES ($1, 'corruptlist-bad-sub', 'Bad', 'https://example.com/hook', 'NOT_A_REAL_STATUS')`, badID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_subscriptions WHERE id = $1`, badID)
		})
	})

	rows, err := repo.FindAll(ctx)
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_SUBSCRIPTION_STATUS")
}
