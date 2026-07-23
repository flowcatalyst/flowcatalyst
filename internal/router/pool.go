package router

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// Pool is a passive dispatch worker that respects:
//   - configured concurrency (semaphore-style worker cap),
//   - configured rate limit (per-pool token bucket),
//   - per-endpoint circuit breakers,
//   - FIFO ordering within message groups (when DispatchMode requires it).
//
// A Pool does NOT own a queue or poll. The Manager polls every queue and
// routes each message to the pool named by its pool_code (DEFAULT-POOL
// fallback), then calls Submit. Because a pool processes messages from many
// queues, ack/nack/defer target each message's SOURCE consumer, resolved by
// the message's QueueIdentifier via resolveConsumer.
type Pool struct {
	cfg      common.PoolConfig
	mediator Mediator
	limiter  *RateLimiter
	tracker  *InFlightTracker
	metrics  *PoolMetricsCollector

	// resolveConsumer maps a message's origin queue (QueueIdentifier) to the
	// consumer that delivered it. nil result → the queue was deregistered
	// between routing and processing; the action is skipped (logged).
	resolveConsumer func(queueID string) queue.Consumer

	// sem is the pool-wide concurrency semaphore. Buffered chan: a send
	// is an acquire (blocks when full); the matching receive is a
	// release. UpdateConcurrency swaps the chan atomically; workers
	// snapshot the chan locally before acquire so the matching release
	// goes back to the same chan they acquired from. During a resize,
	// in-flight workers continue against the old chan (effective
	// concurrency = old_in_flight + new_cap) and the old chan is GC'd
	// once those workers finish.
	sem         atomic.Value // chan struct{}
	concurrency atomic.Uint32

	mu      sync.Mutex
	groupQs map[string]*groupQueue // ordered FIFO queues per message-group

	queueSize     atomic.Uint32 // pending in groupQs (pre-dispatch)
	activeWorkers atomic.Uint32 // currently inside processOne

	// mediating is the live set of messages currently inside processOne — the
	// authoritative "what is being mediated right now" view. It is maintained
	// at the SAME boundary as activeWorkers (enter/exit of processOne), so its
	// size always equals activeWorkers, and — unlike the InFlightTracker — it
	// is never reaped, so a long-running delivery stays visible for its whole
	// duration. Keyed by message id (a message is in at most one worker at a
	// time: FIFO within a group, one worker per message in IMMEDIATE mode).
	mediatingMu sync.Mutex
	mediating   map[string]MediatingEntry

	stopped atomic.Bool
}

// MediatingEntry is one message currently inside a pool worker (in processOne:
// awaiting a rate-limit token or actively being delivered). Snapshotted for the
// dashboard's Mediating view.
type MediatingEntry struct {
	MessageID  string
	PoolCode   string
	Group      string
	Queue      string
	Target     string
	Attempts   uint
	MediatedAt time.Time // when it entered the worker (this attempt)
}

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

// NewPool wires a pool. tracker may be nil; if so, in-flight tracking
// (and consequently stall detection + duplicate filtering) is disabled
// for messages handled by this pool.
func NewPool(cfg common.PoolConfig, mediator Mediator, tracker *InFlightTracker, resolveConsumer func(queueID string) queue.Consumer) *Pool {
	rate := uint32(0)
	if cfg.RateLimitPerMinute != nil {
		rate = *cfg.RateLimitPerMinute
	}
	concurrency := cfg.Concurrency
	if concurrency == 0 {
		// Rust parity (pool.rs): when concurrency is unset, derive it from
		// the rate limit — max(rate_per_minute/60, 1) — rather than always 1.
		concurrency = rate / 60
		if concurrency < 1 {
			concurrency = 1
		}
	}
	p := &Pool{
		cfg:             cfg,
		mediator:        mediator,
		limiter:         NewRateLimiter(rate),
		tracker:         tracker,
		metrics:         NewPoolMetricsCollector(),
		resolveConsumer: resolveConsumer,
		groupQs:         make(map[string]*groupQueue),
		mediating:       make(map[string]MediatingEntry),
	}
	p.sem.Store(make(chan struct{}, concurrency))
	p.concurrency.Store(concurrency)
	return p
}

