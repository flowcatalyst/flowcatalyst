package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The claim lives in the queue table, so every claimed job the engine does not
// publish must have its claim released, or it is stuck until the stale-claim
// sweep. These drive each path against the fake queue (nothing touches a
// database or a broker).

// A failed publish releases the claims, and the jobs are claimed again.
func TestRelease_FailedPublishReleasesTheClaims(t *testing.T) {
	s := newFakeStore(groupJobs("g", 3)...)
	s.publishFn = func(_ context.Context, toks []DispatchJobToken) []string { return tokenIDs(toks) }
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 10, LaneBatch: 100}, s)

	settleOnce(t, p, context.Background())
	assert.Zero(t, s.claimedCount(), "nothing is left claimed")
	var released []string
	for _, call := range s.releaseCalls() { // the lane may split the claim over a few batches
		released = append(released, call...)
	}
	assert.ElementsMatch(t, []string{"g-00", "g-01", "g-02"}, released, "the unpublished jobs are released in bulk statements")

	s.publishFn = nil
	settleOnce(t, p, context.Background())
	assert.Equal(t, []string{"g-00", "g-01", "g-02"}, s.published(), "claimed again, in order")
}

// A job dropped by the poison rule has its claim released, and the group is
// published in order afterwards.
func TestRelease_PoisonedDropReleasesTheClaim(t *testing.T) {
	s := newFakeStore(groupJobs("g", 3)...)
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 10, LaneBatch: 100}, s)
	l := p.lanes[0]
	batch := []laneJob{lj("g-00", "g", 1), lj("g-01", "g", 1), lj("g-02", "g", 1)}
	track(l, batch...)
	for _, j := range s.jobs {
		j.claimed = true
	}
	setPoison(l, "g", 5, time.Now()) // generation 1 is at or below the mark: dropped
	l.process(context.Background(), batch)

	assert.Empty(t, s.published())
	assert.Zero(t, s.claimedCount(), "the dropped jobs are released")
	assert.ElementsMatch(t, []string{"g-00", "g-01", "g-02"}, s.releaseCalls()[0])
	assert.Zero(t, l.p.inflight.size())

	p.claimGeneration.Store(10) // a claim taken after the failure, past the mark
	settleOnce(t, p, context.Background())
	assert.Equal(t, []string{"g-00", "g-01", "g-02"}, s.published())
}

// Jobs published and marked QUEUED leave the queue and are not released.
func TestRelease_PublishedJobsAreNotReleased(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 4)...)
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 10, LaneBatch: 100}, s)
	settleOnce(t, p, context.Background())
	assert.Len(t, s.published(), 4)
	assert.Empty(t, s.releaseCalls(), "a healthy batch needs no release")
	assert.Zero(t, s.claimedCount())
}

// When the QUEUED update fails after a publish the claim is released too, so the
// job is claimed and published again (a double publish, never a lost job).
func TestRelease_MarkQueuedFailureReleasesThePublishedJobs(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 3)...)
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 10, LaneBatch: 100}, s)
	var fail atomic.Bool
	fail.Store(true)
	p.markQueued = func(ctx context.Context, ids []string, v []time.Time, a, b time.Time) (int64, error) {
		if fail.Load() {
			return 0, errors.New("mark failed")
		}
		return s.markQueued(ctx, ids, v, a, b)
	}
	settleOnce(t, p, context.Background())
	assert.Zero(t, s.claimedCount())
	assert.Len(t, s.published(), 3)

	fail.Store(false)
	settleOnce(t, p, context.Background())
	assert.Len(t, s.published(), 6, "published again")
	assert.Zero(t, s.pending())
}

// A claim whose jobs are all held back (BLOCK_ON_ERROR behind a holder) releases
// every claim it took in the same poll.
func TestRelease_HeldBackJobsAreReleased(t *testing.T) {
	jobs := groupJobs("g", 4)
	for _, j := range jobs {
		j.mode = "BLOCK_ON_ERROR"
	}
	s := newFakeStore(jobs...)
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 10}, s)
	p.holdBack = func(context.Context, []string) (map[string]jobKey, error) {
		return map[string]jobKey{"g": {sequence: -1, id: "holder"}}, nil
	}
	res := settleOnce(t, p, context.Background())
	assert.Equal(t, 4, res.claimed)
	assert.Zero(t, res.submitted)
	assert.Zero(t, s.claimedCount(), "the held rows are claimable again")
	assert.Empty(t, s.published())
}

