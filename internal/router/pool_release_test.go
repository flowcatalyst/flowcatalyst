package router

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

func releaseMsg(id, group string, mode common.DispatchMode) common.QueuedMessage {
	m := common.QueuedMessage{
		Message: common.Message{
			ID:              id,
			MediationType:   common.MediationTypeHTTP,
			MediationTarget: "http://example.invalid",
			DispatchMode:    mode,
		},
		ReceiptHandle: id,
	}
	if group != "" {
		g := group
		m.Message.MessageGroupID = &g
	}
	return m
}

func unreachable(status int) *common.MediationOutcome {
	out := common.ErrorProcess(0, "unreachable")
	out.StatusCode = status
	return &out
}

// TestPoolReleasesWholeGroupOnUnreachableTarget is the core of the release rule:
// when the head of an ordered group can't reach a working app, the head AND
// every message buffered behind it go back to the broker.
//
// Releasing only the head would be worse than useless — the head would return to
// the broker while its successors stayed buffered in this process, so on
// redelivery the head would arrive behind them and the group would be delivered
// out of order, which is the one thing an ordered group exists to prevent.
func TestPoolReleasesWholeGroupOnUnreachableTarget(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 0, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1", failWith: unreachable(http.StatusServiceUnavailable)}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })

	submitBatch(context.Background(), pool, []common.QueuedMessage{
		releaseMsg("m1", "g", common.DispatchBlockOnError),
		releaseMsg("m2", "g", common.DispatchBlockOnError),
		releaseMsg("m3", "g", common.DispatchBlockOnError),
	})

	assert.Eventually(t, func() bool {
		cons.mu.Lock()
		defer cons.mu.Unlock()
		return len(cons.nacked) == 3
	}, 3*time.Second, 10*time.Millisecond, "head and both buffered successors must be released")

	cons.mu.Lock()
	nacked := append([]string(nil), cons.nacked...)
	acked := append([]string(nil), cons.acked...)
	seen := append([]string(nil), med.seen...)
	cons.mu.Unlock()

	assert.ElementsMatch(t, []string{"m1", "m2", "m3"}, nacked,
		"the whole group returns to the broker, not just the failed head")
	assert.Empty(t, acked, "nothing is resolved, so nothing may be ACKed")
	assert.Equal(t, []string{"m1"}, seen,
		"successors must NOT be attempted against a target already known unreachable")
}

// TestPoolDiscardsOn500WithoutReleasingGroup: a 500 is about the message, not
// the target, so it is ACKed away and the group carries on. This is the
// difference the whole rule turns on — the same failure shape, opposite handling.
func TestPoolDiscardsOn500WithoutReleasingGroup(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1", failWith: unreachable(http.StatusInternalServerError)}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })

	submitBatch(context.Background(), pool, []common.QueuedMessage{
		releaseMsg("m1", "g", common.DispatchBlockOnError),
		releaseMsg("m2", "g", common.DispatchBlockOnError),
		releaseMsg("m3", "g", common.DispatchBlockOnError),
	})

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the group to drain")
	}

	cons.mu.Lock()
	nacked := append([]string(nil), cons.nacked...)
	acked := append([]string(nil), cons.acked...)
	cons.mu.Unlock()

	assert.Empty(t, nacked, "a 500 is discarded, never released")
	assert.ElementsMatch(t, []string{"m1", "m2", "m3"}, acked,
		"the failed head is ACKed away and the rest of the group still delivers")
}

// TestPoolImmediateReleasesOnlyItself: IMMEDIATE has no group buffer, so an
// unreachable target releases just that message and leaves its peers alone.
func TestPoolImmediateReleasesOnlyItself(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 2, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1", failWith: unreachable(http.StatusBadGateway)}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })

	submitBatch(context.Background(), pool, []common.QueuedMessage{
		releaseMsg("m1", "", common.DispatchImmediate),
		releaseMsg("m2", "", common.DispatchImmediate),
		releaseMsg("m3", "", common.DispatchImmediate),
	})

	assert.Eventually(t, func() bool {
		cons.mu.Lock()
		defer cons.mu.Unlock()
		return len(cons.nacked) == 1 && len(cons.acked) == 2
	}, 3*time.Second, 10*time.Millisecond, "only the failing message is released")

	cons.mu.Lock()
	nacked := append([]string(nil), cons.nacked...)
	acked := append([]string(nil), cons.acked...)
	cons.mu.Unlock()

	assert.Equal(t, []string{"m1"}, nacked)
	assert.ElementsMatch(t, []string{"m2", "m3"}, acked,
		"an unreachable target for one IMMEDIATE message must not affect the others")
}
