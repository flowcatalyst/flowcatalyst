package router

// Regression tests for the consumer poll-loop's handling of a stopped /
// erroring consumer. The bug these guard against: a consumer whose underlying
// queue was Stop()'d (but whose poll loop wasn't torn down) spun ~once a second
// forever logging "Error polling: Queue is stopped", and — because the loop
// stamped its watchdog heartbeat on every poll, including errored ones — looked
// alive to RestartStalledConsumers, so it was never rebuilt and the queue was
// never drained.

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// pollErrConsumer is a fake queue.Consumer whose Poll returns a fixed error and
// counts calls.
type pollErrConsumer struct {
	id      string
	err     error
	polls   atomic.Int64
	stopped atomic.Bool
}

func (c *pollErrConsumer) Identifier() string { return c.id }
func (c *pollErrConsumer) Poll(context.Context, uint32) ([]common.QueuedMessage, error) {
	c.polls.Add(1)
	return nil, c.err
}
func (c *pollErrConsumer) Ack(context.Context, string, string) error       { return nil }
func (c *pollErrConsumer) Nack(context.Context, string, *uint32) error     { return nil }
func (c *pollErrConsumer) Defer(context.Context, string, *uint32) error    { return nil }
func (c *pollErrConsumer) Healthy() bool                                   { return true }
func (c *pollErrConsumer) Stop()                                           { c.stopped.Store(true) }
func (c *pollErrConsumer) Metrics(context.Context) (*queue.Metrics, error) { return nil, nil }
func (c *pollErrConsumer) Counters() *queue.Metrics                        { return nil }

// hangConsumer's Poll never returns on its own — it blocks until the caller's
// context is done, which is what an SQS ReceiveMessage on a dead connection
// does when nothing bounds it.
type hangConsumer struct {
	pollErrConsumer
	entered chan struct{}
	once    sync.Once
}

func (c *hangConsumer) Poll(ctx context.Context, _ uint32) ([]common.QueuedMessage, error) {
	c.polls.Add(1)
	c.once.Do(func() { close(c.entered) })
	<-ctx.Done()
	return nil, ctx.Err()
}

// managerWithCapacity builds a Manager with a default pool that reports
// capacity, so runConsumer proceeds to Poll.
func managerWithCapacity() *Manager {
	m := NewManager(nil, nil)
	m.pools[defaultPoolCode] = NewPool(
		common.PoolConfig{Code: defaultPoolCode, Concurrency: 8},
		nil, nil, func(string) queue.Consumer { return nil },
	)
	return m
}

// A consumer reporting queue.ErrStopped must make runConsumer exit promptly (so
// the restart watchdog respawns it) instead of spinning on the dead consumer,
// and must not advance the heartbeat.
func TestRunConsumerExitsWhenConsumerStopped(t *testing.T) {
	m := managerWithCapacity()
	rc := &runningConsumer{consumer: &pollErrConsumer{id: "q-high.fifo", err: queue.ErrStopped}, cancel: func() {}}
	const sentinel = int64(12345)
	rc.lastPoll.Store(sentinel)

	m.wg.Add(1)
	done := make(chan struct{})
	go func() { m.runConsumer(context.Background(), rc); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runConsumer did not exit on queue.ErrStopped — it is spinning on a dead consumer")
	}
	if got := rc.lastPoll.Load(); got != sentinel {
		t.Fatalf("heartbeat advanced on a stopped poll (got %d, want %d) — a wedged consumer would look alive to the watchdog", got, sentinel)
	}
}

// A non-terminal poll error must NOT advance the heartbeat, so a consumer stuck
// erroring goes stale and the watchdog rebuilds it. The loop keeps retrying
// until the context is cancelled.
func TestRunConsumerNoHeartbeatOnPollError(t *testing.T) {
	m := managerWithCapacity()
	fake := &pollErrConsumer{id: "q.fifo", err: errors.New("transient boom")}
	rc := &runningConsumer{consumer: fake, cancel: func() {}}
	const sentinel = int64(999)
	rc.lastPoll.Store(sentinel)

	ctx, cancel := context.WithCancel(context.Background())
	m.wg.Add(1)
	done := make(chan struct{})
	go func() { m.runConsumer(ctx, rc); close(done) }()

	// Wait until at least one poll happened, then stop the loop.
	deadline := time.Now().Add(2 * time.Second)
	for fake.polls.Load() == 0 {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("consumer never polled")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runConsumer did not exit after context cancel")
	}
	if got := rc.lastPoll.Load(); got != sentinel {
		t.Fatalf("heartbeat advanced on an errored poll (got %d, want %d)", got, sentinel)
	}
}

