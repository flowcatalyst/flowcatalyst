package router

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// primedPool builds a pool at concurrency 1 whose metrics say it completes
// one delivery every perMessage — the rate admissionDelay derives its slots
// from — and whose buffer holds queued messages.
func primedPool(t *testing.T, perMessage time.Duration, completions int, queued uint32) *Pool {
	t.Helper()
	p := NewPool(common.PoolConfig{Code: "SLOW", Concurrency: 1}, nil, nil,
		func(string) queue.Consumer { return nil })
	// Backdate the samples so the span the rate is measured over is
	// completions*perMessage, not "all within the last microsecond".
	p.metrics.mu.Lock()
	now := time.Now()
	for i := completions; i > 0; i-- {
		p.metrics.samples = append(p.metrics.samples,
			metricSample{ts: now.Add(-time.Duration(i) * perMessage), durationMs: 1, success: true})
	}
	p.metrics.mu.Unlock()
	p.queueSize.Store(queued)
	return p
}

// The schedule is a reservation: each deferral is booked one slot after the
// previous one, starting after the buffer's own drain time, so a run of
// deferred messages comes back spaced at the pool's pace and in order.
func TestAdmissionDelayReservesConsecutiveSlots(t *testing.T) {
	const perMessage = 5 * time.Second
	p := primedPool(t, perMessage, 12, 100) // 100 buffered at 0.2/s → 500s to drain
	now := time.Now()

	first := p.admissionDelay(now, true)
	// Buffer drain (500s) + one slot (5s). The measured rate spans
	// completions*perMessage-ish, so allow a slot of slack either side.
	assert.InDelta(t, (505 * time.Second).Seconds(), first.Seconds(), perMessage.Seconds(),
		"the first reservation is one slot after the buffer's drain time")

	prev := first
	for i := range 5 {
		next := p.admissionDelay(now, true)
		assert.InDelta(t, perMessage.Seconds(), (next - prev).Seconds(), 1.0,
			"reservation %d must be one slot (%s) after the previous one", i+2, perMessage)
		prev = next
	}
	assert.Equal(t, uint64(6), p.Deferred())
}

// A reservation is never shorter than deferralMinDelay: with an empty
// buffer and a fast pool the arithmetic says "now", and "now" is a hot loop
// against the broker.
func TestAdmissionDelayFloorsAtMinimum(t *testing.T) {
	p := primedPool(t, 10*time.Millisecond, 50, 0)
	got := p.admissionDelay(time.Now(), true)
	assert.Equal(t, deferralMinDelay, got)
}

// With no completion in the rate window there is nothing to derive a slot
// from; the fallback is a fixed wait, spaced a second apart so a burst of
// fallbacks does not all land at once.
func TestAdmissionDelayFallsBackWithoutARate(t *testing.T) {
	p := NewPool(common.PoolConfig{Code: "NEW", Concurrency: 1}, nil, nil,
		func(string) queue.Consumer { return nil })
	now := time.Now()
	a := p.admissionDelay(now, true)
	b := p.admissionDelay(now, true)
	assert.Equal(t, deferralFallbackWait+deferralOrderedSpacing, a)
	assert.Equal(t, a+deferralOrderedSpacing, b)
}

// Past the horizon the reservation is clamped — and jittered back over the
// last quarter of the horizon, never forward past it, so a large backlog's
// tail trickles in rather than arriving as one wave.
func TestAdmissionDelayClampsToHorizonWithBackwardJitter(t *testing.T) {
	p := primedPool(t, 5*time.Second, 12, 100)
	const horizon = 10 * time.Minute
	p.SetMaxDeferral(horizon)
	now := time.Now()
	// Push the cursor well past the horizon.
	for range 200 {
		p.admissionDelay(now, true)
	}
	for range 50 {
		d := p.admissionDelay(now, true)
		assert.LessOrEqual(t, d, horizon, "never past the horizon")
		assert.GreaterOrEqual(t, d, time.Duration(float64(horizon)*(1-deferralJitterFraction)),
			"jitter only pulls a clamped reservation BACK, within the jitter fraction")
	}
}

