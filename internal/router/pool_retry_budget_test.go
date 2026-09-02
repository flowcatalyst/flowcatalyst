package router

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// The budget exists because an in-pipeline retry never hands the message back,
// so while it loops the broker's expiry, redelivery count and DLQ cannot act on
// it — and the stall detector leaves retrying entries alone. Unbounded, a target
// answering 429 or ack:false for ever pinned the message, its ordered group and
// its (un-reapable) tracker entry in memory for the life of the process.
func TestRetryBudgetReleasesOnceSpent(t *testing.T) {
	c := &grConsumer{id: "q1"}
	p := grPool(&grMediator{outcome: common.Success(http.StatusOK)}, c)

	msg := grMsg("evt", "http://t/x")
	msg.Attempts = maxInPipelineAttempts - 2 // one more retry left
	d := p.retryAfter(msg, 250*time.Millisecond)
	assert.Equal(t, BrokerRetry, d.Action, "inside the budget the message keeps retrying in place")
	assert.Equal(t, 250*time.Millisecond, d.RetryAfter)

	msg.Attempts = maxInPipelineAttempts - 1 // this attempt spends the last of it
	d = p.retryAfter(msg, 250*time.Millisecond)
	assert.Equal(t, BrokerRelease, d.Action,
		"a spent budget must hand the message back to the broker, not keep it")
	assert.Equal(t, 250*time.Millisecond, d.RetryAfter, "the backoff becomes the redelivery delay")
}

func TestNackDelayFromBackoff(t *testing.T) {
	assert.Nil(t, nackDelay(0), "no backoff → no delay")
	assert.Equal(t, uint32(1), *nackDelay(200 * time.Millisecond), "sub-second backoff still waits a second")
	assert.Equal(t, uint32(51), *nackDelay(51200 * time.Millisecond))
}

// An IMMEDIATE message that spends its budget goes back to the broker with the
// backoff as its redelivery delay, rather than looping in this process for ever.
func TestImmediateReleasesToBrokerWhenBudgetSpent(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 1, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1"} // 429 for ever
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })

	msg := common.QueuedMessage{
		Message: common.Message{
			ID: "m1", MediationTarget: "http://t/x", DispatchMode: common.DispatchImmediate,
		},
		ReceiptHandle: "rh-m1",
		Attempts:      maxInPipelineAttempts - 1, // the next failure is the last
	}
	pool.submit(context.Background(), msg)

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("the message was never resolved — it is still looping in-pipeline")
	}
	cons.mu.Lock()
	defer cons.mu.Unlock()
	assert.Equal(t, []string{"rh-m1"}, cons.nacked, "released to the broker")
	assert.Empty(t, cons.acked, "a spent budget is not a success — the message is not deleted")
}

// The ordered path releases the WHOLE group, exactly as it does for an
// unreachable target: leaving successors buffered while the head goes back to
// the broker would reorder them on redelivery.
func TestOrderedGroupReleasedWhenHeadSpendsBudget(t *testing.T) {
	group := "g"
	cons := &cascadeConsumer{wantTotal: 2, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1"} // 429 for ever
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })

	head := mkOrdered("m1", &group)
	head.Attempts = maxInPipelineAttempts - 1
	pool.submit(context.Background(), head)
	pool.submit(context.Background(), mkOrdered("m2", &group))

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("the group was never released")
	}
	cons.mu.Lock()
	defer cons.mu.Unlock()
	assert.ElementsMatch(t, []string{"m1", "m2"}, cons.nacked,
		"head and the message buffered behind it both go back")
	require.Empty(t, cons.acked)
}
