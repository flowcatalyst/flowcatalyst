package router

// Regression tests for the consumer poll-loop's handling of a stopped /
// erroring consumer. The bug these guard against: a consumer whose underlying
// queue was Stop()'d (but whose poll loop wasn't torn down) spun ~once a second
// forever logging "Error polling: Queue is stopped", and — because the loop
// stamped its watchdog heartbeat on every poll, including errored ones — looked
// alive to RestartStalledConsumers, so it was never rebuilt and the queue was
// never drained.

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// pollErrConsumer is a fake queue.Consumer whose Poll returns a fixed error and
// counts calls.
type pollErrConsumer struct {
	id    string
	err   error
	polls atomic.Int64
}

func (c *pollErrConsumer) Identifier() string { return c.id }
func (c *pollErrConsumer) Poll(context.Context, uint32) ([]common.QueuedMessage, error) {
	c.polls.Add(1)
	return nil, c.err
}
func (c *pollErrConsumer) Ack(context.Context, string) error                      { return nil }
func (c *pollErrConsumer) Nack(context.Context, string, *uint32) error            { return nil }
func (c *pollErrConsumer) Defer(context.Context, string, *uint32) error           { return nil }
func (c *pollErrConsumer) ExtendVisibility(context.Context, string, uint32) error { return nil }
func (c *pollErrConsumer) Healthy() bool                                          { return true }
func (c *pollErrConsumer) Stop()                                                  {}
func (c *pollErrConsumer) Metrics(context.Context) (*queue.Metrics, error)        { return nil, nil }
func (c *pollErrConsumer) Counters() *queue.Metrics                               { return nil }

// managerWithCapacity builds a Manager with a default pool that reports
// capacity, so runConsumer proceeds to Poll.
func managerWithCapacity() *Manager {
	m := NewManager(nil, nil)
	m.pools[defaultPoolCode] = NewPool(
		common.PoolConfig{Code: defaultPoolCode, Concurrency: 8},
		nil, nil, func(string) queue.Consumer { return nil },
	)
	return m
}

// A consumer reporting queue.ErrStopped must make runConsumer exit promptly (so
// the restart watchdog respawns it) instead of spinning on the dead consumer,
// and must not advance the heartbeat.
func TestRunConsumerExitsWhenConsumerStopped(t *testing.T) {
	m := managerWithCapacity()
	rc := &runningConsumer{consumer: &pollErrConsumer{id: "q-high.fifo", err: queue.ErrStopped}, cancel: func() {}}
	const sentinel = int64(12345)
	rc.lastPoll.Store(sentinel)

	m.wg.Add(1)
	done := make(chan struct{})
	go func() { m.runConsumer(context.Background(), rc); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runConsumer did not exit on queue.ErrStopped — it is spinning on a dead consumer")
	}
	if got := rc.lastPoll.Load(); got != sentinel {
		t.Fatalf("heartbeat advanced on a stopped poll (got %d, want %d) — a wedged consumer would look alive to the watchdog", got, sentinel)
	}
}

// A non-terminal poll error must NOT advance the heartbeat, so a consumer stuck
// erroring goes stale and the watchdog rebuilds it. The loop keeps retrying
// until the context is cancelled.
func TestRunConsumerNoHeartbeatOnPollError(t *testing.T) {
	m := managerWithCapacity()
	fake := &pollErrConsumer{id: "q.fifo", err: errors.New("transient boom")}
	rc := &runningConsumer{consumer: fake, cancel: func() {}}
	const sentinel = int64(999)
	rc.lastPoll.Store(sentinel)

	ctx, cancel := context.WithCancel(context.Background())
	m.wg.Add(1)
	done := make(chan struct{})
	go func() { m.runConsumer(ctx, rc); close(done) }()

	// Wait until at least one poll happened, then stop the loop.
	deadline := time.Now().Add(2 * time.Second)
	for fake.polls.Load() == 0 {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("consumer never polled")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runConsumer did not exit after context cancel")
	}
	if got := rc.lastPoll.Load(); got != sentinel {
		t.Fatalf("heartbeat advanced on an errored poll (got %d, want %d)", got, sentinel)
	}
}
