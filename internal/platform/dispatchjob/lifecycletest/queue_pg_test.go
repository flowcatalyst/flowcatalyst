//go:build integration

package lifecycletest

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

// cleanTables empties both tables: the drift counts and the sweeps are table-wide.
func cleanTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `DELETE FROM msg_dispatch_jobs`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DELETE FROM msg_dispatch_queue`)
	require.NoError(t, err)
}

func queueCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM msg_dispatch_queue`).Scan(&n))
	return n
}

func requireNoDrift(t *testing.T, lc *dispatchjob.Lifecycle) {
	t.Helper()
	missing, orphaned, err := lc.QueueDrift(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 0, missing, "PENDING jobs with a missing or stale queue row")
	require.EqualValues(t, 0, orphaned, "queue rows of jobs that are not PENDING")
}

// Every created job has a queue row mirroring it; a skipped duplicate adds and
// changes nothing; a batch of N gives N rows.
func TestQueueCreate(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	var jobs []dispatchjob.DispatchJob
	for i := 0; i < 7; i++ {
		j := newJob("g-"+tsid.GenerateUntyped(), common.DispatchBlockOnError, int32(i+1))
		pool := "pool-" + tsid.GenerateUntyped()[:6]
		q := "queue-x"
		j.DispatchPoolID = &pool
		j.Queue = &q
		jobs = append(jobs, j)
	}
	rows, err := lc.CreateBatch(ctx, jobs)
	require.NoError(t, err)
	require.Len(t, rows, 7)
	assert.Equal(t, 7, queueCount(t, pool))
	for _, j := range jobs {
		assertQueueInvariant(t, pool, j.ID)
	}

	// Duplicate id (same created_at, different sequence): skipped, nothing changes.
	before := queueSnapshot(t, pool, jobs[0].ID)
	dup := jobs[0]
	dup.Sequence = 55
	rows, err = lc.CreateBatch(ctx, []dispatchjob.DispatchJob{dup})
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.Equal(t, before, queueSnapshot(t, pool, jobs[0].ID))
	assert.Equal(t, 7, queueCount(t, pool))

	// A mixed batch: one duplicate, one new -> one new queue row.
	fresh := newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, 1)
	rows, err = lc.CreateBatch(ctx, []dispatchjob.DispatchJob{dup, fresh})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, 8, queueCount(t, pool))
	assertQueueInvariant(t, pool, fresh.ID)

	// Fan-out create.
	grp, pid, qn := "fo-"+tsid.GenerateUntyped(), "pool-fo", "fo-queue"
	fo := dispatchjob.FanOutJob{
		ID: tsid.GenerateUntyped(), Code: "lifecycle:fanout", Source: "s", EventID: "ev",
		TargetURL: "http://example.invalid", Payload: "{}", SubscriptionID: "sub",
		Mode: "BLOCK_ON_ERROR", Sequence: 7, TimeoutSeconds: 30, MaxRetries: 3,
		IdempotencyKey: "k", CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
		MessageGroup: &grp, DispatchPoolID: &pid, Queue: &qn,
	}
	_, err = lc.CreateFanOut(ctx, []dispatchjob.FanOutJob{fo})
	require.NoError(t, err)
	assertQueueInvariant(t, pool, fo.ID)
	_, err = lc.CreateFanOut(ctx, []dispatchjob.FanOutJob{fo}) // duplicate
	require.NoError(t, err)
	assert.Equal(t, 9, queueCount(t, pool))
	requireNoDrift(t, lc)
}