// TestRunConsumerBackpressurePauseKeepsHeartbeatFresh pins the deadlock fix:
// a consumer paused because its destination pools are FULL is healthy — it is
// applying backpressure on purpose — and must not look stalled to
// RestartStalledConsumers.
//
// It used to. The loop stamped its heartbeat only after a successful Poll, and
// the backpressure branch continues before reaching that, so a pool at
// capacity aged its own consumer out. The watchdog then rebuilt the consumer,
// the rebuild cancelled workCtx, and that parked the ordered-group drainers
// that were the only thing draining the buffer that was full — so capacity
// never returned and the branch never exited. A permanent deadlock assembled
// entirely from healthy parts.
func TestRunConsumerBackpressurePauseKeepsHeartbeatFresh(t *testing.T) {
	m := managerWithCapacity()
	// Fill the only pool so hasCapacityFor is false for the whole test.
	m.pools[defaultPoolCode].queueSize.Store(10_000)

	fake := &pollErrConsumer{id: "q.fifo", err: errors.New("must never be polled while full")}
	rc := &runningConsumer{consumer: fake, cancel: func() {}}
	stale := time.Now().Add(-10 * time.Minute).UnixNano()
	rc.lastPoll.Store(stale)
	m.consumers["q.fifo"] = rc

	ctx, cancel := context.WithCancel(context.Background())
	m.wg.Add(1)
	done := make(chan struct{})
	go func() { m.runConsumer(ctx, rc); close(done) }()

	deadline := time.Now().Add(2 * time.Second)
	for rc.lastPoll.Load() == stale {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("backpressure pause never refreshed the heartbeat")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// The invariant that matters: the watchdog leaves it alone.
	if n := m.RestartStalledConsumers(ctx, time.Minute); n != 0 {
		t.Fatalf("watchdog restarted %d consumer(s) that were merely applying backpressure", n)
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runConsumer did not exit after context cancel")
	}
	if got := fake.polls.Load(); got != 0 {
		t.Fatalf("polled %d time(s) while every destination pool was full", got)
	}
}

// stalledRC builds a runningConsumer that already looks stalled, wired to a
// queue URI whose backend scheme the caller has registered. cancel AND
// stopPoll are both stubbed (not nil) — a successful restart now detaches
// via stopPoll rather than cancel (R-26/R-49; see
// RestartStalledConsumers), so a real func is needed here too or that call
// panics on a nil CancelFunc.
func stalledRC(uri string) (*runningConsumer, *pollErrConsumer) {
	old := &pollErrConsumer{id: "q", err: errors.New("wedged inside Poll")}
	rc := &runningConsumer{
		consumer: old,
		cancel:   func() {},
		stopPoll: func() {},
		queueCfg: common.QueueConfig{Name: "q", URI: uri},
	}
	rc.lastPoll.Store(time.Now().Add(-time.Hour).UnixNano())
	return rc, old
}

// TestRestartStalledConsumerKeepsEntryWhenRebuildFails pins the destructive
// half of the old rebuild order: it stopped the stalled consumer BEFORE trying
// to build a replacement, so a build failure downgraded "stalled consumer"
// into "no consumer at all" — the queue then filled with nobody polling it,
// and the only visible trace was one ERROR log.
//
// The failure must now be inert: the existing entry stays, un-stopped, and the
// attempt is counted so a queue that can never be rebuilt escalates instead of
// reporting attempt 1 for ever.
func TestRestartStalledConsumerKeepsEntryWhenRebuildFails(t *testing.T) {
	queue.RegisterConsumer("teststallfail", func(context.Context, common.QueueConfig) (queue.Consumer, error) {
		return nil, errors.New("credentials unavailable")
	})

	m := managerWithCapacity()
	m.restartDelay = 0
	rc, old := stalledRC("teststallfail://x")
	m.consumers["q"] = rc

	if n := m.RestartStalledConsumers(context.Background(), time.Minute); n != 0 {
		t.Fatalf("reported %d restart(s) from a failed rebuild", n)
	}
	m.consumerMu.RLock()
	cur := m.consumers["q"]
	m.consumerMu.RUnlock()
	if cur != rc {
		t.Fatal("a failed rebuild replaced or removed the entry — the queue is left unconsumed")
	}
	if old.stopped.Load() {
		t.Fatal("a failed rebuild stopped the existing consumer — nothing polls this queue now")
	}

	// A second failure must count, not reset: this is the case that needs to
	// get louder, and it is the one the old code could never escalate.
	m.RestartStalledConsumers(context.Background(), time.Minute)
	if got := m.restartAttempts["q"].attempts; got != 2 {
		t.Fatalf("restartAttempts = %d, want 2 — a permanently failing rebuild must escalate", got)
	}
}

// TestRestartStalledConsumerBoundsHangingRebuild pins the other half. Building
// an SQS consumer is real network I/O (AWS config load + credential
// resolution) with no client-level timeout, and this runs synchronously inside
// the watchdog tick — so an unbounded build hangs the very loop whose job is
// to fix hangs, and nothing polls that queue again until the process restarts.
func TestRestartStalledConsumerBoundsHangingRebuild(t *testing.T) {
	queue.RegisterConsumer("teststallhang", func(ctx context.Context, _ common.QueueConfig) (queue.Consumer, error) {
		<-ctx.Done() // never returns on its own
		return nil, ctx.Err()
	})

	m := managerWithCapacity()
	m.restartDelay = 0
	m.rebuildTimeout = 100 * time.Millisecond
	rc, _ := stalledRC("teststallhang://x")
	m.consumers["q"] = rc

	// A caller context far longer than the rebuild bound: if the build is not
	// bounded on its own, this blocks until the deadline instead of ~100ms.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	m.RestartStalledConsumers(ctx, time.Minute)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("rebuild took %s — it is not bounded, so a hung build wedges the watchdog", elapsed)
	}
	m.consumerMu.RLock()
	cur := m.consumers["q"]
	m.consumerMu.RUnlock()
	if cur != rc {
		t.Fatal("a timed-out rebuild left the queue without its consumer")
	}
}

