package router

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// TestRestartStalledConsumer_DoesNotAbortInFlight pins [R-26 / R-49] for
// RestartStalledConsumers, same as TestChangedQueueDoesNotAbortInFlight pins
// it for Reconfigure: a restart of a consumer whose POLL loop is stalled
// must never abort an in-flight delivery. "Stalled" says nothing about
// in-flight work — that was precisely the failure mode of the pre-fix
// watchdog in the 2026-09-01 incident, where perfectly healthy deliveries
// were killed on every restart-every-90s cycle alongside the one wedged poll
// loop. The old consumer detaches (stopPoll only; workCtx untouched) via the
// SAME machinery Reconfigure's consumer-removal/change path uses.
func TestRestartStalledConsumer_DoesNotAbortInFlight(t *testing.T) {
	gate := make(chan struct{})
	med := &gatedMediator{gateID: "m1", gate: gate}
	tracker := NewInFlightTracker()
	m := newTestManager(t, med, tracker)
	m.restartDelay = 0

	require.NoError(t, m.Reconfigure(context.Background(), routerCfg([]string{"q-wedge"})))
	q := fakeQueueFor(t, "q-wedge")
	q.enqueue(common.QueuedMessage{
		Message:         common.Message{ID: "m1", MediationTarget: "http://t/x"},
		ReceiptHandle:   "rh-m1",
		BrokerMessageID: "bk-m1",
		QueueIdentifier: "q-wedge",
	})

	pool := m.Pool(defaultPoolCode)
	require.NotNil(t, pool)
	require.Eventually(t, func() bool { return pool.ActiveWorkers() == 1 }, time.Second, 5*time.Millisecond,
		"m1 must be actively mediating (gated) before the watchdog fires")

	m.consumerMu.RLock()
	oldRC := m.consumers["q-wedge"]
	m.consumerMu.RUnlock()
	require.NotNil(t, oldRC)
	require.NoError(t, oldRC.workCtx.Err())

	// Force the watchdog to see q-wedge as stalled, independent of real poll
	// timing — the same trick TestRestartStalledConsumersRebuildsAndEscalates
	// uses via a threshold shorter than the consumer's actual idle time.
	oldRC.lastPoll.Store(time.Now().Add(-time.Hour).UnixNano())
	require.Equal(t, 1, m.RestartStalledConsumers(context.Background(), time.Minute))

	newQ := fakeQueueFor(t, "q-wedge")
	assert.NotSame(t, q, newQ, "the watchdog must rebuild under a fresh consumer")
	require.True(t, polled(t, newQ, 1, time.Second), "the replacement must start polling immediately")

	// The property under test: workCtx must survive the restart untouched,
	// and the outgoing consumer must not be torn down while it still carries
	// the in-flight delivery.
	assert.NoError(t, oldRC.workCtx.Err(),
		"RestartStalledConsumers must never cancel workCtx — a stalled POLL is not a dead in-flight worker")
	assert.False(t, q.stopped.Load(),
		"the outgoing consumer must not be torn down while it still carries an in-flight delivery")

	// Release the blocked delivery; it must run to completion (ack) rather
	// than being aborted by the restart.
	close(gate)
	require.Eventually(t, func() bool { return q.acks.Load()+newQ.acks.Load() == 1 }, time.Second, 5*time.Millisecond,
		"the in-flight delivery started before the restart must still complete and ack, not be lost")

	// Known scope limitation, same as TestChangedQueueDoesNotAbortInFlight:
	// the old and new consumer for a RESTARTED queue share the same
	// Identifier() (the queue name — RestartStalledConsumers rebuilds from
	// the identical QueueConfig), so resolveConsumer's active-map-first
	// lookup resolves this ack through the NEW (already-installed) consumer
	// object, not the physical instance that actually polled the message.
	// That is the documented, deliberate trade-off: the alternative
	// (detaching-first) would misroute every FRESH delivery on the
	// replacement consumer to the old, retiring one for as long as it
	// lingers. Harmless either way for what this watchdog exists to handle —
	// a wedged poll loop, not a dead ack path — and for a genuinely dead
	// broker connection an ack attempted through either object simply fails
	// and the message redelivers on the broker's own clock.
	assert.Equal(t, int64(1), newQ.acks.Load(), "acks through the active (new) consumer entry for the queue name")
	assert.Equal(t, int64(0), q.acks.Load())

	// Once nothing references q-wedge any more, the reaper-tick retirement
	// finishes detaching the old consumer for real.
	require.Eventually(t, func() bool { return tracker.CountForQueue("q-wedge") == 0 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, 1, m.retireDetachedConsumers())
	assert.True(t, q.stopped.Load(), "a fully-drained detached consumer is torn down by retireDetachedConsumers")
}

// TestRestartStalledConsumer_RetiresUnderContinuousReplacementTraffic pins
// the fix for the leak CountForQueue had: a detached consumer's replacement
// keeps polling and delivering FRESH messages under the exact same queue
// identifier, and plain CountForQueue can't tell that traffic apart from
// the pre-detach message the old consumer is actually waiting to see
// resolved — so it would never reach zero for a queue that stays busy, and
// the old consumer (and its broker connection) would never retire.
// CountForQueueBefore (cutoff = the consumer's own detachedAt) excludes the
// replacement's traffic categorically: every message it delivers has
// StartedAt >= detachedAt, since it cannot poll before it exists. This is
// the convergence proof: retirement must still happen while the replacement
// stays continuously busy, the moment (and only the moment) the OLD
// consumer's own pre-detach message resolves.
func TestRestartStalledConsumer_RetiresUnderContinuousReplacementTraffic(t *testing.T) {
	gate := make(chan struct{})
	med := &gatedMediator{gateID: "old1", gate: gate}
	tracker := NewInFlightTracker()
	m := newTestManager(t, med, tracker)
	m.restartDelay = 0

	require.NoError(t, m.Reconfigure(context.Background(), routerCfg([]string{"q-busy"})))
	q := fakeQueueFor(t, "q-busy")
	q.enqueue(common.QueuedMessage{
		Message:         common.Message{ID: "old1", MediationTarget: "http://t/x"},
		ReceiptHandle:   "rh-old1",
		BrokerMessageID: "bk-old1",
		QueueIdentifier: "q-busy",
	})

	pool := m.Pool(defaultPoolCode)
	require.NotNil(t, pool)
	require.Eventually(t, func() bool { return pool.ActiveWorkers() == 1 }, time.Second, 5*time.Millisecond,
		"old1 must be actively mediating (gated) before the watchdog fires")

	m.consumerMu.RLock()
	oldRC := m.consumers["q-busy"]
	m.consumerMu.RUnlock()
	require.NotNil(t, oldRC)

	oldRC.lastPoll.Store(time.Now().Add(-time.Hour).UnixNano())
	require.Equal(t, 1, m.RestartStalledConsumers(context.Background(), time.Minute))

	newQ := fakeQueueFor(t, "q-busy")
	assert.NotSame(t, q, newQ, "the watchdog must rebuild under a fresh consumer")
	require.True(t, polled(t, newQ, 1, time.Second), "the replacement must start polling immediately")

	// Keep the replacement continuously busy: new messages, arriving on the
	// SAME queue identifier as the detached consumer, for as long as old1
	// stays gated. Every one of these must have StartedAt >= oldRC.detachedAt.
	stopTraffic := make(chan struct{})
	var trafficWG sync.WaitGroup
	trafficWG.Add(1)
	go func() {
		defer trafficWG.Done()
		for i := 0; ; i++ {
			select {
			case <-stopTraffic:
				return
			default:
			}
			id := fmt.Sprintf("new%d", i)
			newQ.enqueue(common.QueuedMessage{
				Message:         common.Message{ID: id, MediationTarget: "http://t/x"},
				ReceiptHandle:   "rh-" + id,
				BrokerMessageID: "bk-" + id,
				QueueIdentifier: "q-busy",
			})
			// Paced well below the pool's capacity/throughput: fakeQueue.Poll
			// (unlike a real broker) ignores maxMessages and returns its ENTIRE
			// backlog in one batch, so a producer racing ahead of a
			// temporarily-idle poll loop (which sleeps up to 500ms between
			// partial batches) balloons into one huge batch that can still be
			// draining when t.Cleanup's Shutdown races it. Kept slow, the
			// backlog this generates never has anywhere to build up.
			time.Sleep(20 * time.Millisecond)
		}
	}()
	t.Cleanup(func() { close(stopTraffic); trafficWG.Wait() })

	// Give the replacement a moment to actually start delivering (and
	// acking) its own traffic before asserting anything about retirement.
	// Bounded generously (well past the poll loop's own up-to-1s backoff
	// after an empty poll — see runConsumer) rather than tightly, since all
	// this needs to prove is that flow eventually starts.
	require.Eventually(t, func() bool { return newQ.acks.Load() > 0 }, 3*time.Second, 5*time.Millisecond,
		"the replacement's continuous traffic must actually be flowing before this test means anything")

	// While old1 is still gated (unresolved), the detached consumer must
	// NOT retire — regardless of how much traffic the replacement is
	// handling under the same identifier.
	for i := 0; i < 20; i++ {
		require.Equal(t, 0, m.retireDetachedConsumers(),
			"must not retire while its own pre-detach message is still unresolved, even under continuous replacement traffic")
		time.Sleep(5 * time.Millisecond)
	}
	assert.False(t, q.stopped.Load())

	// Release old1; it completes and resolves. Per the documented,
	// accepted limitation shared with TestChangedQueueDoesNotAbortInFlight
	// and TestRestartStalledConsumer_DoesNotAbortInFlight, its ACK resolves
	// through the currently-active (replacement) consumer, not q itself —
	// resolveConsumer checks the active map first, and old1 shares q-busy's
	// identifier with the replacement. What this test pins is retirement,
	// which is judged by the tracker, not by which physical consumer object
	// happened to issue the ack.
	close(gate)
	require.Eventually(t, func() bool {
		return tracker.CountForQueueBefore("q-busy", oldRC.detachedAt) == 0
	}, time.Second, 5*time.Millisecond,
		"old1 must resolve (its pre-detach tracker entry must clear) once released")

	// Convergence: retirement now succeeds, even with the replacement still
	// busy — the replacement's traffic never held it up, and old1 resolving
	// is what actually clears it.
	require.Eventually(t, func() bool { return m.retireDetachedConsumers() > 0 }, 2*time.Second, 10*time.Millisecond,
		"the detached consumer must retire once its own pre-detach traffic resolves, even while the replacement stays continuously busy")
	assert.True(t, q.stopped.Load(), "a fully-drained detached consumer is torn down by retireDetachedConsumers")

	// The replacement itself must never have been touched by any of this.
	assert.False(t, newQ.stopped.Load())
}

func containsPool(pools []*Pool, target *Pool) bool {
	for _, p := range pools {
		if p == target {
			return true
		}
	}
	return false
}

// TestReconfigure_RemovedPoolDrainsThenStops pins [X-11 / R-26 / R-49]:
// removed-pool-drains-then-stops at the Manager/Reconfigure level (see
// TestPoolDrain_BufferedGroupStillDeliversThenPoolStops in pool_drain_test.go
// for the Pool-local version). A pool dropped from config leaves the
// routing map (Manager.Pool) immediately, but keeps draining its buffer in
// the background — visible via AllPools the whole time — and the group
// buffered on it at the moment of removal still gets delivered rather than
// flushed for broker redelivery.
func TestReconfigure_RemovedPoolDrainsThenStops(t *testing.T) {
	group := "g"
	gate := make(chan struct{})
	med := &gatedMediator{gateID: "m1", gate: gate}
	tracker := NewInFlightTracker()
	m := newTestManager(t, med, tracker)

	require.NoError(t, m.Reconfigure(context.Background(), routerCfg([]string{"q-pooldrop"},
		common.PoolConfig{Code: "DROPME", Concurrency: 2})))

	q := fakeQueueFor(t, "q-pooldrop")
	mk := func(id, rh string) common.QueuedMessage {
		return common.QueuedMessage{
			Message: common.Message{
				ID: id, PoolCode: "DROPME", MediationType: common.MediationTypeHTTP, MediationTarget: "http://t/x",
				MessageGroupID: &group, DispatchMode: common.DispatchNextOnError,
			},
			ReceiptHandle: rh, BrokerMessageID: "bk-" + id, QueueIdentifier: "q-pooldrop",
		}
	}
	q.enqueue(mk("m1", "rh-m1"), mk("m2", "rh-m2"))

	pool := m.Pool("DROPME")
	require.NotNil(t, pool)
	require.Eventually(t, func() bool { return pool.ActiveWorkers() == 1 }, time.Second, 5*time.Millisecond,
		"m1 must be actively mediating before the pool is removed")
	require.Eventually(t, func() bool { return pool.QueueSize() == 1 }, time.Second, 5*time.Millisecond,
		"m2 must be buffered behind m1 at the moment of removal")

	// Remove the pool from config; the queue stays, so only the pool-removal
	// path is exercised.
	require.NoError(t, m.Reconfigure(context.Background(), routerCfg([]string{"q-pooldrop"})))
	assert.Nil(t, m.Pool("DROPME"), "a removed pool must leave the routing map immediately")
	assert.True(t, containsPool(m.AllPools(), pool),
		"a draining pool must still appear in AllPools (blocked-groups / flush-suppression) until it finishes")

	// The buffered group must still be delivered, not flushed for
	// redelivery.
	close(gate)
	require.Eventually(t, func() bool { return q.acks.Load() == 2 }, time.Second, 5*time.Millisecond,
		"the removed pool's buffered group must still be delivered, not flushed")

	// Once drained, the pool is fully stopped and drops out of AllPools.
	require.Eventually(t, func() bool { return pool.stopped.Load() }, time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool { return !containsPool(m.AllPools(), pool) }, time.Second, 5*time.Millisecond,
		"a finished drain must remove the pool from AllPools")
}

// TestDetachedConsumerLingersForAckThenRetires pins [X-11 / R-26 / R-49]:
// removed-queue-acks-still-resolve. A queue dropped from config stops being
// polled immediately, but its consumer stays addressable via
// Manager.resolveConsumer's fallback (see the detaching field's doc
// comment) so a message already buffered on it at the moment of removal —
// mid-buffer, not yet acked — still resolves and ACKs through the lingering
// consumer instead of being silently skipped. Once nothing references the
// queue any more, retireDetachedConsumers tears it down for real.
func TestDetachedConsumerLingersForAckThenRetires(t *testing.T) {
	group := "g"
	gate := make(chan struct{})
	med := &gatedMediator{gateID: "m1", gate: gate}
	tracker := NewInFlightTracker()
	m := newTestManager(t, med, tracker)

	require.NoError(t, m.Reconfigure(context.Background(), routerCfg([]string{"q-linger"},
		common.PoolConfig{Code: defaultPoolCode, Concurrency: 2})))

	q := fakeQueueFor(t, "q-linger")
	mk := func(id, rh string) common.QueuedMessage {
		return common.QueuedMessage{
			Message: common.Message{
				ID: id, MediationType: common.MediationTypeHTTP, MediationTarget: "http://t/x",
				MessageGroupID: &group, DispatchMode: common.DispatchNextOnError,
			},
			ReceiptHandle: rh, BrokerMessageID: "bk-" + id, QueueIdentifier: "q-linger",
		}
	}
	q.enqueue(mk("m1", "rh-m1"), mk("m2", "rh-m2"))

	pool := m.Pool(defaultPoolCode)
	require.NotNil(t, pool)
	require.Eventually(t, func() bool { return pool.ActiveWorkers() == 1 }, time.Second, 5*time.Millisecond,
		"m1 must be actively mediating (gated) before the queue is removed")
	require.Eventually(t, func() bool { return pool.QueueSize() == 1 }, time.Second, 5*time.Millisecond,
		"m2 must be buffered behind m1, mid-buffer, at the moment of removal")

	// Remove the queue from config. The pool config is unchanged, so only the
	// consumer-detach path is exercised.
	require.NoError(t, m.Reconfigure(context.Background(), routerCfg(nil,
		common.PoolConfig{Code: defaultPoolCode, Concurrency: 2})))

	assert.False(t, q.stopped.Load(),
		"a removed queue's consumer must NOT be torn down while messages still reference it")
	polls := q.polls.Load()
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, polls, q.polls.Load(), "polling must stop immediately once the queue is removed")

	// Release m1; both m1 and m2 (mid-buffer at removal) must still resolve
	// and ACK through the lingering consumer.
	close(gate)
	require.Eventually(t, func() bool { return q.acks.Load() == 2 }, time.Second, 5*time.Millisecond,
		"both messages must still ACK through the lingering (detached) consumer")

	// Once nothing references q-linger any more, the reaper-tick retirement
	// finishes the teardown.
	require.Eventually(t, func() bool { return tracker.CountForQueue("q-linger") == 0 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, 1, m.retireDetachedConsumers())
	assert.True(t, q.stopped.Load(), "a fully-drained detached consumer is torn down by retireDetachedConsumers")
}

// TestChangedQueueDoesNotAbortInFlight pins [X-11 / R-26 / R-49]:
// changed-queue-does-not-abort-in-flight. A queue CONFIG CHANGE (same name,
// different connection/visibility settings) must rebuild the consumer —
// but must never cancel the OUTGOING consumer's workCtx, which would abort
// whatever delivery it is carrying mid-flight. The new consumer starts
// polling under the same name immediately; the old one lingers, still
// running its in-flight delivery to completion.
func TestChangedQueueDoesNotAbortInFlight(t *testing.T) {
	med := newBlockingMediator(1)
	tracker := NewInFlightTracker()
	m := newTestManager(t, med, tracker)

	require.NoError(t, m.Reconfigure(context.Background(), routerCfg([]string{"q-chg"})))
	q := fakeQueueFor(t, "q-chg")
	q.enqueue(common.QueuedMessage{
		Message:         common.Message{ID: "m1", MediationTarget: "http://t/x"},
		ReceiptHandle:   "rh-m1",
		BrokerMessageID: "bk-m1",
		QueueIdentifier: "q-chg",
	})
	med.awaitEntered(t, 1) // the delivery is under way, blocked inside Mediate

	m.consumerMu.RLock()
	oldRC := m.consumers["q-chg"]
	m.consumerMu.RUnlock()
	require.NotNil(t, oldRC)
	require.NoError(t, oldRC.workCtx.Err())

	// Change the queue's config — same name, different VisibilityTimeout —
	// which Reconfigure must treat as "changed" and rebuild.
	changedCfg := common.RouterConfig{Queues: []common.QueueConfig{
		{Name: "q-chg", URI: fakeScheme + "://q-chg", VisibilityTimeout: 999},
	}}
	require.NoError(t, m.Reconfigure(context.Background(), changedCfg))

	newQ := fakeQueueFor(t, "q-chg")
	assert.NotSame(t, q, newQ, "a changed queue config must be rebuilt under a fresh consumer")
	require.True(t, polled(t, newQ, 1, time.Second), "the replacement must start polling immediately")

	// The property under test: the OUTGOING consumer's workCtx must survive
	// the change untouched.
	assert.NoError(t, oldRC.workCtx.Err(),
		"a queue CONFIG CHANGE must never cancel workCtx — see runningConsumer's two-cancellation doc comment")
	assert.False(t, q.stopped.Load(),
		"the outgoing consumer must not be torn down while it still carries an in-flight delivery")

	// Release the blocked delivery; it must run to completion (ack) rather
	// than being aborted by the change.
	close(med.release)
	require.Eventually(t, func() bool { return q.acks.Load()+newQ.acks.Load() == 1 }, time.Second, 5*time.Millisecond,
		"the in-flight delivery started before the change must still complete and ack")
	// Known scope limitation, not a bug this test is pinning either way: a
	// CHANGED queue's old and new consumer share the SAME Identifier() (the
	// queue name), so resolveConsumer's active-map-first lookup resolves the
	// ack through the NEW consumer object rather than the physical one that
	// actually polled the message. That is only incorrect if the change
	// swapped to a genuinely different broker connection — out of scope
	// here (see the manager package's Reconfigure doc comment); what THIS
	// test pins is that the delivery is never aborted, which the assertions
	// above already establish.
	assert.Equal(t, int64(1), newQ.acks.Load(), "acks through the active (new) consumer entry for the queue name")
}
