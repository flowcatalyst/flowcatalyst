package router

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// slowAckConsumer holds every Ack until released, as a broker whose
// acknowledgement is a network round trip (SQS DeleteMessage) would.
type slowAckConsumer struct {
	*grConsumer
	acking  atomic.Int64 // acks started
	release chan struct{}
}

func (c *slowAckConsumer) Ack(ctx context.Context, receipt string, brokerID string) error {
	c.acking.Add(1)
	<-c.release
	return c.grConsumer.Ack(ctx, receipt, brokerID)
}

// countMediator records how many deliveries reached the target.
type countMediator struct{ delivered atomic.Int64 }

func (m *countMediator) Mediate(context.Context, *common.Message) common.MediationOutcome {
	m.delivered.Add(1)
	return common.Success(200)
}

// The concurrency slot bounds concurrent deliveries to the target; a broker
// acknowledgement is not one. With a slow ack (SQS), holding the slot through it
// capped a pool near concurrency / (delivery + ack time) however fast the target
// was. A pool of one slot must deliver the second message while the first
// message's ack is still outstanding.
func TestSlotIsReleasedBeforeTheBrokerAck(t *testing.T) {
	med := &countMediator{}
	c := &slowAckConsumer{grConsumer: &grConsumer{id: "q1"}, release: make(chan struct{})}
	p := NewPool(common.PoolConfig{Code: "TEST", Concurrency: 1}, med, NewInFlightTracker(),
		func(string) queue.Consumer { return c })

	for i := range 3 {
		p.submit(context.Background(), grMsg(fmt.Sprintf("m%d", i), "http://t/x"))
	}

	// Every message is delivered although no ack has completed.
	grWaitFor(t, func() bool { return med.delivered.Load() == 3 }, 3*time.Second)
	assert.Equal(t, int64(0), c.acks.Load(), "no ack has completed yet")

	close(c.release)
	grWaitFor(t, func() bool { return c.acks.Load() == 3 }, 3*time.Second)
}
