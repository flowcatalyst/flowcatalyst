package scheduler

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests drive the poller and its lanes against a fake table and a fake
// broker: no database, no network. The ordering ones are the contract of the
// decoupled design (see lane.go).

func groupJobs(group string, n int) []*fakeJob {
	jobs := make([]*fakeJob, n)
	for i := range jobs {
		jobs[i] = &fakeJob{id: fmt.Sprintf("%s-%02d", group, i), group: group, seq: int32(i)}
	}
	return jobs
}

func ungroupedJobs(prefix string, n int) []*fakeJob {
	jobs := make([]*fakeJob, n)
	for i := range jobs {
		jobs[i] = &fakeJob{id: fmt.Sprintf("%s%02d", prefix, i)}
	}
	return jobs
}

func runEngine(t *testing.T, p *PendingJobPoller) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); p.Run(ctx) }()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Error("Run did not return after cancel")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

// randomFailPublisher accepts what it is given except that, one call in oneIn, it
// fails one job and — as the SQS publisher does — every later job of that job's
// group in the same call.
func randomFailPublisher(s *fakeStore, seed int64, oneIn int, failures *atomic.Int64) func(context.Context, []DispatchJobToken) []string {
	var mu sync.Mutex
	rng := rand.New(rand.NewSource(seed))
	return func(_ context.Context, toks []DispatchJobToken) []string {
		mu.Lock()
		failAt := -1
		if rng.Intn(oneIn) == 0 {
			failAt = rng.Intn(len(toks))
		}
		mu.Unlock()
		var unpublished []string
		failed := map[string]bool{}
		for i, tk := range toks {
			if i == failAt {
				failed[tk.MessageGroup] = true
			}
			if failed[tk.MessageGroup] {
				unpublished = append(unpublished, tk.JobID)
				continue
			}
			s.logPublished(tk.JobID)
		}
		if len(unpublished) > 0 {
			failures.Add(int64(len(unpublished)))
		}
		return unpublished
	}
}

// A group's jobs are published in order across many claims and many lanes, also
// when publishes fail midway: the later jobs of the group are dropped, and then
// everything is published again in order, exactly once.
func TestOrdering_GroupsStayInOrderAcrossClaimsLanesAndFailures(t *testing.T) {
	const groups, perGroup = 24, 12
	var jobs []*fakeJob
	for g := range groups {
		jobs = append(jobs, groupJobs(fmt.Sprintf("g%02d", g), perGroup)...)
	}
	s := newFakeStore(jobs...)
	var failures atomic.Int64
	s.publishFn = randomFailPublisher(s, 7, 4, &failures)
	droppedBefore := value(t, MetricsRegistry, "fc_scheduler_jobs_dropped_poisoned_total")

	p := newTestEngine(Config{
		PollInterval: 2 * time.Millisecond, Dispatchers: 4, BufferCapacity: 40, BatchSize: 9, LaneBatch: 5,
	}, s)
	stop := runEngine(t, p)

	require.Eventually(t, func() bool { return s.pending() == 0 }, 30*time.Second, 5*time.Millisecond, "not every job was published")
	stop()

	require.Positive(t, failures.Load(), "the test must exercise failures")
	require.Greater(t, value(t, MetricsRegistry, "fc_scheduler_jobs_dropped_poisoned_total"), droppedBefore,
		"the test must exercise the drop rule")
	published := map[string][]string{}
	for _, id := range s.published() {
		g := id[:3]
		published[g] = append(published[g], id)
	}
	require.Len(t, published, groups)
	for g := range groups {
		key := fmt.Sprintf("g%02d", g)
		want := make([]string, perGroup)
		for i := range want {
			want[i] = fmt.Sprintf("%s-%02d", key, i)
		}
		require.Equal(t, want, published[key], "group %s must be published once each, in order", key)
	}
	assert.Zero(t, len(p.permits), "permits all back")
	assert.Zero(t, p.inflight.size(), "in-flight set empty")
}

