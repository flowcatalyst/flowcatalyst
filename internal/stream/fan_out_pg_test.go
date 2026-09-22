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

// TestFanOut_CopiesSubscriptionNameAndEventContextOntoJob pins 2026-09-22:
// a raised job's descriptor is the raising subscription's NAME, and its
// metadata is the raising event's context_data, verbatim — so the
// dispatch-jobs grid can say what a job is and show the same "additional
// data" its event does. Real SQL (insertJobsInTx + the context_data column
// on the claim), not just buildJobs.
//
// Mutants: drop `descriptor`/`metadata` from insertJobsInTx's column list;
// drop e.context_data from claimUnfannedEvents' RETURNING.
func TestFanOut_CopiesSubscriptionNameAndEventContextOntoJob(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()

	_, err := pool.Exec(ctx,
		`INSERT INTO msg_subscriptions (id, code, name, target, status, mode, source)
		 VALUES ('sub_fo_desc', 'fo-desc', 'Notify Value of user logins',
		         'https://sub.fanout.test/hook', 'ACTIVE', 'IMMEDIATE', 'UI')
		 ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO msg_subscription_event_types (subscription_id, event_type_code)
		 VALUES ('sub_fo_desc', 'fanout:desc:evt')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO msg_events (id, type, source, time, context_data)
		 VALUES ('evt_fo_desc1', 'fanout:desc:evt', 'test://fanout', NOW(),
		         '[{"key":"tenant","value":"acme"},{"key":"actor","value":"usr_1"}]')`)
	require.NoError(t, err)

	n, err := NewFanOut(pool).step(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var descriptor *string
	var metadata []byte
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT descriptor, metadata FROM msg_dispatch_jobs WHERE event_id = 'evt_fo_desc1'`).
		Scan(&descriptor, &metadata))
	require.NotNil(t, descriptor, "the raised job carries the subscription's name as its descriptor")
	assert.Equal(t, "Notify Value of user logins", *descriptor)
	assert.JSONEq(t, `[{"key":"tenant","value":"acme"},{"key":"actor","value":"usr_1"}]`, string(metadata),
		"the raised job carries the event's context_data as its metadata")

	// An event with no context_data leaves the column at its default, not NULL
	// and not a SQL error.
	_, err = pool.Exec(ctx,
		`INSERT INTO msg_events (id, type, source, time)
		 VALUES ('evt_fo_desc2', 'fanout:desc:evt', 'test://fanout', NOW())`)
	require.NoError(t, err)
	_, err = NewFanOut(pool).step(ctx, 10)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT metadata FROM msg_dispatch_jobs WHERE event_id = 'evt_fo_desc2'`).Scan(&metadata))
	assert.JSONEq(t, `[]`, string(metadata))
}