// loadSem returns the current concurrency channel. Callers should
// snapshot it locally before an acquire so that the matching release
// receives from the same channel even if UpdateConcurrency swaps it
// mid-flight.
func (p *Pool) loadSem() chan struct{} { return p.sem.Load().(chan struct{}) }

// consumerFor resolves the source consumer for a message via its origin
// queue (QueueIdentifier); nil when that queue was deregistered between
// routing and processing.
func (p *Pool) consumerFor(qm common.QueuedMessage) queue.Consumer {
	if p.resolveConsumer == nil {
		return nil
	}
	return p.resolveConsumer(qm.QueueIdentifier)
}

// ackTracked / nackMsg resolve a message's source consumer and apply the
// terminal action there — a pool processes messages routed from many queues, so
// the action must target the queue the message arrived on. A missing consumer
// (deregistered queue) is logged and skipped.

// ackTracked ACKs a terminally-resolved message (2xx success, or 4xx which we
// drop to avoid an infinite client-error loop) using the FRESHEST receipt
// handle recorded on its in-flight entry — a broker redelivery may have swapped
// it since dispatch, and the handle captured at dispatch time can be stale by
// the time a long in-pipeline retry finally succeeds. It then clears the entry.
func (p *Pool) ackTracked(ctx context.Context, qm common.QueuedMessage) {
	receipt := qm.ReceiptHandle
	if p.tracker != nil {
		if rh, ok := p.tracker.CurrentReceipt(qm.Message.ID, qm.BrokerMessageID); ok {
			receipt = rh
		}
	}
	if c := p.consumerFor(qm); c != nil {
		if err := c.Ack(ctx, receipt); err != nil {
			slog.Warn("ack failed", "message_id", qm.Message.ID, "err", err)
		}
	} else {
		slog.Warn("ack: no consumer for queue", "queue", qm.QueueIdentifier, "message_id", qm.Message.ID)
	}
	if p.tracker != nil {
		p.tracker.Remove(qm.Message.ID, qm.BrokerMessageID)
	}
}

// nackMsg releases a message back to its source broker. It is used only for the
// non-retryable control paths (pool stopped, pool at capacity, shutdown before
// dispatch). NB: on SQS, Nack is a deliberate no-op — the message simply stays
// invisible until its visibility timeout lapses and is then redelivered fresh.
// Retryable mediation failures do NOT go here; they are retried in-pipeline.
//
// The message is leaving the pipeline, so its in-flight entry (claimed at
// route time) is released first: a lingering entry would classify the coming
// redelivery as a duplicate and drop it — the message would never re-enter.
func (p *Pool) nackMsg(ctx context.Context, qm common.QueuedMessage, delay *uint32, reason string) {
	if p.tracker != nil {
		p.tracker.Remove(qm.Message.ID, qm.BrokerMessageID)
	}
	c := p.consumerFor(qm)
	if c == nil {
		slog.Warn("nack: no consumer for queue", "queue", qm.QueueIdentifier, "message_id", qm.Message.ID, "reason", reason)
		return
	}
	if err := c.Nack(ctx, qm.ReceiptHandle, delay); err != nil {
		slog.Warn("nack failed", "reason", reason, "message_id", qm.Message.ID, "err", err)
	}
}

// Identifier is the pool code.
func (p *Pool) Identifier() string { return p.cfg.Code }

// SetRateLimit hot-swaps the rate-limit-per-minute value.
func (p *Pool) SetRateLimit(perMinute uint32) { p.limiter.SetRate(perMinute) }

// UpdateRateLimit is the API-facing alias for SetRateLimit. A nil value
// disables rate limiting (the Rust equivalent of `Option::None`).
func (p *Pool) UpdateRateLimit(perMinute *uint32) {
	var v uint32
	if perMinute != nil {
		v = *perMinute
	}
	p.limiter.SetRate(v)
}