// On a broker that does not keep a deferred group in order (NATS), two
// reservations a few milliseconds apart could come back swapped, so the
// slot spacing is floored at a second there — and only there.
func TestAdmissionDelaySpacesReservationsOnAnUnorderedBroker(t *testing.T) {
	p := primedPool(t, 10*time.Millisecond, 500, 0) // 100/s: natural slot = 10ms
	now := time.Now()

	// Ordered broker: slots are the natural 10ms, so a run of reservations
	// stays within the minimum-delay floor.
	a := p.admissionDelay(now, true)
	b := p.admissionDelay(now, true)
	assert.Equal(t, a, b, "at 10ms slots both land on the %s floor", deferralMinDelay)

	// Unordered broker: each reservation is at least a second after the last.
	q := primedPool(t, 10*time.Millisecond, 500, 0)
	prev := q.admissionDelay(now, false)
	for range 10 {
		prev2 := q.admissionDelay(now, false)
		if prev2 > deferralMinDelay { // once past the floor the spacing shows
			assert.GreaterOrEqual(t, prev2-prev, deferralOrderedSpacing)
		}
		prev = prev2
	}
	assert.Greater(t, prev, deferralMinDelay+5*deferralOrderedSpacing,
		"eleven reservations on an unordered broker must have spread past the floor")
}

// CompletionRate measures over the span the completions actually cover,
// not the whole window — the rate of a pool that only just started.
func TestCompletionRateUsesTheSpanOfTheSamples(t *testing.T) {
	c := NewPoolMetricsCollector()
	c.mu.Lock()
	now := time.Now()
	for i := 10; i > 0; i-- {
		c.samples = append(c.samples, metricSample{ts: now.Add(-time.Duration(i) * time.Second)})
	}
	c.mu.Unlock()
	rate, ok := c.CompletionRate(5 * time.Minute)
	require.True(t, ok)
	assert.InDelta(t, 1.0, rate, 0.15, "ten completions over ten seconds is one per second, not ten per five minutes")

	_, ok = c.CompletionRate(500 * time.Millisecond)
	assert.False(t, ok, "no completion inside the window → no rate")
}

func TestDeferralLedgerCountsWhatIsStillOut(t *testing.T) {
	var l deferralLedger
	now := time.Now()
	_, ok := l.earliest()
	assert.False(t, ok)

	l.add(now.Add(3 * time.Second))
	l.add(now.Add(-time.Second)) // already due
	l.add(now.Add(time.Second))
	at, ok := l.earliest()
	require.True(t, ok)
	assert.Equal(t, now.Add(-time.Second).UnixNano(), at.UnixNano(), "earliest is the minimum, whatever the insertion order")

	assert.Equal(t, 2, l.outstanding(now), "the one already due is pruned")
	assert.Equal(t, 1, l.outstanding(now.Add(2*time.Second)))
	assert.Equal(t, 0, l.outstanding(now.Add(time.Minute)))
	_, ok = l.earliest()
	assert.False(t, ok)
}

// A poll loop parked with its budget spent gets budget back when its
// earliest deferral comes due, and nothing else signals that — so
// awaitCapacity must wake on the ledger's own clock.
func TestAwaitCapacityWakesWhenADeferralComesDue(t *testing.T) {
	m := NewManager(&grMediator{outcome: common.Success(http.StatusOK)}, nil)
	pool := NewPool(common.PoolConfig{Code: defaultPoolCode, Concurrency: 1}, m.mediator, nil,
		func(string) queue.Consumer { return nil })
	pool.SetCapacityFreed(m.capacityGate.signal)
	m.pools[defaultPoolCode] = pool
	for range int(pool.queueCapacity()) {
		pool.queueInc()
	}

	rc := &runningConsumer{}
	m.SetDeferralBudget(1)
	rc.deferrals.add(time.Now().Add(150 * time.Millisecond))
	require.False(t, m.hasCapacityFor(rc), "full pool, budget spent: parked")

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.True(t, m.awaitCapacity(ctx, rc), "must return true, not time out")
	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond, "it waited for the deferral to come due")
	assert.Less(t, elapsed, time.Second, "and woke on it, not on a slow fallback poll")
}

