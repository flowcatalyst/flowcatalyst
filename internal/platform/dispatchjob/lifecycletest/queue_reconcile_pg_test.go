//go:build integration

package lifecycletest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

const (
	reconMinAge     = time.Minute
	reconClaimStale = 5 * time.Minute
)

func reconcileOnce(t *testing.T, lc *dispatchjob.Lifecycle) dispatchjob.ReconcileResult {
	t.Helper()
	res, err := lc.Reconcile(context.Background(), reconMinAge, reconClaimStale, 5000)
	require.NoError(t, err)
	return res
}

func g() string { return "g-" + tsid.GenerateUntyped() }

// A consistent table reports zero and changes nothing.
func TestReconcile_ConsistentTableIsLeftAlone(t *testing.T) {
	pool := testpg.Pool(t)
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	var ids []string
	for i := range 4 {
		ids = append(ids, seed(t, pool, newJob(g(), common.DispatchBlockOnError, int32(i)), "PENDING").id)
	}
	ids = append(ids, seed(t, pool, newJob(g(), common.DispatchBlockOnError, 1), "COMPLETED").id)
	ids = append(ids, seed(t, pool, newJob(g(), common.DispatchBlockOnError, 1), "QUEUED").id)
	// a claimed row is part of a consistent table too
	_, err := pool.Exec(context.Background(), `UPDATE msg_dispatch_queue SET claimed_at = NOW() WHERE job_id = $1`, ids[0])
	require.NoError(t, err)
	before := map[string]map[string]any{}
	for _, id := range ids {
		before[id] = queueSnapshot(t, pool, id)
	}

	res := reconcileOnce(t, lc)
	assert.Zero(t, res.Total(), "nothing to repair: %+v", res)
	for _, id := range ids {
		assert.Equal(t, before[id], queueSnapshot(t, pool, id), "row %s changed", id)
	}
	requireNoDrift(t, lc)
}

// (a) a PENDING job with no queue row gets one — but only once its updated_at is
// older than the age guard: a job being created right now is left alone.
func TestReconcile_InsertsTheMissingRow(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	old := seed(t, pool, newJob(g(), common.DispatchBlockOnError, 1), "PENDING") // updated_at an hour ago
	young := newJob(g(), common.DispatchBlockOnError, 1)
	mustCreate(t, lc, young) // updated_at now
	soon := time.Now().Add(10 * time.Minute)
	sched := seed(t, pool, newJob(g(), common.DispatchBlockOnError, 1), "PENDING")
	_, err := pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET scheduled_for = $2 WHERE id = $1`, sched.id, soon)
	require.NoError(t, err)
	syncQueueFixture(t, pool, sched.id)

	for _, id := range []string{old.id, young.ID, sched.id} {
		_, err = pool.Exec(ctx, `DELETE FROM msg_dispatch_queue WHERE job_id = $1`, id) // the corruption
		require.NoError(t, err)
	}

	res := reconcileOnce(t, lc)
	assert.EqualValues(t, 2, res.Inserted, "the two old jobs get a row")
	assert.Zero(t, res.Deleted+res.Refreshed)
	assertQueueInvariant(t, pool, old.id)
	assertQueueInvariant(t, pool, sched.id)
	assert.Nil(t, queueSnapshot(t, pool, young.ID), "a job younger than the age guard is left alone")

	// Once it has aged, the next pass repairs it.
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET updated_at = NOW() - interval '2 minutes' WHERE id = $1`, young.ID)
	require.NoError(t, err)
	res = reconcileOnce(t, lc)
	assert.EqualValues(t, 1, res.Inserted)
	assertQueueInvariant(t, pool, young.ID)
	assert.Zero(t, reconcileOnce(t, lc).Total(), "and it is stable")
}

