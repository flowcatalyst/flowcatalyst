//go:build integration

package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The QUEUED update is guarded on PENDING: the router can deliver a job and the
// callback move it on before the lane's update runs, and the update must never
// regress it.
func TestUpdateQueued_DoesNotRegressAJobThatMovedPastPending(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	poller := newTestPoller(pool)

	ids := map[string]string{
		"PENDING":    "djmarkguard01",
		"PROCESSING": "djmarkguard02",
		"COMPLETED":  "djmarkguard03",
		"FAILED":     "djmarkguard04",
	}
	for status, id := range ids {
		seedJob(t, pool, id, status, "grp_markguard_it", "")
	}
	all := []string{ids["PENDING"], ids["PROCESSING"], ids["COMPLETED"], ids["FAILED"]}

	rows, err := poller.updateQueued(ctx, all, createdOf(t, pool, all), versionsOf(t, pool, all))
	require.NoError(t, err)
	assert.Equal(t, int64(1), rows, "only the PENDING row is updated")
	assert.Equal(t, "QUEUED", jobStatus(t, pool, ids["PENDING"]))
	assert.Equal(t, "PROCESSING", jobStatus(t, pool, ids["PROCESSING"]))
	assert.Equal(t, "COMPLETED", jobStatus(t, pool, ids["COMPLETED"]))
	assert.Equal(t, "FAILED", jobStatus(t, pool, ids["FAILED"]))

	var queuedAt *time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT queued_at FROM msg_dispatch_jobs WHERE id = $1`, ids["PENDING"]).Scan(&queuedAt))
	assert.NotNil(t, queuedAt, "queued_at is stamped")
}

func createdOf(t *testing.T, pool *pgxpool.Pool, ids []string) []time.Time {
	t.Helper()
	out := make([]time.Time, len(ids))
	for i, id := range ids {
		require.NoError(t, pool.QueryRow(context.Background(),
			`SELECT created_at FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&out[i]))
	}
	return out
}

func versionsOf(t *testing.T, pool *pgxpool.Pool, ids []string) []time.Time {
	t.Helper()
	out := make([]time.Time, len(ids))
	for i, id := range ids {
		require.NoError(t, pool.QueryRow(context.Background(),
			`SELECT updated_at FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&out[i]))
	}
	return out
}

// The race the status guard alone cannot see: the lane publishes, the router
// delivers, and the callback processes the job and reschedules it back to PENDING
// (a retry, a deferral, a BLOCK_ON_ERROR hold) before the lane's update runs.
// The job is PENDING again but is a newer version of the row, and has no message
// in the queue; setting it QUEUED would strand it until the stale sweep. The
// callback's own repository calls are used, so the test also pins that each of
// them stamps updated_at.
func TestUpdateQueued_DoesNotQueueAJobTheCallbackRescheduledToPending(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	poller := newTestPoller(pool)
	repo := dispatchjob.NewRepository(pool)

	cases := map[string]func(id string, createdAt time.Time) error{
		"reschedule (deferral / group hold)": func(id string, c time.Time) error {
			return repo.Reschedule(ctx, id, c, time.Now().Add(time.Minute))
		},
		"schedule retry": func(id string, c time.Time) error {
			return repo.ScheduleRetry(ctx, id, c, time.Now().Add(time.Minute), new("boom"))
		},
	}
	n := 0
	for name, move := range cases {
		n++
		id := fmt.Sprintf("djmarkresch%02d", n)
		seedJob(t, pool, id, "PENDING", "grp_markresched_it", "")
		var createdAt time.Time
		require.NoError(t, pool.QueryRow(ctx, `SELECT created_at FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&createdAt))
		claimedVersion := versionsOf(t, pool, []string{id})

		// The callback claims the job and moves it back to PENDING.
		claimed, err := repo.ClaimForDelivery(ctx, id, createdAt)
		require.NoError(t, err)
		require.True(t, claimed)
		time.Sleep(2 * time.Millisecond)
		require.NoError(t, move(id, createdAt), name)
		require.Equal(t, "PENDING", jobStatus(t, pool, id))

		rows, err := poller.updateQueued(ctx, []string{id}, []time.Time{createdAt}, claimedVersion)
		require.NoError(t, err)
		assert.Zero(t, rows, "%s: a job moved on and rescheduled is not set QUEUED", name)
		assert.Equal(t, "PENDING", jobStatus(t, pool, id), name)
	}
}

// End to end: a job the callback already moved to PROCESSING when the lane's
// update runs is left alone, and the skip is counted.
func TestLane_DoesNotRegressAJobTheCallbackAlreadyMovedOn(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	const id = "djmarkguard11"
	seedJob(t, pool, id, "PENDING", "grp_markguard_e2e", "")

	// The publisher stands in for "the router delivered and the callback ran":
	// by the time the publish returns the job is already PROCESSING.
	pub := publisherFunc(func(ctx context.Context, items []PublishItem) ([]string, error) {
		_, err := pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET status = 'PROCESSING' WHERE id = ANY($1)`, jobIDs(items))
		return nil, err
	})
	poller := NewPendingJobPoller(DefaultConfig(), pool,
		NewMessageGroupDispatcher(pool, pub, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process"),
		NewPausedConnectionCache(pool, time.Minute))
	before := value(t, MetricsRegistry, "fc_scheduler_mark_queued_not_updated_total")

	mustPoll(t, poller, ctx)
	assert.Equal(t, "PROCESSING", jobStatus(t, pool, id), "never regressed to QUEUED")
	assert.GreaterOrEqual(t, value(t, MetricsRegistry, "fc_scheduler_mark_queued_not_updated_total")-before, 1.0)
}

type publisherFunc func(ctx context.Context, items []PublishItem) ([]string, error)

func (f publisherFunc) Publish(ctx context.Context, items []PublishItem) ([]string, error) {
	return f(ctx, items)
}