// A claim running concurrently with a failure cannot overtake. The generation
// rule, driven deterministically with the hook that lands the failure between a
// claim's generation increment and its in-flight snapshot:
//   - claim A takes j1, j2; the broker rejects them;
//   - claim X has incremented the generation but not yet snapshotted; the
//     failure is handled in that window (the jobs leave the set, the group is
//     poisoned at X's generation), then X snapshots an empty set and gets j1, j2
//     back with the poisoned generation: its jobs of the group are DROPPED;
//   - a claim taken after (a later generation) passes, and everything is
//     published in order.
//
// With the snapshot before the increment, X would snapshot j1, j2 as in flight,
// claim j3, and carry a generation past the poison mark: j3 would be published
// ahead of j1.
func TestOrdering_ClaimConcurrentWithAFailureCannotOvertake(t *testing.T) {
	s := newFakeStore(groupJobs("g", 3)...)
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
	droppedBefore := value(t, MetricsRegistry, "fc_scheduler_jobs_dropped_poisoned_total")

	a := p.claimOnce(ctx)
	require.NoError(t, a.err)
	require.Equal(t, 2, a.submitted)
	<-inPublish

	hookEntered := make(chan struct{})
	hookRelease := make(chan struct{})
	var hookOnce sync.Once
	p.hookGenSnapshot = func() {
		hookOnce.Do(func() {
			close(hookEntered)
			<-hookRelease
		})
	}
	xDone := make(chan claimResult, 1)
	go func() { xDone <- p.claimOnce(ctx) }()
	<-hookEntered // X has its generation and has not snapshotted

	close(gate) // the lane now handles A's failure
	require.Eventually(t, func() bool { return len(p.permits) == 2 }, 5*time.Second, time.Millisecond,
		"the lane must finish A's batch (only X's permits remain)")
	require.Zero(t, p.inflight.size(), "A's jobs left the in-flight set")
	close(hookRelease)
	x := <-xDone
	require.NoError(t, x.err)
	require.Equal(t, 2, x.submitted, "X claims j1, j2 again — they are not in flight")
	waitIdle(t, p)
	require.Empty(t, s.published(), "X's jobs carry the poisoned generation and must have been dropped")
	require.GreaterOrEqual(t, value(t, MetricsRegistry, "fc_scheduler_jobs_dropped_poisoned_total")-droppedBefore, 2.0)

	// Claims taken after the failure pass and publish in order.
	for range 10 {
		if s.pending() == 0 {
			break
		}
		require.NoError(t, p.claimOnce(ctx).err)
		waitIdle(t, p)
	}
	require.Equal(t, []string{"g-00", "g-01", "g-02"}, s.published())
	require.Zero(t, p.inflight.size())
}

// The generation increment must precede the in-flight snapshot, so a claim whose
// jobs overtake a failure is impossible. Same scenario as above, from the
// failure's side: a job already claimed behind the failed one (j3, in claim B
// taken BEFORE the failure was handled) is dropped, not published ahead of j1.
func TestOrdering_JobClaimedBeforeAFailureIsDroppedNotPublishedAhead(t *testing.T) {
	s := newFakeStore(groupJobs("g", 3)...)
	inPublish := make(chan struct{})
	gate := make(chan struct{})
	var calls atomic.Int32
	s.publishFn = func(_ context.Context, toks []DispatchJobToken) []string {
		if calls.Add(1) == 1 {
			close(inPublish)
			<-gate
			return tokenIDs(toks)
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

	require.Equal(t, 2, p.claimOnce(ctx).submitted) // j1, j2 -> the lane, blocked in publish
	<-inPublish
	b := p.claimOnce(ctx) // claim B: j1, j2 are in flight, so it gets j3
	require.Equal(t, 1, b.submitted)
	close(gate) // A fails; j3 (generation of B) is already behind it in the lane's channel
	waitIdle(t, p)
	require.Empty(t, s.published(), "j3 must not be published ahead of the failed j1, j2")
	require.Equal(t, "PENDING", s.status("g-02"))

	for range 10 {
		if s.pending() == 0 {
			break
		}
		require.NoError(t, p.claimOnce(ctx).err)
		waitIdle(t, p)
	}
	require.Equal(t, []string{"g-00", "g-01", "g-02"}, s.published())
}

// The poller blocks when the buffer is full and resumes when a lane releases
// permits.
func TestPoller_BlocksWhenTheBufferIsFullAndResumesWhenALaneReleases(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 10)...)
	inPublish := make(chan struct{})
	gate := make(chan struct{})
	var calls atomic.Int32
	s.publishFn = func(_ context.Context, toks []DispatchJobToken) []string {
		if calls.Add(1) == 1 {
			close(inPublish)
			<-gate
		}
		for _, tk := range toks {
			s.logPublished(tk.JobID)
		}
		return nil
	}
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 4, BatchSize: 4, LaneBatch: 100}, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := p.startLanes(ctx)
	defer func() { cancel(); wg.Wait() }()

	first := p.claimOnce(ctx)
	require.Equal(t, 4, first.submitted)
	<-inPublish

	done := make(chan claimResult, 1)
	go func() { done <- p.claimOnce(ctx) }()
	select {
	case <-done:
		t.Fatal("the poller claimed while the buffer was full")
	case <-time.After(150 * time.Millisecond):
	}
	require.Equal(t, 1, s.claimCount(), "a full buffer must not be claimed against")

	close(gate)
	select {
	case res := <-done:
		require.NoError(t, res.err)
		require.Positive(t, res.submitted, "it resumes with whatever permits the lane has returned so far")
	case <-time.After(5 * time.Second):
		t.Fatal("the poller did not resume after the lane released its permits")
	}
	waitIdle(t, p)
}

