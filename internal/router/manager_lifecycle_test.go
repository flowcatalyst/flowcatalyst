package router

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// --- a fake backend, so the poll loop, hot reload and the consumer watchdog
// can be exercised for real rather than by poking at maps ---

const fakeScheme = "faketest"

// fakeQueue is a registered queue backend that hands out a fixed set of
// messages once and then polls empty. It records polls (so a test can tell
// whether intake is running), acks and stops.
type fakeQueue struct {
	name    string
	polls   atomic.Int64
	acks    atomic.Int64
	nacks   atomic.Int64
	stopped atomic.Bool

	mu      sync.Mutex
	pending []common.QueuedMessage
}

var fakeQueues sync.Map // queue name → *fakeQueue

func init() {
	queue.RegisterConsumer(fakeScheme, func(_ context.Context, cfg common.QueueConfig) (queue.Consumer, error) {
		q := &fakeQueue{name: cfg.Name}
		// Last writer wins: a rebuilt consumer replaces the stalled one, which
		// is what a test asserting on a restart wants to observe.
		fakeQueues.Store(cfg.Name, q)
		return q, nil
	})
}

func fakeQueueFor(t *testing.T, name string) *fakeQueue {
	t.Helper()
	v, ok := fakeQueues.Load(name)
	require.Truef(t, ok, "no fake queue built for %q", name)
	return v.(*fakeQueue)
}

func (q *fakeQueue) Identifier() string { return q.name }

func (q *fakeQueue) Poll(_ context.Context, _ uint32) ([]common.QueuedMessage, error) {
	if q.stopped.Load() {
		return nil, queue.ErrStopped
	}
	q.polls.Add(1)
	q.mu.Lock()
	out := q.pending
	q.pending = nil
	q.mu.Unlock()
	return out, nil
}

func (q *fakeQueue) enqueue(msgs ...common.QueuedMessage) {
	q.mu.Lock()
	q.pending = append(q.pending, msgs...)
	q.mu.Unlock()
}

func (q *fakeQueue) Ack(context.Context, string, string) error   { q.acks.Add(1); return nil }
func (q *fakeQueue) Nack(context.Context, string, *uint32) error { q.nacks.Add(1); return nil }
func (q *fakeQueue) Defer(context.Context, string, *uint32) error {
	return nil
}
func (q *fakeQueue) Healthy() bool { return !q.stopped.Load() }
func (q *fakeQueue) Stop()         { q.stopped.Store(true) }
func (q *fakeQueue) Metrics(context.Context) (*queue.Metrics, error) {
	return &queue.Metrics{QueueIdentifier: q.name}, nil
}
func (q *fakeQueue) Counters() *queue.Metrics { return nil }

func fakeQueueCfg(name string) common.QueueConfig {
	return common.QueueConfig{Name: name, URI: fakeScheme + "://" + name}
}

func routerCfg(queues []string, pools ...common.PoolConfig) common.RouterConfig {
	cfg := common.RouterConfig{ProcessingPools: pools}
	for _, q := range queues {
		cfg.Queues = append(cfg.Queues, fakeQueueCfg(q))
	}
	return cfg
}

func newTestManager(t *testing.T, med Mediator, tracker *InFlightTracker) *Manager {
	t.Helper()
	m := NewManager(med, tracker)
	m.restartDelay = 10 * time.Millisecond
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = m.Shutdown(ctx)
	})
	return m
}

