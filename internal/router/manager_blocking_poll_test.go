package router

import (
	"context"
	"errors"
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

// blockingFakeScheme registers a fake backend whose Poll blocks by
// contract — like the real NATS backend — instead of returning quickly.
// It exists to test the manager's poll loop and stall detector against a
// backend that legitimately parks in Poll waiting for messages, which
// fakeQueue (manager_lifecycle_test.go) never does.
const blockingFakeScheme = "blockingfaketest"

// blockingFakeQueue's Poll blocks until ctx is done, then follows the same
// contract the NATS backend documents (G13, nats.go): a lapsed DEADLINE is
// reported as (nil, nil) — a plain empty result — while a real Cancel
// still returns a non-nil error. This is what "blocks by contract" means:
// nothing about a long-parked Poll is wrong on its own, so it must not be
// indistinguishable from an error.
type blockingFakeQueue struct {
	name    string
	polls   atomic.Int64
	stopped atomic.Bool
}

var blockingFakeQueues sync.Map // queue name -> *blockingFakeQueue

func init() {
	queue.RegisterConsumer(blockingFakeScheme, func(_ context.Context, cfg common.QueueConfig) (queue.Consumer, error) {
		q := &blockingFakeQueue{name: cfg.Name}
		blockingFakeQueues.Store(cfg.Name, q)
		return q, nil
	})
}

func blockingFakeQueueFor(t *testing.T, name string) *blockingFakeQueue {
	t.Helper()
	v, ok := blockingFakeQueues.Load(name)
	require.Truef(t, ok, "no blocking fake queue built for %q", name)
	return v.(*blockingFakeQueue)
}

func (q *blockingFakeQueue) Identifier() string { return q.name }

func (q *blockingFakeQueue) Poll(ctx context.Context, _ uint32) ([]common.QueuedMessage, error) {
	if q.stopped.Load() {
		return nil, queue.ErrStopped
	}
	q.polls.Add(1)
	<-ctx.Done()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, nil
	}
	return nil, ctx.Err()
}

func (q *blockingFakeQueue) Ack(context.Context, string, string) error    { return nil }
func (q *blockingFakeQueue) Nack(context.Context, string, *uint32) error  { return nil }
func (q *blockingFakeQueue) Defer(context.Context, string, *uint32) error { return nil }
func (q *blockingFakeQueue) Healthy() bool                                { return !q.stopped.Load() }
func (q *blockingFakeQueue) Stop()                                        { q.stopped.Store(true) }
func (q *blockingFakeQueue) Metrics(context.Context) (*queue.Metrics, error) {
	return &queue.Metrics{QueueIdentifier: q.name}, nil
}
func (q *blockingFakeQueue) Counters() *queue.Metrics { return nil }

// TestBlockingPollIsNotFlaggedStalled pins G13 end to end through the real
// manager poll loop (not just the nats package in isolation): a consumer
// whose Poll blocks by contract, legitimately waiting for messages that
// never arrive, must never be flagged stalled or restarted — no matter
// how long it has been parked — as long as it keeps returning within the
// manager's own pollTimeout.
//
// pollTimeout is set far shorter than production (20ms) so the consumer
// completes many poll cycles inside the test's real wall-clock budget;
// the stall threshold (150ms) is comfortably longer than that, so a
// correctly-behaving consumer never approaches it, while the OLD
// behaviour (pollCtx's deadline treated as an error, lastPoll never
// refreshed on an empty/errored poll) would have gone stale within the
// first couple of cycles and been restarted well before the 300ms wait
// below elapses.
func TestBlockingPollIsNotFlaggedStalled(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	m.pollTimeout = 20 * time.Millisecond

	cfg := common.RouterConfig{Queues: []common.QueueConfig{
		{Name: "q-blocking", URI: blockingFakeScheme + "://q-blocking"},
	}}
	require.NoError(t, m.Reconfigure(context.Background(), cfg))

	q := blockingFakeQueueFor(t, "q-blocking")

	// Timing pin for "no sleep in the hot path" (G13 / point 1): 20 polls
	// at pollTimeout=20ms should take on the order of 400ms if each cycle
	// is bounded only by pollTimeout, versus 20+ SECONDS if the manager
	// still adds its 1s anti-hot-loop pause on top of a call that already
	// spent the full pollTimeout blocking. The bound below (3s) sits
	// comfortably above realistic scheduling jitter for 20 cycles but
	// hopelessly below what a single added 1s-per-cycle sleep would cost.
	start := time.Now()
	require.Eventually(t, func() bool { return q.polls.Load() >= 20 }, 3*time.Second, 5*time.Millisecond,
		"the blocking consumer must be polled at close to the pollTimeout cadence, not with an added sleep per cycle")
	t.Logf("20 poll cycles at pollTimeout=20ms took %s", time.Since(start))

	// Let real time pass well beyond the stall threshold below — many
	// pollTimeout cycles' worth — before checking.
	time.Sleep(300 * time.Millisecond)

	pollsBefore := q.polls.Load()
	restarted := m.RestartStalledConsumers(context.Background(), 150*time.Millisecond)
	assert.Equal(t, 0, restarted,
		"a Poll that blocks by contract, waiting for messages, must never be flagged stalled")
	assert.False(t, q.stopped.Load(), "an un-flagged consumer must not have been torn down")
	assert.Eventually(t, func() bool { return q.polls.Load() > pollsBefore }, time.Second, 5*time.Millisecond,
		"the loop must still be actively re-polling after the check, not wedged")
}