// The bulk transitions keep the invariant for every job they touch.
func TestQueueBulkTransitions(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)

	t.Run("settle_acked", func(t *testing.T) {
		cleanTables(t, pool)
		var ids []string
		for i := 0; i < 25; i++ {
			st := "QUEUED"
			if i%2 == 0 {
				st = "PROCESSING"
			}
			ids = append(ids, seed(t, pool, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, 1), st).id)
		}
		got, err := lc.SettleAcked(ctx, ids, "test")
		require.NoError(t, err)
		require.Len(t, got, 25)
		for _, id := range ids {
			assertQueueInvariant(t, pool, id)
		}
		// a duplicate hook call is harmless
		got, err = lc.SettleAcked(ctx, ids, "test")
		require.NoError(t, err)
		assert.Empty(t, got)
		requireNoDrift(t, lc)
		assert.Equal(t, 25, queueCount(t, pool))
	})

	t.Run("reaper_sweep", func(t *testing.T) {
		cleanTables(t, pool)
		group := "reap-" + tsid.GenerateUntyped()
		seed(t, pool, newJob(group, common.DispatchBlockOnError, 1), "FAILED")
		var ids []string
		for i := 0; i < 12; i++ {
			st := "QUEUED"
			if i%3 == 0 {
				st = "PROCESSING"
			}
			ids = append(ids, seed(t, pool, newJob(group, common.DispatchBlockOnError, int32(i+2)), st).id)
		}
		got, err := lc.SweepStranded(ctx, time.Now().Add(time.Hour), "test")
		require.NoError(t, err)
		require.Len(t, got, 12)
		for _, id := range ids {
			assertQueueInvariant(t, pool, id)
		}
		requireNoDrift(t, lc)
		assert.Equal(t, 12, queueCount(t, pool))
	})

	t.Run("stale_recovery", func(t *testing.T) {
		cleanTables(t, pool)
		var ids []string
		for i := 0; i < 10; i++ {
			st := "QUEUED"
			if i%2 == 0 {
				st = "PROCESSING"
			}
			ids = append(ids, seed(t, pool, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, 1), st).id)
		}
		n, err := lc.RecoverStaleQueued(ctx, time.Now().Add(-30*time.Minute))
		require.NoError(t, err)
		assert.EqualValues(t, 5, n)
		n, err = lc.RecoverStaleProcessing(ctx, time.Now().Add(-30*time.Minute))
		require.NoError(t, err)
		assert.EqualValues(t, 5, n)
		for _, id := range ids {
			assertQueueInvariant(t, pool, id)
		}
		requireNoDrift(t, lc)
		assert.Equal(t, 10, queueCount(t, pool))
	})

	t.Run("requeue_many", func(t *testing.T) {
		cleanTables(t, pool)
		var js []seeded
		for i, st := range allStatuses {
			js = append(js, seed(t, pool, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, int32(i)), st))
		}
		for _, j := range js {
			ok, err := lc.Requeue(ctx, j.id, j.createdAt)
			require.NoError(t, err)
			require.True(t, ok)
		}
		for _, j := range js {
			assertQueueInvariant(t, pool, j.id)
		}
		requireNoDrift(t, lc)
		assert.Equal(t, len(allStatuses), queueCount(t, pool))
	})
}