// polled reports whether the queue has been polled at least atLeast times
// within the window. An idle poll loop sleeps a second between empty polls, so
// tests assert "it is polling at all" rather than a rate.
func polled(t *testing.T, q *fakeQueue, atLeast int64, within time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if q.polls.Load() >= atLeast {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// --- hot reload ---

// Reconfigure is the whole of hot reload, and it had no test at all. Pools and
// consumers must both reconcile: added, updated in place, and removed.
func TestReconfigureReconcilesPoolsAndConsumers(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	ctx := context.Background()

	rate := uint32(600)
	require.NoError(t, m.Reconfigure(ctx, routerCfg(
		[]string{"q-a"},
		common.PoolConfig{Code: "FAST", Concurrency: 4, RateLimitPerMinute: &rate},
	)))

	assert.NotNil(t, m.Pool("FAST"))
	assert.NotNil(t, m.Pool(defaultPoolCode), "DEFAULT-POOL is always ensured")
	assert.Equal(t, uint32(4), m.Pool("FAST").Concurrency())
	qa := fakeQueueFor(t, "q-a")
	require.True(t, polled(t, qa, 1, time.Second), "a configured queue must be polled")

	// Re-cap FAST in place, drop it... no: update it, add SLOW, swap the queue.
	require.NoError(t, m.Reconfigure(ctx, routerCfg(
		[]string{"q-b"},
		common.PoolConfig{Code: "FAST", Concurrency: 9},
		common.PoolConfig{Code: "SLOW", Concurrency: 1},
	)))

	assert.Equal(t, uint32(9), m.Pool("FAST").Concurrency(), "an existing pool is re-capped, not rebuilt")
	assert.NotNil(t, m.Pool("SLOW"), "a new pool starts")
	assert.True(t, qa.stopped.Load(), "a queue dropped from config must have its consumer stopped")
	qb := fakeQueueFor(t, "q-b")
	assert.True(t, polled(t, qb, 1, time.Second), "the new queue must be polled")

	// Removing a pool stops it.
	require.NoError(t, m.Reconfigure(ctx, routerCfg([]string{"q-b"},
		common.PoolConfig{Code: "FAST", Concurrency: 9})))
	assert.Nil(t, m.Pool("SLOW"), "a pool dropped from config is removed")
}

// TestConsumersOutliveTheReconfigureContext pins the lifetime rule. Callers
// hand Reconfigure whatever context they have — a config-sync fetch context, or
// (as the embedded default-broker path did) a 10s bootstrap context with a
// deferred cancel. Inheriting consumer lifetime from it killed every poll loop
// the moment that context expired, and only the stall watchdog brought them
// back.
func TestConsumersOutliveTheReconfigureContext(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())

	shortCtx, cancel := context.WithCancel(context.Background())
	require.NoError(t, m.Reconfigure(shortCtx, routerCfg([]string{"q-boot"})))
	cancel() // the caller's context is done the moment it returns

	q := fakeQueueFor(t, "q-boot")
	require.True(t, polled(t, q, 1, 2*time.Second),
		"the consumer must keep polling after the caller's context is cancelled")
	assert.False(t, q.stopped.Load())
}

// --- draining ---

// The property the shutdown sequence depends on: stopping intake must not
// abort work already in the pipeline. Cancelling the consumers instead (which
// is what a run-context cancellation used to do) killed the in-flight delivery
// the drain was about to wait for, so the drain sat out its whole timeout on
// work nothing was going to finish.
func TestStopPollingEndsIntakeButLetsInFlightWorkFinish(t *testing.T) {
	med := newBlockingMediator(1)
	tracker := NewInFlightTracker()
	m := newTestManager(t, med, tracker)
	require.NoError(t, m.Reconfigure(context.Background(), routerCfg([]string{"q-drain"})))

	q := fakeQueueFor(t, "q-drain")
	q.enqueue(common.QueuedMessage{
		Message:         common.Message{ID: "m1", MediationTarget: "http://t/x"},
		ReceiptHandle:   "rh-m1",
		BrokerMessageID: "bk-m1",
		QueueIdentifier: "q-drain",
	})
	med.awaitEntered(t, 1) // the delivery is under way

	m.StopPolling()
	require.Equal(t, 1, tracker.Count(), "the in-flight message is still owned")
	polls := q.polls.Load()
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, polls, q.polls.Load(), "intake must have stopped")

	// The delivery completes on its own and the drain then reaches zero.
	close(med.release)
	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, drain(drainCtx, tracker), "the drain must be able to reach zero")
	assert.Equal(t, int64(1), q.acks.Load(), "the in-flight delivery ran to completion and acked")
}

// --- the consumer-restart watchdog ---

// A poll loop wedged inside Poll leaves lastPoll stale; the watchdog rebuilds
// the consumer. Neither the detection nor the escalation had a test.
func TestRestartStalledConsumersRebuildsAndEscalates(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	require.NoError(t, m.Reconfigure(context.Background(), routerCfg([]string{"q-stall"})))
	ws := NewWarningService(DefaultWarningServiceConfig())
	m.SetWarnings(ws)

	original := fakeQueueFor(t, "q-stall")
	require.True(t, polled(t, original, 1, time.Second))

	// Nothing is stalled yet: a threshold longer than this consumer's idle time
	// must not restart it.
	assert.Equal(t, 0, m.RestartStalledConsumers(context.Background(), time.Hour))

	// Any positive threshold shorter than the age of the last poll is a stall.
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 1, m.RestartStalledConsumers(context.Background(), 10*time.Millisecond))
	assert.True(t, original.stopped.Load(), "the wedged consumer is torn down")
	rebuilt := fakeQueueFor(t, "q-stall")
	assert.NotSame(t, original, rebuilt, "a fresh consumer replaces it")
	assert.True(t, polled(t, rebuilt, 1, time.Second), "the replacement polls")

	// Repeated restarts escalate to CRITICAL.
	for range consumerRestartCriticalAfter {
		time.Sleep(15 * time.Millisecond)
		m.RestartStalledConsumers(context.Background(), 10*time.Millisecond)
	}
	assert.NotEmpty(t, ws.BySeverity(WarningCritical),
		"a consumer that keeps stalling must escalate to CRITICAL")
}

