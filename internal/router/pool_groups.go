package router

import (
	"context"
	"log/slog"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// groupQueue is the per-message-group buffer: a single strict FIFO. A message
// group is an ordering contract, so there is deliberately NO priority lane —
// letting a "high priority" message jump ahead of an earlier one in the same
// group would defeat in-order delivery. (Message.HighPriority is a queue-level
// concern, not an intra-group one, and does not reorder here.) On a retryable
// failure the drainer re-inserts the message at the FRONT (enqueueFront) so the
// failed message is the next one attempted — never overtaken by a later one.
type groupQueue struct {
	msgs    []common.QueuedMessage
	working bool
	// parkedAt is when the group was last left buffered with NO drainer
	// (clearWorking). Zero while a drainer owns it. A parked group is normally
	// resumed within a broker redelivery, so a parkedAt that keeps ageing is
	// the signature of a group nothing is coming back for — which is what
	// ReleaseParkedGroups sweeps.
	parkedAt time.Time
	// gated is set when a member was admitted through the hand-back gate
	// (submitRunGated: its broker does not keep a group in order), and makes
	// a release of the group list what it hands back with the gate
	// (takeGroupForRelease). Never set for a group whose members all come
	// from a broker that keeps a group in order.
	gated bool
}

// pop returns the next message to dispatch (FIFO) and whether the queue is now
// empty. Caller holds p.mu.
func (gq *groupQueue) pop() (common.QueuedMessage, bool) {
	m := gq.msgs[0]
	gq.msgs = gq.msgs[1:]
	return m, len(gq.msgs) == 0
}

// empty reports whether the queue holds no pending messages. Caller
// holds p.mu.
func (gq *groupQueue) empty() bool {
	return len(gq.msgs) == 0
}

// submit routes one polled message. It runs capacity backpressure, then
// branches on DispatchMode: IMMEDIATE-mode messages (the default,
// RequiresOrdering()==false) dispatch concurrently via runImmediate (one
// delivery per worker at a time, bounded only by the pool semaphore), while ordered modes enqueue
// into the per-group FIFO buffer and drain serially. Retryable failures are
// retried in-pipeline (see processOne / drainGroup / runImmediate), so ordering
// is preserved by re-inserting a failed message at the FRONT of its group
// rather than by cascade-NACKing the rest of a batch.
func (p *Pool) submit(ctx context.Context, m common.QueuedMessage) {
	if p.gatingConsumer(m) != nil {
		// An ordered message from a broker that does not keep a group in
		// order: admitted through the hand-back gate, as a run of one.
		p.submitRun(ctx, m.Message.GroupID(), []common.QueuedMessage{m})
		return
	}
	p.events.submitted.Add(1)
	// Reject when the pool is stopping or draining (X-11: a removed pool
	// admits nothing new, but — unlike stopped — draining does not touch
	// what's already buffered; see Drain's doc comment). In practice a
	// draining pool is never reachable here at all: Manager.Reconfigure
	// removes it from the routing map in the same step that starts the
	// drain, so no new batch is ever routed to it. This check is the
	// pool-local backstop for a caller holding a stale *Pool reference.
	if p.stopped.Load() || p.draining.Load() {
		p.events.reject(RejectStopped, 1)
		p.nackMsg(ctx, m, new(uint32(10)), "pool stopped")
		return
	}
	// Capacity backpressure: hand the message back, to return when the
	// pool's admission schedule expects to have room for it (see
	// pool_admission.go). This is the ordinary path for a full pool now that
	// a consumer keeps polling while any of its queue's pools has room — not
	// a race backstop — so the delay is computed, not a flat 10s.
	if p.queueSize.Load() >= p.queueCapacity() {
		p.events.reject(RejectCapacity, 1)
		p.deferMsg(ctx, m, "pool at capacity")
		return
	}

	group := m.Message.GroupID()
	if !m.Message.DispatchMode.RequiresOrdering() || group == "" {
		// No ordering to keep — either the mode says so, or the message names
		// no group, which is the same thing: ordering is only ever defined
		// RELATIVE TO A GROUP. Dispatch concurrently.
		//
		// The group check is not a nicety. Ungrouped messages all share the
		// group key "", so treating them as ordered would file every one of
		// them into a single buffer behind a single drainer — the whole pool
		// reduced to one message at a time, however high its concurrency. That
		// is the shape of the trap that comes with an ordered default mode:
		// most producers set no message group, and they must not be silently
		// serialised into one queue for asking for nothing.
		//
		// queueSize is incremented here and decremented once the worker holds a
		// semaphore slot, so the "queued (pre-dispatch)" gauge mirrors the
		// ordered path.
		p.queueInc()
		p.enqueueImmediate(ctx, m)
		return
	}

	if !p.enqueue(group, m) {
		// Raced with Stop: the buffer is flushed and nothing will drain it.
		p.events.reject(RejectStopped, 1)
		p.nackMsg(ctx, m, new(uint32(10)), "pool stopped")
		return
	}
	p.tryDrainGroup(ctx, group)
}

// submitRun admits the members of one ordered group from one poll batch, in
// batch order, as a unit: they enter the group's buffer under one hold of
// p.mu, and only then is a drainer started. Manager.route uses it in place of
// submit for every ordered message (see route's doc comment for the defect
// this closes). Every member is buffered behind the head before the head can
// be attempted, so a release or a BLOCK_ON_ERROR ack of the group takes all
// of them with it.
//
// The admission checks are submit's, applied to the run in order: a stopped
// or draining pool hands every member back, and a pool short of room admits
// the members it has room for and defers the rest, as submitting them one at
// a time would.
//
// A run from a broker that does not keep a group in order itself
// (HonoursDelayedReturn false) is admitted through the hand-back gate
// instead (submitRunGated, pool_handback_gate.go).
func (p *Pool) submitRun(ctx context.Context, group string, run []common.QueuedMessage) {
	p.events.submitted.Add(uint64(len(run)))
	if p.stopped.Load() || p.draining.Load() {
		p.events.reject(RejectStopped, len(run))
		for _, m := range run {
			p.nackMsg(ctx, m, new(uint32(10)), "pool stopped")
		}
		return
	}
	if len(run) == 0 {
		return
	}
	// Every member of a run comes from the one consumer that polled it, so
	// the first member's source answers for the run.
	if c := p.gatingConsumer(run[0]); c != nil {
		p.submitRunGated(ctx, c, group, run)
		return
	}
	room := 0
	if size, capacity := p.queueSize.Load(), p.queueCapacity(); size < capacity {
		room = int(capacity - size)
	}
	admit := run[:min(room, len(run))]
	for _, m := range run[len(admit):] {
		p.events.reject(RejectCapacity, 1)
		p.deferMsg(ctx, m, "pool at capacity")
	}
	if len(admit) == 0 {
		return
	}
	if !p.enqueue(group, admit...) {
		// Raced with Stop: the buffer is flushed and nothing will drain it.
		p.events.reject(RejectStopped, len(admit))
		for _, m := range admit {
			p.nackMsg(ctx, m, new(uint32(10)), "pool stopped")
		}
		return
	}
	p.tryDrainGroup(ctx, group)
}

// enqueue appends newly-arrived messages, in order, to the BACK of their
// group's FIFO, all under one hold of p.mu so a drainer sees all of them or
// none. Returns false without buffering when the pool has stopped. That is
// checked under p.mu so it can't race Stop's buffer flush and strand a
// message (with a live tracker entry) in an abandoned buffer.
//
// queueSize is raised under the same hold of p.mu as the append (see
// countBufferedLocked), and capacityChanged runs after it is let go.
func (p *Pool) enqueue(group string, msgs ...common.QueuedMessage) bool {
	p.mu.Lock()
	if p.stopped.Load() {
		p.mu.Unlock()
		return false
	}
	gq := p.groupLocked(group)
	gq.msgs = append(gq.msgs, msgs...)
	p.countBufferedLocked(len(msgs))
	p.mu.Unlock()
	p.capacityChanged()
	return true
}

// groupLocked returns the group's entry, making it if there is none. Caller
// holds p.mu.
func (p *Pool) groupLocked(group string) *groupQueue {
	gq, ok := p.groupQs[group]
	if !ok {
		gq = &groupQueue{}
		p.groupQs[group] = gq
	}
	return gq
}

// enqueueFront puts a message back at the HEAD of its group's FIFO so that a
// retry is the NEXT message attempted — never overtaken by a later message in
// the same group. Used only by the ordered drainer on a retryable failure or
// a cancellation park. Same stopped-pool and counting contract as enqueue.
func (p *Pool) enqueueFront(group string, m common.QueuedMessage) bool {
	p.mu.Lock()
	if p.stopped.Load() {
		p.mu.Unlock()
		return false
	}
	gq, ok := p.groupQs[group]
	if !ok {
		gq = &groupQueue{}
		p.groupQs[group] = gq
	}
	gq.msgs = append([]common.QueuedMessage{m}, gq.msgs...)
	p.countBufferedLocked(1)
	p.mu.Unlock()
	p.capacityChanged()
	return true
}

// countBufferedLocked raises queueSize by n for messages just put in a group
// buffer. The caller holds p.mu and calls capacityChanged once it has let it
// go.
//
// The increment must land before p.mu is released. Every message leaves a
// group buffer under p.mu (a drainer's pop, takeGroup, Stop's flush) and is
// counted down only after that, so counting up under the lock that put it
// there is what keeps queueSize at or above what is really buffered. Counted
// after the unlock, a drainer could pop the message and count it down first,
// and the Uint32 wrapped to about 4 billion until the increment landed: the
// pool read as over capacity, submit deferred messages it had room for, and
// capacityChanged flipped full and later fired capacityFreed for a pool that
// had never been full.
//
// capacityChanged is not called here because it may run capacityFreed, which
// belongs to the Manager and may take its locks; nothing may be called out to
// with p.mu held.
func (p *Pool) countBufferedLocked(n int) {
	p.queueSize.Add(uint32(n)) //nolint:gosec // G115: n is a batch length, far below 2^32
}

// tryDrainGroup starts a serial drainer for an ordered message group if
// none is running. Only ordered-mode messages (NEXT_ON_ERROR /
// BLOCK_ON_ERROR) reach here — IMMEDIATE-mode messages dispatch
// concurrently via runImmediate. The drainer processes one message per
// group at a time to preserve FIFO order, bounded across groups by `sem`.
func (p *Pool) tryDrainGroup(ctx context.Context, group string) {
	p.mu.Lock()
	gq := p.groupQs[group]
	if gq == nil || gq.working || gq.empty() {
		p.mu.Unlock()
		return
	}
	gq.working = true
	gq.parkedAt = time.Time{}
	p.mu.Unlock()

	go p.drainGroup(ctx, group)
}

// drainGroup is the per-message-group worker goroutine spawned by
// tryDrainGroup. Drains one message at a time from gq.msgs (preserving
// FIFO order within the group), gated by the pool-wide `sem` semaphore.
//
// Exit conditions:
//   - the group buffer is empty (the groupQs entry is removed).
//   - the group was released or, under BLOCK_ON_ERROR, acked away
//     (takeGroup removes the entry with what it held).
//   - ctx is cancelled while waiting for a semaphore slot or sitting out a
//     retry backoff (the in-hand message is re-fronted and the working flag
//     cleared so a replacement drainer resumes the group — spawned by the
//     next submit or by Manager.route's redelivery-dedup kick).
//   - the pool stopped (buffer flushed): the in-hand message is released to
//     the broker via nackMsg instead of parked.
//
// Note: ctx cancellation between processOne calls does NOT stop the loop
// — only the semaphore-acquire and backoff selects are ctx-aware. This is
// intentional; a cancellation mid-process is handled inside processOne /
// mediator.
//
// The ctx here belongs to the SUBMITTING CONSUMER, not the pool: a
// queue-config reconfigure or stalled-consumer restart cancels it while the
// pool — and this group's buffer — live on. Every cancellation exit must
// therefore leave the group resumable: message back in the buffer, working
// flag off. A bare return with working still true wedges the group
// permanently (tryDrainGroup will never spawn another drainer).
func (p *Pool) drainGroup(ctx context.Context, group string) {
	// inHand is the message this drainer has popped and not yet resolved
	// (acked, nacked or put back in the buffer). Read only by the panic
	// boundary below.
	var inHand *common.QueuedMessage
	// owned is the group's entry this drainer was started for. While the
	// drainer runs the entry stays in groupQs (only the drainer itself, or
	// Stop, removes an entry with working set), so it is the same pointer on
	// every pass; once the drainer has settled the group (takeGroup) a later
	// submit may have made a new entry with a drainer of its own, and the
	// pointer is how the panic boundary tells the two apart.
	var owned *groupQueue
	// Panic boundary. processOne recovers the mediator's panics; this catches
	// one anywhere else on the drainer — a broker Ack/Nack, the settled
	// report — which would otherwise kill the process, and would in any case
	// leave the group's working flag set with no drainer, wedging it for
	// good. The message in hand and the rest of the group go back to the
	// broker together (siblings no earlier than the head), and the group's
	// entry is removed, so the group resumes on redelivery. If the panic came
	// after this drainer had already settled the group, takeGroup finds the
	// entry gone (or another drainer's) and takes nothing.
	defer func() {
		if r := recover(); r != nil {
			attrs := []any{"pool", p.cfg.Code, "group", group}
			if inHand != nil {
				attrs = p.msgAttrs(*inHand)
			}
			logRecovered("pool.drainGroup", r, attrs...)
			delay := siblingNackDelay(nil)
			if inHand != nil {
				p.nackSafely(ctx, *inHand, delay, "drainer panicked")
			}
			// Each buffered message is nacked behind its own boundary, so a
			// broker that panics on one nack cannot strand the rest.
			buffered, delays := p.takeGroupForRelease(group, owned, inHand, delay, delay)
			for i := range buffered {
				p.queueDec()
				p.nackSafely(ctx, buffered[i], delays[i], "drainer panicked")
			}
		}
	}()
	for {
		p.mu.Lock()
		gq := p.groupQs[group]
		if gq == nil || gq.empty() {
			// Fully drained — remove the entry so groupQs doesn't accumulate
			// one empty groupQueue per group ID ever seen, and so
			// MessageGroupCount reports only groups actually holding work.
			delete(p.groupQs, group)
			p.mu.Unlock()
			return
		}
		owned = gq
		msg, _ := gq.pop()
		p.mu.Unlock()
		inHand = &msg
		// Pop happens under p.mu before any await, so queueSize stays
		// consistent with what's actually buffered in groupQs.
		p.queueDec()

		// Acquire a concurrency slot, or give the message up on cancellation.
		if err := p.sem.acquire(ctx); err != nil {
			// Re-front the popped message (preserving FIFO — dropping just the
			// head while later messages stay buffered would reorder the group)
			// and clear working so the group resumes under a fresh drainer —
			// respawned by the next submit OR by a broker redelivery of a
			// buffered message (Manager.route kicks tryDrainGroup on the
			// redelivery-dedup path). If the pool stopped meanwhile the buffer
			// is gone; release the in-hand message to the broker instead.
			if !p.enqueueFront(group, msg) {
				p.nackMsg(ctx, msg, new(uint32(10)), "pool stopped during drain")
				return
			}
			inHand = nil // back in the buffer
			p.clearWorking(group)
			return
		}

		// Release the slot per iteration even if processOne panics past its own
		// recover — a bare deferred release would accumulate across the loop, so
		// scope it to a closure.
		d := func() Disposition {
			slot := &slotRelease{p: p}
			defer slot.release()
			return p.processOneReleasing(ctx, msg, slot)
		}()
		if d.Action == BrokerAck {
			inHand = nil // processOne has settled it at the broker
		}

		if d.Action == BrokerRelease {
			// Target unreachable, or the in-pipeline retry budget is spent —
			// hand this message AND everything still buffered behind it back to
			// the broker, then exit. releaseGroup removes the group's entry,
			// so a redelivery spawns a fresh drainer.
			p.releaseGroup(ctx, group, owned, msg, nackDelay(d.RetryAfter), "released to broker")
			return
		}

		if d.Action == BrokerAck && d.Group == GroupBlock {
			// BLOCK_ON_ERROR: "a failed job blocks the group until resolved."
			// The head has failed terminally and been ACKed away, so advancing
			// would deliver its successors PAST the failure — exactly what this
			// mode exists to prevent. (DispositionOf already folded the
			// DispatchMode check into Group, so there's nothing to re-check
			// here — see its MediationErrorConfig case.)
			//
			// The siblings are ACKED, not handed back. Releasing them looked
			// safer and did the opposite: they redeliver on the BROKER'S timer,
			// which has no connection to the failure being resolved, and by then
			// the head has been ACKed away — so the first sibling becomes the new
			// head and is delivered. "Add item" applied to an order that was
			// never created. The nack silently broke the guarantee the mode is
			// named after.
			//
			// Nothing is lost by acking: these messages exist as dispatch-job
			// rows in the platform store, which is the system of record. The
			// queue copy is a delivery attempt, not the data.
			p.ackBuffered(ctx, group, owned, "head failed under BLOCK_ON_ERROR")
			return
		}

		if d.Action == BrokerRetry {
			// Preserve FIFO: re-insert the failed message at the FRONT of its
			// group so it is the next one attempted, then wait out the backoff
			// before the next attempt (holding no semaphore slot). The single
			// drainer + front re-insert blocks the whole group on this message
			// until it succeeds — the intended ordered-delivery (head-of-line)
			// semantic. The in-flight tracker entry is kept across the retry.
			msg.Attempts++
			if !p.enqueueFront(group, msg) {
				// Pool stopped while retrying: buffer gone, nothing will drain
				// it. Release the message to the broker for fresh redelivery.
				p.nackMsg(ctx, msg, new(uint32(10)), "pool stopped during retry")
				return
			}
			inHand = nil // back in the buffer
			select {
			case <-ctx.Done():
				// Cancelled mid-backoff. The message is already re-fronted;
				// clear working so the group resumes under a fresh drainer.
				p.clearWorking(group)
				return
			case <-time.After(d.RetryAfter):
			}
		}
	}
}

// releaseGroup hands the whole of `group` back to the broker after an
// unreachable-target outcome: the message in hand plus every message still
// buffered behind it.
//
// It must be the whole group, not just the head. Releasing the head while its
// successors stayed buffered would put the head behind them on redelivery,
// reordering a group whose entire purpose is ordering. Both brokers redeliver a
// group in order — the Postgres queue makes only the earliest visible message of
// each group claimable, and SQS FIFO orders by MessageGroupId — so a wholly
// released group comes back intact.
//
// nackMsg drops each message's in-flight tracker entry as it goes, which is what
// lets the redelivery re-enter the pipeline instead of being discarded as a
// duplicate of a copy that no longer exists.
//
// gq is the calling drainer's entry for the group (see takeGroup).
//
// On a broker that does not keep a group in order itself the order of the
// redelivery rests on the router: the released messages are listed with the
// hand-back gate, and the siblings' delays are spaced after the head's (see
// takeGroupForRelease).
func (p *Pool) releaseGroup(ctx context.Context, group string, gq *groupQueue, inHand common.QueuedMessage, delay *uint32, reason string) {
	p.nackMsg(ctx, inHand, delay, reason)
	buffered, delays := p.takeGroupForRelease(group, gq, &inHand, delay, siblingNackDelay(delay))
	released := p.releaseBuffered(ctx, group, buffered, delays, reason)
	p.events.reject(RejectReleased, 1+released)
	// delay_seconds is how long the broker was asked to hold the head back —
	// 0 means "redeliver now". Without it a release that parks the group for
	// minutes (a target-named deferral) reads the same as an instant one.
	var delaySeconds uint32
	if delay != nil {
		delaySeconds = *delay
	}
	p.logger(inHand).Info("released message group to broker",
		"buffered_released", released, "delay_seconds", delaySeconds, "reason", reason)
}

// takeGroup is how a drainer settles its group: it removes the group's entry
// from groupQs and returns what was still buffered in it. The terminal group
// outcomes (a release, a BLOCK_ON_ERROR ack, the drainer's panic boundary)
// start here and differ only in what they then do with the messages: hand
// them back, or ack them away.
//
// gq is the entry the calling drainer owns, and nothing is taken unless it is
// still the group's entry. Removing it, rather than leaving it empty with
// working cleared, is what keeps groupQs to groups that hold messages or have
// a drainer: the drainer returns straight after settling, so an entry left
// behind lived until the same group id was submitted again, which for group
// ids that never recur (one per order) is never. The next submit of the group
// makes a fresh entry and starts a fresh drainer, as it would for a group
// never seen.
//
// The ownership check matters only when the drainer panics after it has
// settled (a broker nack or ack of a sibling, the settled report): by then a
// submit may have made a new entry with a drainer of its own, and the panic
// boundary must not take that drainer's buffer or remove its entry.
func (p *Pool) takeGroup(group string, gq *groupQueue) []common.QueuedMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.takeGroupLocked(group, gq)
}

