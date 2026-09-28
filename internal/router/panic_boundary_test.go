package router

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// panickyConsumer is a cascadeConsumer whose first panicNacks Nack calls
// (and first panicAcks Ack calls) panic, the way a broker client with a bug
// would. Later calls record normally.
type panickyConsumer struct {
	cascadeConsumer
	mu2        sync.Mutex
	panicNacks int
	panicAcks  int
}

func (c *panickyConsumer) Nack(ctx context.Context, rh string, delay *uint32) error {
	c.mu2.Lock()
	boom := c.panicNacks > 0
	if boom {
		c.panicNacks--
	}
	c.mu2.Unlock()
	if boom {
		panic("broker client bug in Nack")
	}
	return c.cascadeConsumer.Nack(ctx, rh, delay)
}

func (c *panickyConsumer) Ack(ctx context.Context, rh, bid string) error {
	c.mu2.Lock()
	boom := c.panicAcks > 0
	if boom {
		c.panicAcks--
	}
	c.mu2.Unlock()
	if boom {
		panic("broker client bug in Ack")
	}
	return c.cascadeConsumer.Ack(ctx, rh, bid)
}

// lockedBuffer is a bytes.Buffer safe to read while slog writes to it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// captureLogs sends slog's default logger to a buffer for the test.
func captureLogs(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

// A panic in the broker's Nack, on the ordered drainer's release path, used
// to unwind the drainer goroutine and take the whole process down (and, had
// it been recovered, leave the group's working flag set for good). It is now
// recovered with its stack logged, and the head and its buffered siblings
// all still go back to the broker; the group is left resumable.
func TestDrainerPanicIsRecoveredAndTheGroupHandedBack(t *testing.T) {
	logs := captureLogs(t)
	cons := &panickyConsumer{
		wantTotal: 3, done: make(chan struct{}),
		panicNacks: 1,
	}
	out := common.ErrorConnection("refused")
	med := &cascadeMediator{failID: "m1", failWith: &out}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })
	before := PanicsRecovered()

	submitBatch(context.Background(), pool, []common.QueuedMessage{
		releaseMsg("m1", "g", common.DispatchBlockOnError),
		releaseMsg("m2", "g", common.DispatchBlockOnError),
		releaseMsg("m3", "g", common.DispatchBlockOnError),
	})
	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("the group was not handed back after the drainer panicked")
	}

	cons.mu.Lock()
	nacked := append([]string(nil), cons.nacked...)
	cons.mu.Unlock()
	assert.ElementsMatch(t, []string{"m1", "m2", "m3"}, nacked, "no message may be lost to the panic")
	assert.Greater(t, PanicsRecovered(), before)
	assert.Eventually(t, func() bool { return pool.QueueSize() == 0 && pool.ActiveWorkers() == 0 },
		time.Second, 5*time.Millisecond, "pool bookkeeping must be back to zero")
	pool.mu.Lock()
	gq := pool.groupQs["g"]
	pool.mu.Unlock()
	if gq != nil {
		assert.False(t, gq.working, "the group must not be left owned by a dead drainer")
	}

	out2 := logs.String()
	assert.Contains(t, out2, `"where":"pool.drainGroup"`)
	assert.Contains(t, out2, `"stack":"goroutine `, "a recovered panic is logged with its stack")
	assert.Contains(t, out2, `"message_id":"m1"`)
}

// The IMMEDIATE path's goroutine has the same boundary: a panic outside
// processOne (here the release nack) is recovered and the message nacked.
func TestImmediateWorkerPanicIsRecoveredAndTheMessageNacked(t *testing.T) {
	captureLogs(t)
	cons := &panickyConsumer{
		wantTotal: 1, done: make(chan struct{}),
		panicNacks: 1,
	}
	out := common.ErrorConnection("refused")
	med := &cascadeMediator{failID: "m1", failWith: &out}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })

	submitBatch(context.Background(), pool, []common.QueuedMessage{releaseMsg("m1", "", common.DispatchImmediate)})
	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("the message was not nacked after the worker panicked")
	}
	assert.Eventually(t, func() bool { return pool.QueueSize() == 0 && pool.ActiveWorkers() == 0 },
		time.Second, 5*time.Millisecond)
}

// A panic in the mediator (the target's own code path) is recovered inside
// processOne, as before, but is now logged with its stack and the message's
// correlation set rather than just the panic value.
func TestProcessOnePanicLogsTheStack(t *testing.T) {
	logs := captureLogs(t)
	cons := &cascadeConsumer{}
	pool := grPool(&grMediator{panicMsg: "mediator bug"}, cons)
	d := pool.processOne(context.Background(), grMsg("px", "http://example.invalid"))
	assert.Equal(t, BrokerRetry, d.Action, "a mediator panic is retried in-pipeline")
	out := logs.String()
	assert.Contains(t, out, `"panic":"mediator bug"`)
	assert.Contains(t, out, `"stack":"goroutine `)
	assert.Contains(t, out, `"message_id":"px"`)
	assert.Contains(t, out, `"pool":"TEST"`)
}

// A consumer whose Poll panics becomes a poll error, not a dead process;
// the loop keeps polling.
func TestPollPanicBecomesAPollError(t *testing.T) {
	captureLogs(t)
	c := &pollPanicConsumer{}
	msgs, err := pollSafely(context.Background(), c, 10)
	require.Error(t, err)
	assert.Nil(t, msgs)
	assert.True(t, strings.Contains(err.Error(), "poll panicked"))
}

type pollPanicConsumer struct{ cascadeConsumer }

func (*pollPanicConsumer) Poll(context.Context, uint32) ([]common.QueuedMessage, error) {
	panic("receive bug")
}

// A panic while routing one message of a batch hands that message back and
// routes the rest.
func TestRoutePanicHandsTheMessageBackAndRoutesTheRest(t *testing.T) {
	captureLogs(t)
	m := newTestManager(t, &grMediator{outcome: common.Success(200)}, NewInFlightTracker())
	require.NoError(t, m.Reconfigure(context.Background(), routerCfg(nil)))
	// A pool whose submit panics for one message: the resolver is only
	// consulted on ack/nack, so make the panic come from submit itself by
	// routing to a pool whose semaphore is nil.
	src := &cascadeConsumer{wantTotal: 1, done: make(chan struct{})}
	bad := releaseMsg("bad", "", common.DispatchImmediate)
	bad.Message.PoolCode = "BROKEN"
	m.poolMu.Lock()
	m.pools["BROKEN"] = &Pool{cfg: common.PoolConfig{Code: "BROKEN"}} // bare: submit panics on nil sem
	m.poolMu.Unlock()
	good := releaseMsg("good", "", common.DispatchImmediate)

	fed := m.route(context.Background(), []common.QueuedMessage{bad, good}, src)
	assert.Equal(t, []string{defaultPoolCode}, fed, "the good message is still routed")
	src.mu.Lock()
	defer src.mu.Unlock()
	assert.Equal(t, []string{"bad"}, src.nacked, "the message whose routing panicked goes back to the broker")
	_, tracked := m.tracker.Lookup("bad")
	assert.False(t, tracked, "its tracker entry is released so the redelivery is not dropped as a duplicate")
}
