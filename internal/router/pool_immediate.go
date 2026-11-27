package router

import (
	"context"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// immediateItem is one waiting unordered message with the context it must
// run under — the submitting consumer's, which a reconfigure or a stalled-
// consumer restart cancels while the pool lives on.
type immediateItem struct {
	ctx context.Context
	m   common.QueuedMessage
}

// enqueueImmediate files an unordered message behind those already waiting and
// makes sure a dispatcher is running to start them. queueSize already counts
// m (the caller did queueInc).
//
// The dispatcher is not a permanent goroutine: it exits when the queue is
// empty and the next enqueue starts it again, so an idle pool holds none and
// there is nothing to stop.
func (p *Pool) enqueueImmediate(ctx context.Context, m common.QueuedMessage) {
	p.immMu.Lock()
	p.immQ = append(p.immQ, immediateItem{ctx: ctx, m: m})
	start := !p.immRunning
	if start {
		p.immRunning = true
	}
	p.immMu.Unlock()
	if start {
		go p.dispatchImmediate()
	}
}

// nextImmediate removes and returns the oldest waiting message, or reports
// that none is left (marking the dispatcher stopped, under the same lock, so a
// concurrent enqueueImmediate cannot be missed).
func (p *Pool) nextImmediate() (immediateItem, bool) {
	p.immMu.Lock()
	defer p.immMu.Unlock()
	if p.immHead >= len(p.immQ) {
		p.immQ, p.immHead = nil, 0
		p.immRunning = false
		return immediateItem{}, false
	}
	it := p.immQ[p.immHead]
	p.immQ[p.immHead] = immediateItem{} // drop the references
	p.immHead++
	// Compact once the consumed prefix is both large and most of the slice, so
	// the backing array does not grow for ever under a steady stream.
	if p.immHead >= 4096 && p.immHead*2 >= len(p.immQ) {
		p.immQ = append(p.immQ[:0], p.immQ[p.immHead:]...)
		p.immHead = 0
	}
	return it, true
}

// dispatchImmediate starts waiting unordered messages one at a time, oldest
// first, each as soon as a concurrency slot is free. It owns the wait for the
// slot, so the number of goroutines is the number of messages in flight, not
// the number buffered.
func (p *Pool) dispatchImmediate() {
	for {
		it, ok := p.nextImmediate()
		if !ok {
			return
		}
		p.startImmediate(it)
	}
}

// startImmediate waits for a slot for it, then hands the message to a worker
// (workers.run). A message whose consumer context is already cancelled never
// takes a slot: it is handed straight back, so a cancelled consumer's messages
// drain through the head of the queue without waiting for slots they will not
// use.
func (p *Pool) startImmediate(it immediateItem) {
	queued := true // it.m still counts in queueSize
	held := false  // this goroutine holds a slot nobody else will release
	// Panic boundary for the dispatcher: a broker nack, the semaphore or the
	// capacity callback panicking must not take the process down, strand the
	// message or leak the slot it was granted.
	defer func() {
		if r := recover(); r != nil {
			logRecovered("pool.startImmediate", r, p.msgAttrs(it.m)...)
			if held {
				p.sem.release()
			}
			if queued {
				p.queueDec()
			}
			p.nackSafely(it.ctx, it.m, new(siblingNackDelaySeconds), "dispatcher panicked")
		}
	}()
	// Each flag is cleared BEFORE the call that settles it, so a panic inside
	// that call (queueDec runs the capacity callback) cannot undo it twice.
	if it.ctx.Err() != nil {
		queued = false
		p.queueDec()
		p.nackMsg(it.ctx, it.m, new(uint32(10)), "shutdown before dispatch")
		return
	}
	if err := p.sem.acquire(it.ctx); err != nil {
		// Shutdown before we could start. nackMsg releases the route-time
		// tracker entry so the broker's redelivery (NACK is a no-op on SQS;
		// the message reappears after the visibility timeout) re-enters the
		// pipeline as a fresh copy instead of being dropped as a duplicate.
		queued = false
		p.queueDec()
		p.nackMsg(it.ctx, it.m, new(uint32(10)), "shutdown before dispatch")
		return
	}
	held = true
	// The slot can be granted in the same instant the consumer's context is
	// cancelled (the wait's select may pick either). A message whose consumer is
	// gone must be handed back, not run under a dead context, so check again.
	if it.ctx.Err() != nil {
		held = false
		p.sem.release()
		queued = false
		p.queueDec()
		p.nackMsg(it.ctx, it.m, new(uint32(10)), "shutdown before dispatch")
		return
	}
	queued = false
	p.queueDec() // now active, not queued
	// The worker owns the slot from here: runImmediate releases it on every
	// path. The hand-off cannot fail (a buffered send to a worker that is
	// waiting for exactly this, or a go statement).
	held = false
	p.workers.run(p, it)
}

// runImmediate processes one unordered message that already holds a
// concurrency slot (startImmediate acquired it, and this releases it). IMMEDIATE
// messages have no group buffer, so a retryable failure re-queues the same
// message after the backoff, keeping it in-pipeline rather than releasing it to
// the broker.
func (p *Pool) runImmediate(ctx context.Context, m common.QueuedMessage) {
	// Panic boundary. processOne recovers its own panics (the mediator is
	// the target's code); this catches one anywhere else on this goroutine
	// — a broker Nack — which would otherwise take the whole process down.
	// The message is handed back to the broker rather than dropped: its
	// tracker entry is released by nackMsg, so the redelivery re-enters the
	// pipeline.
	defer func() {
		if r := recover(); r != nil {
			logRecovered("pool.runImmediate", r, p.msgAttrs(m)...)
			p.nackSafely(ctx, m, new(siblingNackDelaySeconds), "worker panicked")
		}
	}()
	d := func() Disposition {
		slot := &slotRelease{p: p}
		defer slot.release() // on every exit path (acquired by the caller); a no-op once released before the ack
		return p.processOneReleasing(ctx, m, slot)
	}()
	if d.Action == BrokerRelease {
		// Target unreachable, or the in-pipeline retry budget is spent.
		// IMMEDIATE mode has no group buffer, so there is nothing behind this
		// message to release with it — hand just this one back and let the
		// broker redeliver after d.RetryAfter: the outcome's own delay (30s for
		// an unreachable target, the breaker's reset for an open breaker, the
		// backoff when the budget is spent), never an immediate redelivery.
		p.events.reject(RejectReleased, 1)
		p.nackMsg(ctx, m, nackDelay(d.RetryAfter), "released to broker")
		return
	}
	if d.Action != BrokerRetry {
		return
	}
	// Retry in-pipeline: wait out the backoff, then queue it again. The
	// in-flight tracker entry is kept (so redeliveries are deduped against it),
	// and Attempts grows the backoff and tells processOne not to re-track.
	//
	// The retry goroutine captures its own copy: capturing (and mutating) m
	// itself would move the whole message to the heap on EVERY call, while a
	// retry is the rare path.
	retry := m
	retry.Attempts++
	p.queueInc() // re-queued (pre-dispatch) for the duration of the backoff
	go func() {
		select {
		case <-ctx.Done():
			// Shutdown/consumer-restart mid-backoff: the message leaves the
			// pipeline (IMMEDIATE mode has no buffer to park in), so release
			// its tracker entry — otherwise every future redelivery would be
			// dropped as a duplicate of a copy that no longer exists, and the
			// message would cycle on the broker untouchable until retention.
			p.queueDec()
			if p.tracker != nil {
				p.tracker.Remove(retry.Message.ID, retry.BrokerMessageID)
			}
			return
		case <-time.After(d.RetryAfter):
		}
		p.enqueueImmediate(ctx, retry)
	}()
}