// (b) a queue row whose job is missing or not PENDING is deleted, when it is
// unclaimed or its claim is old; a recently claimed row and a recently refreshed
// one are left.
func TestReconcile_DeletesOrphanRows(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	keep := seed(t, pool, newJob(g(), common.DispatchBlockOnError, 1), "PENDING")
	notPending := seed(t, pool, newJob(g(), common.DispatchBlockOnError, 1), "PENDING")
	claimedRecent := seed(t, pool, newJob(g(), common.DispatchBlockOnError, 1), "PENDING")
	claimedOld := seed(t, pool, newJob(g(), common.DispatchBlockOnError, 1), "PENDING")
	justRefreshed := seed(t, pool, newJob(g(), common.DispatchBlockOnError, 1), "PENDING")
	for _, id := range []string{notPending.id, claimedRecent.id, claimedOld.id, justRefreshed.id} {
		// the job left PENDING without the queue row being removed (an old binary)
		_, err := pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET status = 'COMPLETED' WHERE id = $1`, id)
		require.NoError(t, err)
	}
	_, err := pool.Exec(ctx, `UPDATE msg_dispatch_queue SET claimed_at = NOW() WHERE job_id = $1`, claimedRecent.id)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_queue SET claimed_at = NOW() - interval '10 minutes' WHERE job_id = $1`, claimedOld.id)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_queue SET version = NOW() WHERE job_id = $1`, justRefreshed.id)
	require.NoError(t, err)
	// a queue row whose job does not exist at all
	_, err = pool.Exec(ctx, `INSERT INTO msg_dispatch_queue (job_id, job_created_at, sequence, mode, version)
		VALUES ('ghost-job-002', NOW() - interval '1 hour', 1, 'IMMEDIATE', NOW() - interval '1 hour')`)
	require.NoError(t, err)

	res := reconcileOnce(t, lc)
	assert.EqualValues(t, 3, res.Deleted, "the non-PENDING job's row, the old-claim row and the ghost")
	assert.Zero(t, res.Inserted+res.Refreshed)
	assert.NotNil(t, queueSnapshot(t, pool, keep.id))
	assert.Nil(t, queueSnapshot(t, pool, notPending.id))
	assert.Nil(t, queueSnapshot(t, pool, claimedOld.id))
	assert.Nil(t, queueSnapshot(t, pool, "ghost-job-002"))
	assert.NotNil(t, queueSnapshot(t, pool, claimedRecent.id), "a recent claim may be mid-publish")
	assert.NotNil(t, queueSnapshot(t, pool, justRefreshed.id), "a row refreshed within the age guard may be mid-transition")
}

// (c) a queue row that no longer mirrors its PENDING job is refreshed from the
// job, and its claim cleared; a row refreshed within the age guard is left.
func TestReconcile_RefreshesAStaleRow(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	staleVersion := seed(t, pool, newJob(g(), common.DispatchBlockOnError, 1), "PENDING")
	staleSchedule := seed(t, pool, newJob(g(), common.DispatchBlockOnError, 1), "PENDING")
	claimedRecent := seed(t, pool, newJob(g(), common.DispatchBlockOnError, 1), "PENDING")
	claimedOld := seed(t, pool, newJob(g(), common.DispatchBlockOnError, 1), "PENDING")
	young := seed(t, pool, newJob(g(), common.DispatchBlockOnError, 1), "PENDING")

	_, err := pool.Exec(ctx, `UPDATE msg_dispatch_queue SET version = version - interval '30 seconds', sequence = 99 WHERE job_id = ANY($1)`,
		[]string{staleVersion.id, claimedRecent.id, claimedOld.id})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_queue SET scheduled_for = NOW() + interval '1 hour' WHERE job_id = $1`, staleSchedule.id)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_queue SET claimed_at = NOW() WHERE job_id = $1`, claimedRecent.id)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_queue SET claimed_at = NOW() - interval '10 minutes' WHERE job_id = $1`, claimedOld.id)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_queue SET version = NOW(), sequence = 99 WHERE job_id = $1`, young.id)
	require.NoError(t, err)
	youngBefore := queueSnapshot(t, pool, young.id)
	claimedRecentBefore := queueSnapshot(t, pool, claimedRecent.id)

	res := reconcileOnce(t, lc)
	assert.EqualValues(t, 3, res.Refreshed)
	assert.Zero(t, res.Inserted+res.Deleted)
	for _, id := range []string{staleVersion.id, staleSchedule.id, claimedOld.id} {
		assertQueueInvariant(t, pool, id)
	}
	assert.Equal(t, youngBefore, queueSnapshot(t, pool, young.id), "refreshed within the age guard: left alone")
	assert.Equal(t, claimedRecentBefore, queueSnapshot(t, pool, claimedRecent.id), "recently claimed: left alone")
}

// Each kind of repair is bounded per pass.
func TestReconcile_IsBoundedPerPass(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	for i := range 6 {
		s := seed(t, pool, newJob(g(), common.DispatchBlockOnError, int32(i)), "PENDING")
		_, err := pool.Exec(ctx, `DELETE FROM msg_dispatch_queue WHERE job_id = $1`, s.id)
		require.NoError(t, err)
	}
	res, err := lc.Reconcile(ctx, reconMinAge, reconClaimStale, 4)
	require.NoError(t, err)
	assert.EqualValues(t, 4, res.Inserted)
	res, err = lc.Reconcile(ctx, reconMinAge, reconClaimStale, 4)
	require.NoError(t, err)
	assert.EqualValues(t, 2, res.Inserted)
	requireNoDrift(t, lc)
}

// A reconcile that races a live lifecycle transition never loses a job: a job
// retried while the sweep runs keeps a queue row that mirrors it.
func TestReconcile_RunningAlongsideTransitionsKeepsEveryPendingJobQueued(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	var jobs []dispatchjob.DispatchJob
	for i := range 40 {
		jobs = append(jobs, newJob(g(), common.DispatchBlockOnError, int32(i)))
	}
	mustCreate(t, lc, jobs...)
	_, err := pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET updated_at = updated_at - interval '1 hour'`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_queue SET version = version - interval '1 hour'`)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 30 {
			for _, j := range jobs {
				_, _ = lc.Defer(ctx, j.ID, j.CreatedAt, time.Now().Add(time.Minute))
			}
		}
	}()
	for {
		select {
		case <-done:
			requireNoDrift(t, lc)
			return
		default:
			_, err := lc.Reconcile(ctx, reconMinAge, reconClaimStale, 5000)
			require.NoError(t, err)
		}
	}
}