// Mark-QUEUED: marked rows leave the queue; a job that re-entered PENDING since
// the claim keeps its refreshed queue row; a stale queue row at the claimed
// version for a job that moved on is removed.
func TestQueueMarkQueued(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	a := seed(t, pool, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, 1), "PENDING")
	b := seed(t, pool, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, 1), "PENDING")
	c := seed(t, pool, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, 1), "PENDING")
	ids := []string{a.id, b.id, c.id}
	versions := []time.Time{a.updatedAt, b.updatedAt, c.updatedAt}

	// b re-enters PENDING after the claim (a retry): new version in job and queue.
	ok, err := lc.Defer(ctx, b.id, b.createdAt, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.True(t, ok)
	bQueue := queueSnapshot(t, pool, b.id)

	// c moved on without the queue row being refreshed (test-only write).
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET status = 'COMPLETED', updated_at = NOW() WHERE id = $1`, c.id)
	require.NoError(t, err)
	require.NotNil(t, queueSnapshot(t, pool, c.id), "fixture: a stale queue row for c")

	n, err := lc.MarkQueued(ctx, ids, versions, a.createdAt.Add(-time.Hour), time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "only a was still at the claimed version")

	assert.Nil(t, queueSnapshot(t, pool, a.id), "a is QUEUED: no queue row")
	assert.Equal(t, bQueue, queueSnapshot(t, pool, b.id), "b re-entered PENDING since the claim: its refreshed row is untouched")
	assertQueueInvariant(t, pool, b.id)
	assert.Nil(t, queueSnapshot(t, pool, c.id), "c's stale queue row at the claimed version is removed")
	requireNoDrift(t, lc)
}

// The migration's backfill produces exactly the PENDING jobs, and re-running it
// is a no-op.
func TestQueueBackfill(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	want := map[string]bool{}
	for i, st := range allStatuses {
		j := seed(t, pool, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, int32(i)), st)
		if st == "PENDING" {
			want[j.id] = true
		}
	}
	for i := 0; i < 3; i++ { // more PENDING ones
		want[seed(t, pool, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, 1), "PENDING").id] = true
	}
	_, err := pool.Exec(ctx, `DELETE FROM msg_dispatch_queue`) // as if the jobs predated the table

	_, file, _, _ := runtime.Caller(0)
	root := file
	for i := 0; i < 5; i++ { // .../internal/platform/dispatchjob/lifecycletest/x.go -> repo root
		root = filepath.Dir(root)
	}
	b, err := os.ReadFile(filepath.Join(root, "internal/migrate/sql/066_dispatch_queue.sql"))
	require.NoError(t, err)
	up := string(b)
	i := strings.Index(up, "-- Backfill")
	j := strings.Index(up, "-- +goose Down")
	require.True(t, i > 0 && j > i, "backfill section not found in 066")
	backfill := up[i:j]

	_, err = pool.Exec(ctx, backfill)
	require.NoError(t, err)
	got := map[string]bool{}
	rows, err := pool.Query(ctx, `SELECT job_id FROM msg_dispatch_queue`)
	require.NoError(t, err)
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		got[id] = true
	}
	rows.Close()
	assert.Equal(t, want, got)
	requireNoDrift(t, lc)

	before := queueCount(t, pool)
	_, err = pool.Exec(ctx, backfill)
	require.NoError(t, err)
	assert.Equal(t, before, queueCount(t, pool), "re-running the backfill is a no-op")
}

// QueueDrift is zero on a consistent state and counts each corruption.
func TestQueueDrift(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	p1 := seed(t, pool, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, 1), "PENDING")
	p2 := seed(t, pool, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, 1), "PENDING")
	p3 := seed(t, pool, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, 1), "PENDING")
	done := seed(t, pool, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, 1), "COMPLETED")
	requireNoDrift(t, lc)

	_, err := pool.Exec(ctx, `DELETE FROM msg_dispatch_queue WHERE job_id = $1`, p1.id) // missing
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_queue SET version = version + interval '1 second' WHERE job_id = $1`, p2.id) // stale version
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_queue SET scheduled_for = NOW() WHERE job_id = $1`, p3.id) // stale schedule
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO msg_dispatch_queue (job_id, job_created_at, sequence, mode, version)
		VALUES ($1, $2, 1, 'IMMEDIATE', NOW()), ($3, NOW(), 1, 'IMMEDIATE', NOW())`, done.id, done.createdAt, "ghost-job-001") // orphans
	require.NoError(t, err)

	missing, orphaned, err := lc.QueueDrift(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 3, missing)
	assert.EqualValues(t, 2, orphaned)
}