// takeGroupLocked is takeGroup for a caller that holds p.mu.
func (p *Pool) takeGroupLocked(group string, gq *groupQueue) []common.QueuedMessage {
	if gq == nil || p.groupQs[group] != gq {
		return nil
	}
	delete(p.groupQs, group)
	buffered := gq.msgs
	gq.msgs = nil
	gq.working = false
	return buffered
}

// releaseBuffered hands back buffered, the messages that were queued behind a
// group's head (taken from the group with takeGroup, or by
// ReleaseParkedGroups), WITHOUT touching the head itself. Used when the TARGET
// is unreachable: the messages are blameless and the broker should redeliver
// them once it recovers. The group's entry is already gone, so a redelivery
// starts a fresh drainer.
//
// delays holds the nack delay of each message, in buffered's order: the
// sibling delay (siblingNackDelay), so they come back no earlier than the
// head, and on a broker that does not keep a group in order each a spacing
// after the one before it (takeGroupForRelease).
//
// Returns how many were released.
func (p *Pool) releaseBuffered(ctx context.Context, group string, buffered []common.QueuedMessage, delays []*uint32, reason string) int {
	for i := range buffered {
		p.queueDec()
		p.nackMsg(ctx, buffered[i], delays[i], reason)
	}
	if len(buffered) > 0 {
		slog.Info("released buffered group messages to broker",
			"group", group, "pool", p.cfg.Code, "released", len(buffered), "reason", reason)
	}
	return len(buffered)
}

