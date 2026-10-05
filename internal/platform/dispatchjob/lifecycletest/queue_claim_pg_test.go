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

func claimedAtOf(t *testing.T, pool *pgxpool.Pool, id string) *time.Time {
	t.Helper()
	var at *time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT claimed_at FROM msg_dispatch_queue WHERE job_id = $1`, id).Scan(&at))
	return at
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

// The claim returns due, unclaimed, unpaused rows in delivery order — group
// (ungrouped last), sequence, created_at, id — up to the limit, and stamps them
// claimed.
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

	got, err := lc.ClaimQueue(ctx, 100, []string{pausedSub})
	require.NoError(t, err)
	assert.Equal(t, []string{a1early.ID, a1late.ID, a2.ID, b1.ID, b2.ID, u1.ID, u2.ID}, claimIDsOf(got),
		"group order, then sequence, created_at; ungrouped last; the future and paused rows are left")
	for _, c := range got {
		require.NotNil(t, claimedAtOf(t, pool, c.JobID), "claimed rows are stamped")
	}
	assert.Nil(t, claimedAtOf(t, pool, future.ID))
	assert.Nil(t, claimedAtOf(t, pool, paused.ID))

	// Claimed rows are never returned again.
	again, err := lc.ClaimQueue(ctx, 100, []string{pausedSub})
	require.NoError(t, err)
	assert.Empty(t, again)

	// The limit bounds the claim and leaves the rest unclaimed: the first two in order.
	cleanTables(t, pool)
	mustCreate(t, lc, mk("a", 1, time.Second), mk("a", 2, 2*time.Second), mk("a", 3, 3*time.Second), mk("a", 4, 4*time.Second))
	two, err := lc.ClaimQueue(ctx, 2, nil) // nil paused is treated as empty
	require.NoError(t, err)
	require.Len(t, two, 2)
	assert.EqualValues(t, []int32{1, 2}, []int32{two[0].Sequence, two[1].Sequence})
	rest, err := lc.ClaimQueue(ctx, 100, []string{})
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

	got, err := lc.ClaimQueue(ctx, 10, []string{})
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

	const n = 300
	jobs := make([]dispatchjob.DispatchJob, n)
	for i := range jobs {
		jobs[i] = newJob(fmt.Sprintf("g-%03d", i%20), common.DispatchNextOnError, int32(i))
	}
	mustCreate(t, lc, jobs...)

	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 { // bounded: a claim that never runs dry must fail the test, not hang it
				got, err := lc.ClaimQueue(ctx, 7, []string{})
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

// A claimed row is not claimable again until its claim is released or the job
// re-enters PENDING.
func TestQueueClaim_ClaimedUntilReleasedOrReentered(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	j := newJob("g-"+tsid.GenerateUntyped(), common.DispatchBlockOnError, 1)
	mustCreate(t, lc, j)

	first, err := lc.ClaimQueue(ctx, 10, []string{})
	require.NoError(t, err)
	require.Len(t, first, 1)
	second, err := lc.ClaimQueue(ctx, 10, []string{})
	require.NoError(t, err)
	assert.Empty(t, second, "a claimed row is skipped")

	n, err := lc.ReleaseClaims(ctx, []string{j.ID})
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	assert.Nil(t, claimedAtOf(t, pool, j.ID))
	third, err := lc.ClaimQueue(ctx, 10, []string{})
	require.NoError(t, err)
	require.Len(t, third, 1, "a released row is claimed again")

	// Re-entering PENDING (a retry) refreshes the row and clears the claim.
	ok, err := lc.Retry(ctx, j.ID, j.CreatedAt, time.Now().Add(-time.Second), nil)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Nil(t, claimedAtOf(t, pool, j.ID), "re-entering PENDING resets the claim")
	fourth, err := lc.ClaimQueue(ctx, 10, []string{})
	require.NoError(t, err)
	require.Len(t, fourth, 1)
	assert.False(t, fourth[0].Version.Equal(first[0].Version), "the refreshed row carries the new version")

	// Published and marked QUEUED: the row is gone and cannot be claimed.
	moved, err := lc.MarkQueued(ctx, []string{j.ID}, []time.Time{fourth[0].Version}, j.CreatedAt, j.CreatedAt)
	require.NoError(t, err)
	assert.EqualValues(t, 1, moved)
	assert.Zero(t, queueCount(t, pool))
}

// ReleaseClaims is one bulk statement that clears only claims: an unclaimed or
// missing row is a no-op, and the count says what was released.
func TestQueueRelease_BulkAndIdempotent(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	var jobs []dispatchjob.DispatchJob
	for i := range 5 {
		jobs = append(jobs, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, int32(i)))
	}
	mustCreate(t, lc, jobs...)
	claimed, err := lc.ClaimQueue(ctx, 3, []string{})
	require.NoError(t, err)
	require.Len(t, claimed, 3)

	ids := make([]string, len(jobs))
	for i, j := range jobs {
		ids[i] = j.ID
	}
	n, err := lc.ReleaseClaims(ctx, append(ids, "no-such-job"))
	require.NoError(t, err)
	assert.EqualValues(t, 3, n, "only the three claimed rows had a claim")
	n, err = lc.ReleaseClaims(ctx, ids)
	require.NoError(t, err)
	assert.Zero(t, n, "releasing again changes nothing")
	n, err = lc.ReleaseClaims(ctx, nil)
	require.NoError(t, err)
	assert.Zero(t, n)
	requireNoDrift(t, lc)
}

// Released rows are claimed again in order, behind nothing they should not be
// behind: a group's jobs come back as a run from the first released one.
func TestQueueRelease_ReleasedJobsAreClaimedAgainInOrder(t *testing.T) {
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
	first, err := lc.ClaimQueue(ctx, 6, []string{})
	require.NoError(t, err)
	require.Len(t, first, 6)
	// jobs 1 and 2 publish (their rows leave the queue); 3..6 are released.
	for _, c := range first[:2] {
		_, err := lc.MarkQueued(ctx, []string{c.JobID}, []time.Time{c.Version}, c.JobCreatedAt, c.JobCreatedAt)
		require.NoError(t, err)
	}
	_, err = lc.ReleaseClaims(ctx, claimIDsOf(first[2:]))
	require.NoError(t, err)

	again, err := lc.ClaimQueue(ctx, 10, []string{})
	require.NoError(t, err)
	assert.Equal(t, claimIDsOf(first[2:]), claimIDsOf(again), "released jobs return in order, nothing before them")
}

// ReleaseStaleClaims: a zero cutoff releases every claim the caller does not
// hold; a cutoff releases only the older ones; the excluded ids are never touched.
func TestQueueReleaseStaleClaims(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	cleanTables(t, pool)

	var jobs []dispatchjob.DispatchJob
	for i := range 4 {
		jobs = append(jobs, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, int32(i)))
	}
	mustCreate(t, lc, jobs...)
	oldA, oldHeld, fresh, unclaimed := jobs[0], jobs[1], jobs[2], jobs[3]
	_, err := pool.Exec(ctx, `UPDATE msg_dispatch_queue SET claimed_at = NOW() - interval '10 minutes' WHERE job_id = ANY($1)`,
		[]string{oldA.ID, oldHeld.ID})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_queue SET claimed_at = NOW() WHERE job_id = $1`, fresh.ID)
	require.NoError(t, err)

	// Periodic form: older than five minutes and not held in memory.
	n, err := lc.ReleaseStaleClaims(ctx, time.Now().Add(-5*time.Minute), []string{oldHeld.ID})
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	assert.Nil(t, claimedAtOf(t, pool, oldA.ID), "an old claim nobody holds is released")
	assert.NotNil(t, claimedAtOf(t, pool, oldHeld.ID), "a job in the in-flight set is not released, however old")
	assert.NotNil(t, claimedAtOf(t, pool, fresh.ID), "a recent claim is not released")
	assert.Nil(t, claimedAtOf(t, pool, unclaimed.ID))

	// Leader-start form: every claim not held in memory, however recent.
	n, err = lc.ReleaseStaleClaims(ctx, time.Time{}, []string{oldHeld.ID})
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	assert.Nil(t, claimedAtOf(t, pool, fresh.ID))
	assert.NotNil(t, claimedAtOf(t, pool, oldHeld.ID), "still held in memory")

	n, err = lc.ReleaseStaleClaims(ctx, time.Time{}, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	assert.Nil(t, claimedAtOf(t, pool, oldHeld.ID))
}

// The backlog is the unclaimed, due rows and the age of the oldest.
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
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_queue SET claimed_at = NOW() WHERE job_id = $1`, jobs[1].ID)
	require.NoError(t, err)

	rows, oldest, err = lc.QueueBacklog(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 3, rows, "five rows, one claimed, one not yet due")
	assert.InDelta(t, 90, oldest.Seconds(), 5)
}
