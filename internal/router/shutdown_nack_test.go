package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// Pool.Stop abandons its buffer. It used to drop the buffered messages
// without telling the broker, so each came back only when its visibility
// timeout lapsed, its group stuck behind it all that time. It now nacks
// every one: a group whose head is still being delivered with the sibling
// delay (its successors must not surface first), an idle group at once.
func TestPoolStopNacksEveryBufferedMessage(t *testing.T) {
	cons := &cascadeConsumer{}
	tracker := NewInFlightTracker()
	pool := NewPool(common.PoolConfig{Code: "P", Concurrency: 1}, &cascadeMediator{}, tracker,
		func(string) queue.Consumer { return cons })

	busy := []common.QueuedMessage{releaseMsg("b1", "busy", common.DispatchBlockOnError), releaseMsg("b2", "busy", common.DispatchBlockOnError)}
	idle := []common.QueuedMessage{releaseMsg("i1", "idle", common.DispatchBlockOnError)}
	for _, qm := range append(append([]common.QueuedMessage{}, busy...), idle...) {
		tracker.Register(common.NewInFlightMessage(&qm.Message, qm.BrokerMessageID, qm.QueueIdentifier, "", qm.ReceiptHandle))
	}
	pool.mu.Lock()
	pool.groupQs["busy"] = &groupQueue{msgs: busy, working: true}
	pool.groupQs["idle"] = &groupQueue{msgs: idle, parkedAt: time.Now()}
	pool.mu.Unlock()
	pool.queueSize.Store(3)

	pool.Stop()

	cons.mu.Lock()
	defer cons.mu.Unlock()
	assert.ElementsMatch(t, []string{"b1", "b2", "i1"}, cons.nacked, "every buffered message is nacked, none dropped")
	for _, id := range []string{"b1", "b2"} {
		require.NotNil(t, cons.nackDelays[id])
		assert.Equal(t, siblingNackDelaySeconds, *cons.nackDelays[id], "%s waits behind its in-delivery head", id)
	}
	assert.Nil(t, cons.nackDelays["i1"], "an idle group comes back at once")
	assert.Equal(t, uint32(0), pool.QueueSize())
	assert.Equal(t, 0, tracker.Count(), "tracker entries are released so the redeliveries are not dropped as duplicates")
}

// holdingMediator holds every delivery until release is closed.
type holdingMediator struct {
	started atomic.Int64
	release chan struct{}
}

func (b *holdingMediator) Mediate(ctx context.Context, _ *common.Message) common.MediationOutcome {
	b.started.Add(1)
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	return common.Success(http.StatusOK)
}

// Manager.Shutdown stops pools BEFORE it stops their consumers, so the
// buffered messages' nacks still reach the broker. It used to stop the
// consumers first; every nack then found no consumer and was skipped.
func TestManagerShutdownNacksBufferedMessagesThroughLiveConsumers(t *testing.T) {
	med := &holdingMediator{release: make(chan struct{})}
	defer close(med.release)
	m := newTestManager(t, med, NewInFlightTracker())
	require.NoError(t, m.Reconfigure(context.Background(), routerCfg([]string{"q-shut"},
		common.PoolConfig{Code: "ORD", Concurrency: 1})))
	q := fakeQueueFor(t, "q-shut")

	group := "g"
	for _, id := range []string{"s1", "s2", "s3"} {
		q.enqueue(common.QueuedMessage{
			Message: common.Message{
				ID: id, PoolCode: "ORD", MediationType: common.MediationTypeHTTP,
				MediationTarget: "http://example.invalid", DispatchMode: common.DispatchBlockOnError,
				MessageGroupID: &group,
			},
			ReceiptHandle: "rh-" + id, BrokerMessageID: "bk-" + id, QueueIdentifier: "q-shut",
		})
	}
	require.Eventually(t, func() bool { return med.started.Load() == 1 }, 2*time.Second, 5*time.Millisecond,
		"the head is being delivered, the other two are buffered")

	m.StopPolling()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, m.Shutdown(ctx))
	assert.Equal(t, int64(2), q.nacks.Load(), "both buffered messages are nacked back to their queue")
}

// Shutdown used to bridge its WaitGroup to a channel with a helper
// goroutine; when the deadline won, the helper stayed parked in Wait until
// the stragglers exited — a leak per leadership flap. idleGroup waits
// without one.
func TestManagerShutdownTimeoutLeaksNoGoroutine(t *testing.T) {
	m := NewManager(&grMediator{outcome: common.Success(http.StatusOK)}, nil)
	m.wg.Add(1) // a poll loop that never exits
	defer m.wg.Done()

	expired, cancel := context.WithCancel(context.Background())
	cancel()
	baseline := runtime.NumGoroutine()
	for range 200 {
		assert.ErrorIs(t, m.Shutdown(expired), context.Canceled)
	}
	// Allow for unrelated runtime goroutines coming and going; 200 leaked
	// helpers would dwarf that.
	assert.Less(t, runtime.NumGoroutine()-baseline, 20, "Shutdown must not leave a goroutine behind per timeout")
}

func TestIdleGroup(t *testing.T) {
	var g idleGroup
	select {
	case <-g.Idle():
	default:
		t.Fatal("an empty group is idle")
	}
	g.Add(2)
	idle := g.Idle()
	select {
	case <-idle:
		t.Fatal("not idle with two running")
	default:
	}
	g.Done()
	g.Done()
	select {
	case <-idle:
	case <-time.After(time.Second):
		t.Fatal("the channel closes when the count reaches zero")
	}
	g.Add(1)
	select {
	case <-g.Idle():
		t.Fatal("a new generation starts busy")
	default:
	}
	g.Done()
	assert.Panics(t, func() { g.Done() }, "a negative count is a bug")
}

// A burst of CRITICAL warnings used to start one flush goroutine per
// warning. Now Add only signals Run's loop, which flushes everything queued
// in one request; at most one flush request is ever pending.
func TestNotifierCriticalBurstIsFlushedWithoutAGoroutineEach(t *testing.T) {
	var posts atomic.Int64
	hold := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		<-hold // a slow webhook
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer close(hold)

	n := NewNotifier(srv.URL, 1000, time.Hour)
	ctx := t.Context()
	go n.Run(ctx)

	baseline := runtime.NumGoroutine()
	for range 500 {
		n.Add(NewWarning(WarningCategoryStall, WarningCritical, "incident", "test"))
	}
	assert.Less(t, runtime.NumGoroutine()-baseline, 20, "no goroutine per warning")
	require.Eventually(t, func() bool { return posts.Load() >= 1 }, 2*time.Second, 5*time.Millisecond,
		"a CRITICAL warning still flushes promptly")
	assert.LessOrEqual(t, posts.Load(), int64(2), "the burst is sent as one batch, not one request per warning")
}