// No job id is submitted twice while it is in flight, ids leave the set on
// success, on failure and on drop, and permits are all back when idle.
func TestInflight_NoJobIsSubmittedTwiceWhileInFlight(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 8)...)
	inPublish := make(chan struct{})
	gate := make(chan struct{})
	var calls atomic.Int32
	s.publishFn = func(_ context.Context, toks []DispatchJobToken) []string {
		if calls.Add(1) == 1 {
			close(inPublish)
			<-gate
		}
		for _, tk := range toks {
			s.logPublished(tk.JobID)
		}
		return nil
	}
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 4, LaneBatch: 100}, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := p.startLanes(ctx)
	defer func() { cancel(); wg.Wait() }()

	require.Equal(t, 4, p.claimOnce(ctx).submitted)
	<-inPublish
	require.Equal(t, 4, p.claimOnce(ctx).submitted)
	require.Zero(t, p.claimOnce(ctx).claimed, "everything left is in flight: a third claim finds nothing")

	seen := map[string]int{}
	s.mu.Lock()
	for _, claim := range s.claims {
		for _, id := range claim {
			seen[id]++
		}
	}
	s.mu.Unlock()
	require.Len(t, seen, 8)
	for id, n := range seen {
		require.Equal(t, 1, n, "%s was claimed twice while in flight", id)
	}
	require.Equal(t, 8, p.inflight.size())

	close(gate)
	waitIdle(t, p)
	assert.Zero(t, p.inflight.size(), "ids leave the set on success")
	assert.Zero(t, len(p.permits), "permits all back")
	assert.Len(t, s.published(), 8)
}

func TestInflight_IdsLeaveTheSetAndPermitsReturnOnFailure(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 3)...)
	s.publishFn = func(_ context.Context, toks []DispatchJobToken) []string { return tokenIDs(toks) }
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 2, BufferCapacity: 10, BatchSize: 10, LaneBatch: 100}, s)

	res := settleOnce(t, p, context.Background())
	require.Equal(t, 3, res.submitted)
	assert.Zero(t, p.inflight.size(), "ids leave the set on failure")
	assert.Zero(t, len(p.permits))
	assert.Equal(t, 3, s.pending(), "failed jobs stay PENDING")
	assert.True(t, p.laneFailed.Load(), "the failure is reported to the poller")

	// and they are claimable again.
	s.publishFn = nil
	res = settleOnce(t, p, context.Background())
	assert.Equal(t, 3, res.claimed)
	assert.Zero(t, s.pending())
}

// The exclusion arrays are never nil, however empty: `<> ALL(NULL)` would
// exclude every row.
func TestClaim_ArraysAreNeverNil(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 2)...) // claimRows errors on a nil array
	p := newTestEngine(Config{PollInterval: time.Millisecond}, s)
	res := settleOnce(t, p, context.Background())
	assert.Equal(t, 2, res.submitted)
}

// The poller does not hot-loop when the broker fails, when everything is held,
// or on a short claim. PollInterval 50ms over 400ms allows ~8 claims; a hot loop
// makes thousands.
func hotLoopWindow(t *testing.T, p *PendingJobPoller, s *fakeStore) int {
	t.Helper()
	stop := runEngine(t, p)
	time.Sleep(400 * time.Millisecond)
	stop()
	return s.claimCount()
}

func TestPoller_DoesNotHotLoopWhenTheBrokerFails(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 3)...)
	s.publishFn = func(_ context.Context, toks []DispatchJobToken) []string { return tokenIDs(toks) }
	// Buffer = batch, so the next claim is full the moment the lane releases:
	// only the failure back-off stops a spin.
	p := newTestEngine(Config{PollInterval: 50 * time.Millisecond, Dispatchers: 1, BufferCapacity: 3, BatchSize: 3}, s)
	assert.LessOrEqual(t, hotLoopWindow(t, p, s), 14)
}

