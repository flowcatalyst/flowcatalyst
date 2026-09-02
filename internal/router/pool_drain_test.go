package router

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// gatedMediator blocks Mediate for one specific message id until gate is
// closed, and succeeds immediately for every other id. Used to hold a
// message "actively mediating" (ActiveWorkers()==1) at a controlled point so
// a test can observe Pool state mid-delivery.
type gatedMediator struct {
	gateID string
	gate   chan struct{}

	mu   sync.Mutex
	seen []string
}

func (m *gatedMediator) Mediate(_ context.Context, msg *common.Message) common.MediationOutcome {
	m.mu.Lock()
	m.seen = append(m.seen, msg.ID)
	m.mu.Unlock()
	if msg.ID == m.gateID {
		<-m.gate
	}
	return common.MediationOutcome{Result: common.MediationSuccess}
}

// TestPoolDrain_BufferedGroupStillDeliversThenPoolStops pins [X-11 / R-26 /
// R-49]: removed-pool-drains-then-stops. Unlike Stop (which flushes the
// buffer immediately for broker redelivery), Drain lets the group already
// buffered at the moment of removal keep draining through its existing
// drainer and complete delivery normally — nothing is flushed or dropped.
// New admission is refused the moment Drain is called. Once the buffer and
// active workers are both empty, the pool transitions to fully stopped.
func TestPoolDrain_BufferedGroupStillDeliversThenPoolStops(t *testing.T) {
	group := "g"
	gate := make(chan struct{})
	med := &gatedMediator{gateID: "m1", gate: gate}
	cons := &cascadeConsumer{wantTotal: 999, done: make(chan struct{})}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })

	mk := func(id string) common.QueuedMessage {
		return common.QueuedMessage{
			Message: common.Message{
				ID:              id,
				MediationType:   common.MediationTypeHTTP,
				MediationTarget: "http://example.invalid",
				MessageGroupID:  &group,
				DispatchMode:    common.DispatchNextOnError,
			},
			ReceiptHandle: id,
		}
	}
	submitBatch(context.Background(), pool, []common.QueuedMessage{mk("m1"), mk("m2")})

	require.Eventually(t, func() bool { return pool.ActiveWorkers() == 1 }, time.Second, 5*time.Millisecond,
		"m1 must be actively mediating (gated) before Drain is called")
	require.Eventually(t, func() bool { return pool.QueueSize() == 1 }, time.Second, 5*time.Millisecond,
		"m2 must be buffered behind m1, not dropped, at the moment of removal")

	drained := make(chan struct{})
	pool.Drain(func() { close(drained) })

	// New admission is refused immediately, even while the existing buffer
	// keeps draining (X-11: a removal drains, it does not admit).
	pool.submit(context.Background(), mk("m3"))
	require.Eventually(t, func() bool {
		cons.mu.Lock()
		defer cons.mu.Unlock()
		return len(cons.nacked) == 1 && cons.nacked[0] == "m3"
	}, time.Second, 5*time.Millisecond, "a message submitted after Drain must be refused, not buffered")

	// Release m1; both m1 and m2 must still be delivered (ACKed) normally —
	// the buffer already held at removal is never flushed by Drain.
	close(gate)
	require.Eventually(t, func() bool {
		cons.mu.Lock()
		defer cons.mu.Unlock()
		return len(cons.acked) == 2
	}, time.Second, 5*time.Millisecond, "the buffered group must still be delivered after Drain, not flushed for redelivery")
	cons.mu.Lock()
	acked := append([]string(nil), cons.acked...)
	cons.mu.Unlock()
	assert.ElementsMatch(t, []string{"m1", "m2"}, acked)

	// Once the buffer and active workers are both empty, Drain completes and
	// the pool transitions to fully stopped.
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("Drain did not complete after its buffer emptied")
	}
	assert.True(t, pool.stopped.Load(), "Drain must fully stop the pool once its buffer has drained")
}

// TestPoolDrain_IdempotentAndCallsOnDoneOnce guards Drain's CompareAndSwap
// gate: a second call while a drain is already in flight must not spawn a
// second background goroutine or fire onDone twice.
func TestPoolDrain_IdempotentAndCallsOnDoneOnce(t *testing.T) {
	med := &gatedMediator{gateID: "", gate: make(chan struct{})}
	cons := &cascadeConsumer{wantTotal: 999, done: make(chan struct{})}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })

	var calls int
	var mu sync.Mutex
	onDone := func() {
		mu.Lock()
		calls++
		mu.Unlock()
	}
	pool.Drain(onDone)
	pool.Drain(onDone) // must be a no-op

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return calls >= 1
	}, time.Second, 5*time.Millisecond)

	time.Sleep(50 * time.Millisecond) // let a wrongly-spawned second goroutine have its chance
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, calls, "Drain must be idempotent: onDone fires exactly once")
}