// ackBuffered deletes everything still queued behind a head that failed
// terminally under BLOCK_ON_ERROR, and removes the group's entry (takeGroup;
// gq is the calling drainer's entry).
//
// The delete is the point. Handing these back would have the broker redeliver
// them on its own timer, unconnected to the failure being resolved, and with
// the head already ACKed away the first sibling would simply become the new
// head and deliver — the precise reordering the mode exists to prevent. Their
// dispatch-job rows in the platform store are the system of record and survive
// this; the queue copies are delivery attempts, not data.
//
// Known gap, shared with the Java implementation and tracked separately:
// nothing marks those job rows for review, so they sit at QUEUED/PROCESSING
// rather than FAILED. A settled-message hook carrying the reason would make the
// ids recoverable.
//
// Returns how many were ACKed.
func (p *Pool) ackBuffered(ctx context.Context, group string, gq *groupQueue, reason string) int {
	buffered := p.takeGroup(group, gq)
	p.events.reject(RejectBlocked, len(buffered))
	for i := range buffered {
		p.queueDec()
		p.ackTracked(ctx, buffered[i])
	}
	if len(buffered) > 0 {
		slog.Warn("acked untried group messages behind a failed head",
			"group", group, "pool", p.cfg.Code, "acked", len(buffered), "reason", reason)
		// Fired AFTER the ACK loop above, not before or alongside it — the
		// broker actions are what actually matter and must never wait on or
		// be affected by this. See reportSettled's own doc comment.
		p.reportSettled(group, reason, buffered)
	}
	return len(buffered)
}

