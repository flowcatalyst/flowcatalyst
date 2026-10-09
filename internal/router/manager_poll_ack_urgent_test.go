package router

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// ackUrgencyRecorder records, per receipt, whether each Ack carried the urgent
// marker.
type ackUrgencyRecorder struct {
	*grConsumer
	mu     sync.Mutex
	urgent map[string]bool
}

func (c *ackUrgencyRecorder) Ack(ctx context.Context, receipt string, brokerID string) error {
	c.mu.Lock()
	c.urgent[receipt] = queue.IsUrgentAck(ctx)
	c.mu.Unlock()
	return c.grConsumer.Ack(ctx, receipt, brokerID)
}

// The acks routeOne issues itself run on the consumer's poll goroutine and
// block until the broker answers. Unmarked, SQS's delete batcher holds each for
// its linger (up to 5 s) waiting for a full batch, so a burst of external
// requeues — the duplicate-storm shape of republished dispatch jobs — or of
// strictly-rejected messages throttles the queue's intake to about one message
// per linger. Both must be urgent.
func TestPollLoopAcksAreUrgent(t *testing.T) {
	newManager := func() (*Manager, *InFlightTracker, *ackUrgencyRecorder) {
		med := &cascadeMediator{}
		tr := NewInFlightTracker()
		m := NewManager(med, tr)
		m.pools[defaultPoolCode] = NewPool(common.PoolConfig{Code: defaultPoolCode}, med, tr, func(string) queue.Consumer { return nil })
		return m, tr, &ackUrgencyRecorder{grConsumer: &grConsumer{id: "q"}, urgent: map[string]bool{}}
	}

	t.Run("external requeue duplicate", func(t *testing.T) {
		m, tr, cons := newManager()
		require.Equal(t, RegisterNew, tr.Register(common.NewInFlightMessage(&common.Message{ID: "app1"}, "broker1", "q", "b", "rh-orig")))

		m.route(context.Background(), []common.QueuedMessage{{
			Message:         common.Message{ID: "app1"},
			BrokerMessageID: "broker2",
			ReceiptHandle:   "rh-requeue",
			QueueIdentifier: "q",
		}}, cons)

		cons.mu.Lock()
		defer cons.mu.Unlock()
		urgent, acked := cons.urgent["rh-requeue"]
		require.True(t, acked, "the external-requeue duplicate is acked on the poll loop")
		assert.True(t, urgent, "and the ack is urgent")
	})

	t.Run("strict routing reject", func(t *testing.T) {
		m, _, cons := newManager()
		m.SetStrictRouting(true)

		m.route(context.Background(), []common.QueuedMessage{{
			Message:         common.Message{ID: "app2"}, // no pool_code, no dispatch_mode
			BrokerMessageID: "broker3",
			ReceiptHandle:   "rh-malformed",
			QueueIdentifier: "q",
		}}, cons)

		cons.mu.Lock()
		defer cons.mu.Unlock()
		urgent, acked := cons.urgent["rh-malformed"]
		require.True(t, acked, "the strictly-rejected message is acked on the poll loop")
		assert.True(t, urgent, "and the ack is urgent")
	})
}
