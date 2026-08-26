package router

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// Ordering is only ever defined relative to a message group, so an ordered-mode
// message that names no group has nothing to order and must dispatch
// concurrently.
//
// This is what makes an ordered DEFAULT mode safe. Ungrouped messages all share
// the group key "", so treating them as ordered would file the pool's entire
// ungrouped volume into one buffer behind one drainer — concurrency 20
// delivering one message at a time. The test therefore asserts concurrency, not
// just completion: it holds every worker inside the mediator at once, which
// only succeeds if they were dispatched in parallel.
func TestUngroupedOrderedMessagesStillDispatchConcurrently(t *testing.T) {
	const n = 8
	med := newBlockingMediator(n)
	c := &grConsumer{id: "q1"}
	p := NewPool(common.PoolConfig{Code: "TEST", Concurrency: n}, med, nil,
		func(string) queue.Consumer { return c })

	for i := range n {
		p.submit(context.Background(), common.QueuedMessage{
			Message: common.Message{
				ID:              fmt.Sprintf("m%d", i),
				MediationTarget: "http://t/x",
				DispatchMode:    common.DispatchNextOnError, // ordered mode…
				MessageGroupID:  nil,                        // …but no group
			},
			ReceiptHandle: fmt.Sprintf("rh-%d", i),
		})
	}

	// All n reach the mediator together. Serialised through one drainer, only
	// the first would arrive and this would time out.
	med.awaitEntered(t, n)
	assert.Equal(t, uint32(n), p.ActiveWorkers(),
		"ungrouped messages must occupy the pool's workers in parallel")

	close(med.release)
	grWaitFor(t, func() bool { return c.acks.Load() == n }, 3*time.Second)
}

// The counterpart: name a group and the same mode serialises, one at a time, in
// arrival order.
func TestGroupedOrderedMessagesRunOneAtATimeInOrder(t *testing.T) {
	group := "g"
	cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
	med := &orderRecordingMediator{}
	p := NewPool(common.PoolConfig{Code: "TEST", Concurrency: 8}, med, nil,
		func(string) queue.Consumer { return cons })

	for _, id := range []string{"m1", "m2", "m3"} {
		p.submit(context.Background(), common.QueuedMessage{
			Message: common.Message{
				ID:              id,
				MediationTarget: "http://t/x",
				DispatchMode:    common.DispatchNextOnError,
				MessageGroupID:  &group,
			},
			ReceiptHandle: id,
		})
	}

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("group never drained")
	}
	assert.Equal(t, []string{"m1", "m2", "m3"}, med.order(), "a named group keeps its sequence")
	assert.LessOrEqual(t, med.maxConcurrent(), 1, "and runs one at a time")
}

// orderRecordingMediator records delivery order and the peak number of
// deliveries in flight at once.
type orderRecordingMediator struct {
	mu      sync.Mutex
	seen    []string
	inFlt   int
	peakInF int
}

func (m *orderRecordingMediator) Mediate(_ context.Context, msg *common.Message) common.MediationOutcome {
	m.mu.Lock()
	m.seen = append(m.seen, msg.ID)
	m.inFlt++
	if m.inFlt > m.peakInF {
		m.peakInF = m.inFlt
	}
	m.mu.Unlock()

	time.Sleep(10 * time.Millisecond) // a window in which an overlap could show

	m.mu.Lock()
	m.inFlt--
	m.mu.Unlock()
	return common.Success(http.StatusOK)
}

func (m *orderRecordingMediator) order() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.seen...)
}

func (m *orderRecordingMediator) maxConcurrent() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.peakInF
}

// The scheduler's own default must reach the router as an ordering mode — the
// end-to-end statement of the change.
func TestDefaultModeOrdersWhenGrouped(t *testing.T) {
	require.True(t, common.DefaultDispatchMode.RequiresOrdering(),
		"an unspecified mode must order")
	require.False(t, common.DispatchImmediate.RequiresOrdering(),
		"IMMEDIATE stays the explicit opt-out")
}
