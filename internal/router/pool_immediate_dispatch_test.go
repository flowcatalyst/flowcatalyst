package router

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// An unordered message waiting for a concurrency slot must cost no goroutine.
//
// The pool buffers up to concurrency*40 messages. When each one parked its own
// goroutine on the semaphore, 100 pools of concurrency 64 held 256,000
// goroutines — about 1.2GB of stack — and the router stalled and was killed at
// its memory limit. The goroutine count must track the messages in flight, not
// the messages buffered.
func TestWaitingUnorderedMessagesDoNotCostAGoroutineEach(t *testing.T) {
	const (
		concurrency = 100
		waiting     = 3000 // within the 4,000 buffer, far above the slots
	)
	med := &gateMediator{release: make(chan struct{})}
	c := &grConsumer{id: "q1"}
	p := NewPool(common.PoolConfig{Code: "TEST", Concurrency: concurrency}, med, NewInFlightTracker(),
		func(string) queue.Consumer { return c })

	before := runtime.NumGoroutine()
	for i := range concurrency + waiting {
		p.submit(context.Background(), grMsg(fmt.Sprintf("m%d", i), "http://t/x"))
	}
	grWaitFor(t, func() bool { return med.entered.Load() == concurrency }, 3*time.Second)

	grWaitFor(t, func() bool { return p.QueueSize() == waiting }, 3*time.Second)
	grew := runtime.NumGoroutine() - before
	assert.Less(t, grew, 3*concurrency,
		"%d messages waiting for a slot grew the goroutine count by %d; it should track the %d in flight, not the buffer",
		waiting, grew, concurrency)
	assert.Equal(t, uint32(concurrency), p.ActiveWorkers())

	close(med.release)
	grWaitFor(t, func() bool { return c.acks.Load() == concurrency+waiting }, 10*time.Second)
	assert.Equal(t, uint32(0), p.QueueSize(), "every waiting message was started and finished")
}

// gateMediator holds every delivery until release is closed and counts how many
// have entered, without the bounded channel blockingMediator uses (which stalls
// once its buffer fills).
type gateMediator struct {
	entered atomic.Int64
	release chan struct{}
}

func (g *gateMediator) Mediate(context.Context, *common.Message) common.MediationOutcome {
	g.entered.Add(1)
	<-g.release
	return common.Success(200)
}

// Waiting messages run in the order they arrived.
func TestWaitingUnorderedMessagesStartOldestFirst(t *testing.T) {
	order := make(chan string, 16)
	med := &orderingMediator{order: order, release: make(chan struct{})}
	c := &grConsumer{id: "q1"}
	p := NewPool(common.PoolConfig{Code: "TEST", Concurrency: 1}, med, NewInFlightTracker(),
		func(string) queue.Consumer { return c })

	for i := range 6 {
		p.submit(context.Background(), grMsg(fmt.Sprintf("m%d", i), "http://t/x"))
	}
	for range 6 {
		med.release <- struct{}{}
	}
	grWaitFor(t, func() bool { return c.acks.Load() == 6 }, 3*time.Second)
	close(order)
	var got []string
	for id := range order {
		got = append(got, id)
	}
	assert.Equal(t, []string{"m0", "m1", "m2", "m3", "m4", "m5"}, got)
}

type orderingMediator struct {
	order   chan string
	release chan struct{}
}

func (o *orderingMediator) Mediate(_ context.Context, m *common.Message) common.MediationOutcome {
	o.order <- m.ID
	<-o.release
	return common.Success(200)
}

// A consumer whose context is cancelled gets its waiting messages handed back —
// promptly, without each needing a slot — and none stay counted in the queue.
func TestCancelledConsumerContextHandsWaitingMessagesBack(t *testing.T) {
	const waiting = 50
	med := newBlockingMediator(1)
	c := &grConsumer{id: "q1"}
	p := NewPool(common.PoolConfig{Code: "TEST", Concurrency: 1}, med, NewInFlightTracker(),
		func(string) queue.Consumer { return c })

	// One message holds the only slot; the rest wait behind it, under a context
	// we then cancel.
	p.submit(context.Background(), grMsg("holder", "http://t/x"))
	med.awaitEntered(t, 1)

	ctx, cancel := context.WithCancel(context.Background())
	for i := range waiting {
		p.submit(ctx, grMsg(fmt.Sprintf("w%d", i), "http://t/x"))
	}
	require.Equal(t, uint32(waiting), p.QueueSize())

	cancel()
	close(med.release) // free the slot so the dispatcher moves past the head

	grWaitFor(t, func() bool { return c.nacks.Load() == waiting && c.acks.Load() == 1 }, 3*time.Second)
	assert.Equal(t, uint32(0), p.QueueSize(), "cancelled messages must not stay counted as queued")
}
