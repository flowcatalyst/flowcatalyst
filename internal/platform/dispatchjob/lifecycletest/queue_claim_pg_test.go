//go:build integration

package lifecycletest

import (
	"context"
	"fmt"
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

func inQueue(t *testing.T, pool *pgxpool.Pool, id string) bool {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM msg_dispatch_queue WHERE job_id = $1`, id).Scan(&n))
	return n == 1
}

func claimIDsOf(cs []dispatchjob.QueueClaim) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.JobID
	}
	return out
}

func mustCreate(t *testing.T, lc *dispatchjob.Lifecycle, jobs ...dispatchjob.DispatchJob) {
	t.Helper()
	rows, err := lc.CreateBatch(context.Background(), jobs)
	require.NoError(t, err)
	require.Len(t, rows, len(jobs))
}

// The claim returns due, unpaused rows outside the held groups in delivery order —
// group (ungrouped last), sequence, created_at, id — up to the limit, and deletes
// them from the queue.
func TestQueueClaim_OrderAndFilters(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	mk := func(group string, seq int32, age time.Duration) dispatchjob.DispatchJob {
		j := newJob(group, common.DispatchBlockOnError, seq)
		j.CreatedAt = base.Add(age)
		if group == "" {
			j.MessageGroup = nil
		}
		return j
	}
	// created out of order on purpose
	b2 := mk("b", 2, 3*time.Second)
	a2 := mk("a", 2, 2*time.Second)
	b1 := mk("b", 1, 5*time.Second)
	a1late := mk("a", 1, 9*time.Second)
	a1early := mk("a", 1, time.Second)
	u1 := mk("", 1, time.Second)
	u2 := mk("", 1, 2*time.Second)
	// not claimable: not yet due, paused subscription
	future := mk("a", 3, 0)
	soon := time.Now().Add(time.Hour)
	future.ScheduledFor = &soon
	paused := mk("a", 4, 0)
	pausedSub := "sub-paused-" + tsid.GenerateUntyped()[:6]
	paused.SubscriptionID = &pausedSub
	mustCreate(t, lc, b2, a2, b1, a1late, a1early, u1, u2, future, paused)

	// group "b" is held: skipped by the walk
	got, err := lc.ClaimQueue(ctx, 100, []string{pausedSub}, []string{"b"})
	require.NoError(t, err)
	assert.Equal(t, []string{a1early.ID, a1late.ID, a2.ID, u1.ID, u2.ID}, claimIDsOf(got),
		"group order, then sequence, created_at; ungrouped last; the future, paused and held rows are left")
	for _, c := range got {
		assert.False(t, inQueue(t, pool, c.JobID), "claimed rows are deleted from the queue")
	}
	assert.True(t, inQueue(t, pool, future.ID))
	assert.True(t, inQueue(t, pool, paused.ID))
	assert.True(t, inQueue(t, pool, b1.ID) && inQueue(t, pool, b2.ID), "a held group's rows are untouched")

	// Claimed rows are never returned again; the held group is claimed once it is no longer held.
	again, err := lc.ClaimQueue(ctx, 100, []string{pausedSub}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{b1.ID, b2.ID}, claimIDsOf(again))

	// The limit bounds the claim and leaves the rest unclaimed: the first two in order.
	cleanTables(t, pool)
	mustCreate(t, lc, mk("a", 1, time.Second), mk("a", 2, 2*time.Second), mk("a", 3, 3*time.Second), mk("a", 4, 4*time.Second))
	two, err := lc.ClaimQueue(ctx, 2, nil, nil) // nil arrays are treated as empty
	require.NoError(t, err)
	require.Len(t, two, 2)
	assert.EqualValues(t, []int32{1, 2}, []int32{two[0].Sequence, two[1].Sequence})
	rest, err := lc.ClaimQueue(ctx, 100, []string{}, []string{})
	require.NoError(t, err)
	assert.EqualValues(t, []int32{3, 4}, []int32{rest[0].Sequence, rest[1].Sequence})
}

// The claim returns everything the scheduler publishes from, including the
// version the mark-QUEUED update is optimistic on.
func TestQueueClaim_ReturnsTheRowsColumns(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	j := newJob("g-"+tsid.GenerateUntyped(), common.DispatchBlockOnError, 7)
	sub, poolID, client, queue := "sub-1", "pool-1", "client-1", "HIGH"
	j.SubscriptionID, j.DispatchPoolID, j.ClientID, j.Queue = &sub, &poolID, &client, &queue
	mustCreate(t, lc, j)

	got, err := lc.ClaimQueue(ctx, 10, []string{}, []string{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	c := got[0]
	assert.Equal(t, j.ID, c.JobID)
	assert.True(t, j.CreatedAt.Equal(c.JobCreatedAt))
	assert.Equal(t, j.MessageGroup, c.MessageGroup)
	assert.EqualValues(t, 7, c.Sequence)
	assert.Equal(t, "BLOCK_ON_ERROR", c.Mode)
	assert.Equal(t, &sub, c.SubscriptionID)
	assert.Equal(t, &poolID, c.DispatchPoolID)
	assert.Equal(t, &client, c.ClientID)
	assert.Equal(t, &queue, c.Queue)
	var updatedAt time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT updated_at FROM msg_dispatch_jobs WHERE id = $1`, j.ID).Scan(&updatedAt))
	assert.True(t, updatedAt.Equal(c.Version), "version is the job's updated_at")
}