// UpdateConcurrency swaps the semaphore to a new capacity. Returns false
// on n==0 (invalid). Existing in-flight workers continue to release into
// the old channel, which is GC'd once empty; new work uses the new
// channel. Effective concurrency during the transition is bounded by
// old_in_flight + new_cap; steady state is new_cap.
func (p *Pool) UpdateConcurrency(n uint32) bool {
	if n == 0 {
		return false
	}
	old := p.concurrency.Load()
	if n == old {
		return true
	}
	p.sem.Store(make(chan struct{}, n))
	p.concurrency.Store(n)
	slog.Info("pool concurrency updated", "pool", p.cfg.Code, "from", old, "to", n)
	return true
}

// Metrics exposes the pool's metric collector. The HTTP API hits this
// when building EnhancedPoolMetrics for /monitoring/pool-stats.
func (p *Pool) Metrics() *PoolMetricsCollector { return p.metrics }

// submit routes one polled message. It runs capacity backpressure, then
// branches on DispatchMode: IMMEDIATE-mode messages (the default,
// RequiresOrdering()==false) dispatch concurrently via runImmediate (one worker
// per message, bounded only by the pool semaphore), while ordered modes enqueue
// into the per-group FIFO buffer and drain serially. Retryable failures are
// retried in-pipeline (see processOne / drainGroup / runImmediate), so ordering
// is preserved by re-inserting a failed message at the FRONT of its group
// rather than by cascade-NACKing the rest of a batch.
func (p *Pool) submit(ctx context.Context, m common.QueuedMessage) {
	// Reject when the pool is stopping.
	if p.stopped.Load() {
		p.nackMsg(ctx, m, ptrU32(10), "pool stopped")
		return
	}
	// Capacity backpressure: NACK (delay 10) when the pre-dispatch buffer is
	// already at capacity = max(concurrency*20, 50).
	capacity := p.concurrency.Load() * queueCapacityMultiplier
	if capacity < minQueueCapacity {
		capacity = minQueueCapacity
	}
	if p.queueSize.Load() >= capacity {
		p.nackMsg(ctx, m, ptrU32(10), "pool at capacity")
		return
	}

	if !m.Message.DispatchMode.RequiresOrdering() {
		// IMMEDIATE: no ordering — dispatch concurrently. queueSize is
		// incremented here and decremented once the worker holds a semaphore
		// slot, so the "queued (pre-dispatch)" gauge mirrors the ordered path.
		p.queueSize.Add(1)
		go p.runImmediate(ctx, m)
		return
	}

	group := ""
	if m.Message.MessageGroupID != nil {
		group = *m.Message.MessageGroupID
	}
	if !p.enqueue(group, m) {
		// Raced with Stop: the buffer is flushed and nothing will drain it.
		p.nackMsg(ctx, m, ptrU32(10), "pool stopped")
		return
	}
	p.tryDrainGroup(ctx, group)
}

// runImmediate dispatches a single IMMEDIATE-mode message concurrently:
// acquire a pool semaphore slot, then process it. IMMEDIATE messages have no
// group buffer, so a retryable failure re-dispatches the same message after the
// backoff (one chained goroutine per failing message — sequential, not a leak),
// keeping it in-pipeline rather than releasing it to the broker.
func (p *Pool) runImmediate(ctx context.Context, m common.QueuedMessage) {
	sem := p.loadSem()
	select {
	case <-ctx.Done():
		// Shutdown before we could start. nackMsg releases the route-time
		// tracker entry so the broker's redelivery (NACK is a no-op on SQS;
		// the message reappears after the visibility timeout) re-enters the
		// pipeline as a fresh copy instead of being dropped as a duplicate.
		p.queueSize.Add(^uint32(0))
		p.nackMsg(ctx, m, ptrU32(10), "shutdown before dispatch")
		return
	case sem <- struct{}{}:
	}
	p.queueSize.Add(^uint32(0)) // now active, not queued
	result, retryAfter := func() (processResult, time.Duration) {
		defer func() { <-sem }() // release on every exit path (acquired above)
		return p.processOne(ctx, m)
	}()
	if result != processRetry {
		return
	}
	// Retry in-pipeline: wait out the backoff, then re-dispatch. The in-flight
	// tracker entry is kept (so redeliveries are deduped against it), and
	// Attempts grows the backoff and tells processOne not to re-track.
	m.Attempts++
	p.queueSize.Add(1) // re-queued (pre-dispatch) for the duration of the backoff
	go func() {
		select {
		case <-ctx.Done():
			// Shutdown/consumer-restart mid-backoff: the message leaves the
			// pipeline (IMMEDIATE mode has no buffer to park in), so release
			// its tracker entry — otherwise every future redelivery would be
			// dropped as a duplicate of a copy that no longer exists, and the
			// message would cycle on the broker untouchable until retention.
			p.queueSize.Add(^uint32(0))
			if p.tracker != nil {
				p.tracker.Remove(m.Message.ID, m.BrokerMessageID)
			}
			return
		case <-time.After(retryAfter):
		}
		p.runImmediate(ctx, m)
	}()
}