// reportSettled is ackBuffered's T3/A-01 hook: tells the platform which
// dispatch-job rows it just ACKed off the broker so they can be marked
// PENDING instead of stranding at QUEUED/PROCESSING (see ackBuffered's
// "Known gap" comment above, and settled.go's header comment for the full
// contract). Best-effort by design — the platform's reaper
// (dispatchjob.RunReaper) is the crash backstop — so this:
//   - does nothing at all, silently, when no reporter is wired (nil is the
//     default until the deferred wiring in settled.go lands, and remains
//     correct forever for a standalone router with no platform);
//   - filters to only messages carrying scheduler-signed material
//     (dispatchJobFromMessage) — a message with no AuthToken is not a
//     platform dispatch job and there is nothing for the hook to mark;
//   - runs the actual HTTP call in a spawned goroutine on its own bounded
//     timeout, decoupled from ctx (which belongs to the submitting
//     consumer and may already be cancelled or about to be — see
//     drainGroup's doc comment) so a consumer restart can never cut the
//     report short; the timeout itself is the goroutine's exit guarantee.
//   - never returns an error to its caller and never blocks it: this call
//     returns as soon as the (synchronous, in-memory) filtering is done,
//     with the network call already handed off.
func (p *Pool) reportSettled(group, reason string, buffered []common.QueuedMessage) {
	p.srMu.RLock()
	reporter := p.settledReporter
	p.srMu.RUnlock()
	if reporter == nil {
		return
	}

	jobs := make([]SettledJob, 0, len(buffered))
	for _, qm := range buffered {
		if job, ok := dispatchJobFromMessage(qm.Message); ok {
			jobs = append(jobs, job)
		}
	}
	if len(jobs) == 0 {
		// Nothing here was a platform dispatch job (or nothing verifiable) —
		// no call to make.
		return
	}

	report := SettledReport{
		PoolCode: p.cfg.Code,
		Group:    group,
		Reason:   reason,
		Jobs:     jobs,
	}
	go func() {
		// Panic boundary: the reporter is an HTTP client call; a panic in it
		// must not take the process down. Nothing is in hand — the ACKs are
		// done, and the platform reaper is the backstop for a lost report.
		defer func() {
			if r := recover(); r != nil {
				logRecovered("pool.reportSettled", r, "pool", p.cfg.Code, "group", group, "jobs", len(jobs))
			}
		}()
		rctx, cancel := context.WithTimeout(context.Background(), settledReportTimeout)
		defer cancel()
		if err := reporter.ReportSettled(rctx, report); err != nil {
			// The reaper is the backstop for exactly this failure mode, so a
			// CONFIGURATION-class notice rather than an error: this pool has
			// no WarningService access (unlike manager.go/mediator.go, which
			// thread one through), so slog.Warn is the available signal.
			slog.Warn("settled-message hook failed; the platform reaper is the backstop",
				"pool", p.cfg.Code, "group", group, "jobs", len(jobs), "err", err)
		}
	}()
}

