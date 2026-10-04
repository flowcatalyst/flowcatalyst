//go:build integration

package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// midPublishPublisher checks, while it is publishing, what another connection
// sees of the claimed rows — their status and whether anything holds a lock on
// them — and then accepts nothing, as a worker that died mid-publish would.
type midPublishPublisher struct {
	t        *testing.T
	pool     *pgxpool.Pool
	seenMid  map[string]string
	lockErr  map[string]error
	observed bool
}

func (d *midPublishPublisher) Publish(ctx context.Context, items []PublishItem) ([]string, error) {
	d.seenMid = map[string]string{}
	d.lockErr = map[string]error{}
	for _, it := range items {
		var status string
		require.NoError(d.t, d.pool.QueryRow(ctx, `SELECT status FROM msg_dispatch_jobs WHERE id = $1`, it.JobID).Scan(&status))
		d.seenMid[it.JobID] = status
		// The claim holds no row lock: another connection can lock the row at once.
		tx, err := d.pool.Begin(ctx)
		require.NoError(d.t, err)
		var id string
		d.lockErr[it.JobID] = tx.QueryRow(ctx, `SELECT id FROM msg_dispatch_jobs WHERE id = $1 FOR UPDATE NOWAIT`, it.JobID).Scan(&id)
		_ = tx.Rollback(ctx)
	}
	d.observed = true
	return jobIDs(items), nil
}

// The claim is neither committed QUEUED before the batch is published (a worker
// killed during the publish stranded every unpublished job QUEUED, with no
// queue message, for stale recovery's 75 minutes) nor held under a lock or a
// transaction while it is: a worker that dies mid-publish leaves the whole claim
// PENDING and unlocked, and the next poll publishes it.
func TestPollOnce_NothingIsQueuedOrLockedDuringThePublish(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	const (
		id1   = "djpubdie0001"
		id2   = "djpubdie0002"
		group = "grp_pubdie_it01"
	)
	seedJob(t, pool, id1, "PENDING", group, "")
	seedJob(t, pool, id2, "PENDING", group, "")

	mid := &midPublishPublisher{t: t, pool: pool}
	dispatcher := NewMessageGroupDispatcher(pool, mid, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process")
	poller := NewPendingJobPoller(DefaultConfig(), pool, dispatcher, NewPausedConnectionCache(pool, time.Minute))

	mustPoll(t, poller, ctx)
	require.True(t, mid.observed, "the publisher was reached")
	assert.Equal(t, "PENDING", mid.seenMid[id1], "nothing is committed QUEUED before the publish")
	assert.Equal(t, "PENDING", mid.seenMid[id2])
	assert.NoError(t, mid.lockErr[id1], "no row lock is held across the publish")
	assert.NoError(t, mid.lockErr[id2])
	assert.Equal(t, "PENDING", jobStatus(t, pool, id1), "a publish that never completed strands nothing")
	assert.Equal(t, "PENDING", jobStatus(t, pool, id2))

	// The restarted worker's poll publishes the claim again.
	capture := &capturePublisher{}
	dispatcher = NewMessageGroupDispatcher(pool, capture, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process")
	poller = NewPendingJobPoller(DefaultConfig(), pool, dispatcher, NewPausedConnectionCache(pool, time.Minute))
	mustPoll(t, poller, ctx)
	assert.Equal(t, "QUEUED", jobStatus(t, pool, id1))
	assert.Equal(t, "QUEUED", jobStatus(t, pool, id2))
	capture.mu.Lock()
	defer capture.mu.Unlock()
	assert.Subset(t, capture.ids, []string{id1, id2})
}

// partialPublisher accepts only the first item of the batch.
type partialPublisher struct{}

func (partialPublisher) Publish(_ context.Context, items []PublishItem) ([]string, error) {
	var unpublished []string
	for i, it := range items {
		if i > 0 {
			unpublished = append(unpublished, it.JobID)
		}
	}
	return unpublished, nil
}

// Only what the broker accepted is marked QUEUED; the rest stay PENDING for
// the next poll.
func TestPollOnce_PartialPublishMarksOnlyThePublishedQueued(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	const (
		id1   = "djpubpart001"
		id2   = "djpubpart002"
		group = "grp_pubpart_it01"
	)
	seedJob(t, pool, id1, "PENDING", group, "")
	seedJob(t, pool, id2, "PENDING", group, "")
	dispatcher := NewMessageGroupDispatcher(pool, partialPublisher{}, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process")
	poller := NewPendingJobPoller(DefaultConfig(), pool, dispatcher, NewPausedConnectionCache(pool, time.Minute))

	mustPoll(t, poller, ctx)
	assert.Equal(t, "QUEUED", jobStatus(t, pool, id1), "the published job is QUEUED")
	assert.Equal(t, "PENDING", jobStatus(t, pool, id2), "the unpublished job stays PENDING")
}

// A job left PROCESSING by a dead attempt that no queue copy will ever come
// back for is returned to PENDING once stale, with the reason recorded.
func TestStaleRecovery_ReturnsAStrandedProcessingJobToPending(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	const id = "djstaleproc1"
	seedJob(t, pool, id, "PROCESSING", "", "")
	_, err := pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET updated_at = NOW() - INTERVAL '2 hours' WHERE id = $1`, id)
	require.NoError(t, err)

	rec := NewStaleQueuedJobPoller(pool, 75*time.Minute, time.Minute)
	n, err := rec.recoverOnce(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, int64(1))
	assert.Equal(t, "PENDING", jobStatus(t, pool, id))
	var lastErr *string
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_error FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&lastErr))
	require.NotNil(t, lastErr)
	assert.Equal(t, StaleProcessingReason, *lastErr)
}
