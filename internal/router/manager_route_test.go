package router

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// TestManagerPoolForMessage verifies R2 routing resolution: a message routes
// to the pool named by its pool_code; an empty or unknown pool_code falls
// back to DEFAULT-POOL.
func TestManagerPoolForMessage(t *testing.T) {
	med := &cascadeMediator{}
	m := NewManager(med, nil)
	resolve := func(string) queue.Consumer { return nil }
	poolA := NewPool(common.PoolConfig{Code: "A"}, med, nil, resolve)
	poolDefault := NewPool(common.PoolConfig{Code: defaultPoolCode}, med, nil, resolve)
	m.pools["A"] = poolA
	m.pools[defaultPoolCode] = poolDefault

	mk := func(poolCode string) common.QueuedMessage {
		return common.QueuedMessage{Message: common.Message{ID: "x", PoolCode: poolCode}}
	}

	assert.Same(t, poolA, m.poolForMessage(mk("A")), "pool_code A → pool A")
	assert.Same(t, poolDefault, m.poolForMessage(mk("")), "empty pool_code → DEFAULT-POOL")
	assert.Same(t, poolDefault, m.poolForMessage(mk("NOPE")), "unknown pool_code → DEFAULT-POOL")
}

// TestManagerUnknownPoolRecordsRoutingWarning verifies the manager surfaces an
// unknown pool_code as a Routing warning (matching the Rust router), while the
// normal empty-pool_code default does not warn.
func TestManagerUnknownPoolRecordsRoutingWarning(t *testing.T) {
	med := &cascadeMediator{}
	m := NewManager(med, nil)
	ws := NewWarningService(DefaultWarningServiceConfig())
	m.SetWarnings(ws)
	resolve := func(string) queue.Consumer { return nil }
	m.pools[defaultPoolCode] = NewPool(common.PoolConfig{Code: defaultPoolCode}, med, nil, resolve)

	// Unknown (non-empty) pool_code → DEFAULT-POOL + exactly one Routing warning.
	if p := m.poolForMessage(common.QueuedMessage{Message: common.Message{ID: "x", PoolCode: "NOPE"}}); p == nil {
		t.Fatal("expected DEFAULT-POOL fallback for unknown pool_code")
	}
	if got := len(ws.ByCategory(WarningCategoryRouting)); got != 1 {
		t.Fatalf("unknown pool_code must record exactly one Routing warning; got %d", got)
	}

	// Empty pool_code is the normal default — it must NOT warn.
	m.poolForMessage(common.QueuedMessage{Message: common.Message{ID: "y", PoolCode: ""}})
	if got := len(ws.ByCategory(WarningCategoryRouting)); got != 1 {
		t.Fatalf("empty pool_code must not warn; routing warnings now %d", got)
	}
}

// TestInFlightTrackerRegisterOutcomes covers the dedup classification: the
// same app message id under a DIFFERENT broker id is an external requeue; the
// same broker id is a redelivery; blank broker ids dedupe by app id as
// redeliveries (Postgres-style queues); an unknown message is new.
func TestInFlightTrackerRegisterOutcomes(t *testing.T) {
	mk := func(id, broker, rh string) *common.InFlightMessage {
		return common.NewInFlightMessage(&common.Message{ID: id}, broker, "q", "b", rh)
	}
	tr := NewInFlightTracker()
	assert.Equal(t, RegisterNew, tr.Register(mk("app1", "broker1", "rh")))

	assert.Equal(t, RegisterExternalRequeue, tr.Register(mk("app1", "broker2", "rh2")), "different broker id → external requeue")
	assert.Equal(t, RegisterRedelivery, tr.Register(mk("app1", "broker1", "rh3")), "same broker id → redelivery")
	assert.Equal(t, RegisterRedelivery, tr.Register(mk("app1", "", "rh4")), "blank broker id → blank-id redelivery")
	assert.Equal(t, RegisterNew, tr.Register(mk("app2", "broker9", "rh5")), "unknown app id → new")
}

// TestManagerRouteExternalRequeueAcks verifies R4 end-to-end: route() ACK-drops
// a message whose app id is already in flight under a different broker id,
// rather than submitting it to a pool.
func TestManagerRouteExternalRequeueAcks(t *testing.T) {
	med := &cascadeMediator{}
	tr := NewInFlightTracker()
	m := NewManager(med, tr)
	m.pools[defaultPoolCode] = NewPool(common.PoolConfig{Code: defaultPoolCode}, med, tr, func(string) queue.Consumer { return nil })

	// The original is in flight under broker1.
	tr.Register(common.NewInFlightMessage(&common.Message{ID: "app1"}, "broker1", "q", "b", "rh-orig"))

	cons := &cascadeConsumer{wantTotal: 1, done: make(chan struct{})}
	requeued := common.QueuedMessage{
		Message:         common.Message{ID: "app1"},
		BrokerMessageID: "broker2",
		ReceiptHandle:   "rh-requeue",
		QueueIdentifier: "q",
	}
	m.route(context.Background(), []common.QueuedMessage{requeued}, cons)

	select {
	case <-cons.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the requeued duplicate to be ACKed")
	}

	cons.mu.Lock()
	acked := append([]string(nil), cons.acked...)
	cons.mu.Unlock()
	assert.Equal(t, []string{"rh-requeue"}, acked, "the external-requeue duplicate should be ACKed and not submitted")

	med.mu.Lock()
	seen := append([]string(nil), med.seen...)
	med.mu.Unlock()
	assert.Empty(t, seen, "the requeued duplicate must not be mediated")
}

