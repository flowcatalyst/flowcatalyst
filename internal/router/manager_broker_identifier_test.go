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

// fakeBrokerIdentityQueue is a registered backend whose own Identifier()
// differs from the config queue name it was built under — mirroring NATS,
// whose Identifier() is "<stream>/<consumer>", not the operator-chosen
// config-name label (G10; see nats.go's `identifier: cfg.StreamName + "/" +
// cfg.ConsumerName`). Every polled QueuedMessage carries that same
// identifier as its QueueIdentifier, exactly as the real NATS backend's
// Poll does — see the doc on fakeQueue in manager_lifecycle_test.go, whose
// Identifier() equals its config name and so never exercised this mismatch.
const fakeBrokerIdentityScheme = "fakebrokerid"

type fakeBrokerIdentityQueue struct {
	configName string
	identifier string
	acks       atomic.Int64

	mu      sync.Mutex
	pending []common.QueuedMessage
}

var fakeBrokerIdentityQueues sync.Map // config name → *fakeBrokerIdentityQueue

func init() {
	queue.RegisterConsumer(fakeBrokerIdentityScheme, func(_ context.Context, cfg common.QueueConfig) (queue.Consumer, error) {
		q := &fakeBrokerIdentityQueue{configName: cfg.Name, identifier: cfg.Name + "/router"}
		fakeBrokerIdentityQueues.Store(cfg.Name, q)
		return q, nil
	})
}

func (q *fakeBrokerIdentityQueue) Identifier() string { return q.identifier }

func (q *fakeBrokerIdentityQueue) Poll(_ context.Context, _ uint32) ([]common.QueuedMessage, error) {
	q.mu.Lock()
	out := q.pending
	q.pending = nil
	q.mu.Unlock()
	return out, nil
}

// enqueue stamps QueueIdentifier with the consumer's OWN identifier, not the
// config name — the point being pinned.
func (q *fakeBrokerIdentityQueue) enqueue(m common.Message, receipt, brokerID string) {
	q.mu.Lock()
	q.pending = append(q.pending, common.QueuedMessage{
		Message:         m,
		ReceiptHandle:   receipt,
		BrokerMessageID: brokerID,
		QueueIdentifier: q.identifier,
	})
	q.mu.Unlock()
}

func (q *fakeBrokerIdentityQueue) Ack(context.Context, string, string) error {
	q.acks.Add(1)
	return nil
}
func (q *fakeBrokerIdentityQueue) Nack(context.Context, string, *uint32) error  { return nil }
func (q *fakeBrokerIdentityQueue) Defer(context.Context, string, *uint32) error { return nil }
func (q *fakeBrokerIdentityQueue) Healthy() bool                               { return true }
func (q *fakeBrokerIdentityQueue) Stop()                                       {}
func (q *fakeBrokerIdentityQueue) Metrics(context.Context) (*queue.Metrics, error) {
	return &queue.Metrics{QueueIdentifier: q.identifier}, nil
}
func (q *fakeBrokerIdentityQueue) Counters() *queue.Metrics { return nil }

// TestAckResolvesByConsumerIdentifierNotConfigName pins G10: NATS deliveries
// were never acknowledged because ack/nack resolution looked a message's
// QueueIdentifier (the queue backend's own identity — for NATS,
// "<stream>/<consumer>") up in a table keyed by the config queue name (an
// operator-chosen label the backend never reports as its own Identifier()).
// That lookup misses on every single delivery for a backend where the two
// differ, so nothing is ever acked and the (real) broker redelivers forever
// until MaxDeliver dead-letters it.
//
// This fixture reproduces the mismatch directly: registered under config
// name "BENCH-1", but its own Identifier() (and therefore every polled
// message's QueueIdentifier) is "BENCH-1/router" — the NATS
// "<stream>/<consumer>" shape.
//
// Load-bearing assertion: the ack must land on the fixture's own consumer,
// exactly once. A mutant that resolves consumers by config name (reverting
// resolveConsumer to look the queue up in the name-keyed table instead of
// the identifier-keyed one) makes this assertion fail: the poll loop's
// eventual ack call finds "no consumer for queue BENCH-1/router" and drops
// it, so acks.Load() never leaves zero.
func TestAckResolvesByConsumerIdentifierNotConfigName(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	require.NoError(t, m.Reconfigure(context.Background(),
		common.RouterConfig{Queues: []common.QueueConfig{
			{Name: "BENCH-1", URI: fakeBrokerIdentityScheme + "://BENCH-1"},
		}}))

	qv, ok := fakeBrokerIdentityQueues.Load("BENCH-1")
	require.True(t, ok, "fake consumer must have been built")
	q := qv.(*fakeBrokerIdentityQueue)
	require.Equal(t, "BENCH-1/router", q.Identifier(),
		"the fixture's identifier must differ from its config name — that gap is what G10 is about")

	q.enqueue(common.Message{ID: "m1", MediationTarget: "http://t/x"}, "rh-m1", "bk-m1")

	require.Eventually(t, func() bool { return q.acks.Load() == 1 },
		2*time.Second, 5*time.Millisecond,
		"the delivery must be ACKed on its own consumer, resolved by the consumer's own identifier")
	assert.Equal(t, int64(1), q.acks.Load(), "exactly one ack — no duplicate resolution/redelivery")
}