// Stop signals the pool to exit and flushes every buffered (not yet
// dispatched) message, releasing their in-flight tracker entries. The buffer
// is abandoned on stop — nothing will ever drain it — so a retained entry
// would classify the broker's redeliveries as duplicates forever and the
// messages would never be processed anywhere. With the entries released, the
// redeliveries re-enter the pipeline fresh (routed per the new config).
// In-flight workers drain out on their own and ack/remove per outcome.
func (p *Pool) Stop() {
	p.stopped.Store(true)
	p.mu.Lock()
	var flushed []common.QueuedMessage
	for _, gq := range p.groupQs {
		flushed = append(flushed, gq.msgs...)
		gq.msgs = nil
	}
	p.groupQs = make(map[string]*groupQueue)
	p.mu.Unlock()
	for i := range flushed {
		p.queueSize.Add(^uint32(0))
		if p.tracker != nil {
			p.tracker.Remove(flushed[i].Message.ID, flushed[i].BrokerMessageID)
		}
	}
	if len(flushed) > 0 {
		slog.Info("pool stopped; flushed buffered messages for broker redelivery",
			"pool", p.cfg.Code, "count", len(flushed))
	}
}

// trackMediating records a message as actively inside a worker. Called at the
// top of processOne, paired with untrackMediating on exit.
func (p *Pool) trackMediating(qm common.QueuedMessage) {
	group := ""
	if qm.Message.MessageGroupID != nil {
		group = *qm.Message.MessageGroupID
	}
	p.mediatingMu.Lock()
	p.mediating[qm.Message.ID] = MediatingEntry{
		MessageID:  qm.Message.ID,
		PoolCode:   p.cfg.Code,
		Group:      group,
		Queue:      qm.QueueIdentifier,
		Target:     qm.Message.MediationTarget,
		Attempts:   qm.Attempts,
		MediatedAt: time.Now(),
	}
	p.mediatingMu.Unlock()
}

func (p *Pool) untrackMediating(messageID string) {
	p.mediatingMu.Lock()
	delete(p.mediating, messageID)
	p.mediatingMu.Unlock()
}

// MediatingSnapshot returns the messages currently inside this pool's workers.
// Never reaped, so a long-running delivery stays listed for its full duration —
// the reliable answer to "which records are mediating right now" (its length
// equals ActiveWorkers).
func (p *Pool) MediatingSnapshot() []MediatingEntry {
	p.mediatingMu.Lock()
	defer p.mediatingMu.Unlock()
	out := make([]MediatingEntry, 0, len(p.mediating))
	for _, e := range p.mediating {
		out = append(out, e)
	}
	return out
}

// InFlight returns the count of messages currently in worker goroutines.
// Backward-compat shim for callers that still read inFlight as int64.
func (p *Pool) InFlight() int64 { return int64(p.activeWorkers.Load()) }

// ActiveWorkers returns the count of goroutines currently inside processOne.
func (p *Pool) ActiveWorkers() uint32 { return p.activeWorkers.Load() }

// QueueSize returns the count of messages buffered in group queues
// awaiting dispatch (pre-semaphore).
func (p *Pool) QueueSize() uint32 { return p.queueSize.Load() }

// Concurrency returns the current concurrency cap.
func (p *Pool) Concurrency() uint32 { return p.concurrency.Load() }

// RateLimitPerMinute returns the current rate-limit (or nil if disabled).
// Mirrors the way Rust's PoolStats reports the field.
func (p *Pool) RateLimitPerMinute() *uint32 {
	rate := p.limiter.Rate()
	if rate == 0 {
		return nil
	}
	return &rate
}