// Only the held jobs are released: the dispatchable ones of the claim are
// submitted and stay claimed until their lane has finished them.
func TestRelease_OnlyTheHeldRowsOfAMixedClaimAreReleased(t *testing.T) {
	held := groupJobs("h", 2)
	for _, j := range held {
		j.mode = "BLOCK_ON_ERROR"
	}
	s := newFakeStore(append(held, groupJobs("g", 2)...)...)
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 10}, s)
	p.holdBack = func(context.Context, []string) (map[string]jobKey, error) {
		return map[string]jobKey{"h": {sequence: -1, id: "holder"}}, nil
	}
	settleOnce(t, p, context.Background())
	assert.ElementsMatch(t, []string{"g-00", "g-01"}, s.published())
	require.NotEmpty(t, s.releaseCalls())
	assert.ElementsMatch(t, []string{"h-00", "h-01"}, s.releaseCalls()[0])
}

// A failed hold-back lookup releases what was claimed: none of it will be
// submitted.
func TestRelease_HoldBackErrorReleasesTheClaim(t *testing.T) {
	s := newFakeStore(groupJobs("g", 3)...)
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 10}, s)
	p.holdBack = func(context.Context, []string) (map[string]jobKey, error) { return nil, errors.New("db down") }
	res := p.claimOnce(context.Background())
	require.Error(t, res.err)
	assert.Zero(t, s.claimedCount())
	assert.Zero(t, len(p.permits))
}

// The poller-side release retries and then gives up without failing the poll:
// the stale-claim sweep is the backstop.
func TestRelease_PollerSideReleaseRetriesThenLeavesItToTheSweep(t *testing.T) {
	old1, old2 := releaseAttempts, releaseBackoff
	releaseAttempts, releaseBackoff = 3, time.Millisecond
	defer func() { releaseAttempts, releaseBackoff = old1, old2 }()

	s := newFakeStore(groupJobs("g", 2)...)
	s.releaseErr = func(call int) error {
		if call <= 2 {
			return errors.New("transient")
		}
		return nil
	}
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 10}, s)
	p.holdBack = func(context.Context, []string) (map[string]jobKey, error) { return nil, errors.New("boom") }
	_ = p.claimOnce(context.Background())
	assert.Zero(t, s.claimedCount(), "released on the third attempt")
	assert.Len(t, s.releaseCalls(), 3)

	// Every attempt failing: the claims stay (the sweep releases them), the poll goes on.
	s2 := newFakeStore(groupJobs("g", 2)...)
	s2.releaseErr = func(int) error { return errors.New("down") }
	p2 := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 10}, s2)
	p2.holdBack = func(context.Context, []string) (map[string]jobKey, error) { return nil, errors.New("boom") }
	before := value(t, MetricsRegistry, "fc_scheduler_claim_restore_failures_total")
	_ = p2.claimOnce(context.Background())
	assert.Equal(t, 2, s2.claimedCount())
	assert.Equal(t, 2.0, value(t, MetricsRegistry, "fc_scheduler_claim_restore_failures_total")-before)
}

// A lane whose release fails does not let the job leave the in-flight set (and so
// does not read the poison generation) until the release succeeds: otherwise a
// later claim could take the group's next jobs past the one still claimed.
func TestRelease_ALaneRetriesUntilTheReleaseSucceeds(t *testing.T) {
	s := newFakeStore(groupJobs("g", 2)...)
	s.publishFn = func(_ context.Context, toks []DispatchJobToken) []string { return tokenIDs(toks) }
	var calls atomic.Int32
	s.releaseErr = func(call int) error {
		calls.Add(1)
		if call <= 3 {
			return errors.New("transient")
		}
		return nil
	}
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 10, LaneBatch: 100}, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := p.startLanes(ctx)
	defer func() { cancel(); wg.Wait() }()

	res := p.claimOnce(ctx)
	require.Equal(t, 2, res.submitted)
	require.Eventually(t, func() bool { return calls.Load() >= 2 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, 2, p.inflight.size(), "still in flight while its claim cannot be released")
	waitIdle(t, p)
	assert.Zero(t, p.inflight.size())
	assert.Zero(t, s.claimedCount())
	assert.GreaterOrEqual(t, calls.Load(), int32(4))
}