// TestRestartStalledConsumerCountAccumulatesAcrossRestarts pins the escalation
// bug that production surfaced: a consumer restarted every 90 seconds for
// hours logged "attempt 1" every single time, so it never reached the CRITICAL
// warning at consumerRestartCriticalAfter and nothing ever got loud about it.
//
// The cause was the recovery rule. Rebuilding a consumer stamps its lastPoll
// fresh, so on the very next tick it is not stalled — and the counter was
// cleared on exactly that basis, every cycle. Recovery has to mean quiet for a
// while, not "not stalled at this instant".
func TestRestartStalledConsumerCountAccumulatesAcrossRestarts(t *testing.T) {
	queue.RegisterConsumer("teststallcount", func(context.Context, common.QueueConfig) (queue.Consumer, error) {
		return &pollErrConsumer{id: "q", err: errors.New("still wedged")}, nil
	})

	m := managerWithCapacity()
	m.restartDelay = 0
	const threshold = time.Minute

	for cycle := 1; cycle <= 3; cycle++ {
		// Each cycle: the current consumer is stale, so the watchdog restarts
		// it — and the replacement is immediately stale again, exactly as a
		// consumer that keeps wedging (or keeps pausing) would be.
		m.consumerMu.Lock()
		for _, rc := range m.consumers {
			rc.lastPoll.Store(time.Now().Add(-time.Hour).UnixNano())
		}
		if len(m.consumers) == 0 {
			rc, _ := stalledRC("teststallcount://x")
			m.consumers["q"] = rc
		}
		m.consumerMu.Unlock()

		if n := m.RestartStalledConsumers(context.Background(), threshold); n != 1 {
			t.Fatalf("cycle %d: restarted %d, want 1", cycle, n)
		}
		// The tick in between, where the freshly rebuilt consumer looks
		// healthy. This is the call that used to wipe the counter.
		if n := m.RestartStalledConsumers(context.Background(), threshold); n != 0 {
			t.Fatalf("cycle %d: a freshly restarted consumer must not restart again immediately (got %d)", cycle, n)
		}
		if got := m.restartAttempts["q"].attempts; got != cycle {
			t.Fatalf("after %d restart(s) the count is %d — a restart loop must accumulate, not report attempt 1 for ever", cycle, got)
		}
	}

	// A consumer quiet for longer than the recovery window is forgotten, so an
	// unrelated stall later does not inherit this history.
	rec := m.restartAttempts["q"]
	rec.last = time.Now().Add(-4 * threshold)
	m.restartAttempts["q"] = rec
	m.consumerMu.Lock()
	for _, rc := range m.consumers {
		rc.lastPoll.Store(time.Now().UnixNano())
	}
	m.consumerMu.Unlock()
	m.RestartStalledConsumers(context.Background(), threshold)
	if _, still := m.restartAttempts["q"]; still {
		t.Fatal("a consumer quiet past the recovery window must have its count cleared")
	}
}