// IsRateLimited reports whether the limiter currently has no spare tokens.
func (p *Pool) IsRateLimited() bool { return p.limiter.IsLimited() }

// MessageGroupCount returns the number of message groups currently
// holding buffered messages.
func (p *Pool) MessageGroupCount() uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return uint32(len(p.groupQs))
}

// Stats returns the dashboard-shaped snapshot of this pool.
func (p *Pool) Stats() PoolStats {
	concurrency := p.concurrency.Load()
	capacity := concurrency * queueCapacityMultiplier
	if capacity < minQueueCapacity {
		capacity = minQueueCapacity
	}
	m := p.metrics.Snapshot()
	return PoolStats{
		PoolCode:           p.cfg.Code,
		Concurrency:        concurrency,
		ActiveWorkers:      p.activeWorkers.Load(),
		QueueSize:          p.queueSize.Load(),
		QueueCapacity:      capacity,
		MessageGroupCount:  p.MessageGroupCount(),
		RateLimitPerMinute: p.RateLimitPerMinute(),
		IsRateLimited:      p.IsRateLimited(),
		Metrics:            &m,
		Histogram:          p.metrics.HistogramSnapshot(),
	}
}

// queueCapacityMultiplier and minQueueCapacity mirror the Java/Rust
// derivation: capacity = max(concurrency * 20, 50). Used by Stats() so
// the dashboard's "queue capacity" matches the reference implementations.
const (
	queueCapacityMultiplier uint32 = 20
	minQueueCapacity        uint32 = 50
)

// enqueue appends a newly-arrived message to the BACK of its group's FIFO.
// Returns false without buffering when the pool has stopped — checked under
// p.mu so it can't race Stop's buffer flush and strand a message (with a live
// tracker entry) in an abandoned buffer.
func (p *Pool) enqueue(group string, m common.QueuedMessage) bool {
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
	gq.msgs = append(gq.msgs, m)
	p.mu.Unlock()
	p.queueSize.Add(1)
	return true
}

// enqueueFront puts a message back at the HEAD of its group's FIFO so that a
// retry is the NEXT message attempted — never overtaken by a later message in
// the same group. Used only by the ordered drainer on a retryable failure or
// a cancellation park. Same stopped-pool contract as enqueue.
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
	p.mu.Unlock()
	p.queueSize.Add(1)
	return true
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
	p.mu.Unlock()

	go p.drainGroup(ctx, group)
}

// drainGroup is the per-message-group worker goroutine spawned by
// tryDrainGroup. Drains one message at a time from gq.msgs (preserving
// FIFO order within the group), gated by the pool-wide `sem` semaphore.
//
// Exit conditions:
//   - the group buffer is empty (the groupQs entry is removed).
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
		msg, _ := gq.pop()
		p.mu.Unlock()
		// Pop happens under p.mu before any await, so queueSize stays
		// consistent with what's actually buffered in groupQs.
		p.queueSize.Add(^uint32(0)) // atomic decrement

		// Acquire a concurrency slot. Snapshot the channel locally so a
		// resize between acquire and release doesn't cross channels.
		// Wakeup conditions:
		//   <-ctx.Done()         — consumer stopping; park the message and exit.
		//   sem <- struct{}{}    — slot acquired; proceed.
		sem := p.loadSem()
		select {
		case <-ctx.Done():
			// Re-front the popped message (preserving FIFO — dropping just the
			// head while later messages stay buffered would reorder the group)
			// and clear working so the group resumes under a fresh drainer —
			// respawned by the next submit OR by a broker redelivery of a
			// buffered message (Manager.route kicks tryDrainGroup on the
			// redelivery-dedup path). If the pool stopped meanwhile the buffer
			// is gone; release the in-hand message to the broker instead.
			if !p.enqueueFront(group, msg) {
				p.nackMsg(ctx, msg, ptrU32(10), "pool stopped during drain")
				return
			}
			p.clearWorking(group)
			return
		case sem <- struct{}{}:
		}

		// Release the slot per iteration even if processOne panics past its own
		// recover — a bare deferred <-sem would accumulate across the loop, so
		// scope it to a closure.
		result, retryAfter := func() (processResult, time.Duration) {
			defer func() { <-sem }()
			return p.processOne(ctx, msg)
		}()

		if result == processRetry {
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
				p.nackMsg(ctx, msg, ptrU32(10), "pool stopped during retry")
				return
			}
			select {
			case <-ctx.Done():
				// Cancelled mid-backoff. The message is already re-fronted;
				// clear working so the group resumes under a fresh drainer.
				p.clearWorking(group)
				return
			case <-time.After(retryAfter):
			}
		}
	}
}