// A drain stops every poll loop on purpose, which makes every lastPoll stale.
// The watchdog must not read that as a fleet-wide stall and re-open intake
// behind the drain's back.
func TestRestartSkippedWhileDraining(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	require.NoError(t, m.Reconfigure(context.Background(), routerCfg([]string{"q-quiet"})))
	q := fakeQueueFor(t, "q-quiet")
	require.True(t, polled(t, q, 1, time.Second))

	m.StopPolling()
	time.Sleep(20 * time.Millisecond)

	assert.Equal(t, 0, m.RestartStalledConsumers(context.Background(), 10*time.Millisecond),
		"no consumer may be respawned once polling has been stopped for a drain")
	assert.False(t, q.stopped.Load(), "and the existing consumer is left alone to finish acking")
}

// --- per-queue backpressure ---

// Poll-level backpressure used to ask whether ANY pool had room, so one idle
// pool kept every consumer polling into a saturated one. A queue is now judged
// by the pools its own traffic feeds.
func TestBackpressureFollowsTheQueuesOwnPools(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, nil)
	require.NoError(t, m.Reconfigure(context.Background(), routerCfg(nil,
		common.PoolConfig{Code: "BUSY", Concurrency: 1},
		common.PoolConfig{Code: "IDLE", Concurrency: 1},
	)))
	busy := m.Pool("BUSY")
	require.NotNil(t, busy)

	rc := &runningConsumer{}
	assert.True(t, m.hasCapacityFor(rc), "with nothing routed yet, any pool with room admits the first batch")

	rc.setPools([]string{"BUSY"})
	assert.True(t, m.hasCapacityFor(rc))

	// Fill BUSY's pre-dispatch buffer.
	busy.queueSize.Store(busy.queueCapacity())
	assert.False(t, m.hasCapacityFor(rc),
		"a queue feeding a saturated pool must pause even though IDLE has room")

	rc.setPools([]string{"IDLE"})
	assert.True(t, m.hasCapacityFor(rc),
		"a queue feeding an idle pool must keep flowing even though BUSY is saturated")

	// A pool that disappears under a reconfigure falls back rather than wedging.
	rc.setPools([]string{"GONE"})
	assert.True(t, m.hasCapacityFor(rc))
}

// --- drain semantics ---

func TestDrainWaitsForZeroThenReturns(t *testing.T) {
	tracker := NewInFlightTracker()
	im := common.NewInFlightMessage(&common.Message{ID: "m1"}, "b1", "q", "batch", "rh")
	tracker.Register(im)

	go func() {
		time.Sleep(50 * time.Millisecond)
		tracker.Remove("m1", "b1")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, drain(ctx, tracker))
	assert.Equal(t, 0, tracker.Count())
}

func TestDrainReturnsImmediatelyWhenEmptyAndErrsOnTimeout(t *testing.T) {
	tracker := NewInFlightTracker()
	ctx := context.Background()
	require.NoError(t, drain(ctx, tracker), "an empty tracker drains at once")

	tracker.Register(common.NewInFlightMessage(&common.Message{ID: "stuck"}, "b", "q", "", "rh"))
	timeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	err := drain(timeout, tracker)
	require.ErrorIs(t, err, context.DeadlineExceeded,
		"a drain that cannot finish must report it rather than hang")
}

// Shutdown is not terminal — in standby mode it runs on leadership loss and a
// regain reconfigures the same Manager. A consumer root that stayed cancelled
// would make every later consumer die on arrival.
func TestManagerIsReusableAfterShutdown(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	ctx := context.Background()
	require.NoError(t, m.Reconfigure(ctx, routerCfg([]string{"q-ha"})))
	require.True(t, polled(t, fakeQueueFor(t, "q-ha"), 1, time.Second))

	shutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	require.NoError(t, m.Shutdown(shutCtx))
	assert.Equal(t, 0, m.PoolCount())

	// Leadership regained.
	require.NoError(t, m.Reconfigure(ctx, routerCfg([]string{"q-ha"})))
	require.True(t, polled(t, fakeQueueFor(t, "q-ha"), 1, 2*time.Second),
		"consumers started after a shutdown must run under a live root context")
}
