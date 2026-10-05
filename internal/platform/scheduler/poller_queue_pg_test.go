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

// queueRow reads a job's queue row: whether it exists and whether it is claimed.
func queueRow(t *testing.T, pool *pgxpool.Pool, id string) (exists, claimed bool) {
	t.Helper()
	var at *time.Time
	err := pool.QueryRow(context.Background(), `SELECT claimed_at FROM msg_dispatch_queue WHERE job_id = $1`, id).Scan(&at)
	if err != nil {
		return false, false
	}
	return true, at != nil
}

func pollerWith(pool *pgxpool.Pool, pub DispatchPublisher) *PendingJobPoller {
	return NewPendingJobPoller(DefaultConfig(), pool,
		NewMessageGroupDispatcher(pool, pub, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process"),
		NewPausedConnectionCache(pool, time.Minute))
}

// publishedIn filters a capture publisher's ids to one test's jobs, in order.
func publishedIn(p *capturePublisher, ids ...string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []string
	for _, id := range p.ids {
		if want[id] {
			out = append(out, id)
		}
	}
	return out
}

// A failed publish releases the claim: the rows stay PENDING with a queue row
// that is claimable again, and the next claim publishes the group IN ORDER.
func TestQueuePoll_FailedPublishReleasesTheClaimAndTheGroupIsClaimedAgainInOrder(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	const group = "grp_qrelease_fail"
	ids := []string{"djqrelfail001", "djqrelfail002", "djqrelfail003"}
	for _, id := range ids {
		seedJob(t, pool, id, "PENDING", group, "")
		time.Sleep(2 * time.Millisecond) // distinct created_at: the order is (sequence, created_at)
	}

	mustPoll(t, pollerWith(pool, failPublisher{}), ctx)
	for _, id := range ids {
		exists, claimed := queueRow(t, pool, id)
		assert.True(t, exists, "%s stays in the queue", id)
		assert.False(t, claimed, "%s: a failed publish releases the claim", id)
		assert.Equal(t, "PENDING", jobStatus(t, pool, id))
	}

	good := &capturePublisher{}
	mustPoll(t, pollerWith(pool, good), ctx)
	assert.Equal(t, ids, publishedIn(good, ids...), "claimed again and published in order")
	for _, id := range ids {
		exists, _ := queueRow(t, pool, id)
		assert.False(t, exists, "published and marked QUEUED: the queue row is gone")
		assert.Equal(t, "QUEUED", jobStatus(t, pool, id))
	}
}

// A job held back behind a failed sibling is claimed and then released in the
// same poll, so it is not stuck claimed.
func TestQueuePoll_HeldBackJobsHaveTheirClaimReleased(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	const group = "grp_qrelease_held"
	seedJob(t, pool, "djqrelheld_f1", "FAILED", group, "")
	seedModeJob(t, pool, "djqrelheld_p1", "PENDING", group, "BLOCK_ON_ERROR")
	seedModeJob(t, pool, "djqrelheld_p2", "PENDING", group, "BLOCK_ON_ERROR")

	pub := &capturePublisher{}
	mustPoll(t, pollerWith(pool, pub), ctx)
	assert.Empty(t, publishedIn(pub, "djqrelheld_p1", "djqrelheld_p2"), "held behind the FAILED sibling")
	for _, id := range []string{"djqrelheld_p1", "djqrelheld_p2"} {
		exists, claimed := queueRow(t, pool, id)
		assert.True(t, exists)
		assert.False(t, claimed, "%s: held rows are released, not left claimed", id)
	}

	// The operator resolves the failure: the group flows on the next poll.
	_, err := pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET status = 'COMPLETED', updated_at = NOW() WHERE id = 'djqrelheld_f1'`)
	require.NoError(t, err)
	testpg.SyncDispatchQueue(t, pool, "djqrelheld_f1")
	mustPoll(t, pollerWith(pool, pub), ctx)
	assert.Equal(t, []string{"djqrelheld_p1", "djqrelheld_p2"}, publishedIn(pub, "djqrelheld_p1", "djqrelheld_p2"))
}

// A backed-off sibling holds the group too, and it is read from the queue: the
// jobs behind it are claimed and released while it waits.
func TestQueuePoll_AJobBehindABackedOffSiblingIsHeldUsingTheQueue(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	const group = "grp_qrelease_backoff"
	now := time.Now().UTC()
	later := now.Add(time.Hour)
	seedSequencedJob(t, pool, "djqrelbo_head", group, "PENDING", now.Add(-time.Hour), &later)
	seedSequencedJob(t, pool, "djqrelbo_next", group, "PENDING", now.Add(-time.Minute), nil)

	pub := &capturePublisher{}
	mustPoll(t, pollerWith(pool, pub), ctx)
	assert.Empty(t, publishedIn(pub, "djqrelbo_head", "djqrelbo_next"))
	_, claimed := queueRow(t, pool, "djqrelbo_next")
	assert.False(t, claimed)
	assert.Equal(t, "PENDING", jobStatus(t, pool, "djqrelbo_next"))
}

// At the start of leadership every claim the process does not hold in memory is
// an orphan and is released — the ones in the in-flight set are not.
func TestQueueStaleClaims_LeaderStartReleasesWhatTheProcessDoesNotHold(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	ids := []string{"djqorph000001", "djqorph000002", "djqorph000003"}
	for _, id := range ids {
		seedJob(t, pool, id, "PENDING", "grp_qorphan", "")
	}
	_, err := pool.Exec(ctx, `UPDATE msg_dispatch_queue SET claimed_at = NOW() WHERE job_id = ANY($1)`, ids)
	require.NoError(t, err)

	p := pollerWith(pool, &capturePublisher{})
	p.inflight.add([]laneJob{{tok: DispatchJobToken{JobID: ids[0], MessageGroup: "grp_qorphan"}, gen: 1}})
	require.NoError(t, p.releaseOrphanClaims(ctx))

	_, claimed := queueRow(t, pool, ids[0])
	assert.True(t, claimed, "a claim this process holds in memory is kept")
	for _, id := range ids[1:] {
		_, claimed := queueRow(t, pool, id)
		assert.False(t, claimed, "%s: an orphan claim is released", id)
	}
}

// Run releases orphan claims before its first claim, so a job claimed by a dead
// process is published by the new leader.
func TestQueueStaleClaims_RunReleasesOrphansBeforeTheFirstClaim(t *testing.T) {
	pool := testpg.Pool(t)
	const id = "djqorphrun001"
	seedJob(t, pool, id, "PENDING", "grp_qorphanrun", "")
	_, err := pool.Exec(context.Background(), `UPDATE msg_dispatch_queue SET claimed_at = NOW() WHERE job_id = $1`, id)
	require.NoError(t, err)

	pub := &capturePublisher{}
	p := pollerWith(pool, pub)
	p.IsLeader = func() bool { return true }
	stop := runEngine(t, p)
	require.Eventually(t, func() bool { return jobStatus(t, pool, id) == "QUEUED" }, 20*time.Second, 10*time.Millisecond,
		"the orphan claim is released at leader start and the job is published")
	stop()
	assert.Equal(t, []string{id}, publishedIn(pub, id))
}

// A non-leader releases nothing.
func TestQueueStaleClaims_NonLeaderDoesNotRelease(t *testing.T) {
	pool := testpg.Pool(t)
	const id = "djqorphnon001"
	seedJob(t, pool, id, "PENDING", "grp_qorphannon", "")
	_, err := pool.Exec(context.Background(), `UPDATE msg_dispatch_queue SET claimed_at = NOW() WHERE job_id = $1`, id)
	require.NoError(t, err)

	p := pollerWith(pool, &capturePublisher{})
	p.cfg.PollInterval = 5 * time.Millisecond
	p.IsLeader = func() bool { return false }
	stop := runEngine(t, p)
	time.Sleep(150 * time.Millisecond)
	stop()
	_, claimed := queueRow(t, pool, id)
	assert.True(t, claimed)
}

// The periodic sweep releases claims older than five minutes that this process
// does not hold, counts them, and leaves recent claims and in-flight jobs alone.
func TestQueueStaleClaims_PeriodicSweep(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	oldDead, oldHeld, recent := "djqsweep00001", "djqsweep00002", "djqsweep00003"
	for _, id := range []string{oldDead, oldHeld, recent} {
		seedJob(t, pool, id, "PENDING", "grp_qsweep", "")
	}
	_, err := pool.Exec(ctx, `UPDATE msg_dispatch_queue SET claimed_at = NOW() - interval '10 minutes' WHERE job_id = ANY($1)`, []string{oldDead, oldHeld})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_queue SET claimed_at = NOW() WHERE job_id = $1`, recent)
	require.NoError(t, err)

	m := newQueueMaintainer(pool, func() []string { return []string{oldHeld} })
	before := counterVec(t, "fc_scheduler_stale_claims_released_total", "periodic")
	n := m.sweepClaims(ctx)
	assert.GreaterOrEqual(t, n, int64(1))
	_, claimed := queueRow(t, pool, oldDead)
	assert.False(t, claimed, "an old claim nobody holds is released")
	_, claimed = queueRow(t, pool, oldHeld)
	assert.True(t, claimed, "a job in this process's in-flight set is not released")
	_, claimed = queueRow(t, pool, recent)
	assert.True(t, claimed, "a recent claim is not released")
	assert.Equal(t, float64(n), counterVec(t, "fc_scheduler_stale_claims_released_total", "periodic")-before, "counted in the metric")
}

