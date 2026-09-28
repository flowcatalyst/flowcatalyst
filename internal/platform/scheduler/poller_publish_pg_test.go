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

// dyingPublisher checks, while it is publishing, what another connection
// sees of the claimed rows, then "dies" (panics) part way through the batch
// — a worker SIGKILLed mid-publish.
type dyingPublisher struct {
	t        *testing.T
	pool     *pgxpool.Pool
	seenMid  map[string]string
	observed bool
}

func (d *dyingPublisher) Publish(ctx context.Context, items []PublishItem) ([]string, error) {
	d.seenMid = map[string]string{}
	for _, it := range items {
		var status string
		require.NoError(d.t, d.pool.QueryRow(ctx, `SELECT status FROM msg_dispatch_jobs WHERE id = $1`, it.JobID).Scan(&status))
		d.seenMid[it.JobID] = status
	}
	d.observed = true
	panic("worker killed mid-publish")
}

// The claim used to be committed QUEUED before the batch was published, so a
// worker killed during the publish stranded every unpublished job QUEUED,
// with no queue message, for stale recovery's 75 minutes (the delivery
// harness lost 2 of 40 on worker-restart). The claim now commits only after
// the publish: a worker that dies mid-publish leaves the whole claim
// PENDING, and the next poll publishes it.
func TestPollOnce_WorkerDeathMidPublishLeavesTheClaimPending(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	const (
		id1   = "djpubdie0001"
		id2   = "djpubdie0002"
		group = "grp_pubdie_it01"
	)
	seedJob(t, pool, id1, "PENDING", group, "")
	seedJob(t, pool, id2, "PENDING", group, "")

	dying := &dyingPublisher{t: t, pool: pool}
	dispatcher := NewMessageGroupDispatcher(pool, dying, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process")
	poller := NewPendingJobPoller(DefaultConfig(), pool, dispatcher, NewPausedConnectionCache(pool, time.Minute))

	func() {
		defer func() { _ = recover() }()
		_ = poller.pollOnce(ctx)
	}()
	require.True(t, dying.observed, "the publisher was reached")
	assert.Equal(t, "PENDING", dying.seenMid[id1], "nothing is committed QUEUED before the publish")
	assert.Equal(t, "PENDING", dying.seenMid[id2])
	assert.Equal(t, "PENDING", jobStatus(t, pool, id1), "a worker that dies mid-publish strands nothing")
	assert.Equal(t, "PENDING", jobStatus(t, pool, id2))

	// The restarted worker's poll publishes the claim again.
	capture := &capturePublisher{}
	dispatcher = NewMessageGroupDispatcher(pool, capture, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process")
	poller = NewPendingJobPoller(DefaultConfig(), pool, dispatcher, NewPausedConnectionCache(pool, time.Minute))
	require.NoError(t, poller.pollOnce(ctx))
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

	require.NoError(t, poller.pollOnce(ctx))
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