// The head-of-line block itself, end to end: one queue feeding a dedicated
// pool that is full and a sibling pool that is not. The consumer must keep
// polling — the full pool's messages go back to the broker with a scheduled
// delay, and the sibling's are delivered — where it used to park the queue
// until the full pool drained.
func TestFullPoolDefersInsteadOfBlockingTheQueue(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	require.NoError(t, m.Reconfigure(context.Background(), routerCfg([]string{"q-hol"},
		common.PoolConfig{Code: "SLOW", Concurrency: 1},
		common.PoolConfig{Code: "FAST", Concurrency: 4},
	)))
	q := fakeQueueFor(t, "q-hol")
	require.True(t, polled(t, q, 1, time.Second), "must be polling before the test begins")

	slow := m.Pool("SLOW")
	require.NotNil(t, slow)
	for range int(slow.queueCapacity()) {
		slow.queueInc() // SLOW is full; nothing drains it for the test's duration
	}

	// First a batch of nothing but the slow pool's traffic: after it the
	// consumer's known destinations are {SLOW}, which is full — the exact
	// state that used to park the queue. Then the fast pool's traffic
	// arrives behind it.
	var slowBatch []common.QueuedMessage
	for i := range 5 {
		slowBatch = append(slowBatch, common.QueuedMessage{
			Message:         common.Message{ID: fmt.Sprintf("slow-%d", i), PoolCode: "SLOW", MediationTarget: "http://t/slow"},
			ReceiptHandle:   fmt.Sprintf("rh-slow-%d", i),
			BrokerMessageID: fmt.Sprintf("bk-slow-%d", i),
			QueueIdentifier: "q-hol",
		})
	}
	q.enqueue(slowBatch...)
	require.Eventually(t, func() bool { return len(q.deferredSnapshot()) == 5 },
		2*time.Second, 5*time.Millisecond, "SLOW's five messages must be deferred")

	var fastBatch []common.QueuedMessage
	for i := range 3 {
		fastBatch = append(fastBatch, common.QueuedMessage{
			Message:         common.Message{ID: fmt.Sprintf("fast-%d", i), PoolCode: "FAST", MediationTarget: "http://t/fast"},
			ReceiptHandle:   fmt.Sprintf("rh-fast-%d", i),
			BrokerMessageID: fmt.Sprintf("bk-fast-%d", i),
			QueueIdentifier: "q-hol",
		})
	}
	q.enqueue(fastBatch...)

	require.Eventually(t, func() bool { return q.acks.Load() == 3 },
		3*time.Second, 5*time.Millisecond,
		"FAST's three messages must be delivered while SLOW is full — the consumer must not have parked on SLOW")

	deferred := q.deferredSnapshot()
	require.Len(t, deferred, 5, "SLOW's five messages must have been deferred, not nacked and not dropped")
	assert.Zero(t, q.nacks.Load(), "a full pool is a deferral, not a failure")
	for i, d := range deferred {
		assert.Equal(t, fmt.Sprintf("rh-slow-%d", i), d.receipt, "deferred in arrival order")
		assert.GreaterOrEqual(t, d.delaySeconds, uint32(deferralMinDelay/time.Second),
			"each deferral carries a scheduled delay, never the old flat 10s-or-nothing")
	}
	assert.Equal(t, uint64(5), slow.Deferred())
	assert.Zero(t, m.tracker.Count(), "deferred messages leave the pipeline: no tracker entries linger to dedup their redelivery")

	// The consumer went on polling throughout: the deferrals are on its ledger
	// and it is nowhere near its budget.
	m.consumerMu.RLock()
	rc := m.consumers["q-hol"]
	m.consumerMu.RUnlock()
	assert.Equal(t, 5, rc.deferrals.outstanding(time.Now()))
	assert.True(t, m.hasCapacityFor(rc))
}

// While a message's copy is deferred, a republished copy under a new broker
// id is ACKed (deleted) rather than deferred beside it — the "500 in flight,
// 200 pending" duplicate storm (2026-09-22).
func TestDeferredMessageRepublishedCopyIsDeletedNotDeferred(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	require.NoError(t, m.Reconfigure(context.Background(), routerCfg([]string{"q-dup"},
		common.PoolConfig{Code: "SLOW", Concurrency: 1},
		common.PoolConfig{Code: "FAST", Concurrency: 4},
	)))
	q := fakeQueueFor(t, "q-dup")
	require.True(t, polled(t, q, 1, time.Second))
	slow := m.Pool("SLOW")
	for range int(slow.queueCapacity()) {
		slow.queueInc()
	}

	msg := func(broker, receipt string) common.QueuedMessage {
		return common.QueuedMessage{
			Message:         common.Message{ID: "job-dup", PoolCode: "SLOW", MediationTarget: "http://t/slow"},
			ReceiptHandle:   receipt,
			BrokerMessageID: broker,
			QueueIdentifier: "q-dup",
		}
	}
	q.enqueue(msg("sqs-1", "rh-1"))
	require.Eventually(t, func() bool { return len(q.deferredSnapshot()) == 1 },
		2*time.Second, 5*time.Millisecond, "the first copy is deferred")
	assert.Equal(t, 1, m.tracker.DeferredCount())

	q.enqueue(msg("sqs-2", "rh-2"))
	require.Eventually(t, func() bool { return q.acks.Load() == 1 },
		2*time.Second, 5*time.Millisecond, "the republished copy is deleted from the broker")
	assert.Len(t, q.deferredSnapshot(), 1, "and NOT deferred beside the first")
	assert.Equal(t, 1, m.tracker.DeferredCount())
}
