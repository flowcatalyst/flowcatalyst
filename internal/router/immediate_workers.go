package router

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// immediateWorkerIdleTimeout is how long an IMMEDIATE worker waits for its
// next message before it exits. It keeps a busy pool's workers alive between
// one message and the next, and still lets a pool that has gone quiet shed
// them all within seconds.
const immediateWorkerIdleTimeout = 2 * time.Second

// immediateWorkers runs a pool's IMMEDIATE deliveries on reused goroutines.
//
// Each delivery used to start a goroutine of its own. A goroutine starts on a
// small stack, and a delivery is deep (the HTTP/2 client's RoundTrip below the
// mediator), so every one of them grew its stack, copying it each time it
// doubled: 7-8% of the router's CPU (docs/router-scaling-results.md). A worker
// that has already run a delivery keeps the stack it grew, so the next one
// runs without the copies.
//
// The set is elastic rather than fixed, which is what keeps every existing
// limit as it was:
//   - The dispatcher acquires a pool slot first and only then calls run, so
//     the number of deliveries running is still exactly what the semaphore
//     allows. Workers never decide concurrency; they only carry it out.
//   - run hands the message to an idle worker if there is one and starts a new
//     one only if there is none. So the set is never larger than the most
//     messages that have been in runImmediate at once (a message stays there
//     through its ack, after it gave its slot back), which is the number of
//     goroutines the old scheme had at that moment. A concurrency change at runtime needs no
//     resize: a grown cap starts workers as slots are granted, and after a
//     shrink the workers the pool no longer needs sit idle and time out.
//   - An idle worker exits after immediateWorkerIdleTimeout, and at once when
//     the pool stops. A quiet pool ends up holding no goroutine, as before.
//
// Idle workers are reused last-in first-out. Under light load the same few
// workers take every message and the rest time out; first-in first-out would
// cycle through all of them and keep every one alive.
type immediateWorkers struct {
	mu   sync.Mutex
	idle []*immediateWorker // waiting for a message; the most recent last

	// idleTimeout overrides immediateWorkerIdleTimeout when positive (tests).
	idleTimeout time.Duration

	live    atomic.Int64  // workers running now, idle or busy
	started atomic.Uint64 // workers ever started
}

// immediateWorker is one worker's mailbox. Its capacity of one is enough: a
// worker receives exactly one message per time run takes it off the idle list,
// and it is back on that list only once it is waiting again.
type immediateWorker struct {
	work chan immediateItem
}

// run delivers it on a worker. The caller has acquired a slot for it, and
// runImmediate gives that slot back on every path. run never blocks and
// cannot fail.
func (ws *immediateWorkers) run(p *Pool, it immediateItem) {
	ws.mu.Lock()
	if n := len(ws.idle); n > 0 {
		w := ws.idle[n-1]
		ws.idle[n-1] = nil
		ws.idle = ws.idle[:n-1]
		ws.mu.Unlock()
		w.work <- it
		return
	}
	ws.mu.Unlock()
	ws.live.Add(1)
	ws.started.Add(1)
	go ws.loop(p, it)
}

// loop is one worker: deliver, wait for the next message, and exit once none
// comes within the idle timeout or the pool stops.
//
// runImmediate is the panic boundary for each delivery: it recovers whatever
// a delivery or its ack throws, hands the message back, and releases the slot,
// so the worker always comes back here and stays usable.
func (ws *immediateWorkers) loop(p *Pool, it immediateItem) {
	defer ws.live.Add(-1)
	var reserve [workerStackReserve]byte
	holdStackReserve(&reserve)
	w := &immediateWorker{work: make(chan immediateItem, 1)}
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		p.runImmediate(it.ctx, it.m)
		it = immediateItem{} // hold no message, or its context, while idle
		if !ws.park(p, w) {
			return
		}
		d := ws.timeout()
		if timer == nil {
			timer = time.NewTimer(d)
		} else {
			timer.Reset(d)
		}
		select {
		case it = <-w.work:
			timer.Stop()
			continue
		case <-timer.C:
		case <-p.stopCh:
		}
		if ws.unpark(w) {
			return
		}
		// run took this worker off the idle list at the same moment, so a
		// message is on its way and this worker must deliver it.
		it = <-w.work
	}
}

// park puts w on the idle list. It reports false, and leaves w off the list,
// once the pool has stopped: a stopped pool keeps no idle workers. Messages
// still queued then each get a fresh worker, which leaves after that one.
func (ws *immediateWorkers) park(p *Pool, w *immediateWorker) bool {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if p.stopped.Load() {
		return false
	}
	ws.idle = append(ws.idle, w)
	return true
}

// unpark takes w off the idle list and reports whether it was still there.
// false means run has already claimed it.
func (ws *immediateWorkers) unpark(w *immediateWorker) bool {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	// The idle list is short (at most the pool's concurrency), and a worker
	// that timed out is the oldest, so it is near the front.
	if i := slices.Index(ws.idle, w); i >= 0 {
		ws.idle = slices.Delete(ws.idle, i, i+1)
		return true
	}
	return false
}

// workerStackReserve is stack a worker keeps in use for its whole life, so
// that the garbage collector leaves the stack its deliveries grew alone.
//
// The collector halves a goroutine's stack when it finds the goroutine using
// less than a quarter of it. A worker spends most of its time shallow: idle,
// or waiting for its ack after the delivery returned. So without the reserve
// almost every collection shrank every worker's stack, and the next delivery
// grew it back with the same copying that one goroutine per message paid
// (measured: two thirds of the saving lost). A delivery needs 8-10KB of stack,
// so a worker runs on 16KB; a 4KB reserve in loop's frame is a quarter of
// that, which is the most the collector leaves unshrunk. A rare deeper
// delivery may still grow one worker to 32KB, and that is shrunk back as
// before.
const workerStackReserve = 4 << 10

// holdStackReserve keeps the reserve in its caller's frame: a local array
// whose address goes to a call the compiler cannot see into cannot be
// optimised away, and it does not escape to the heap.
//
//go:noinline
func holdStackReserve(*[workerStackReserve]byte) {}

func (ws *immediateWorkers) timeout() time.Duration {
	if ws.idleTimeout > 0 {
		return ws.idleTimeout
	}
	return immediateWorkerIdleTimeout
}
