//go:build integration

package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// capturePublisher is a queue.Publisher that records published message
// IDs. Submit dispatches asynchronously, so it must be race-safe.
type capturePublisher struct {
	mu  sync.Mutex
	ids []string
}

func (p *capturePublisher) Identifier() string { return "capture" }

func (p *capturePublisher) Publish(_ context.Context, m common.Message) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ids = append(p.ids, m.ID)
	return m.ID, nil
}

func (p *capturePublisher) PublishBatch(ctx context.Context, msgs []common.Message) ([]string, error) {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		id, _ := p.Publish(ctx, m)
		out = append(out, id)
	}
	return out, nil
}

// failPublisher fails every publish — exercises the batch revert path.
type failPublisher struct{}

func (failPublisher) Identifier() string { return "fail" }
func (failPublisher) Publish(context.Context, common.Message) (string, error) {
	return "", errors.New("publish boom")
}
func (failPublisher) PublishBatch(context.Context, []common.Message) ([]string, error) {
	return nil, errors.New("batch publish boom")
}

// TestPollOnce_BatchPublishFailureRevertsToPending pins the batched-dispatch
// revert: when the single PublishBatch fails, the whole claimed batch must be
// rolled back QUEUED→PENDING so the next poll re-dispatches it (rather than
// stranding rows in QUEUED for stale recovery).
func TestPollOnce_BatchPublishFailureRevertsToPending(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	const (
		id1 = "djbatchfail01"
		id2 = "djbatchfail02"
	)
	seedJob(t, pool, id1, "PENDING", "grp_batchfail_it", "")
	seedJob(t, pool, id2, "PENDING", "grp_batchfail_it", "")

	dispatcher := NewMessageGroupDispatcher(pool, failPublisher{}, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process")
	poller := NewPendingJobPoller(DefaultConfig(), pool, dispatcher, NewPausedConnectionCache(pool, time.Minute))

	require.NoError(t, poller.pollOnce(ctx))

	require.Equal(t, "PENDING", jobStatus(t, pool, id1), "failed batch publish must revert to PENDING")
	require.Equal(t, "PENDING", jobStatus(t, pool, id2), "failed batch publish must revert to PENDING")
}

// newTestPoller wires a poller against the shared pool with a fresh
// paused cache (first PausedSubscriptionIDs call always refreshes).
func newTestPoller(pool *pgxpool.Pool) *PendingJobPoller {
	dispatcher := NewMessageGroupDispatcher(pool, &capturePublisher{}, NewDispatchAuthService("test-secret"), "http://localhost:8080/api/dispatch/process")
	cache := NewPausedConnectionCache(pool, time.Minute)
	return NewPendingJobPoller(DefaultConfig(), pool, dispatcher, cache)
}

// seedJob inserts a msg_dispatch_jobs row. Empty group/subID seed NULL.
func seedJob(t *testing.T, pool *pgxpool.Pool, id, status, group, subID string) {
	t.Helper()
	var groupPtr, subPtr *string
	if group != "" {
		groupPtr = &group
	}
	if subID != "" {
		subPtr = &subID
	}
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_dispatch_jobs (id, code, target_url, status, message_group, subscription_id)
		 VALUES ($1, 'scheduler:poller:test', 'http://example.invalid/hook', $2, $3, $4)`,
		id, status, groupPtr, subPtr)
	require.NoError(t, err)
}

func jobStatus(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()
	var status string
	err := pool.QueryRow(context.Background(),
		`SELECT status FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&status)
	require.NoError(t, err)
	return status
}

// TestPollOnce_BlockedGroupHoldback pins the blocked-group filter: a
// FAILED sibling holds the whole message group in PENDING (no QUEUED
// flip), and resolving the failure releases the group on the next poll.
func TestPollOnce_BlockedGroupHoldback(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	poller := newTestPoller(pool)

	const (
		group     = "grp_blkhold_it01" // unique to this test
		failedID  = "djblkhfail001"
		pendingID = "djblkhpend001"
	)
	seedJob(t, pool, failedID, "FAILED", group, "")
	seedJob(t, pool, pendingID, "PENDING", group, "")

	require.NoError(t, poller.pollOnce(ctx))
	require.Equal(t, "PENDING", jobStatus(t, pool, pendingID),
		"sibling of a FAILED job must be held back, not claimed")
	require.Equal(t, "FAILED", jobStatus(t, pool, failedID),
		"the failed job itself must be untouched")

	// Operator resolves the failure → the group unblocks next poll.
	_, err := pool.Exec(ctx,
		`UPDATE msg_dispatch_jobs SET status = 'COMPLETED', updated_at = NOW() WHERE id = $1`,
		failedID)
	require.NoError(t, err)

	require.NoError(t, poller.pollOnce(ctx))
	require.Equal(t, "QUEUED", jobStatus(t, pool, pendingID),
		"resolved group must dispatch on the next poll")
}

// TestPollOnce_NullGroupFailureDoesNotBlock pins the NULL semantics of
// the blocked-group query: `message_group = ANY($1)` never matches NULL,
// so a failed ungrouped job must not hold back other ungrouped jobs
// (the in-memory "default" bucket).
func TestPollOnce_NullGroupFailureDoesNotBlock(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	poller := newTestPoller(pool)

	const (
		failedID  = "djnullfail001"
		pendingID = "djnullpend001"
	)
	seedJob(t, pool, failedID, "FAILED", "", "")
	seedJob(t, pool, pendingID, "PENDING", "", "")

	require.NoError(t, poller.pollOnce(ctx))
	require.Equal(t, "QUEUED", jobStatus(t, pool, pendingID),
		"an ungrouped FAILED job must not block other ungrouped jobs")
}

// TestPollOnce_PausedConnectionHoldsJob pins the paused-connection
// filter: a PENDING job whose subscription's connection is PAUSED stays
// PENDING, and reactivating the connection releases it.
func TestPollOnce_PausedConnectionHoldsJob(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	const (
		connID = "conn_pausedit001"
		subID  = "sub_pausedit0001"
		jobID  = "djpausedjob01"
	)
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_connections (id, code, name, service_account_id, status)
		 VALUES ($1, 'scheduler-poller-paused-conn', 'Paused conn (poller IT)', 'sa_pausedit00001', 'PAUSED')`,
		connID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO msg_subscriptions (id, code, name, target, connection_id)
		 VALUES ($1, 'scheduler-poller-paused-sub', 'Paused sub (poller IT)', 'http://example.invalid/hook', $2)`,
		subID, connID)
	require.NoError(t, err)
	seedJob(t, pool, jobID, "PENDING", "grp_paused_it01", subID)

	poller := newTestPoller(pool)
	require.NoError(t, poller.pollOnce(ctx))
	require.Equal(t, "PENDING", jobStatus(t, pool, jobID),
		"job behind a PAUSED connection must stay PENDING")

	// Reactivate. A fresh poller (fresh cache) sees the change at once —
	// the long-lived cache would converge within its 60s TTL.
	_, err = pool.Exec(ctx,
		`UPDATE msg_connections SET status = 'ACTIVE', updated_at = NOW() WHERE id = $1`,
		connID)
	require.NoError(t, err)

	poller = newTestPoller(pool)
	require.NoError(t, poller.pollOnce(ctx))
	require.Equal(t, "QUEUED", jobStatus(t, pool, jobID),
		"reactivated connection must release the job")
}