// clearWorking flips a group's working flag back off so a subsequent submit
// can spawn a replacement drainer. Called only by the exiting drainer itself
// on a ctx-cancelled exit — the single-drainer invariant holds because
// tryDrainGroup spawns only when working is false, and only the drainer that
// set the flag clears it.
func (p *Pool) clearWorking(group string) {
	p.mu.Lock()
	if gq := p.groupQs[group]; gq != nil {
		gq.working = false
	}
	p.mu.Unlock()
}

// processResult is processOne's verdict, consumed by the caller (drainGroup /
// runImmediate) to decide whether to ACK-and-drop, retry in-pipeline, or
// discard a deduplicated copy.
type processResult int

const (
	// processDone — terminally resolved (ACKed on 2xx success or 4xx drop);
	// the in-flight entry has been cleared and the message leaves the pipeline.
	processDone processResult = iota
	// processRetry — retryable failure; the in-flight entry was KEPT and the
	// caller should re-dispatch after the returned backoff (front of the group
	// for ordered, delayed re-spawn for IMMEDIATE). Never released to the broker.
	processRetry
	// processDuplicate — a different copy of this app message owns the
	// pipeline (an external requeue that slipped past route-time dedup, e.g.
	// across a tracker reap); this copy was ACK-deleted from the broker with
	// its own receipt handle and dropped. The owner's entry is untouched.
	processDuplicate
)

const (
	// retryMinDelay / retryMaxDelay bound the in-pipeline backoff; panicRetryDelay
	// is the fixed backoff after a recovered panic.
	retryMinDelay   = 100 * time.Millisecond
	retryMaxDelay   = 5 * time.Minute
	panicRetryDelay = 10 * time.Second
)

// retryDelay computes the in-pipeline backoff before the next attempt:
// exponential in the attempt count (starting at retryMinDelay), with any
// server-requested delay (Retry-After on 429, the breaker reset on circuit-open,
// the 5xx retry hint) applied as a floor, capped at retryMaxDelay.
func retryDelay(attempts uint, outcomeDelaySec int) time.Duration {
	shift := attempts
	if shift > 12 { // cap the shift so the bit-shift can't overflow
		shift = 12
	}
	d := retryMinDelay << shift
	if floor := time.Duration(outcomeDelaySec) * time.Second; d < floor {
		d = floor
	}
	if d > retryMaxDelay {
		d = retryMaxDelay
	}
	return d
}