func TestPoller_DoesNotHotLoopWhenEverythingIsHeld(t *testing.T) {
	jobs := groupJobs("g", 4)
	for _, j := range jobs {
		j.mode = "BLOCK_ON_ERROR"
	}
	s := newFakeStore(jobs...)
	p := newTestEngine(Config{PollInterval: 50 * time.Millisecond, Dispatchers: 1, BufferCapacity: 4, BatchSize: 4}, s)
	// A holder in front of every job of the group.
	p.holdBack = func(context.Context, map[string]jobKey) (map[string]jobKey, error) {
		return map[string]jobKey{"g": {sequence: -1, id: "holder"}}, nil
	}
	assert.LessOrEqual(t, hotLoopWindow(t, p, s), 14)
	assert.Empty(t, s.published(), "held jobs are not published")
}

func TestPoller_DoesNotHotLoopOnAShortClaim(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 3)...)
	p := newTestEngine(Config{PollInterval: 50 * time.Millisecond, Dispatchers: 1, BufferCapacity: 100, BatchSize: 10}, s)
	// The rows never leave PENDING, so every claim is short but non-empty and
	// submits: only the short-claim back-off stops a spin.
	p.markQueued = func(_ context.Context, ids []string, _, _ []time.Time) (int64, error) {
		return int64(len(ids)), nil
	}
	assert.LessOrEqual(t, hotLoopWindow(t, p, s), 14)
}

// A full, healthy claim loops at once rather than sleeping a PollInterval: that
// is what lets the poller keep up with a deep backlog.
func TestPoller_FullClaimsLoopWithoutSleeping(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 60)...)
	p := newTestEngine(Config{PollInterval: time.Hour, Dispatchers: 2, BufferCapacity: 20, BatchSize: 10}, s)
	runEngine(t, p)
	require.Eventually(t, func() bool { return s.pending() == 0 }, 5*time.Second, time.Millisecond,
		"a PollInterval of an hour must not matter while claims come back full")
}

func TestPoller_NonLeaderDoesNotClaim(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 3)...)
	p := newTestEngine(Config{PollInterval: 10 * time.Millisecond, Dispatchers: 1}, s)
	p.IsLeader = func() bool { return false }
	assert.Zero(t, hotLoopWindow(t, p, s))
	assert.Equal(t, 3, s.pending())

	// A claim already past the leader check in Run (waiting for permits when
	// leadership went) must not claim either, and gives its permits back.
	res := p.claimOnce(context.Background())
	assert.Zero(t, res.claimed)
	assert.Zero(t, s.claimCount())
	assert.Zero(t, len(p.permits))
}

func TestPoller_LeaderClaims(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 3)...)
	p := newTestEngine(Config{PollInterval: 10 * time.Millisecond, Dispatchers: 1}, s)
	p.IsLeader = func() bool { return true }
	runEngine(t, p)
	require.Eventually(t, func() bool { return s.pending() == 0 }, 5*time.Second, time.Millisecond)
}

// Shutdown mid-batch: the lane finishes (and marks) the batch it is sending, the
// jobs still buffered are left PENDING, and Run returns promptly.
func TestShutdown_FinishesTheBatchInFlightAndLeavesTheRestPending(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 6)...)
	inPublish := make(chan struct{})
	gate := make(chan struct{})
	var calls atomic.Int32
	s.publishFn = func(ctx context.Context, toks []DispatchJobToken) []string {
		if calls.Add(1) == 1 {
			close(inPublish)
			<-gate
		}
		if ctx.Err() != nil {
			return tokenIDs(toks) // a publish still tied to the cancelled context would fail here
		}
		for _, tk := range toks {
			s.logPublished(tk.JobID)
		}
		return nil
	}
	// One lane, two jobs per batch: the first batch is two jobs, four stay buffered.
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 6, BatchSize: 6, LaneBatch: 2}, s)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); p.Run(ctx) }()

	<-inPublish
	cancel()
	select {
	case <-done:
		t.Fatal("Run returned while a batch was mid-publish")
	case <-time.After(100 * time.Millisecond):
	}
	close(gate)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the in-flight batch finished")
	}
	assert.Len(t, s.published(), 2, "only the batch being sent was published")
	queued := 0
	for _, id := range s.published() {
		assert.Equal(t, "QUEUED", s.status(id), "the published batch is marked QUEUED despite the cancel")
		queued++
	}
	assert.Equal(t, 2, queued)
	assert.Equal(t, 4, s.pending(), "buffered jobs are left PENDING")
}