// mkGrouped builds an ordered (BLOCK_ON_ERROR) queued message with explicit
// broker id + receipt handle, in group "g" on queue "q" — the shape the
// route-time dedup tests below feed through Manager.route.
func mkGrouped(id, brokerID, receipt string) common.QueuedMessage {
	group := "g"
	return common.QueuedMessage{
		Message: common.Message{
			ID:              id,
			MediationType:   common.MediationTypeHTTP,
			MediationTarget: "http://example.invalid",
			MessageGroupID:  &group,
			DispatchMode:    common.DispatchBlockOnError,
		},
		BrokerMessageID: brokerID,
		ReceiptHandle:   receipt,
		QueueIdentifier: "q",
	}
}

// bufferedCopies counts how many copies of the given app message id sit in a
// group's pre-dispatch buffer (the retrying head is re-fronted there during
// its backoff, so total buffer size is not a stable dedup signal).
func bufferedCopies(p *Pool, group, id string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	gq := p.groupQs[group]
	if gq == nil {
		return 0
	}
	n := 0
	for _, m := range gq.msgs {
		if m.Message.ID == id {
			n++
		}
	}
	return n
}

// newRouteHarness wires a Manager + tracker + single default pool
// (concurrency 1) whose consumer resolution returns cons — the plumbing the
// route-time dedup tests share.
func newRouteHarness(med Mediator, cons queue.Consumer) (*Manager, *InFlightTracker, *Pool) {
	tr := NewInFlightTracker()
	m := NewManager(med, tr)
	m.consumers["q"] = &runningConsumer{consumer: cons}
	pool := NewPool(common.PoolConfig{Code: defaultPoolCode, Concurrency: 1}, med, tr, m.resolveConsumer)
	m.pools[defaultPoolCode] = pool
	return m, tr, pool
}

// TestManagerRouteRequeueOfBufferedMessageAcked pins the requeue-storm fix:
// a message BUFFERED in an ordered group (accepted but not yet dispatched —
// previously invisible to dedup) is now registered at route time, so an
// external requeue of it (same app id, different broker id) is ACK-deleted
// instead of being appended to the group as a duplicate.
func TestManagerRouteRequeueOfBufferedMessageAcked(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 99, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1"} // head fails forever → m2 stays buffered
	m, _, pool := newRouteHarness(med, cons)

	ctx := context.Background()
	m.route(ctx, []common.QueuedMessage{
		mkGrouped("m1", "b1", "rh-m1"),
		mkGrouped("m2", "b2", "rh-m2"),
	}, cons)

	// Wait until the head is being retried, i.e. m2 sits buffered behind it.
	require.Eventually(t, func() bool {
		med.mu.Lock()
		defer med.mu.Unlock()
		return len(med.seen) >= 1
	}, 2*time.Second, 5*time.Millisecond, "head was never attempted")

	// The external requeue of the buffered m2 arrives (new broker id).
	m.route(ctx, []common.QueuedMessage{mkGrouped("m2", "b2-requeued", "rh-m2-requeue")}, cons)

	require.Eventually(t, func() bool {
		cons.mu.Lock()
		defer cons.mu.Unlock()
		return len(cons.acked) == 1 && cons.acked[0] == "rh-m2-requeue"
	}, 2*time.Second, 5*time.Millisecond, "requeued copy of the buffered message must be ACK-deleted")
	assert.Equal(t, 1, bufferedCopies(pool, "g", "m2"), "the group buffer must still hold exactly one m2")
}

// TestManagerRouteRedeliveryOfBufferedMessageDropped: a broker redelivery
// (same broker id) of a message buffered in an ordered group is dropped and
// its fresher receipt handle adopted — previously it was re-enqueued as a
// duplicate on every visibility-timeout lapse for as long as the head blocked.
func TestManagerRouteRedeliveryOfBufferedMessageDropped(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 99, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1"}
	m, tr, pool := newRouteHarness(med, cons)

	ctx := context.Background()
	m.route(ctx, []common.QueuedMessage{
		mkGrouped("m1", "b1", "rh-m1"),
		mkGrouped("m2", "b2", "rh-m2"),
	}, cons)
	require.Eventually(t, func() bool {
		med.mu.Lock()
		defer med.mu.Unlock()
		return len(med.seen) >= 1
	}, 2*time.Second, 5*time.Millisecond, "head was never attempted")

	// Visibility lapsed on the buffered m2; SQS redelivers with a new handle.
	m.route(ctx, []common.QueuedMessage{mkGrouped("m2", "b2", "rh-m2-redelivered")}, cons)

	assert.Equal(t, 1, bufferedCopies(pool, "g", "m2"), "redelivery must not duplicate the buffered message")
	rh, ok := tr.CurrentReceipt("m2", "b2")
	require.True(t, ok, "m2 must remain tracked")
	assert.Equal(t, "rh-m2-redelivered", rh, "the owner must adopt the redelivery's fresher handle")
	cons.mu.Lock()
	defer cons.mu.Unlock()
	assert.Empty(t, cons.acked, "a same-broker-id redelivery is dropped, not ACKed")
}