// processOne runs the per-message pipeline: track (first dispatch only), rate
// limit, mediate, and resolve by outcome. It does NOT release messages to the
// broker on failure — a retryable outcome keeps the in-flight entry and returns
// processRetry so the caller retries in-pipeline (preserving order for grouped
// messages). Only a terminal 2xx/4xx ACKs and clears the entry.
func (p *Pool) processOne(ctx context.Context, qm common.QueuedMessage) (result processResult, retryAfter time.Duration) {
	p.activeWorkers.Add(1)
	defer p.activeWorkers.Add(^uint32(0)) // atomic decrement
	p.trackMediating(qm)
	defer p.untrackMediating(qm.Message.ID)

	// Panic isolation: a panic mid-mediation must not crash the process (an
	// unrecovered panic in a goroutine takes down the program) or strand the
	// message. Recover and retry in-pipeline — the in-flight entry is kept, so
	// the redelivery dedup stays intact and the worker survives. Named returns
	// let the deferred recover set the verdict.
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in processOne; retrying in-pipeline",
				"message_id", qm.Message.ID, "panic", r)
			result = processRetry
			retryAfter = panicRetryDelay
			if p.tracker != nil {
				p.tracker.MarkRetrying(qm.Message.ID, qm.BrokerMessageID)
			}
		}
	}()

	// The message was registered in-flight at ROUTE time; first dispatch
	// re-asserts the entry as a backstop (restores it if the reaper pruned it
	// during a very long buffer wait). A retry re-dispatch (Attempts>0) is
	// already tracked — keep the existing entry (which may have had its
	// receipt handle swapped by a redelivery) and skip. EnsureTracked never
	// swaps handles: the entry's handle may be fresher than this copy's.
	if p.tracker != nil && qm.Attempts == 0 {
		im := common.NewInFlightMessage(&qm.Message, qm.BrokerMessageID, qm.QueueIdentifier, qm.BatchID, qm.ReceiptHandle)
		if !p.tracker.EnsureTracked(im) {
			// A different copy of this app message owns the pipeline (external
			// requeue that slipped past route-time dedup). ACK-delete THIS
			// copy with its own receipt handle — leaving it un-acked would let
			// it redeliver forever — and leave the owner's entry alone.
			slog.Info("external requeue duplicate (process-time backstop); ACKing copy",
				"message_id", qm.Message.ID, "queue", qm.QueueIdentifier)
			if c := p.consumerFor(qm); c != nil {
				if err := c.Ack(ctx, qm.ReceiptHandle); err != nil {
					slog.Warn("ack (requeue duplicate) failed", "message_id", qm.Message.ID, "err", err)
				}
			}
			return processDuplicate, 0
		}
	}

	// Rate limit (per-pool token bucket). Record a rate-limited event when the
	// limiter actually held us back (current tokens exhausted).
	if p.limiter.IsLimited() {
		p.metrics.RecordRateLimited()
	}
	if err := p.limiter.Wait(ctx); err != nil {
		// Context cancelled mid-wait — keep the entry and retry in-pipeline.
		if p.tracker != nil {
			p.tracker.MarkRetrying(qm.Message.ID, qm.BrokerMessageID)
		}
		return processRetry, retryDelay(qm.Attempts, 5)
	}

	start := time.Now()
	outcome := p.mediator.Mediate(ctx, &qm.Message)
	durationMs := uint64(time.Since(start).Milliseconds())

	switch outcome.Result {
	case common.MediationSuccess:
		p.metrics.RecordSuccess(durationMs)
		p.ackTracked(ctx, qm)
		return processDone, 0

	case common.MediationErrorConfig:
		// The mediator already recorded the breaker success (4xx = reachable).
		// 4xx — ACK to avoid an infinite client-error retry loop. Do NOT trip
		// the breaker. Counted against total_failure (a non-success terminal).
		p.metrics.RecordFailure(durationMs)
		p.ackTracked(ctx, qm)
		return processDone, 0

	case common.MediationErrorProcess:
		// Transient (5xx/timeout): retry in-pipeline. Don't penalise the
		// all-time failure counter.
		p.metrics.RecordTransient(durationMs)
		return p.retry(qm, outcome.DelaySeconds)

	case common.MediationErrorConnection:
		p.metrics.RecordFailure(durationMs)
		return p.retry(qm, outcome.DelaySeconds)

	case common.MediationRateLimited:
		// 429 — retry in-pipeline honouring Retry-After; NOT a breaker failure.
		p.metrics.RecordRateLimited()
		return p.retry(qm, outcome.DelaySeconds)

	case common.MediationCircuitOpen:
		// Breaker open (decided by the mediator): no delivery was attempted.
		// Retry in-pipeline once the breaker reset timeout (carried in the
		// outcome) elapses.
		return p.retry(qm, outcome.DelaySeconds)
	}
	return processDone, 0
}

// retry marks the in-flight entry as retrying (so the stall detector / reaper
// skip it) and returns the processRetry verdict with the computed backoff.
func (p *Pool) retry(qm common.QueuedMessage, outcomeDelaySec int) (processResult, time.Duration) {
	if p.tracker != nil {
		p.tracker.MarkRetrying(qm.Message.ID, qm.BrokerMessageID)
	}
	return processRetry, retryDelay(qm.Attempts, outcomeDelaySec)
}

func ptrU32(v uint32) *uint32 { return &v }