// settledReportTimeout bounds reportSettled's spawned goroutine — its exit
// guarantee, since the goroutine deliberately does not derive from the
// caller's ctx (see reportSettled's doc comment).
const settledReportTimeout = 10 * time.Second

// clearWorking flips a group's working flag back off so a subsequent submit
// can spawn a replacement drainer, and stamps parkedAt so an unclaimed park
// can be found later. Called only by the exiting drainer itself on a
// ctx-cancelled exit — the single-drainer invariant holds because
// tryDrainGroup spawns only when working is false, and only the drainer that
// set the flag clears it.
//
// The resume is normally external: the next submit for the group, or
// Manager.route's redelivery-dedup kick. Both need the source consumer to be
// POLLING, which is precisely what a pool at capacity stops it doing — and a
// parked group's own buffer is what holds that capacity. ReleaseParkedGroups
// is the backstop that keeps that from being a closed loop.
//
// The drainer parks only after putting its message back (enqueueFront), so the
// entry holds at least that one; an entry found empty is removed instead, so
// that no entry outlives both its messages and its drainer.
func (p *Pool) clearWorking(group string) {
	p.mu.Lock()
	if gq := p.groupQs[group]; gq != nil {
		if gq.empty() {
			delete(p.groupQs, group)
		} else {
			gq.working = false
			gq.parkedAt = time.Now()
		}
	}
	p.mu.Unlock()
}

