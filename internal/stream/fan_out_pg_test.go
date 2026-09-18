//go:build integration

package stream

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

func insertActiveSubscription(t *testing.T, pool *pgxpool.Pool, id, code, eventTypePattern string, queue *string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_subscriptions (id, code, name, target, status, queue, mode, source)
		 VALUES ($1, $2, $2, 'https://sub.fanout.test/hook', 'ACTIVE', $3, 'IMMEDIATE', 'UI')
		 ON CONFLICT (id) DO NOTHING`,
		id, code, queue)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(),
		`INSERT INTO msg_subscription_event_types (subscription_id, event_type_code)
		 VALUES ($1, $2)`, id, eventTypePattern)
	require.NoError(t, err)
}

func insertUnfannedEvent(t *testing.T, pool *pgxpool.Pool, id, eventType string) {
	t.Helper()
	// msg_events is partitioned by created_at (migration 019), so its unique
	// constraint is composite (id, created_at) — a bare ON CONFLICT (id)
	// matches nothing. Each test uses a fresh id, so no conflict handling is
	// needed.
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_events (id, type, source, time)
		 VALUES ($1, $2, 'test://fanout', NOW())`,
		id, eventType)
	require.NoError(t, err)
}

// TestFanOut_CopiesSubscriptionQueueOntoJob is the end-to-end pin of R2
// (docs/spec/dispatch-job-priority.md): a job raised from a HIGH_PRIORITY
// subscription carries that value on its own queue column, and one raised
// from a subscription with no queue set carries none. Exercises the real
// SQL (insertJobsInTx), not just the pure buildJobs mapping — a mismatched
// column/param index in the hand-rolled batch insert only shows up against
// a real database.
//
// Mutant (T5): leave the column nil and rely on the lookup — the
// HIGH_PRIORITY-subscription assertion below must fail under it.
func TestFanOut_CopiesSubscriptionQueueOntoJob(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()

	hi := "HIGH_PRIORITY"
	insertActiveSubscription(t, pool, "sub_fo_hi", "fo-hi", "fanout:test:evt", &hi)
	insertActiveSubscription(t, pool, "sub_fo_lo", "fo-lo", "fanout:test:evt", nil)

	insertUnfannedEvent(t, pool, "evt_fo_01", "fanout:test:evt")

	fo := NewFanOut(pool)
	n, err := fo.step(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "one event claimed and fanned out")

	rows, err := pool.Query(ctx,
		`SELECT subscription_id, queue FROM msg_dispatch_jobs WHERE event_id = $1`, "evt_fo_01")
	require.NoError(t, err)
	defer rows.Close()

	byRaiser := map[string]*string{}
	for rows.Next() {
		var subID string
		var queue *string
		require.NoError(t, rows.Scan(&subID, &queue))
		byRaiser[subID] = queue
	}
	require.NoError(t, rows.Err())
	require.Len(t, byRaiser, 2, "one job per matching subscription")

	hiQueue := byRaiser["sub_fo_hi"]
	require.NotNil(t, hiQueue, "the HIGH_PRIORITY subscription's job must carry a queue value")
	assert.Equal(t, "HIGH_PRIORITY", *hiQueue)

	loQueue := byRaiser["sub_fo_lo"]
	assert.Nil(t, loQueue, "a subscription with no queue set must write none onto its job")

	// The event itself must be stamped fanned-out, independent of the
	// queue-copy assertion above.
	var fannedOut time.Time
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT fanned_out_at FROM msg_events WHERE id = $1`, "evt_fo_01").Scan(&fannedOut))
	assert.False(t, fannedOut.IsZero())
}
