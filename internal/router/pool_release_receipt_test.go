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

// A release hands a message back with the receipt the broker last gave it,
// as an ack does (ackTracked), not with the one it was routed under.
//
// On Postgres a message held in the router past its visibility timeout is
// claimed again by the next poll under a new receipt handle. Manager.route
// sees the copy as a redelivery (RegisterRedelivery): the tracker entry
// adopts the new handle and the copy is dropped, so the buffered message
// still carries the old one. A Nack with the old handle is an UPDATE ...
// WHERE receipt_handle = <old> that matches no row and reports no error: the
// row stays claimed rather than nacked with a delay, and a claimed row does
// not hold back the rest of its group (Poll's NOT EXISTS clauses), so a later
// member is claimed and delivered before the released head comes back.
// TestNackWithAStaleReceiptLeavesTheGroupOpen (internal/queue/postgres) shows
// that half against a real Postgres.
func TestReleaseNacksWithTheFreshestReceipt(t *testing.T) {
	tracker := NewInFlightTracker()
	cons := &cascadeConsumer{wantTotal: 2, done: make(chan struct{})}
	hold := make(chan struct{})
	med := &cascadeMediator{failID: "m1", failWith: unreachable(503), hold: hold}
	pool := NewPool(common.PoolConfig{Code: "test", Concurrency: 1}, med, tracker,
		func(string) queue.Consumer { return cons })

	mk := func(id, receipt string) common.QueuedMessage {
		m := releaseMsg(id, "g", common.DispatchNextOnError)
		m.BrokerMessageID = id
		m.QueueIdentifier = "q"
		m.ReceiptHandle = receipt
		return m
	}
	register := func(m common.QueuedMessage) RegisterOutcome {
		return tracker.Register(common.NewInFlightMessage(&m.Message, m.BrokerMessageID, m.QueueIdentifier, m.BatchID, m.ReceiptHandle))
	}

	// Routed: m1 and m2 claimed under their first receipts and admitted; m1
	// is in delivery (held), m2 buffered behind it.
	first := []common.QueuedMessage{mk("m1", "poll1:m1"), mk("m2", "poll1:m2")}
	for _, m := range first {
		require.Equal(t, RegisterNew, register(m))
	}
	pool.submitRun(context.Background(), "g", first)

	// Both outlive their visibility timeout and are claimed again.
	require.Equal(t, RegisterRedelivery, register(mk("m1", "poll2:m1")))
	require.Equal(t, RegisterRedelivery, register(mk("m2", "poll2:m2")))

	// The target is unreachable: the group is released.
	close(hold)
	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("the group was not released")
	}
	cons.mu.Lock()
	nacked := append([]string(nil), cons.nacked...)
	cons.mu.Unlock()
	assert.ElementsMatch(t, []string{"poll2:m1", "poll2:m2"}, nacked,
		"a release must hand each message back under the receipt the broker gave it last")
	assert.Zero(t, tracker.Count(), "the released messages leave the tracker")
}
