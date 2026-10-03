package router

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// urgentRecorder records whether the last Ack carried the urgent marker.
type urgentRecorder struct {
	*grConsumer
	urgent atomic.Bool
}

func (c *urgentRecorder) Ack(ctx context.Context, receipt string, brokerID string) error {
	c.urgent.Store(queue.IsUrgentAck(ctx))
	return c.grConsumer.Ack(ctx, receipt, brokerID)
}

// An ordered message's group waits for its ack before the next message goes, so
// the ack must be marked urgent (a batching broker must not linger on it);
// unordered messages, and ordered-mode messages without a group, are not.
func TestAckIsUrgentOnlyForOrderedGroupedMessages(t *testing.T) {
	c := &urgentRecorder{grConsumer: &grConsumer{id: "q1"}}
	p := NewPool(common.PoolConfig{Code: "TEST", Concurrency: 1}, &countMediator{}, NewInFlightTracker(),
		func(string) queue.Consumer { return c })
	group := "g1"

	mk := func(mode common.DispatchMode, grp *string) common.QueuedMessage {
		qm := grMsg("m", "http://t/x")
		qm.Message.DispatchMode = mode
		qm.Message.MessageGroupID = grp
		return qm
	}
	cases := []struct {
		name   string
		qm     common.QueuedMessage
		urgent bool
	}{
		{"ordered NEXT_ON_ERROR grouped", mk(common.DispatchNextOnError, &group), true},
		{"ordered BLOCK_ON_ERROR grouped", mk(common.DispatchBlockOnError, &group), true},
		{"immediate grouped", mk(common.DispatchImmediate, &group), false},
		{"ordered but ungrouped", mk(common.DispatchNextOnError, nil), false},
	}
	for _, tc := range cases {
		c.urgent.Store(!tc.urgent) // ensure Ack overwrites it
		p.ackTracked(context.Background(), tc.qm)
		assert.Equal(t, tc.urgent, c.urgent.Load(), tc.name)
	}
}