// TestRunConsumerBoundsAHangingPoll pins the gap that made a wedged consumer
// invisible: the backends take the caller's context and add no deadline of
// their own (the SQS client is built with no HTTP or request timeout), so a
// poll that never returns parked the loop with no heartbeat, no error and no
// log at all — indistinguishable from a loop that never reached the poll. The
// deadline must turn it into an ordinary, logged, retried poll error.
func TestRunConsumerBoundsAHangingPoll(t *testing.T) {
	m := managerWithCapacity()
	m.pollTimeout = 100 * time.Millisecond

	fake := &hangConsumer{
		pollErrConsumer: pollErrConsumer{id: "q.fifo"},
		entered:         make(chan struct{}),
	}
	rc := &runningConsumer{consumer: fake, cancel: func() {}}
	rc.lastPoll.Store(time.Now().UnixNano())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.wg.Add(1)
	done := make(chan struct{})
	go func() { m.runConsumer(ctx, rc); close(done) }()

	select {
	case <-fake.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("consumer never entered Poll")
	}

	// The loop must come back from the hung poll and keep retrying, rather
	// than parking there until the watchdog rebuilds it.
	deadline := time.Now().Add(3 * time.Second)
	for fake.polls.Load() < 2 {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("the poll was not bounded — the loop is parked inside consumer.Poll")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done

	// started/returned must be balanced once the loop is out, which is the
	// signal that tells an operator this was a hung poll and not a pause.
	if st, rt := rc.pollsStarted.Load(), rc.pollsReturned.Load(); st == 0 || st != rt {
		t.Fatalf("poll counters unbalanced after exit: started=%d returned=%d", st, rt)
	}
}

// A consumer that completes a poll during the restart pause is no longer
// stalled, and must not be torn down — the teardown cancels workCtx, which
// parks the ordered-group drainers.
func TestRestartStalledConsumerSkipsOneThatRecovered(t *testing.T) {
	m := managerWithCapacity()
	m.restartDelay = 0
	rc, old := stalledRC("teststallrecover://x")
	m.consumers["q"] = rc

	// A backend that builds fine — so the only thing that can stop the restart
	// is the recovery re-check — and that completes a poll WHILE being built,
	// which is the window (pause + build) the check has to cover.
	queue.RegisterConsumer("teststallrecover", func(context.Context, common.QueueConfig) (queue.Consumer, error) {
		rc.lastPoll.Store(time.Now().UnixNano())
		return &pollErrConsumer{id: "q"}, nil
	})

	if n := m.RestartStalledConsumers(context.Background(), time.Minute); n != 0 {
		t.Fatalf("restarted %d consumer(s) that had already recovered", n)
	}
	if old.stopped.Load() {
		t.Fatal("a recovered consumer was torn down anyway")
	}
}
