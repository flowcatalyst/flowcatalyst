package router

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// A message group is a strict FIFO: messages drain in arrival order regardless
// of HighPriority (which is a queue-level concern and must NOT reorder within a
// group — doing so would defeat in-order delivery).
func TestGroupQueueIsStrictFIFO(t *testing.T) {
	gq := &groupQueue{}

	mk := func(id string, hi bool) common.QueuedMessage {
		return common.QueuedMessage{Message: common.Message{ID: id, HighPriority: hi}}
	}

	// Interleave regular and "high priority" — HighPriority must be ignored.
	gq.msgs = append(gq.msgs, []common.QueuedMessage{
		mk("a", false), mk("b", true), mk("c", false), mk("d", true),
	}...)

	var drained []string
	for !gq.empty() {
		m, _ := gq.pop()
		drained = append(drained, m.Message.ID)
	}
	assert.Equal(t, []string{"a", "b", "c", "d"}, drained, "group drains in strict arrival order")
}

func TestGroupQueueEmptyAfterAllPopped(t *testing.T) {
	gq := &groupQueue{}
	gq.msgs = append(gq.msgs, common.QueuedMessage{Message: common.Message{ID: "r1"}})
	assert.False(t, gq.empty())
	_, empty := gq.pop()
	assert.True(t, empty)
	assert.True(t, gq.empty())
}

func TestPoolEnqueueAppendsToBackEnqueueFrontPrepends(t *testing.T) {
	p := &Pool{groupQs: map[string]*groupQueue{}}
	p.enqueue("g1", common.QueuedMessage{Message: common.Message{ID: "m1"}})
	p.enqueue("g1", common.QueuedMessage{Message: common.Message{ID: "m2"}})
	// A retry re-inserts at the front so it is attempted before m1/m2.
	p.enqueueFront("g1", common.QueuedMessage{Message: common.Message{ID: "retry"}})

	g1 := p.groupQs["g1"]
	got := make([]string, 0, len(g1.msgs))
	for _, m := range g1.msgs {
		got = append(got, m.Message.ID)
	}
	assert.Equal(t, []string{"retry", "m1", "m2"}, got, "enqueue → back, enqueueFront → head")
}

// R-52/R-53: a message ACKed because its group is currently suppressed by
// the GroupFlushRegistry must never reach the mediator (that's the whole
// point of a flush — no HTTP call, no rate-limit token, no concurrency
// slot spent) and must now show up on the pool's own metrics as a
// TotalSuppressed count, not silently vanish leaving the pool looking idle.
func TestPoolProcessOne_SuppressedGroupSkipsMediationAndRecordsMetric(t *testing.T) {
	c := &grConsumer{id: "q1"}
	med := &grMediator{outcome: common.Success(http.StatusOK)}
	p := grPool(med, c)

	group := "g1"
	require.True(t, p.flushes.Flush(group, time.Minute), "flush must suppress the group")

	msg := grMsg("evt_suppressed", "http://t/x")
	msg.Message.MessageGroupID = &group

	d := p.processOne(context.Background(), msg)

	assert.Equal(t, BrokerAck, d.Action, "a suppressed group is ACKed, not retried or released")
	assert.False(t, med.called.Load(), "a suppressed group must never reach the mediator")
	assert.EqualValues(t, 1, c.acks.Load())
	assert.EqualValues(t, 0, c.nacks.Load())

	snap := p.metrics.Snapshot()
	assert.EqualValues(t, 1, snap.TotalSuppressed, "the suppressed ACK must be visible on the pool's own metrics")
	assert.EqualValues(t, 0, snap.TotalSuccess, "a suppressed ACK is not a delivery success")
	assert.EqualValues(t, 0, snap.TotalFailure, "a suppressed ACK is not a delivery failure")
}