// A broker that has stopped answering cannot hold shutdown for ever: the publish
// is cut off shutdownGrace after the cancel and everything is left PENDING.
func TestShutdown_ABrokerThatHangsIsCutOffAfterTheGrace(t *testing.T) {
	old := shutdownGrace
	shutdownGrace = 50 * time.Millisecond
	defer func() { shutdownGrace = old }()

	s := newFakeStore(ungroupedJobs("u", 3)...)
	inPublish := make(chan struct{})
	s.publishFn = func(ctx context.Context, toks []DispatchJobToken) []string {
		close(inPublish)
		<-ctx.Done()
		return tokenIDs(toks)
	}
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 3, BatchSize: 3}, s)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); p.Run(ctx) }()
	<-inPublish
	start := time.Now()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return: the hung publish was not cut off")
	}
	assert.Less(t, time.Since(start), 2*time.Second)
	assert.Equal(t, 3, s.pending())
}

// --- lane.process, directly -------------------------------------------------

// laneFixture is one lane with its permits and in-flight ids set up as the
// poller would have left them for a batch.
func laneFixture(t *testing.T, s *fakeStore, batch []laneJob) *lane {
	t.Helper()
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 50}, s)
	l := p.lanes[0]
	track(l, batch...)
	return l
}

// track registers jobs as a claim would have left them: a permit each, and in the
// in-flight set.
func track(l *lane, jobs ...laneJob) {
	for range jobs {
		l.p.permits <- struct{}{}
	}
	l.p.inflight.add(jobs)
}

// poisonOf reads a group's poison mark.
func poisonOf(l *lane, group string) (uint64, bool) {
	l.p.inflight.mu.Lock()
	defer l.p.inflight.mu.Unlock()
	e, ok := l.p.inflight.poison[group]
	return e.gen, ok
}

func setPoison(l *lane, group string, gen uint64, at time.Time) {
	l.p.inflight.mu.Lock()
	defer l.p.inflight.mu.Unlock()
	l.p.inflight.poison[group] = poisonEntry{gen: gen, at: at}
}

func lj(id, group string, gen uint64) laneJob {
	return laneJob{tok: DispatchJobToken{JobID: id, MessageGroup: group}, gen: gen, createdAt: time.Now(), updatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func TestLane_UngroupedJobsAreNeverPoisoned(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 2)...)
	s.publishFn = func(_ context.Context, toks []DispatchJobToken) []string {
		return []string{"u00"} // u00 fails
	}
	batch := []laneJob{lj("u00", "", 1)}
	l := laneFixture(t, s, batch)
	l.p.claimGeneration.Store(5)
	l.process(context.Background(), batch)
	_, poisoned := poisonOf(l, "")
	assert.False(t, poisoned, "an ungrouped job has no group to poison")

	s.publishFn = nil
	batch = []laneJob{lj("u01", "", 1)} // an OLD generation: would be dropped if ungrouped jobs were poisonable
	track(l, batch...)
	l.process(context.Background(), batch)
	assert.Equal(t, []string{"u01"}, s.published())
}

func TestLane_FirstJobPastThePoisonMarkClearsIt(t *testing.T) {
	s := newFakeStore(groupJobs("g", 2)...)
	s.publishFn = func(_ context.Context, toks []DispatchJobToken) []string { return tokenIDs(toks) }
	batch := []laneJob{lj("g-00", "g", 1)}
	l := laneFixture(t, s, batch)
	l.p.claimGeneration.Store(5)
	l.process(context.Background(), batch)
	gen, _ := poisonOf(l, "g")
	require.Equal(t, uint64(5), gen, "poisoned at the generation read after the failure")

	s.publishFn = nil
	older := []laneJob{lj("g-00", "g", 3)} // claimed before the failure was handled
	track(l, older...)
	l.process(context.Background(), older)
	assert.Empty(t, s.published(), "a job at or below the mark is dropped")
	assert.Zero(t, l.p.inflight.size(), "a dropped job leaves the in-flight set")
	assert.Zero(t, len(l.p.permits), "a dropped job's permit is released")

	newer := []laneJob{lj("g-00", "g", 6), lj("g-01", "g", 6)}
	track(l, newer...)
	l.process(context.Background(), newer)
	assert.Equal(t, []string{"g-00", "g-01"}, s.published(), "past the mark: published in order")
	_, poisoned := poisonOf(l, "g")
	assert.False(t, poisoned, "the first job past the mark cleared it")
}

