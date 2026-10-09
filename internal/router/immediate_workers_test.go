package router

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

func (ws *immediateWorkers) idleCount() int {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return len(ws.idle)
}

func workerPool(t *testing.T, concurrency uint32, med Mediator, c queue.Consumer) *Pool {
	t.Helper()
	p := NewPool(common.PoolConfig{Code: "TEST", Concurrency: concurrency}, med, NewInFlightTracker(),
		func(string) queue.Consumer { return c })
	t.Cleanup(p.Stop)
	return p
}

// peakMediator counts deliveries running at once and remembers the most it saw.
// Each delivery holds for hold, or until release is closed when hold is 0.
type peakMediator struct {
	hold    time.Duration
	release chan struct{}
	running atomic.Int64
	peak    atomic.Int64
	entered atomic.Int64
}

func (m *peakMediator) Mediate(context.Context, *common.Message) common.MediationOutcome {
	n := m.running.Add(1)
	m.entered.Add(1)
	for {
		p := m.peak.Load()
		if n <= p || m.peak.CompareAndSwap(p, n) {
			break
		}
	}
	if m.hold > 0 {
		time.Sleep(m.hold)
	} else if m.release != nil {
		<-m.release
	}
	m.running.Add(-1)
	return common.Success(200)
}

// One message at a time goes through one worker: the goroutine is reused, not
// started per message.
func TestImmediateWorkerIsReusedAcrossMessages(t *testing.T) {
	c := &grConsumer{id: "q1"}
	p := workerPool(t, 4, &peakMediator{}, c)

	for i := range 50 {
		p.submit(context.Background(), grMsg(fmt.Sprintf("m%d", i), "http://t/x"))
		grWaitFor(t, func() bool { return c.acks.Load() == int64(i+1) && p.workers.idleCount() == 1 }, 3*time.Second)
	}
	assert.Equal(t, uint64(1), p.workers.started.Load(), "fifty sequential messages must run on one worker")
	assert.Equal(t, int64(1), p.workers.live.Load())
}

// The worker set never lets more deliveries run than the pool's concurrency,
// and every message is delivered.
func TestImmediateWorkersHonourTheConcurrencyLimit(t *testing.T) {
	const concurrency, total = 8, 300 // total stays within the 320 the pool buffers
	med := &peakMediator{hold: time.Millisecond}
	c := &grConsumer{id: "q1"}
	p := workerPool(t, concurrency, med, c)

	var wg sync.WaitGroup
	for s := range 4 { // several submitters at once, as several poll loops would
		wg.Go(func() {
			for i := range total / 4 {
				p.submit(context.Background(), grMsg(fmt.Sprintf("s%d-m%d", s, i), "http://t/x"))
			}
		})
	}
	wg.Wait()
	grWaitFor(t, func() bool { return c.acks.Load() == total }, 10*time.Second)

	assert.LessOrEqual(t, med.peak.Load(), int64(concurrency), "deliveries running at once")
	assert.Equal(t, int64(concurrency), med.peak.Load(), "the pool should have reached its concurrency")
	assert.Equal(t, uint32(0), p.QueueSize())
	assert.LessOrEqual(t, p.workers.live.Load(), int64(2*concurrency),
		"workers are bounded by the messages in flight (deliveries plus acks), not by the messages submitted")
	assert.Less(t, p.workers.started.Load(), uint64(total/4), "workers must be reused")
}

// Ordered groups keep their per-group order with the IMMEDIATE workers busy
// beside them in the same pool.
func TestOrderedGroupsKeepOrderAlongsideImmediateWorkers(t *testing.T) {
	const groups, perGroup = 4, 30 // 150 messages, within the 160 the pool buffers
	var mu sync.Mutex
	seen := map[string][]int{}
	med := mediatorFunc(func(_ context.Context, m *common.Message) common.MediationOutcome {
		if g := m.GroupID(); g != "" {
			var n int
			_, _ = fmt.Sscanf(m.ID, g+"-%d", &n)
			mu.Lock()
			seen[g] = append(seen[g], n)
			mu.Unlock()
		}
		time.Sleep(100 * time.Microsecond)
		return common.Success(200)
	})
	c := &grConsumer{id: "q1"}
	p := workerPool(t, 4, med, c)

	for i := range perGroup {
		for g := range groups {
			group := fmt.Sprintf("g%d", g)
			m := grMsg(fmt.Sprintf("%s-%d", group, i), "http://t/x")
			m.Message.DispatchMode = common.DispatchNextOnError
			m.Message.MessageGroupID = &group
			p.submit(context.Background(), m)
		}
		p.submit(context.Background(), grMsg(fmt.Sprintf("imm-%d", i), "http://t/x"))
	}
	grWaitFor(t, func() bool { return c.acks.Load() == groups*perGroup+perGroup }, 10*time.Second)

	mu.Lock()
	defer mu.Unlock()
	for g := range groups {
		got := seen[fmt.Sprintf("g%d", g)]
		require.Len(t, got, perGroup)
		for i := range got {
			assert.Equal(t, i, got[i], "group g%d delivered out of order: %v", g, got)
		}
	}
}