// A lane stuck releasing on shutdown gives up rather than blocking the process.
func TestRelease_ALaneStopsRetryingOnShutdown(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 1)...)
	s.publishFn = func(_ context.Context, toks []DispatchJobToken) []string { return tokenIDs(toks) }
	s.releaseErr = func(int) error { return errors.New("down") }
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 10, LaneBatch: 100}, s)
	ctx, cancel := context.WithCancel(context.Background())
	wg := p.startLanes(ctx)
	require.Equal(t, 1, p.claimOnce(ctx).submitted)
	time.Sleep(60 * time.Millisecond)
	cancel()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the lane kept retrying the release after shutdown")
	}
}

// Leadership: the first thing a leader does is release every claim it does not
// hold, before any claim of its own — claimed rows of a dead process are then
// claimable. A non-leader releases nothing.
func TestLeaderStart_ReleasesOrphanClaimsBeforeTheFirstClaim(t *testing.T) {
	jobs := ungroupedJobs("u", 3)
	s := newFakeStore(jobs...)
	for _, j := range s.jobs {
		j.claimed = true // claimed by a process that died
	}
	var mu sync.Mutex
	var events []string
	p := newTestEngine(Config{PollInterval: 5 * time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 10}, s)
	releaseOrphans := p.restoreOrphans
	p.restoreOrphans = func(ctx context.Context, excluding []string) (int64, error) {
		mu.Lock()
		events = append(events, "release-orphans")
		mu.Unlock()
		require.NotNil(t, excluding, "the in-flight list is never nil")
		return releaseOrphans(ctx, excluding)
	}
	claimRows := p.claimRows
	p.claimRows = func(ctx context.Context, limit int, paused, held []string) ([]dispatchClaim, error) {
		mu.Lock()
		events = append(events, "claim")
		mu.Unlock()
		return claimRows(ctx, limit, paused, held)
	}
	p.IsLeader = func() bool { return true }
	runEngine(t, p)
	require.Eventually(t, func() bool { return s.pending() == 0 }, 5*time.Second, time.Millisecond,
		"the orphaned rows are released and published")
	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, events)
	assert.Equal(t, "release-orphans", events[0], "released before the first claim")
	n := 0
	for _, e := range events {
		if e == "release-orphans" {
			n++
		}
	}
	assert.Equal(t, 1, n, "once per start of leadership")
}

// Losing and regaining leadership runs the start-of-leadership pass again.
func TestLeaderStart_RunsAgainWhenLeadershipIsRegained(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 1)...)
	var leader atomic.Bool
	var passes atomic.Int32
	p := newTestEngine(Config{PollInterval: 2 * time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 10}, s)
	p.restoreOrphans = func(context.Context, []string) (int64, error) { passes.Add(1); return 0, nil }
	p.IsLeader = leader.Load
	runEngine(t, p)
	time.Sleep(30 * time.Millisecond)
	assert.Zero(t, passes.Load(), "a non-leader does not release")
	leader.Store(true)
	require.Eventually(t, func() bool { return passes.Load() == 1 }, 5*time.Second, time.Millisecond)
	leader.Store(false)
	time.Sleep(30 * time.Millisecond)
	leader.Store(true)
	require.Eventually(t, func() bool { return passes.Load() == 2 }, 5*time.Second, time.Millisecond)
}