// TestManagerRouteRedeliveryResumesParkedGroup: with route-time dedup,
// redeliveries of buffered messages no longer re-enter via submit — which was
// the accidental resume mechanism for a group whose drainer died with its
// cancelled consumer. The redelivery-dedup path must therefore kick
// tryDrainGroup explicitly, or a restart would park the group forever.
func TestManagerRouteRedeliveryResumesParkedGroup(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 1, done: make(chan struct{})}
	med := &cascadeMediator{}
	m, _, pool := newRouteHarness(med, cons)

	// Occupy the only concurrency slot so the drainer parks on the acquire.
	sem := pool.loadSem()
	sem <- struct{}{}

	ctx1, cancel1 := context.WithCancel(context.Background())
	m.route(ctx1, []common.QueuedMessage{mkGrouped("m1", "b1", "rh-m1")}, cons)
	require.Eventually(t, func() bool { return pool.QueueSize() == 0 },
		time.Second, 5*time.Millisecond, "drainer never popped m1")
	cancel1() // the consumer restart
	require.Eventually(t, func() bool { return groupIdleWithBuffered(pool, "g", 1) },
		time.Second, 5*time.Millisecond, "cancelled drainer must park the group resumable")
	<-sem

	// The broker redelivers m1 (same broker id) under the restarted consumer.
	m.route(context.Background(), []common.QueuedMessage{mkGrouped("m1", "b1", "rh-m1-redelivered")}, cons)

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("group wedged: redelivery did not resume the parked drainer")
	}
	cons.mu.Lock()
	acked := append([]string(nil), cons.acked...)
	cons.mu.Unlock()
	assert.Equal(t, []string{"rh-m1-redelivered"}, acked,
		"m1 delivers once, ACKed with the redelivery's fresher handle")
}

// TestPoolStoppedNackReleasesTrackerEntry: every path where a message leaves
// the pipeline without an ACK must release its route-time tracker entry —
// otherwise the broker's redelivery is classified as a duplicate of a copy
// that no longer exists and the message is never processed again.
func TestPoolStoppedNackReleasesTrackerEntry(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 99, done: make(chan struct{})}
	med := &cascadeMediator{}
	m, tr, pool := newRouteHarness(med, cons)

	pool.Stop()
	m.route(context.Background(), []common.QueuedMessage{mkGrouped("m1", "b1", "rh-m1")}, cons)

	require.Eventually(t, func() bool {
		cons.mu.Lock()
		defer cons.mu.Unlock()
		return len(cons.nacked) == 1
	}, time.Second, 5*time.Millisecond, "stopped pool must NACK the message")
	assert.Equal(t, 0, tr.Count(), "the NACKed message must not stay tracked")

	// Its redelivery re-enters the pipeline as a fresh copy.
	im := common.NewInFlightMessage(&common.Message{ID: "m1"}, "b1", "q", "", "rh-m1-again")
	assert.Equal(t, RegisterNew, tr.Register(im))
}

// TestPoolStopFlushesBufferedTrackerEntries: stopping a pool abandons its
// group buffers, so the buffered messages' tracker entries must be released —
// a retained entry would dedup-drop the broker's redeliveries forever while
// no pool holds the message.
func TestPoolStopFlushesBufferedTrackerEntries(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 99, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1"} // head fails forever → m2 stays buffered
	m, tr, pool := newRouteHarness(med, cons)

	m.route(context.Background(), []common.QueuedMessage{
		mkGrouped("m1", "b1", "rh-m1"),
		mkGrouped("m2", "b2", "rh-m2"),
	}, cons)
	require.Eventually(t, func() bool {
		med.mu.Lock()
		defer med.mu.Unlock()
		return len(med.seen) >= 1
	}, 2*time.Second, 5*time.Millisecond, "head was never attempted")
	require.Equal(t, 2, tr.Count())

	pool.Stop()

	// m2 (buffered) is flushed and released; m1 (in-hand, retrying) stays
	// tracked until its own pipeline exit.
	assert.Equal(t, uint32(0), pool.QueueSize())
	im := common.NewInFlightMessage(&common.Message{ID: "m2"}, "b2", "q", "", "rh-m2-again")
	assert.Equal(t, RegisterNew, tr.Register(im), "flushed m2 must be re-registrable on redelivery")
}