// Deterministic version of the documented race: a job's row lock is held while
// an enter and a leave of the same job queue behind it.
//
//	leave-then-enter: the job ends PENDING and MUST have its queue row (no lost job).
//	enter-then-leave: the leave's DELETE cannot see the queue row the enter just
//	inserted, so an orphan queue row is the accepted, harmless outcome.
func TestQueueEnterLeaveRace(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)

	blocked := func(n int) {
		require.Eventually(t, func() bool {
			var c int
			_ = pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
				WHERE wait_event_type = 'Lock' AND query LIKE '%msg_dispatch_jobs%'`).Scan(&c)
			return c >= n
		}, 10*time.Second, 20*time.Millisecond)
	}
	run := func(t *testing.T, first, second func(*dispatchjob.Lifecycle, seeded) error) (j seeded) {
		cleanTables(t, pool)
		j = seed(t, pool, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, 1), "PROCESSING")
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, `SELECT 1 FROM msg_dispatch_jobs WHERE id = $1 FOR UPDATE`, j.id)
		require.NoError(t, err)
		var wg sync.WaitGroup
		errs := make([]error, 2)
		wg.Add(1)
		go func() { defer wg.Done(); errs[0] = first(lc, j) }()
		blocked(1)
		wg.Add(1)
		go func() { defer wg.Done(); errs[1] = second(lc, j) }()
		blocked(2)
		require.NoError(t, tx.Commit(ctx))
		wg.Wait()
		require.NoError(t, errs[0])
		require.NoError(t, errs[1])
		return j
	}
	leave := func(lc *dispatchjob.Lifecycle, j seeded) error {
		_, err := lc.Complete(ctx, j.id, j.createdAt, 1)
		return err
	}
	enter := func(lc *dispatchjob.Lifecycle, j seeded) error {
		_, err := lc.Requeue(ctx, j.id, j.createdAt)
		return err
	}

	t.Run("leave_then_enter_never_loses_the_job", func(t *testing.T) {
		j := run(t, leave, enter)
		assert.Equal(t, "PENDING", snapshot(t, pool, j.id)["status"])
		assertQueueInvariant(t, pool, j.id)
		requireNoDrift(t, lc)
	})

	t.Run("enter_then_leave_may_orphan", func(t *testing.T) {
		j := run(t, enter, leave)
		assert.Equal(t, "COMPLETED", snapshot(t, pool, j.id)["status"])
		missing, orphaned, err := lc.QueueDrift(ctx)
		require.NoError(t, err)
		assert.EqualValues(t, 0, missing)
		t.Logf("enter-then-leave race: orphaned queue rows = %d (accepted anomaly)", orphaned)
		assert.LessOrEqual(t, orphaned, int64(1))
	})
}

// A few thousand random lifecycle operations over a few hundred jobs from
// several workers: no PENDING job may be missing from the queue (or stale).
// Orphans are the documented race's harmless residue; they are counted.
func TestQueueRandomized(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	const nJobs, workers, opsPerWorker = 300, 8, 500
	var jobs []dispatchjob.DispatchJob
	for i := 0; i < nJobs; i++ {
		jobs = append(jobs, newJob("g-"+tsid.GenerateUntyped()[:8], common.DispatchBlockOnError, int32(i%5+1)))
	}
	_, err := lc.CreateBatch(ctx, jobs)
	require.NoError(t, err)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	fail := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(seedN int64) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(seedN))
			for i := 0; i < opsPerWorker; i++ {
				j := jobs[rng.Intn(nJobs)]
				soon := time.Now().Add(time.Duration(rng.Intn(120)) * time.Second)
				msg := "random"
				var err error
				switch rng.Intn(11) {
				case 0:
					_, err = lc.Retry(ctx, j.ID, j.CreatedAt, soon, &msg)
				case 1:
					_, err = lc.Defer(ctx, j.ID, j.CreatedAt, soon)
				case 2:
					_, err = lc.Hold(ctx, j.ID, j.CreatedAt, soon)
				case 3:
					_, err = lc.Requeue(ctx, j.ID, j.CreatedAt)
				case 4, 5:
					var v time.Time
					if err = pool.QueryRow(ctx, `SELECT updated_at FROM msg_dispatch_jobs WHERE id = $1`, j.ID).Scan(&v); err == nil {
						_, err = lc.MarkQueued(ctx, []string{j.ID}, []time.Time{v}, j.CreatedAt, j.CreatedAt)
					}
				case 6:
					_, err = lc.ClaimForDelivery(ctx, j.ID, j.CreatedAt)
				case 7:
					_, err = lc.Complete(ctx, j.ID, j.CreatedAt, 3)
				case 8:
					_, err = lc.Fail(ctx, j.ID, j.CreatedAt, &msg, 3)
				case 9:
					_, err = lc.SettleAcked(ctx, []string{j.ID, jobs[rng.Intn(nJobs)].ID}, "random")
				case 10:
					_, err = lc.ReclaimStaleDelivery(ctx, j.ID, j.CreatedAt, time.Now().Add(time.Hour))
				}
				if err != nil {
					fail(err)
					return
				}
			}
		}(int64(w + 1))
	}
	wg.Wait()
	require.NoError(t, firstErr)

	missing, orphaned, err := lc.QueueDrift(ctx)
	require.NoError(t, err)
	t.Logf("randomized: %d ops over %d jobs: missing-or-stale=%d orphaned=%d", workers*opsPerWorker, nJobs, missing, orphaned)
	assert.EqualValues(t, 0, missing, "a PENDING job without an exact queue row is a lost job")
}