// The start-of-leadership release excludes the in-flight set, and a failure is
// retried before any claim.
func TestLeaderStart_ExcludesInFlightAndRetriesOnFailure(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 1)...)
	p := newTestEngine(Config{PollInterval: 2 * time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 10}, s)
	p.inflight.add([]laneJob{lj("held-by-this-process", "", 1)})
	var got []string
	var attempts atomic.Int32
	p.restoreOrphans = func(_ context.Context, excluding []string) (int64, error) {
		got = excluding
		if attempts.Add(1) == 1 {
			return 0, errors.New("db down")
		}
		return 0, nil
	}
	p.IsLeader = func() bool { return true }
	runEngine(t, p)
	require.Eventually(t, func() bool { return s.claimCount() > 0 }, 5*time.Second, time.Millisecond)
	assert.GreaterOrEqual(t, attempts.Load(), int32(2), "retried after the failure, before claiming")
	assert.Equal(t, []string{"held-by-this-process"}, got)
}

// A lane forgets only the in-flight entries that are its own. A job released by a
// failure can be claimed again by a later claim before the lane removes the batch;
// that claim's entry has a newer generation and must survive, or the copy waiting
// in a lane is invisible to the claims that follow and they take the group's later
// jobs past it (found by the stress test: first deliveries out of order, one run in
// fifteen under -race).
func TestInflight_ASettledBatchDoesNotForgetAReclaimedJobsNewEntry(t *testing.T) {
	set := newInflightSet()
	set.add([]laneJob{lj("g-00", "g", 25)}) // the later claim's copy
	set.removeSettled([]laneJob{lj("g-00", "g", 24)})
	assert.Equal(t, 1, set.size(), "the older claim's settle leaves the newer entry")
	set.removeSettled([]laneJob{lj("g-00", "g", 25), lj("not-there", "g", 25)})
	assert.Zero(t, set.size(), "a settle of its own entry removes it")
}

// The scenario end to end: a claim lands between the lane's restore of a failed
// batch and its removal from the in-flight set. It returns the restored jobs while
// the old copies are still in flight: it must not submit them again (their old
// copies' entries are about to be removed); it restores them and the next claim
// takes them. Nothing overtakes: the group is published once, in order.
func TestOrdering_AClaimBetweenTheRestoreAndTheRemovalDoesNotResubmitInFlightJobs(t *testing.T) {
	s := newFakeStore(groupJobs("g", 4)...)
	inPublish := make(chan struct{})
	gate := make(chan struct{})
	var calls atomic.Int32
	s.publishFn = func(_ context.Context, toks []DispatchJobToken) []string {
		if calls.Add(1) == 1 {
			close(inPublish)
			<-gate
			return tokenIDs(toks) // the broker rejects the first batch
		}
		for _, tk := range toks {
			s.logPublished(tk.JobID)
		}
		return nil
	}
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 2, LaneBatch: 100}, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := p.startLanes(ctx)
	defer func() { cancel(); wg.Wait() }()

	require.Equal(t, 2, p.claimOnce(ctx).submitted) // g-00, g-01 -> the lane, blocked in publish
	<-inPublish

	var mu sync.Mutex
	var seen []string
	var claimed int
	var once sync.Once
	p.hookReleased = func() {
		once.Do(func() {
			// the failed batch's claims are released; this claim takes them again
			res := p.claimOnce(ctx)
			if res.err != nil {
				t.Error(res.err)
			}
			mu.Lock()
			claimed = res.submitted
			mu.Unlock()
		})
	}
	var settleOnceHook sync.Once
	p.hookSettle = func() { // after the old batch has been removed from the in-flight set
		settleOnceHook.Do(func() {
			mu.Lock()
			seen = p.inflight.ids()
			mu.Unlock()
		})
	}
	close(gate)
	waitIdle(t, p)
	mu.Lock()
	assert.Zero(t, claimed, "a restored job still in flight is not submitted a second time")
	assert.Empty(t, seen, "the old batch's entries are gone once it has settled, and the hook claim added none")
	mu.Unlock()
	for range 10 {
		if s.pending() == 0 {
			break
		}
		require.NoError(t, p.claimOnce(ctx).err)
		waitIdle(t, p)
	}
	require.Equal(t, []string{"g-00", "g-01", "g-02", "g-03"}, s.published())
	require.Zero(t, p.inflight.size())
}
