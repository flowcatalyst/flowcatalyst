package router

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// TestConsumerParksOnFullPoolAndResumesEventDriven pins G12(a): the owner's
// 2026-09-07 ruling that a consumer whose only destination pool is at
// capacity must park UNTIMED on a signal the pool raises when it crosses
// back under capacity, not sleep(2s) and retry.
//
// Reproduces the measured defect directly: saturating the pool's
// pre-dispatch buffer (queueInc is the exact increment every real admission
// goes through — see Pool.submit/enqueue) and observing the poll loop via
// the fake queue's poll counter, the same observable a NATS JetStream
// consumer's fetch rate would show in production.
func TestConsumerParksOnFullPoolAndResumesEventDriven(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	ws := NewWarningService(DefaultWarningServiceConfig())
	m.SetWarnings(ws)
	require.NoError(t, m.Reconfigure(context.Background(), routerCfg([]string{"q-cap"})))
	q := fakeQueueFor(t, "q-cap")
	require.True(t, polled(t, q, 1, time.Second), "must be polling before the test begins")

	pool := m.Pool(defaultPoolCode)
	require.NotNil(t, pool)
	capacity := int(pool.queueCapacity())

	pollsAtFull := q.polls.Load()
	for range capacity {
		pool.queueInc()
	}

	// Wait for the loop to actually OBSERVE the full pool (the
	// once-per-transition PoolCapacity warning this fix keeps), so the
	// resume-timing measurement below starts once the loop is genuinely
	// parked, not mid-way through its own up-to-1s idle-poll sleep.
	require.Eventually(t, func() bool { return len(ws.ByCategory(WarningCategoryPoolCapacity)) >= 1 },
		2*time.Second, 5*time.Millisecond, "the loop must observe and warn about the full pool")
	time.Sleep(30 * time.Millisecond) // let it finish parking in awaitCapacity

	// Load-bearing assertion #1: while genuinely full, the queue must not
	// be polled again for a good while — this alone would also pass
	// against the old 2s sleep (2s > 300ms), so it does not by itself prove
	// event-driven waiting; assertion #2 below is what the mutant fails.
	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, pollsAtFull, q.polls.Load(),
		"a queue whose only destination pool is at capacity must not be polled")

	start := time.Now()
	for range capacity {
		pool.queueDec()
	}
	require.Eventually(t, func() bool { return q.polls.Load() > pollsAtFull },
		time.Second, time.Millisecond,
		"the consumer must resume polling once its pool signals capacity")
	elapsed := time.Since(start)

	// Load-bearing assertion #2 — the resume-timing pin: resumption must be
	// event-driven (near-instant off the pool's own signal), not bounded
	// below by a fixed sleep. Mutant: put runConsumer's old
	// time.After(2*time.Second) back in place of awaitCapacity — this
	// assertion fails because the resume can then take up to ~2s.
	assert.Less(t, elapsed, 100*time.Millisecond,
		"resuming must be event-driven, not wait out a fixed sleep")
}

// TestPartialBatchRepollsImmediately pins G12(b): a partial batch (fewer
// than maxPoll messages) used to pause 500ms on the theory that the queue
// was draining. The ruling removes that pause outright — a partial batch is
// routine on a fast broker and says nothing about the queue being empty.
func TestPartialBatchRepollsImmediately(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	require.NoError(t, m.Reconfigure(context.Background(), routerCfg([]string{"q-partial"})))
	q := fakeQueueFor(t, "q-partial")
	require.True(t, polled(t, q, 1, time.Second), "must be polling before the test begins")

	q.enqueue(common.QueuedMessage{
		Message:         common.Message{ID: "m1", MediationTarget: "http://t/x"},
		ReceiptHandle:   "rh-m1",
		BrokerMessageID: "bk-m1",
		QueueIdentifier: "q-partial",
	})

	// The batch (size 1, well under maxPoll=10) gets polled and delivered.
	require.Eventually(t, func() bool { return q.acks.Load() == 1 },
		time.Second, time.Millisecond, "the partial batch must be delivered and acked")

	// Find the poll that actually carried the batch (size 1) in the log,
	// rather than comparing counters taken after some delay: by the time a
	// test goroutine notices the ack, an immediate re-poll may already have
	// happened, and diffing live counters at that point would measure the
	// gap to the poll AFTER that one — which, being an empty poll, sits
	// behind the untouched 1s idle pace and would flake this assertion for
	// a reason that has nothing to do with G12.
	log := q.pollLogSnapshot()
	batchIdx := -1
	for i, r := range log {
		if r.size > 0 {
			batchIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, batchIdx, 0, "the batch poll must appear in the log")

	// Give the re-poll a generous window to be logged — the timing pin
	// below is the 100ms gap check, not this wait.
	require.Eventually(t, func() bool { return len(q.pollLogSnapshot()) > batchIdx+1 },
		time.Second, time.Millisecond, "a re-poll must follow the batch poll")
	log = q.pollLogSnapshot()

	gap := log[batchIdx+1].at.Sub(log[batchIdx].at)
	// Load-bearing timing assertion: the re-poll immediately following a
	// partial batch must land within 100ms of it — fails against the old
	// 500ms pause (mutant: put the
	// `if len(msgs) < maxPoll { time.Sleep(500ms) }` branch back).
	assert.Less(t, gap, 100*time.Millisecond,
		"a partial batch must be followed by an immediate re-poll, not a 500ms pause")
}

// TestAwaitCapacityReturnsPromptlyOnCancel pins G12's context-cancellation
// requirement directly against the new primitive: a poll loop parked
// untimed in awaitCapacity must still exit promptly when its context is
// cancelled (consumer restart, reconfigure detach, shutdown) rather than
// being stranded on a signal that may never come once nothing is left to
// raise it.
func TestAwaitCapacityReturnsPromptlyOnCancel(t *testing.T) {
	m := NewManager(&grMediator{outcome: common.Success(http.StatusOK)}, nil)
	pool := NewPool(common.PoolConfig{Code: defaultPoolCode, Concurrency: 1}, m.mediator, nil,
		func(string) queue.Consumer { return nil })
	pool.SetCapacityFreed(m.capacityGate.signal)
	m.pools[defaultPoolCode] = pool

	// Saturate: nothing has capacity, so awaitCapacity has no choice but to
	// park.
	capacity := int(pool.queueCapacity())
	for range capacity {
		pool.queueInc()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() { done <- m.awaitCapacity(ctx, &runningConsumer{}) }()

	time.Sleep(30 * time.Millisecond) // let the goroutine actually park
	start := time.Now()
	cancel()

	select {
	case ok := <-done:
		assert.False(t, ok, "cancellation must report false, not a spurious capacity grant")
		assert.Less(t, time.Since(start), 500*time.Millisecond,
			"must return promptly on context cancellation, not wait out a timed sleep")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("awaitCapacity did not return within 500ms of context cancellation")
	}
}
