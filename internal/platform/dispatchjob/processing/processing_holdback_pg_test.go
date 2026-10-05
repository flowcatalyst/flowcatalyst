//go:build integration

package processing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// seedPositionedJob inserts a BLOCK_ON_ERROR job at an explicit position in its
// group, so a test can say which sibling is in front.
func seedPositionedJob(t *testing.T, pool *pgxpool.Pool, id, group, status, targetURL string, createdAt time.Time, scheduledFor *time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_dispatch_jobs
		     (id, code, target_url, status, data_only, payload, max_retries,
		      message_group, mode, sequence, created_at, updated_at, scheduled_for)
		 VALUES ($1, 'proc:test:evt', $2, $3, FALSE, '{"hello":"world"}', 3,
		         $4, 'BLOCK_ON_ERROR', 10, $5, $5, $6)`,
		id, targetURL, status, group, createdAt, scheduledFor)
	require.NoError(t, err)
}

// The delivery-time half of the hold-back, for the case the claim-time filter
// cannot cover: these messages were already on the queue when the sibling in
// front of them failed and went into backoff. The router will deliver them —
// /api/dispatch/process is where they have to be stopped.
//
// A backed-off sibling is PENDING with a future scheduled_for. Nothing about it
// looks like a failure, which is why it was previously waved through here and
// its successors delivered past it.
func TestProcess_HeldByABackedOffSiblingInFront(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harness(t, pool)

	var delivered atomic.Bool
	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		delivered.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(sub.Close)

	const group = "grp-holdback-boff"
	start := time.Now().UTC().Add(-time.Hour)
	backoff := time.Now().UTC().Add(30 * time.Second)

	// djhold0001 failed transiently and is waiting out its backoff.
	// djhold0002 is behind it and already on the queue.
	seedPositionedJob(t, pool, "djhold000001", group, "PENDING", sub.URL, start, &backoff)
	seedPositionedJob(t, pool, "djhold000002", group, "QUEUED", sub.URL, start.Add(time.Second), nil)

	code, out := callProcess(t, base, "djhold000002", auth.Sign("djhold000002"))

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, out["ack"], "the queue message is acked away, not redelivered")
	assert.False(t, delivered.Load(),
		"a job must not deliver past a sibling that is waiting out its backoff")

	status, attempts, _ := jobRow(t, pool, "djhold000002")
	assert.Equal(t, "PENDING", status, "it goes back to PENDING for the poller to re-queue in order")
	assert.Equal(t, int32(0), attempts, "a hold-back must not spend retry budget")

	// Once the job in front gets through, the held one delivers.
	_, err := pool.Exec(context.Background(),
		`UPDATE msg_dispatch_jobs SET status = 'COMPLETED', scheduled_for = NULL
		  WHERE id = 'djhold000001'`)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(),
		`UPDATE msg_dispatch_jobs SET status = 'QUEUED' WHERE id = 'djhold000002'`)
	require.NoError(t, err)

	code, _ = callProcess(t, base, "djhold000002", auth.Sign("djhold000002"))
	assert.Equal(t, http.StatusOK, code)
	assert.True(t, delivered.Load(), "with the group moving again it delivers")
}

// The hold is positional: a backed-off sibling BEHIND a job does not hold it.
// Under the old set-membership rule the whole group stopped regardless of
// position, which also meant a backed-off job blocked itself — the group would
// never have moved again once its own backoff expired.
func TestProcess_NotHeldByABackedOffSiblingBehind(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harness(t, pool)

	var delivered atomic.Bool
	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		delivered.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(sub.Close)

	const group = "grp-holdback-behind"
	start := time.Now().UTC().Add(-time.Hour)
	backoff := time.Now().UTC().Add(30 * time.Second)

	seedPositionedJob(t, pool, "djhold000011", group, "QUEUED", sub.URL, start, nil)
	seedPositionedJob(t, pool, "djhold000012", group, "PENDING", sub.URL, start.Add(time.Second), &backoff)

	code, _ := callProcess(t, base, "djhold000011", auth.Sign("djhold000011"))

	assert.Equal(t, http.StatusOK, code)
	assert.True(t, delivered.Load(), "the head of the group is not held by what is behind it")
	status, _, _ := jobRow(t, pool, "djhold000011")
	assert.Equal(t, "COMPLETED", status)
}