type mediatorFunc func(context.Context, *common.Message) common.MediationOutcome

func (f mediatorFunc) Mediate(ctx context.Context, m *common.Message) common.MediationOutcome {
	return f(ctx, m)
}

// A delivery that panics, an ack that panics and a nack that panics each leave
// the worker alive and the slot free: at concurrency 1 the next message still
// goes through, on the same worker.
func TestImmediateWorkerSurvivesPanics(t *testing.T) {
	ctx := t.Context() // its end also ends the panicked messages' in-pipeline retry waits

	med := mediatorFunc(func(_ context.Context, m *common.Message) common.MediationOutcome {
		switch m.ID {
		case "boom-mediate":
			panic("target code panicked")
		case "boom-nack":
			return common.ErrorProcess(30, "gateway") // released to the broker: a nack
		}
		return common.Success(200)
	})
	c := &panicBrokerConsumer{id: "q1"}
	p := workerPool(t, 1, med, c)
	before := PanicsRecovered()

	step := func(id string, cond func() bool) {
		t.Helper()
		p.submit(ctx, grMsg(id, "http://t/x"))
		grWaitFor(t, func() bool { return cond() && p.workers.idleCount() == 1 }, 3*time.Second)
	}
	// processOne recovers these two and keeps the message for an in-pipeline
	// retry; the slot is free at once.
	step("boom-mediate", func() bool { return PanicsRecovered()-before == 1 })
	step("boom-ack", func() bool { return PanicsRecovered()-before == 2 })
	step("ok-1", func() bool { return c.acks.Load() == 1 })
	// The nack after a release panics outside processOne: runImmediate's own
	// boundary catches it (and its nackSafely retry panics too, contained).
	step("boom-nack", func() bool { return PanicsRecovered()-before == 4 })
	step("ok-2", func() bool { return c.acks.Load() == 2 })

	assert.Equal(t, uint64(1), p.workers.started.Load(), "the worker survives every panic")
	assert.Equal(t, uint32(0), p.ActiveWorkers())
	p.sem.mu.Lock()
	assert.Equal(t, uint32(0), p.sem.held, "no slot leaked")
	p.sem.mu.Unlock()
}

// panicBrokerConsumer panics on the ack of "boom-ack" and on every nack.
type panicBrokerConsumer struct {
	grConsumer
}

func (c *panicBrokerConsumer) Ack(ctx context.Context, receipt, brokerID string) error {
	if receipt == "rh-boom-ack" {
		panic("broker client panicked on ack")
	}
	return c.grConsumer.Ack(ctx, receipt, brokerID)
}

func (c *panicBrokerConsumer) Nack(context.Context, string, *uint32) error {
	panic("broker client panicked on nack")
}

// A panic in the dispatcher after it has been granted a slot (here: the
// capacity callback, which queueDec runs) gives the slot back and hands the
// message back. Before, the slot leaked, and at concurrency 1 the pool never
// delivered again.
func TestDispatcherPanicReleasesItsSlot(t *testing.T) {
	med := &peakMediator{release: make(chan struct{})}
	c := &grConsumer{id: "q1"}
	p := workerPool(t, 1, med, c)
	var panicked atomic.Bool
	p.SetCapacityFreed(func() {
		if panicked.CompareAndSwap(false, true) {
			panic("capacity callback panicked")
		}
	})

	// Fill the pool to its capacity (100 at concurrency 1) behind one held
	// delivery, so the next start crosses back under capacity and fires the
	// callback.
	p.submit(context.Background(), grMsg("holder", "http://t/x"))
	grWaitFor(t, func() bool { return med.entered.Load() == 1 }, 3*time.Second)
	waiting := int(p.queueCapacity())
	for i := range waiting {
		p.submit(context.Background(), grMsg(fmt.Sprintf("w%d", i), "http://t/x"))
	}
	require.Equal(t, uint32(waiting), p.QueueSize())

	close(med.release)
	grWaitFor(t, func() bool { return c.total() == int64(waiting)+1 }, 5*time.Second)
	assert.True(t, panicked.Load())
	assert.Equal(t, int64(1), c.nacks.Load(), "the message whose start panicked is handed back")
	assert.Equal(t, int64(waiting), c.acks.Load(), "every other message is delivered")
	p.sem.mu.Lock()
	held := p.sem.held
	p.sem.mu.Unlock()
	assert.Equal(t, uint32(0), held, "no slot leaked")
	assert.Equal(t, uint32(0), p.QueueSize())
}

