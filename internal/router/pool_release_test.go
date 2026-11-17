package router

import (
	"context"
	"net/http"
	"slices"
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

// rejected mirrors the mediator's R-57 classification for a 5xx that is not
// 502/503/504: the app ran and answered, so the outcome is ErrorConfig —
// permanent, ACKed away, terminal FAILURE for its group.
func rejected(status int) *common.MediationOutcome {
	out := common.ErrorConfig(status, "rejected")
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
//
// This test used to fail about once in a few thousand runs: submitBatch
// submitted the group one message at a time, and a drainer that failed and
// released m1 before m3 (or m2) had been submitted left the late member to
// start a drainer of its own, which delivered and acked it. submitBatch now
// admits the group as one run, as route() does; TestRouteSettlesTheWholeBatchOfAGroup
// pins the same window through route() deterministically.
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
	cons.mu.Unlock()
	med.mu.Lock()
	seen := append([]string(nil), med.seen...)
	med.mu.Unlock()

	assert.ElementsMatch(t, []string{"m1", "m2", "m3"}, nacked,
		"the whole group returns to the broker, not just the failed head")
	assert.Empty(t, acked, "nothing is resolved, so nothing may be ACKed")
	assert.Equal(t, []string{"m1"}, seen,
		"successors must NOT be attempted against a target already known unreachable")
}

// TestPoolDiscardsOn500AndContinuesUnderNextOnError — NEXT_ON_ERROR is ordered
// but "the group moves on" past a failure. The failed head is ACKed away and its
// successors still deliver, in order.
func TestPoolDiscardsOn500AndContinuesUnderNextOnError(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1", failWith: rejected(http.StatusInternalServerError)}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })

	submitBatch(context.Background(), pool, []common.QueuedMessage{
		releaseMsg("m1", "g", common.DispatchNextOnError),
		releaseMsg("m2", "g", common.DispatchNextOnError),
		releaseMsg("m3", "g", common.DispatchNextOnError),
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
		"NEXT_ON_ERROR moves on: the head is ACKed away and the rest still deliver")
}

// TestPoolDiscardsOn500AndStopsUnderBlockOnError — the other half, and the ONLY
// place the two ordered modes diverge. BLOCK_ON_ERROR means "a failed job blocks
// the group until resolved", so the successors must NOT be delivered past the
// failure. They are ACKED off the queue, never attempted.
//
// Releasing them instead — which this test used to require — did the opposite of
// what it looked like: the broker redelivers on its own timer, unconnected to
// the failure being resolved, and with the head already ACKed away the first
// sibling becomes the new head and is delivered. The messages survive as
// dispatch-job rows in the platform store, which is the system of record; the
// queue copy is a delivery attempt, not the data.
//
// Both modes share the same FIFO buffer and the same ordering guarantee; they
// differ only in what a terminal failure does to the rest of the group.
func TestPoolDiscardsOn500AndStopsUnderBlockOnError(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1", failWith: rejected(http.StatusInternalServerError)}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })

	submitBatch(context.Background(), pool, []common.QueuedMessage{
		releaseMsg("m1", "g", common.DispatchBlockOnError),
		releaseMsg("m2", "g", common.DispatchBlockOnError),
		releaseMsg("m3", "g", common.DispatchBlockOnError),
	})

	assert.Eventually(t, func() bool {
		cons.mu.Lock()
		defer cons.mu.Unlock()
		return len(cons.acked) == 3
	}, 3*time.Second, 10*time.Millisecond, "head and its untried successors all settled")

	cons.mu.Lock()
	nacked := append([]string(nil), cons.nacked...)
	acked := append([]string(nil), cons.acked...)
	cons.mu.Unlock()
	med.mu.Lock()
	seen := append([]string(nil), med.seen...)
	med.mu.Unlock()

	assert.ElementsMatch(t, []string{"m1", "m2", "m3"}, acked,
		"the failed head and its untried successors are ACKed off the queue")
	assert.Empty(t, nacked,
		"nothing goes back to the broker: a redelivery would deliver past the failure")
	assert.Equal(t, []string{"m1"}, seen,
		"BLOCK_ON_ERROR must not attempt the successors at all")
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

// stallingSource is a source consumer whose Ack of one receipt handle holds
// its caller until the group's head (rh-m1) has been settled at the broker,
// or until a short wait runs out. route() acks an external-requeue copy on
// its own goroutine, between two messages of the batch, so this holds
// route() at that point for as long as a broker round trip can take in
// production.
type stallingSource struct {
	*cascadeConsumer
	stallOn string
}

func (s *stallingSource) Ack(ctx context.Context, rh, brokerID string) error {
	if rh == s.stallOn {
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) && !s.settled("rh-m1") {
			time.Sleep(time.Millisecond)
		}
	}
	return s.cascadeConsumer.Ack(ctx, rh, brokerID)
}

func (s *stallingSource) settled(rh string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Contains(s.nacked, rh) || slices.Contains(s.acked, rh)
}

// TestRouteSettlesTheWholeBatchOfAGroup pins the release rule and the
// BLOCK_ON_ERROR rule through route(), across the gap between two messages of
// one poll batch. route() admits a batch one message at a time, and anything
// that keeps it busy between two members of a group (here the synchronous ack
// of an external-requeue copy; in production a broker round trip, or the
// routing goroutine simply being descheduled) used to let the group's drainer
// attempt the head, fail it and settle the group before the rest of the batch
// had arrived. The late members then found no group to join and started a
// fresh drainer of their own: delivered behind a head the broker would
// redeliver, or past a failure that was meant to block them.
func TestRouteSettlesTheWholeBatchOfAGroup(t *testing.T) {
	cases := []struct {
		name       string
		failWith   *common.MediationOutcome
		wantNacked []string
		wantAcked  []string
	}{
		{
			name:       "unreachable target releases the whole batch",
			failWith:   unreachable(http.StatusServiceUnavailable),
			wantNacked: []string{"rh-m1", "rh-m2", "rh-m3"},
			wantAcked:  []string{"rh-x-copy"},
		},
		{
			name:       "BLOCK_ON_ERROR failure acks the whole batch untried",
			failWith:   rejected(http.StatusInternalServerError),
			wantNacked: nil,
			wantAcked:  []string{"rh-x-copy", "rh-m1", "rh-m2", "rh-m3"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := &stallingSource{
				cascadeConsumer: &cascadeConsumer{wantTotal: 99, done: make(chan struct{})},
				stallOn:         "rh-x-copy",
			}
			med := &cascadeMediator{failID: "m1", failWith: tc.failWith}
			m, tr, _ := newRouteHarness(med, src)

			// "x" is already in the pipeline under another broker id, so the
			// copy in the batch is an external requeue that route() acks
			// itself, on its own goroutine.
			tr.Register(common.NewInFlightMessage(&common.Message{ID: "x"}, "bk-x", "q", "b0", "rh-x"))
			copyOfX := common.QueuedMessage{
				Message:         common.Message{ID: "x", MediationTarget: "http://example.invalid"},
				BrokerMessageID: "bk-x-copy",
				ReceiptHandle:   "rh-x-copy",
				QueueIdentifier: "q",
			}

			m.route(context.Background(), []common.QueuedMessage{
				mkGrouped("m1", "bk-m1", "rh-m1"),
				copyOfX,
				mkGrouped("m2", "bk-m2", "rh-m2"),
				mkGrouped("m3", "bk-m3", "rh-m3"),
			}, src)

			want := len(tc.wantNacked) + len(tc.wantAcked)
			settled := func() (nacked, acked []string) {
				src.mu.Lock()
				defer src.mu.Unlock()
				return append([]string(nil), src.nacked...), append([]string(nil), src.acked...)
			}
			assert.Eventually(t, func() bool {
				nacked, acked := settled()
				return len(nacked)+len(acked) >= want
			}, 3*time.Second, 5*time.Millisecond, "every message of the batch must be settled")

			nacked, acked := settled()
			med.mu.Lock()
			seen := append([]string(nil), med.seen...)
			med.mu.Unlock()
			assert.ElementsMatch(t, tc.wantNacked, nacked)
			assert.ElementsMatch(t, tc.wantAcked, acked)
			assert.Equal(t, []string{"m1"}, seen,
				"no later member of the batch may be delivered once its head has settled the group")
		})
	}
}