// ReleaseParkedGroups hands back to the broker every group that has sat
// parked — buffered, with no drainer — for longer than minAge. Returns the
// number of messages released.
//
// It exists because parking is only safe if something is guaranteed to come
// back for it, and nothing is. A drainer parks on ctx cancellation (consumer
// restart or reconfigure) and the resume must then arrive from outside: a new
// submit, or a redelivery. Both require the source consumer to be polling, and
// the consumer stops polling exactly when the destination pool is at capacity
// — a state this very buffer is holding it in. Left alone the three facts
// close into a deadlock that no amount of waiting resolves, and on a FIFO
// queue the un-deleted group head blocks every message behind it until broker
// retention.
//
// Releasing rather than resuming is the deliberate choice: it needs no live
// delivery context, it returns the messages to the authority that can redeliver
// them in order (both brokers redeliver a group intact — see releaseGroup), and
// it drains the buffer, which is what actually restores the pool capacity the
// consumer needs to poll again. minAge keeps it clear of the fast path, so a
// group that a redelivery is about to resume is never snatched away from it.
//
// A parked group is chosen and taken (its entry removed) under one hold of
// p.mu. Choosing under the lock and taking after it let a drainer claim the
// group in between, pop its head and start delivering it; the take then
// emptied the buffer behind that drainer and cleared its working flag while
// it still ran, so the group's next submit started a second drainer beside
// it (TestReleaseParkedGroupsLeavesAGroupADrainerClaimedInTheGap).
func (p *Pool) ReleaseParkedGroups(ctx context.Context, minAge time.Duration) int {
	type parkedGroup struct {
		group  string
		msgs   []common.QueuedMessage
		delays []*uint32
	}
	now := time.Now()
	p.mu.Lock()
	var parked []parkedGroup
	for group, gq := range p.groupQs {
		if gq.working || gq.empty() || gq.parkedAt.IsZero() {
			continue
		}
		if now.Sub(gq.parkedAt) > minAge {
			// Every message goes back with the same sibling delay, so the
			// group returns together and in order rather than all visible at
			// once; on a broker that does not keep a group in order the
			// delays are spaced and the group listed with the hand-back gate
			// (takeGroupForRelease), in this same hold of p.mu.
			delays := sameDelay(len(gq.msgs), siblingNackDelay(nil))
			if gq.gated {
				delays = p.recordReleaseLocked(group, nil, nil, gq.msgs, siblingNackDelay(nil))
			}
			parked = append(parked, parkedGroup{group: group, msgs: gq.msgs, delays: delays})
			gq.msgs = nil
			delete(p.groupQs, group) // deleting the current key during range is allowed
		}
	}
	// The reaper's tick is also what clears the hand-back gate of a pool
	// that sees no more traffic for its groups.
	p.handbacks.sweep(p.clock())
	p.mu.Unlock()

	released := 0
	for _, g := range parked {
		if n := p.releaseBuffered(ctx, g.group, g.msgs, g.delays, "group parked with no drainer"); n > 0 {
			p.events.reject(RejectReleased, n)
			slog.Warn("released a parked message group to the broker; nothing had resumed it",
				"group", g.group, "pool", p.cfg.Code, "released", n, "parked_for", minAge)
			released += n
		}
	}
	return released
}