func counterVec(t *testing.T, name, label string) float64 {
	t.Helper()
	families, err := MetricsRegistry.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetValue() == label {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	t.Fatalf("metric %s{%s} not found", name, label)
	return 0
}

// The maintainer's reconcile repairs a missing queue row and reports it; the
// backlog gauge counts the unclaimed, due rows.
func TestQueueMaintenance_ReconcileAndBacklogGauge(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	const id = "djqrecon00001"
	seedJob(t, pool, id, "PENDING", "grp_qrecon", "")
	_, err := pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET updated_at = NOW() - interval '5 minutes' WHERE id = $1`, id)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DELETE FROM msg_dispatch_queue WHERE job_id = $1`, id) // the corruption
	require.NoError(t, err)

	m := newQueueMaintainer(pool, func() []string { return nil })
	res := m.reconcile(ctx)
	assert.GreaterOrEqual(t, res.Inserted, int64(1))
	exists, _ := queueRow(t, pool, id)
	assert.True(t, exists, "the missing row is back")

	m.sampleBacklog(ctx)
	assert.GreaterOrEqual(t, value(t, MetricsRegistry, "fc_scheduler_queue_backlog_rows"), 1.0)
}

// Stale-QUEUED recovery runs at 15 minutes; PROCESSING keeps its own, longer
// threshold.
func TestStaleRecovery_QueuedAtFifteenMinutesProcessingAtSeventyFive(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	cfg := DefaultConfig()
	assert.Equal(t, 15*time.Minute, cfg.StaleQueuedAfter)
	assert.Equal(t, 75*time.Minute, cfg.StaleProcessingAfter)

	ids := map[string]string{
		"queued 20m":     "djstale00q20",
		"queued 10m":     "djstale00q10",
		"processing 20m": "djstale0p020",
		"processing 80m": "djstale0p080",
	}
	status := map[string]string{"queued 20m": "QUEUED", "queued 10m": "QUEUED", "processing 20m": "PROCESSING", "processing 80m": "PROCESSING"}
	age := map[string]string{"queued 20m": "20 minutes", "queued 10m": "10 minutes", "processing 20m": "20 minutes", "processing 80m": "80 minutes"}
	for name, id := range ids {
		seedJob(t, pool, id, status[name], "", "")
		_, err := pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET updated_at = NOW() - $2::interval WHERE id = $1`, id, age[name])
		require.NoError(t, err)
	}

	rec := NewStaleQueuedJobPoller(pool, cfg.StaleQueuedAfter, cfg.StaleProcessingAfter, time.Minute)
	n, err := rec.recoverOnce(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, int64(2))
	assert.Equal(t, "PENDING", jobStatus(t, pool, ids["queued 20m"]), "QUEUED past 15 minutes is recovered")
	assert.Equal(t, "QUEUED", jobStatus(t, pool, ids["queued 10m"]), "QUEUED under 15 minutes is left")
	assert.Equal(t, "PROCESSING", jobStatus(t, pool, ids["processing 20m"]), "PROCESSING keeps its longer threshold")
	assert.Equal(t, "PENDING", jobStatus(t, pool, ids["processing 80m"]))
	exists, claimed := queueRow(t, pool, ids["queued 20m"])
	assert.True(t, exists && !claimed, "a recovered job re-enters the queue, unclaimed")
}
