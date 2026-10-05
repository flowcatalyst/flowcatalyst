//go:build integration

package lifecycletest

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Mark-QUEUED is optimistic: it only queues a job that is still PENDING at the
// version (updated_at) the claim read. A sequential test cannot show the race
// that matters under READ COMMITTED: the UPDATE reaches the row while a callback
// transaction holds its lock and has already rescheduled the job to PENDING at a
// NEW version. When the callback commits, Postgres re-evaluates only the UPDATE's
// own WHERE against the new row version — so the version check must be in that
// WHERE, not only in a sub-query that read the statement's snapshot. Otherwise
// the job is marked QUEUED at its new version although the message published
// for the old one is already consumed: QUEUED with no message until the stale sweep.
func TestMarkQueued_ConcurrentRescheduleIsNotQueued(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)

	j := newJob("g-markrace", common.DispatchBlockOnError, 1)
	rows, err := lc.CreateBatch(ctx, []dispatchjob.DispatchJob{j})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	// The claimed version: what the scheduler's claim read.
	var createdAt, claimedVersion time.Time
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT created_at, updated_at FROM msg_dispatch_jobs WHERE id = $1`, j.ID).Scan(&createdAt, &claimedVersion))
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM msg_dispatch_jobs WHERE id = $1`, j.ID) })

	// Connection A: the callback reschedules the job and holds the row lock, uncommitted.
	time.Sleep(5 * time.Millisecond) // the new version's NOW() must differ from the claimed one
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	ok, err := lc.In(tx).Retry(ctx, j.ID, createdAt, time.Now().Add(time.Minute), new("boom"))
	require.NoError(t, err)
	require.True(t, ok, "the callback's reschedule moved the job")

	// Connection B: the lane's mark-QUEUED for the OLD version.
	type result struct {
		n   int64
		err error
	}
	done := make(chan result, 1)
	go func() {
		n, err := lc.MarkQueued(ctx, []string{j.ID}, []time.Time{createdAt}, []time.Time{claimedVersion})
		done <- result{n, err}
	}()

	// B must be waiting on A's row lock before A commits.
	waitBlocked(t, pool, "UPDATE msg_dispatch_jobs j SET status = 'QUEUED'")
	select {
	case r := <-done:
		t.Fatalf("mark-QUEUED returned while the row lock was held: %+v", r)
	default:
	}

	require.NoError(t, tx.Commit(ctx))
	r := <-done
	require.NoError(t, r.err)
	assert.Zero(t, r.n, "a job rescheduled to a new version while mark-QUEUED waited must not be marked QUEUED")

	var status string
	var version time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, updated_at FROM msg_dispatch_jobs WHERE id = $1`, j.ID).Scan(&status, &version))
	assert.Equal(t, "PENDING", status)
	assert.True(t, version.After(claimedVersion), "the job is at the callback's new version")
}

// waitBlocked waits until a backend running a statement that contains fragment
// (a LIKE-escaped snippet) is waiting on a lock.
func waitBlocked(t *testing.T, pool *pgxpool.Pool, fragment string) {
	t.Helper()
	require.Eventually(t, func() bool {
		var waiting int
		require.NoError(t, pool.QueryRow(context.Background(), `
			SELECT count(*) FROM pg_stat_activity
			 WHERE datname = current_database() AND wait_event_type = 'Lock'
			   AND query LIKE '%' || $1 || '%'`, fragment).Scan(&waiting))
		return waiting > 0
	}, 10*time.Second, 20*time.Millisecond, "the statement was not blocked on the row lock")
}

// The same class on the reaper's sweep: it resets QUEUED/PROCESSING members of a
// group behind a FAILED head, sparing a PROCESSING job touched after liveBefore.
// A sibling that was QUEUED (so exempt) when the sweep read its snapshot and that
// a delivery then claims (PROCESSING, updated_at = now) while the sweep waits for
// the row lock is a live delivery now, and the sweep must leave it alone: the
// liveness test has to be in the UPDATE's own WHERE, where the re-check sees it.
func TestSweepStranded_ConcurrentlyClaimedSiblingIsNotReset(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)

	group := "g-sweeprace-" + tsid.GenerateUntyped()
	head := seed(t, pool, newJob(group, common.DispatchBlockOnError, 1), "FAILED")
	sibJob := newJob(group, common.DispatchBlockOnError, 2)
	sib := seed(t, pool, sibJob, "QUEUED")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM msg_dispatch_jobs WHERE id = ANY($1)`, []string{head.id, sib.id})
	})

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	ok, err := lc.In(tx).ClaimForDelivery(ctx, sib.id, sib.createdAt)
	require.NoError(t, err)
	require.True(t, ok)

	type result struct {
		ids []string
		err error
	}
	done := make(chan result, 1)
	go func() {
		ids, err := lc.SweepStranded(ctx, time.Now().Add(-10*time.Minute), "reaper")
		done <- result{ids, err}
	}()
	waitBlocked(t, pool, "WITH stranded AS")

	require.NoError(t, tx.Commit(ctx))
	r := <-done
	require.NoError(t, r.err)
	assert.NotContains(t, r.ids, sib.id, "a sibling a delivery claimed while the sweep waited is live and must not be reset")
	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM msg_dispatch_jobs WHERE id = $1`, sib.id).Scan(&status))
	assert.Equal(t, "PROCESSING", status)
}
