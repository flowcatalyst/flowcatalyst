//go:build integration

package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestPostgresStorePrune pins A-20: PostgresStore.Prune actually deletes
// rows older than the given retention and leaves newer ones — the query the
// purger (internal/server/subsystems.go StartPurger) now calls on every
// tick with retention = ratelimit.PoliciesFromEnv().MaxWindow() + margin.
func TestPostgresStorePrune(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	store := ratelimit.NewPostgresStore(pool)
	const bucket = ratelimit.BucketOAuthTokenIP
	const key = "a20-prune-test-key"

	// One old row (2 hours ago) and one fresh row, inserted directly so
	// occurred_at is under our control (CheckAndRecord always stamps NOW()).
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_rate_limit_events (bucket, key, occurred_at) VALUES ($1, $2, $3)`,
		string(bucket), key, time.Now().UTC().Add(-2*time.Hour))
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_rate_limit_events (bucket, key, occurred_at) VALUES ($1, $2, $3)`,
		string(bucket), key, time.Now().UTC())
	require.NoError(t, err)

	countRows := func() int {
		var n int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM iam_rate_limit_events WHERE bucket = $1 AND key = $2`,
			string(bucket), key).Scan(&n))
		return n
	}
	require.Equal(t, 2, countRows(), "precondition: both rows present")

	// Retention of 1h: only the 2h-old row is older than the cutoff.
	removed, err := store.Prune(ctx, time.Hour)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, removed, int64(1), "at least the old row should be pruned")
	assert.Equal(t, 1, countRows(), "only the fresh row should survive a 1h retention prune")
}

// TestPostgresStoreCheckAndRecordCountsWithinWindow pins the read side the
// prune retention has to stay compatible with: CheckAndRecord counts rows
// strictly newer than now-window, so a retention shorter than any
// configured policy window would prune rows a live check still needs.
func TestPostgresStoreCheckAndRecordCountsWithinWindow(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	store := ratelimit.NewPostgresStore(pool)
	const bucket = ratelimit.BucketOAuthTokenClient
	const key = "a20-window-test-key"

	policy := ratelimit.Policy{Window: time.Minute, Limit: 2}
	d, err := store.CheckAndRecord(ctx, bucket, key, policy)
	require.NoError(t, err)
	assert.True(t, d.Allowed)

	d, err = store.CheckAndRecord(ctx, bucket, key, policy)
	require.NoError(t, err)
	assert.True(t, d.Allowed, "2nd of 2 allowed under the limit")

	d, err = store.CheckAndRecord(ctx, bucket, key, policy)
	require.NoError(t, err)
	assert.False(t, d.Allowed, "3rd exceeds the limit")
	assert.Positive(t, d.RetryAfterSecs)
}