// Stop lets idle workers go at once, without waiting out the idle timeout.
func TestStopReleasesIdleWorkers(t *testing.T) {
	med := &peakMediator{release: make(chan struct{})}
	c := &grConsumer{id: "q1"}
	p := workerPool(t, 8, med, c)
	p.workers.idleTimeout = time.Hour

	for i := range 8 {
		p.submit(context.Background(), grMsg(fmt.Sprintf("m%d", i), "http://t/x"))
	}
	grWaitFor(t, func() bool { return med.entered.Load() == 8 }, 3*time.Second)
	close(med.release)
	grWaitFor(t, func() bool { return c.acks.Load() == 8 && p.workers.idleCount() == 8 }, 3*time.Second)
	require.Equal(t, int64(8), p.workers.live.Load())

	p.Stop()
	grWaitFor(t, func() bool { return p.workers.live.Load() == 0 }, time.Second)
	assert.Equal(t, 0, p.workers.idleCount())
}

// A stopped pool still starts what is already queued (handing back what its
// consumer cancelled), on workers that leave right after.
func TestStoppedPoolStillDrainsItsQueueAndKeepsNoWorkers(t *testing.T) {
	med := &peakMediator{release: make(chan struct{})}
	c := &grConsumer{id: "q1"}
	p := workerPool(t, 1, med, c)
	p.workers.idleTimeout = time.Hour

	p.submit(context.Background(), grMsg("holder", "http://t/x"))
	grWaitFor(t, func() bool { return med.entered.Load() == 1 }, 3*time.Second)
	for i := range 5 {
		p.submit(context.Background(), grMsg(fmt.Sprintf("w%d", i), "http://t/x"))
	}
	p.Stop()
	close(med.release)
	grWaitFor(t, func() bool { return c.acks.Load() == 6 }, 3*time.Second)
	grWaitFor(t, func() bool { return p.workers.live.Load() == 0 }, time.Second)
}

// An idle pool sheds its workers once they have waited out the idle timeout.
func TestIdleWorkersExitAfterTheIdleTimeout(t *testing.T) {
	med := &peakMediator{release: make(chan struct{})}
	c := &grConsumer{id: "q1"}
	p := workerPool(t, 16, med, c)
	p.workers.idleTimeout = 50 * time.Millisecond

	for i := range 16 {
		p.submit(context.Background(), grMsg(fmt.Sprintf("m%d", i), "http://t/x"))
	}
	grWaitFor(t, func() bool { return med.entered.Load() == 16 }, 3*time.Second)
	assert.Equal(t, int64(16), p.workers.live.Load())
	close(med.release)
	grWaitFor(t, func() bool { return c.acks.Load() == 16 }, 3*time.Second)
	grWaitFor(t, func() bool { return p.workers.live.Load() == 0 }, 2*time.Second)

	// And the pool works as before afterwards.
	p.submit(context.Background(), grMsg("later", "http://t/x"))
	grWaitFor(t, func() bool { return c.acks.Load() == 17 }, 3*time.Second)
}

// A concurrency change at runtime needs no resize step: a raised cap starts
// workers as slots are granted, and after a cut the deliveries running at once
// stay within the new cap while the surplus workers go idle and time out.
func TestImmediateWorkersFollowAConcurrencyChange(t *testing.T) {
	med := &peakMediator{release: make(chan struct{})}
	c := &grConsumer{id: "q1"}
	p := workerPool(t, 2, med, c)
	p.workers.idleTimeout = 100 * time.Millisecond

	for i := range 40 {
		p.submit(context.Background(), grMsg(fmt.Sprintf("a%d", i), "http://t/x"))
	}
	grWaitFor(t, func() bool { return med.entered.Load() == 2 }, 3*time.Second)
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, int64(2), med.running.Load())

	require.True(t, p.UpdateConcurrency(6))
	grWaitFor(t, func() bool { return med.running.Load() == 6 }, 3*time.Second)
	assert.Equal(t, int64(6), p.workers.live.Load())

	// Cut to 1, then let the held deliveries finish: from then on only one
	// runs at a time.
	require.True(t, p.UpdateConcurrency(1))
	close(med.release)
	grWaitFor(t, func() bool { return c.acks.Load() == 40 }, 5*time.Second)
	// The six that were running finish; nothing new starts above 1.
	med2 := &peakMediator{hold: time.Millisecond}
	p.mediator = med2
	for i := range 30 {
		p.submit(context.Background(), grMsg(fmt.Sprintf("b%d", i), "http://t/x"))
	}
	grWaitFor(t, func() bool { return c.acks.Load() == 70 }, 5*time.Second)
	assert.Equal(t, int64(1), med2.peak.Load(), "after the cut, one delivery at a time")
	grWaitFor(t, func() bool { return p.workers.live.Load() == 0 }, 2*time.Second)
}