// Once a job of a group is dropped, every later job of the group in the batch is
// dropped too, even one from a newer claim that is past the mark: the dropped job
// has to be claimed and published first.
func TestLane_ADroppedJobDropsTheRestOfItsGroupInTheBatch(t *testing.T) {
	s := newFakeStore(groupJobs("g", 3)...)
	batch := []laneJob{lj("g-01", "g", 1), lj("g-02", "g", 2), lj("other", "h", 2)}
	l := laneFixture(t, s, batch)
	setPoison(l, "g", 1, time.Now())
	l.p.claimGeneration.Store(2)
	l.process(context.Background(), batch)
	assert.Equal(t, []string{"other"}, s.published(), "only the unaffected group is published")
	gen, _ := poisonOf(l, "g")
	assert.Equal(t, uint64(1), gen, "a drop does not renew the mark")
}

func TestLane_StalePoisonMarksAreEvicted(t *testing.T) {
	s := newFakeStore()
	l := laneFixture(t, s, nil)
	setPoison(l, "old", 1, time.Now().Add(-poisonTTL-time.Minute))
	setPoison(l, "recent", 1, time.Now())
	l.process(context.Background(), nil)
	_, oldOK := poisonOf(l, "old")
	_, recentOK := poisonOf(l, "recent")
	assert.False(t, oldOK)
	assert.True(t, recentOK)
}

// The QUEUED update skips a job that already moved on, and says so.
func TestLane_MarkQueuedSkipsAJobThatMovedOn(t *testing.T) {
	s := newFakeStore(groupJobs("g", 2)...)
	batch := []laneJob{lj("g-00", "g", 1), lj("g-01", "g", 1)}
	l := laneFixture(t, s, batch)
	// The router delivered g-00 and the callback moved it on before the update.
	s.mu.Lock()
	s.jobs[0].status = "PROCESSING"
	s.mu.Unlock()
	before := value(t, MetricsRegistry, "fc_scheduler_mark_queued_not_updated_total")
	l.process(context.Background(), batch)
	assert.Equal(t, "PROCESSING", s.status("g-00"), "never regressed to QUEUED")
	assert.Equal(t, "QUEUED", s.status("g-01"))
	assert.Equal(t, 1.0, value(t, MetricsRegistry, "fc_scheduler_mark_queued_not_updated_total")-before)
}

// A drop does NOT renew the poison. Renewing it (poisoning again at the
// generation read after a drop) livelocks when the poller claims faster than a
// lane drains: every claim made while a batch is being dropped is older than the
// renewed mark and is dropped in turn.
func TestLane_ADropDoesNotRenewThePoison(t *testing.T) {
	s := newFakeStore(groupJobs("g", 3)...)
	l := laneFixture(t, s, []laneJob{lj("g-01", "g", 1)})
	setPoison(l, "g", 1, time.Now()) // j1 failed in generation 1
	l.p.claimGeneration.Store(5)     // the poller has claimed on since

	l.process(context.Background(), []laneJob{lj("g-01", "g", 1)}) // j2: dropped
	assert.Empty(t, s.published())
	gen, ok := poisonOf(l, "g")
	require.True(t, ok)
	assert.Equal(t, uint64(1), gen, "the mark stands at the failure's generation: a drop does not move it")

	// So a claim taken after the failure (generation 2, well below the current
	// generation 5 a renewal would have used) is NOT dropped.
	x := []laneJob{lj("g-00", "g", 2), lj("g-01", "g", 2), lj("g-02", "g", 2)}
	track(l, x...)
	l.process(context.Background(), x)
	assert.Equal(t, []string{"g-00", "g-01", "g-02"}, s.published())
}

// A claim that excluded a doomed in-flight job of a group must not submit the
// group's jobs: they are behind the doomed job, and would be published ahead of
// it. They stay PENDING, and are claimed again, in order, once the doomed job has
// gone.
func TestPoller_AClaimThatSawADoomedInFlightJobDoesNotSubmitTheJobsBehindIt(t *testing.T) {
	s := newFakeStore(append(groupJobs("g", 3), groupJobs("h", 1)...)...)
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 10}, s)
	l := p.lanes[0]
	// g-01 waits in the lane (generation 1) and its group was poisoned at 1: doomed.
	track(l, lj("g-01", "g", 1))
	setPoison(l, "g", 1, time.Now())
	p.claimGeneration.Store(1)
	before := value(t, MetricsRegistry, "fc_scheduler_jobs_withheld_doomed_total")

	res := p.claimOnce(context.Background())
	require.NoError(t, res.err)
	assert.Equal(t, 3, res.claimed, "the store returns g-00, g-02 and h-00 (g-01 is excluded as in flight)")
	assert.Equal(t, 1, res.submitted, "only the other group's job is submitted")
	assert.Equal(t, 1, len(l.in), "g-00 and g-02 never reached a lane")
	assert.Equal(t, 2.0, value(t, MetricsRegistry, "fc_scheduler_jobs_withheld_doomed_total")-before)
	assert.Equal(t, 2, p.inflight.size(), "withheld jobs are not in flight (g-01 and h-00 are)")

	// The doomed job has gone (dropped by its lane): the next claim takes the
	// whole group, in order.
	<-l.in
	p.inflight.remove([]string{"h-00"})
	p.release(1)
	l.p.inflight.remove([]string{"g-01"})
	p.release(1)
	require.Zero(t, len(p.permits))
	res = settleOnce(t, p, context.Background())
	assert.Equal(t, 4, res.submitted)
	assert.Equal(t, []string{"g-00", "g-01", "g-02", "h-00"}, s.published())
}