// Two claims running at once never return the same row, and together return
// every row exactly once.
func TestQueueClaim_ConcurrentClaimsAreDisjoint(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	const n = 3000
	jobs := make([]dispatchjob.DispatchJob, n)
	for i := range jobs {
		jobs[i] = newJob(fmt.Sprintf("g-%03d", i%20), common.DispatchNextOnError, int32(i))
	}
	mustCreate(t, lc, jobs...)

	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 400 { // bounded: a claim that never runs dry must fail the test, not hang it
				got, err := lc.ClaimQueue(ctx, 150, []string{}, []string{})
				if err != nil {
					t.Error(err)
					return
				}
				if len(got) == 0 {
					return
				}
				mu.Lock()
				for _, c := range got {
					seen[c.JobID]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	assert.Len(t, seen, n, "every row was claimed")
	for id, k := range seen {
		assert.Equal(t, 1, k, "row %s claimed by more than one claim", id)
	}
}

// A claimed job has no queue row; it is claimable again once RestoreClaims puts it
// back, or when the job re-enters PENDING.
func TestQueueClaim_GoneUntilRestoredOrReentered(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	j := newJob("g-"+tsid.GenerateUntyped(), common.DispatchBlockOnError, 1)
	mustCreate(t, lc, j)

	first, err := lc.ClaimQueue(ctx, 10, []string{}, []string{})
	require.NoError(t, err)
	require.Len(t, first, 1)
	second, err := lc.ClaimQueue(ctx, 10, []string{}, []string{})
	require.NoError(t, err)
	assert.Empty(t, second, "a claimed job has no queue row")

	n, err := lc.RestoreClaims(ctx, []string{j.ID}, []time.Time{j.CreatedAt})
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	assertQueueInvariant(t, pool, j.ID)
	third, err := lc.ClaimQueue(ctx, 10, []string{}, []string{})
	require.NoError(t, err)
	require.Len(t, third, 1, "a restored job is claimed again")
	assert.True(t, third[0].Version.Equal(first[0].Version), "restored from the job: same version")

	// Re-entering PENDING (a retry) re-creates the row.
	ok, err := lc.Retry(ctx, j.ID, j.CreatedAt, time.Now().Add(-time.Second), nil)
	require.NoError(t, err)
	require.True(t, ok)
	fourth, err := lc.ClaimQueue(ctx, 10, []string{}, []string{})
	require.NoError(t, err)
	require.Len(t, fourth, 1)
	assert.False(t, fourth[0].Version.Equal(first[0].Version), "the refreshed row carries the new version")

	// Published and marked QUEUED: nothing to claim, and a restore does not resurrect it.
	moved, err := lc.MarkQueued(ctx, []string{j.ID}, []time.Time{fourth[0].Version}, j.CreatedAt, j.CreatedAt)
	require.NoError(t, err)
	assert.EqualValues(t, 1, moved)
	n, err = lc.RestoreClaims(ctx, []string{j.ID}, []time.Time{j.CreatedAt})
	require.NoError(t, err)
	assert.Zero(t, n, "a job that is no longer PENDING is not put back")
	assert.Zero(t, queueCount(t, pool))
}

// A row refreshed to a future scheduled_for between the claim's two statements is
// not taken by the delete.
func TestQueueClaim_ARowMadeNotDueBetweenTheStatementsIsNotClaimed(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	j := newJob("g-"+tsid.GenerateUntyped(), common.DispatchBlockOnError, 1)
	mustCreate(t, lc, j)
	sel, del := dispatchjob.ClaimQueueStatements(10, nil, nil, nil)

	rows, err := pool.Query(ctx, sel.SQL, sel.Args...)
	require.NoError(t, err)
	var ids []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	rows.Close()
	require.Equal(t, []string{j.ID}, ids)

	ok, err := lc.Defer(ctx, j.ID, j.CreatedAt, time.Now().Add(time.Hour)) // the job is backed off meanwhile
	require.NoError(t, err)
	require.True(t, ok)

	tag, err := pool.Exec(ctx, del.SQL, ids)
	require.NoError(t, err)
	assert.Zero(t, tag.RowsAffected(), "the delete takes only rows still due")
	assert.True(t, inQueue(t, pool, j.ID))
}

// Restore does not overwrite a queue row the lifecycle wrote since.
func TestQueueRestore_DoesNotOverwriteANewerRow(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	j := newJob("g-"+tsid.GenerateUntyped(), common.DispatchBlockOnError, 1)
	mustCreate(t, lc, j)
	claimed, err := lc.ClaimQueue(ctx, 10, []string{}, []string{})
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	ok, err := lc.Defer(ctx, j.ID, j.CreatedAt, time.Now().Add(time.Hour)) // re-enters PENDING: a newer row
	require.NoError(t, err)
	require.True(t, ok)
	before := queueSnapshot(t, pool, j.ID)

	n, err := lc.RestoreClaims(ctx, []string{j.ID}, []time.Time{j.CreatedAt})
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Equal(t, before, queueSnapshot(t, pool, j.ID))
}

// Restored jobs are claimed again in order, behind nothing they should not be
// behind: a group's jobs come back as a run from the first restored one.
func TestQueueRestore_RestoredJobsAreClaimedAgainInOrder(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	g := "g-" + tsid.GenerateUntyped()
	var jobs []dispatchjob.DispatchJob
	for i := 1; i <= 6; i++ {
		jobs = append(jobs, newJob(g, common.DispatchBlockOnError, int32(i)))
	}
	mustCreate(t, lc, jobs...)
	first, err := lc.ClaimQueue(ctx, 6, []string{}, []string{})
	require.NoError(t, err)
	require.Len(t, first, 6)
	// jobs 1 and 2 publish (QUEUED); 3..6 are restored.
	for _, c := range first[:2] {
		_, err := lc.MarkQueued(ctx, []string{c.JobID}, []time.Time{c.Version}, c.JobCreatedAt, c.JobCreatedAt)
		require.NoError(t, err)
	}
	var ids []string
	var created []time.Time
	for _, c := range first[2:] {
		ids, created = append(ids, c.JobID), append(created, c.JobCreatedAt)
	}
	n, err := lc.RestoreClaims(ctx, ids, created)
	require.NoError(t, err)
	assert.EqualValues(t, 4, n)
	n, err = lc.RestoreClaims(ctx, ids, created)
	require.NoError(t, err)
	assert.Zero(t, n, "restoring again changes nothing")

	again, err := lc.ClaimQueue(ctx, 10, []string{}, []string{})
	require.NoError(t, err)
	assert.Equal(t, claimIDsOf(first[2:]), claimIDsOf(again), "restored jobs return in order, nothing before them")
}

// RestoreOrphans (the leader's start-up pass): every PENDING job without a queue
// row is restored with no age guard, except the ones the caller has in flight;
// the pass repeats in bounded batches until it inserts nothing.
func TestQueueRestoreOrphans(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	var jobs []dispatchjob.DispatchJob
	for i := range 7 {
		jobs = append(jobs, newJob(g(), common.DispatchBlockOnError, int32(i)))
	}
	mustCreate(t, lc, jobs...) // created now: far inside any age guard
	claimed, err := lc.ClaimQueue(ctx, 100, []string{}, []string{})
	require.NoError(t, err)
	require.Len(t, claimed, 7) // a process claimed them all and died

	inFlight := []string{jobs[0].ID}
	n, err := lc.RestoreOrphans(ctx, 3, inFlight) // batches of 3
	require.NoError(t, err)
	assert.EqualValues(t, 6, n)
	assert.False(t, inQueue(t, pool, jobs[0].ID), "the in-flight job is not re-queued")
	for _, j := range jobs[1:] {
		assertQueueInvariant(t, pool, j.ID)
	}
	missing, orphaned, err := lc.QueueDrift(ctx, jobs[0].ID)
	require.NoError(t, err)
	assert.Zero(t, missing)
	assert.Zero(t, orphaned)

	n, err = lc.RestoreOrphans(ctx, 3, inFlight)
	require.NoError(t, err)
	assert.Zero(t, n)
}

// The backlog is the due rows and the age of the oldest.
func TestQueueBacklog(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	rows, oldest, err := lc.QueueBacklog(ctx)
	require.NoError(t, err)
	assert.Zero(t, rows)
	assert.Zero(t, oldest)

	var jobs []dispatchjob.DispatchJob
	for i := range 5 {
		jobs = append(jobs, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, int32(i)))
	}
	soon := time.Now().Add(time.Hour)
	jobs[4].ScheduledFor = &soon
	mustCreate(t, lc, jobs...)
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_queue SET enqueued_at = NOW() - interval '90 seconds' WHERE job_id = $1`, jobs[0].ID)
	require.NoError(t, err)

	rows, oldest, err = lc.QueueBacklog(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 4, rows, "five rows, one not yet due")
	assert.InDelta(t, 90, oldest.Seconds(), 5)
}