// The failure's poison generation is read AFTER its ids left the in-flight set.
// Driven with the hook that runs a whole claim between those two steps: that
// claim snapshots the emptied set, so it re-claims j1, j2 and carries a
// generation the poison mark must cover. Read before the removal, the mark would
// sit below that claim's generation, a claim taken just before the removal (its
// snapshot still holding j1, j2) would claim j3 past the mark, and j3 would be
// published ahead of j1.
func TestOrdering_PoisonGenerationIsReadAfterTheIdsLeaveTheSet(t *testing.T) {
	s := newFakeStore(groupJobs("g", 3)...)
	inPublish := make(chan struct{})
	gate := make(chan struct{})
	var calls atomic.Int32
	s.publishFn = func(_ context.Context, toks []DispatchJobToken) []string {
		if calls.Add(1) == 1 {
			close(inPublish)
			<-gate
			return tokenIDs(toks)
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

	require.Equal(t, 2, p.claimOnce(ctx).submitted)
	<-inPublish
	var once sync.Once
	p.hookSettle = func() {
		once.Do(func() { _ = p.claimOnce(ctx) }) // a claim lands between the removal and the read
	}
	close(gate)
	waitIdle(t, p)
	for range 10 {
		if s.pending() == 0 {
			break
		}
		require.NoError(t, p.claimOnce(ctx).err)
		waitIdle(t, p)
	}
	require.Equal(t, []string{"g-00", "g-01", "g-02"}, s.published())
}

// The same ordering rule where it is observable: with a lane batch of ONE, the
// second job of the group (g-01) waits in the lane's channel, still in flight,
// while g-00 fails and leaves the set. With the default lane batch both jobs
// settle together, nothing stays in flight behind the failure, and reading the
// generation before the removal is unobservable (the test above passes either
// way). Here the hook's claim excludes g-01 (in flight) and re-claims g-00 and
// g-02 under a new generation the poison must cover; read before the removal, the
// mark sits below it, g-00 passes, and g-02 is published ahead of the dropped
// g-01. Mutant: read claimGeneration before p.inflight.remove in lane.process.
func TestOrdering_PoisonGenerationIsReadAfterTheIdsLeaveTheSet_SingleJobLaneBatches(t *testing.T) {
	s := newFakeStore(groupJobs("g", 3)...)
	inPublish := make(chan struct{})
	gate := make(chan struct{})
	var calls atomic.Int32
	s.publishFn = func(_ context.Context, toks []DispatchJobToken) []string {
		if calls.Add(1) == 1 {
			close(inPublish)
			<-gate
			return tokenIDs(toks)
		}
		for _, tk := range toks {
			s.logPublished(tk.JobID)
		}
		return nil
	}
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 2, LaneBatch: 1}, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := p.startLanes(ctx)
	defer func() { cancel(); wg.Wait() }()

	require.Equal(t, 2, p.claimOnce(ctx).submitted)
	<-inPublish
	var once sync.Once
	p.hookSettle = func() {
		once.Do(func() { _ = p.claimOnce(ctx) }) // a claim lands between the removal and the read
	}
	close(gate)
	waitIdle(t, p)
	for range 10 {
		if s.pending() == 0 {
			break
		}
		require.NoError(t, p.claimOnce(ctx).err)
		waitIdle(t, p)
	}
	require.Equal(t, []string{"g-00", "g-01", "g-02"}, s.published())
}

// A job the callback has already processed and rescheduled back to PENDING
// (retry, deferral, a BLOCK_ON_ERROR hold) between the publish and the mark has
// a newer row version, so the optimistic QUEUED update leaves it alone — and
// counts it. A status guard alone would set it QUEUED with no message in the
// queue.
func TestLane_MarkQueuedSkipsAJobRescheduledToPendingMeanwhile(t *testing.T) {
	s := newFakeStore(groupJobs("g", 2)...)
	batch := []laneJob{lj("g-00", "g", 1), lj("g-01", "g", 1)}
	l := laneFixture(t, s, batch)
	// Version 0 is what the claim read (lj leaves updatedAt zero, so set it).
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	batch[0].updatedAt, batch[1].updatedAt = base, base
	// The callback ran and put g-00 back to PENDING: still PENDING, new version.
	s.mu.Lock()
	s.jobs[0].ver++
	s.mu.Unlock()
	before := value(t, MetricsRegistry, "fc_scheduler_mark_queued_not_updated_total")
	l.process(context.Background(), batch)
	assert.Equal(t, "PENDING", s.status("g-00"), "a rescheduled job is not set QUEUED")
	assert.Equal(t, "QUEUED", s.status("g-01"))
	assert.Equal(t, 1.0, value(t, MetricsRegistry, "fc_scheduler_mark_queued_not_updated_total")-before)
}

// Stress: many groups, random publish failures, random mark failures (which
// publish a job twice), random lane stalls so claims and failures interleave in
// every order. Duplicates are allowed; the FIRST delivery of each group's jobs
// must be in order.
func TestOrdering_StressFirstDeliveriesStayInOrder(t *testing.T) {
	const groups, perGroup = 30, 10
	var jobs []*fakeJob
	for g := range groups {
		jobs = append(jobs, groupJobs(fmt.Sprintf("g%02d", g), perGroup)...)
	}
	s := newFakeStore(jobs...)
	var failures atomic.Int64
	fail := randomFailPublisher(s, 11, 3, &failures)
	var smu sync.Mutex
	srng := rand.New(rand.NewSource(5))
	s.publishFn = func(ctx context.Context, toks []DispatchJobToken) []string {
		smu.Lock()
		d := time.Duration(srng.Intn(1500)) * time.Microsecond
		smu.Unlock()
		time.Sleep(d)
		return fail(ctx, toks)
	}
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 6, BufferCapacity: 50, BatchSize: 7, LaneBatch: 4}, s)
	var mmu sync.Mutex
	mrng := rand.New(rand.NewSource(9))
	p.markQueued = func(ctx context.Context, ids []string, c, v []time.Time) (int64, error) {
		mmu.Lock()
		skip := mrng.Intn(15) == 0
		mmu.Unlock()
		if skip {
			return 0, fmt.Errorf("mark failed")
		}
		return s.markQueued(ctx, ids, c, v)
	}
	stop := runEngine(t, p)
	require.Eventually(t, func() bool { return s.pending() == 0 }, 60*time.Second, 5*time.Millisecond)
	stop()

	first := map[string]map[string]bool{}
	order := map[string][]string{}
	for _, id := range s.published() {
		g := id[:3]
		if first[g] == nil {
			first[g] = map[string]bool{}
		}
		if !first[g][id] {
			first[g][id] = true
			order[g] = append(order[g], id)
		}
	}
	for g, ids := range order {
		require.Len(t, ids, perGroup, "group %s: every job delivered", g)
		for i := 1; i < len(ids); i++ {
			require.Less(t, ids[i-1], ids[i], "group %s: first delivery out of order: %v", g, ids)
		}
	}
}

func TestStarveWarn_RateLimitedToOncePerMinute(t *testing.T) {
	p := newPoller(Config{})
	t0 := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	assert.True(t, p.starveWarnDue(t0), "first warning is due")
	assert.False(t, p.starveWarnDue(t0.Add(59*time.Second)), "suppressed inside the minute")
	assert.True(t, p.starveWarnDue(t0.Add(61*time.Second)), "due again after a minute")
	assert.False(t, p.starveWarnDue(t0.Add(62*time.Second)))
}

func TestConfig_NormalizedFillsZeroValues(t *testing.T) {
	c := Config{}.normalized()
	assert.Equal(t, DefaultBufferCapacity, c.BufferCapacity)
	assert.Equal(t, DefaultDispatchers, c.Dispatchers)
	assert.Equal(t, DefaultBatchSize, c.BatchSize)
	assert.Equal(t, DefaultLaneBatch, c.LaneBatch)
	assert.Equal(t, 1000, DefaultBufferCapacity)
	assert.Equal(t, 10, DefaultDispatchers)
	assert.Equal(t, 500, DefaultBatchSize)
	assert.Equal(t, 100, DefaultLaneBatch)
	assert.Equal(t, time.Second, DefaultConfig().PollInterval)
}
