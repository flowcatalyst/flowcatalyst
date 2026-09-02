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
	flushes  *GroupFlushRegistry

	// resolveConsumer maps a message's origin queue (QueueIdentifier) to the
	// consumer that delivered it. nil result → the queue was deregistered
	// between routing and processing; the action is skipped (logged).
	resolveConsumer func(queueID string) queue.Consumer

	// sem caps concurrent deliveries pool-wide, across both dispatch paths.
	// It also owns the concurrency figure itself (sem.capacity()).
	sem *semaphore

	mu      sync.Mutex
	groupQs map[string]*groupQueue // ordered FIFO queues per message-group

	// Two populations describe a pool's own work, and each has exactly one
	// owner here so nothing has to be kept in step by hand:
	//
	//   queueSize — accepted but not yet being delivered: buffered in groupQs,
	//     or an IMMEDIATE message waiting on a slot, or a retry sitting out its
	//     backoff. Drives capacity backpressure. A counter rather than a sum
	//     over groupQs because the IMMEDIATE path never buffers.
	//   mediating — inside processOne right now. ActiveWorkers is its size, so
	//     there is no second counter to disagree with it.
	//
	// The third view, the InFlightTracker, is deliberately not a pool concern:
	// it is process-wide (shared by every pool), tracks pipeline OWNERSHIP for
	// dedup/stall/drain rather than "being worked on", and is reaped. A message
	// is tracked from route time until it leaves the pipeline, which spans both
	// populations above and the gaps between them.
	queueSize atomic.Uint32

	// mediating is keyed per WORKER, not per message: the process-time dedup
	// backstop means two copies of one message id can briefly sit in two
	// workers, and keying by id would then under-report the count and let the
	// loser's exit delete the owner's entry. Unlike the InFlightTracker it is
	// never reaped, so a long-running delivery stays visible for its whole
	// duration.
	mediatingMu sync.Mutex
	mediating   map[uint64]MediatingEntry
	workerSeq   atomic.Uint64

	stopped atomic.Bool

	// draining is set by Drain (X-11: a pool REMOVAL drains rather than
	// flushing). It closes admission the same way stopped does — checked
	// alongside it at the top of submit — but, unlike stopped, does NOT
	// touch groupQs: existing buffered work keeps draining normally. See
	// Drain's doc comment.
	draining atomic.Bool

	// srMu guards settledReporter. nil (the default) disables the T3/A-01
	// settled-message hook entirely — a standalone router with no platform
	// wiring must behave exactly as it does today. See SetSettledReporter
	// and reportSettled in settled.go/pool.go.
	srMu            sync.RWMutex
	settledReporter SettledReporter
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
	// parkedAt is when the group was last left buffered with NO drainer
	// (clearWorking). Zero while a drainer owns it. A parked group is normally
	// resumed within a broker redelivery, so a parkedAt that keeps ageing is
	// the signature of a group nothing is coming back for — which is what
	// ReleaseParkedGroups sweeps.
	parkedAt time.Time
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
		// When concurrency is unset, derive it from
		// the rate limit — max(rate_per_minute/60, 1) — rather than always 1.
		concurrency = rate / 60
		if concurrency < 1 {
			concurrency = 1
		}
	}
	return &Pool{
		cfg:             cfg,
		mediator:        mediator,
		limiter:         NewRateLimiter(rate),
		tracker:         tracker,
		metrics:         NewPoolMetricsCollector(),
		flushes:         NewGroupFlushRegistry(),
		resolveConsumer: resolveConsumer,
		sem:             newSemaphore(concurrency),
		groupQs:         make(map[string]*groupQueue),
		mediating:       make(map[uint64]MediatingEntry),
	}
}

// queueDec drops the pre-dispatch count by one. (atomic.Uint32 has no Sub;
// adding ^0 is the two's-complement decrement.)
func (p *Pool) queueDec() { p.queueSize.Add(^uint32(0)) }

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
		if err := c.Ack(ctx, receipt, qm.BrokerMessageID); err != nil {
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
// disables rate limiting.
func (p *Pool) UpdateRateLimit(perMinute *uint32) {
	var v uint32
	if perMinute != nil {
		v = *perMinute
	}
	p.limiter.SetRate(v)
}

// SetSettledReporter wires (or clears, with nil) the T3/A-01
// settled-message-hook reporter this pool's ackBuffered fires when a
// BLOCK_ON_ERROR head fails terminally. NOT a NewPool parameter — it is set
// after construction (see settled.go's header comment for the intended
// one-line wiring) so every existing NewPool call site keeps working
// unchanged. nil is the correct default for a standalone router with no
// platform behind it, and ackBuffered stays completely silent in that case:
// no goroutine, no log, ACKs proceed exactly as before this feature existed.
func (p *Pool) SetSettledReporter(r SettledReporter) {
	p.srMu.Lock()
	p.settledReporter = r
	p.srMu.Unlock()
}

// UpdateConcurrency re-caps the semaphore. Returns false on n==0 (invalid).
// A shrink applies to admission only — deliveries already running are never
// interrupted, so the pool converges on the new cap as they finish.
func (p *Pool) UpdateConcurrency(n uint32) bool {
	if n == 0 {
		return false
	}
	old := p.sem.capacity()
	if n == old {
		return true
	}
	p.sem.setLimit(n)
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
	// Reject when the pool is stopping or draining (X-11: a removed pool
	// admits nothing new, but — unlike stopped — draining does not touch
	// what's already buffered; see Drain's doc comment). In practice a
	// draining pool is never reachable here at all: Manager.Reconfigure
	// removes it from the routing map in the same step that starts the
	// drain, so no new batch is ever routed to it. This check is the
	// pool-local backstop for a caller holding a stale *Pool reference.
	if p.stopped.Load() || p.draining.Load() {
		p.nackMsg(ctx, m, ptrU32(10), "pool stopped")
		return
	}
	// Capacity backpressure: NACK (delay 10) when the pre-dispatch buffer is
	// already at capacity = max(concurrency*20, 50).
	if p.queueSize.Load() >= p.queueCapacity() {
		p.nackMsg(ctx, m, ptrU32(10), "pool at capacity")
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
		p.queueSize.Add(1)
		go p.runImmediate(ctx, m)
		return
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
	if err := p.sem.acquire(ctx); err != nil {
		// Shutdown before we could start. nackMsg releases the route-time
		// tracker entry so the broker's redelivery (NACK is a no-op on SQS;
		// the message reappears after the visibility timeout) re-enters the
		// pipeline as a fresh copy instead of being dropped as a duplicate.
		p.queueDec()
		p.nackMsg(ctx, m, ptrU32(10), "shutdown before dispatch")
		return
	}
	p.queueDec() // now active, not queued
	d := func() Disposition {
		defer p.sem.release() // release on every exit path (acquired above)
		return p.processOne(ctx, m)
	}()
	if d.Action == BrokerRelease {
		// Target unreachable, or the in-pipeline retry budget is spent.
		// IMMEDIATE mode has no group buffer, so there is nothing behind this
		// message to release with it — hand just this one back and let the
		// broker redeliver. d.RetryAfter is non-zero only in the budget case,
		// where it becomes the redelivery delay.
		p.nackMsg(ctx, m, nackDelay(d.RetryAfter), "released to broker")
		return
	}
	if d.Action != BrokerRetry {
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
			p.queueDec()
			if p.tracker != nil {
				p.tracker.Remove(m.Message.ID, m.BrokerMessageID)
			}
			return
		case <-time.After(d.RetryAfter):
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
		p.queueDec()
		if p.tracker != nil {
			p.tracker.Remove(flushed[i].Message.ID, flushed[i].BrokerMessageID)
		}
	}
	if len(flushed) > 0 {
		slog.Info("pool stopped; flushed buffered messages for broker redelivery",
			"pool", p.cfg.Code, "count", len(flushed))
	}
}

// drainPollInterval is how often Drain's background goroutine checks whether
// the buffer and active workers have both emptied.
const drainPollInterval = 250 * time.Millisecond

// poolDrainBudget bounds Drain's background goroutine (CONVENTIONS.md §9:
// every goroutine needs an exit path). It is sized well past the mediator's
// per-attempt timeout (15m — see InFlightMessage's doc comment) so a
// legitimately slow delivery is never cut off. Past the budget Drain gives
// up waiting and force-stops (flushing whatever is left for broker
// redelivery, same as Stop always has) rather than leaking the goroutine —
// the reaper's ReleaseParkedGroups sweep (which also sweeps a draining pool;
// see Manager.drainingPools) is the FIRST line of defence against a
// genuinely stuck group, and should almost always retire the pool long
// before this budget matters.
const poolDrainBudget = 30 * time.Minute

// Drain begins a graceful, ASYNCHRONOUS stop for pool REMOVAL (X-11:
// reconfiguration is in-place and pools are never rebuilt, but a removal
// drains — it does not flush). Unlike Stop, it does not touch the buffer:
// new admission is refused immediately (submit checks draining alongside
// stopped, at the very top — before enqueue/enqueueFront, so neither of
// those needs to change), but every group already buffered keeps draining
// through its EXISTING drainer goroutine exactly as it would have if the
// pool were still routed — the drainer runs under the SUBMITTING CONSUMER's
// workCtx, not anything this Pool owns, so removing the pool from
// Manager.pools does not touch it. An IMMEDIATE-mode message mid-backoff
// keeps retrying the same way.
//
// Once the buffer and active workers are both empty (or the drain budget
// expires), Stop is called to complete the shutdown — its flush is then a
// no-op in the normal case — and onDone runs so the caller can drop this
// pool's last reference (Manager.drainingPools). onDone runs on the drain
// goroutine, never on the caller's.
//
// Returns immediately; idempotent (a second call is a no-op — see the
// draining CompareAndSwap).
func (p *Pool) Drain(onDone func()) {
	if !p.draining.CompareAndSwap(false, true) {
		return
	}
	go p.awaitDrainAndStop(onDone)
}

func (p *Pool) awaitDrainAndStop(onDone func()) {
	deadline := time.Now().Add(poolDrainBudget)
	for (p.QueueSize() > 0 || p.ActiveWorkers() > 0) && time.Now().Before(deadline) {
		time.Sleep(drainPollInterval)
	}
	if p.QueueSize() > 0 || p.ActiveWorkers() > 0 {
		slog.Warn("pool drain exceeded its budget; force-stopping (remaining buffer will flush for broker redelivery)",
			"pool", p.cfg.Code, "budget", poolDrainBudget, "queue_size", p.QueueSize(), "active_workers", p.ActiveWorkers())
	}
	p.Stop()
	slog.Info("pool drained after removal; fully stopped", "pool", p.cfg.Code)
	if onDone != nil {
		onDone()
	}
}

// beginMediating records a message as actively inside a worker and returns the
// key to end it with. Called at the top of processOne, paired with
// endMediating on exit — the pair is also what makes ActiveWorkers true, so
// neither half is optional.
func (p *Pool) beginMediating(qm common.QueuedMessage) uint64 {
	worker := p.workerSeq.Add(1)
	p.mediatingMu.Lock()
	p.mediating[worker] = MediatingEntry{
		MessageID:  qm.Message.ID,
		PoolCode:   p.cfg.Code,
		Group:      qm.Message.GroupID(),
		Queue:      qm.QueueIdentifier,
		Target:     qm.Message.MediationTarget,
		Attempts:   qm.Attempts,
		MediatedAt: time.Now(),
	}
	p.mediatingMu.Unlock()
	return worker
}

func (p *Pool) endMediating(worker uint64) {
	p.mediatingMu.Lock()
	delete(p.mediating, worker)
	p.mediatingMu.Unlock()
}

// OldestMediatingAge is how long this pool's longest-running delivery has been
// inside the mediator, or 0 when no worker is active.
//
// It is the number that makes a wedged pool legible. ActiveWorkers alone
// cannot: a pool at 1/1 is indistinguishable from a healthy busy pool and a
// pool whose single slot has been held by one delivery for forty minutes. The
// mediator's production timeout is 15 minutes over 3 attempts, so a worker can
// legitimately hold its slot — and, at concurrency 1, the whole pool and every
// message group in it — for the better part of an hour.
func (p *Pool) OldestMediatingAge() time.Duration {
	p.mediatingMu.Lock()
	defer p.mediatingMu.Unlock()
	var oldest time.Time
	for _, e := range p.mediating {
		if oldest.IsZero() || e.MediatedAt.Before(oldest) {
			oldest = e.MediatedAt
		}
	}
	if oldest.IsZero() {
		return 0
	}
	return time.Since(oldest)
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

// ActiveWorkers returns the count of goroutines currently inside processOne —
// the size of the mediating set, not a counter kept alongside it.
func (p *Pool) ActiveWorkers() uint32 {
	p.mediatingMu.Lock()
	defer p.mediatingMu.Unlock()
	return uint32(len(p.mediating))
}

// QueueSize returns the count of messages accepted but not yet being
// delivered: buffered in a group queue, waiting on a concurrency slot, or
// sitting out a retry backoff.
func (p *Pool) QueueSize() uint32 { return p.queueSize.Load() }

// Concurrency returns the current concurrency cap.
func (p *Pool) Concurrency() uint32 { return p.sem.capacity() }

// RateLimitPerMinute returns the current rate-limit (or nil if disabled)
// — the shape PoolStats reports on the wire.
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

// GroupInfo is one live message group's snapshot row for the operator
// "blocked groups" view (R-04): which groups this pool is currently holding,
// how full each is, and why nothing may be moving. A "live" entry is one
// with a groupQueue in groupQs — buffered awaiting a drainer, actively being
// drained, or parked; a fully-drained group is deleted from groupQs (see
// drainGroup's empty-buffer exit), so it never shows up here.
type GroupInfo struct {
	// Group is the message group id (Message.GroupID()).
	Group string
	// PoolCode is this pool's code — carried per-row so a caller merging
	// snapshots across pools (the dashboard's blocked-groups panel spans
	// every pool) doesn't have to track which Pool a GroupInfo came from.
	PoolCode string
	// Buffered is how many messages are sitting in this group's FIFO right
	// now (not counting one a drainer currently holds mid-delivery — that
	// message has already been popped off the buffer).
	Buffered int
	// Working is true while a drainer goroutine owns this group (draining
	// it one message at a time). False means the group is buffered with no
	// drainer running — either freshly enqueued (about to be picked up) or
	// parked (see ParkedAt).
	Working bool
	// ParkedAt is when the group was last left with no drainer (clearWorking)
	// — the zero time while Working is true, or if it has never been parked.
	// A parkedAt that keeps ageing with Working still false is the signature
	// ReleaseParkedGroups sweeps: nothing has come back to resume the group.
	ParkedAt time.Time
	// Suppressed reports whether GroupFlushRegistry currently suppresses
	// this group (a target asked to stop receiving it — see
	// GroupFlushRegistry's doc comment). SuppressedUntil is the zero time
	// when Suppressed is false.
	Suppressed      bool
	SuppressedUntil time.Time
}

// GroupSnapshot returns a point-in-time view of every live message group
// this pool is holding, for the operator "blocked groups" view (R-04).
//
// Thread-safe and allocation-light: one slice sized to len(groupQs) up
// front, no further allocation beyond the returned GroupInfo values
// themselves. p.mu is held only long enough to copy each groupQueue's
// group/working/parkedAt/len(msgs) — the GroupFlushRegistry lookup (which
// takes its OWN lock) happens after p.mu is released, so the two locks are
// never nested.
func (p *Pool) GroupSnapshot() []GroupInfo {
	p.mu.Lock()
	out := make([]GroupInfo, 0, len(p.groupQs))
	for group, gq := range p.groupQs {
		out = append(out, GroupInfo{
			Group:    group,
			PoolCode: p.cfg.Code,
			Buffered: len(gq.msgs),
			Working:  gq.working,
			ParkedAt: gq.parkedAt,
		})
	}
	p.mu.Unlock()

	for i := range out {
		if until, ok := p.flushes.SuppressedUntil(out[i].Group); ok {
			out[i].Suppressed = true
			out[i].SuppressedUntil = until
		}
	}
	return out
}

// FlushStats returns this pool's group-flush suppression stats (R-52/R-53):
// how many groups are currently suppressed, and the lifetime flush/suppressed
// counters — see GroupFlushRegistry.Stats.
func (p *Pool) FlushStats() (active int, flushes, suppressed uint64) {
	return p.flushes.Stats()
}

// ActiveFlushes returns every group currently suppressed on this pool
// (R-52's monitoring listing).
func (p *Pool) ActiveFlushes() []GroupSuppression {
	return p.flushes.ActiveSuppressions()
}

// ClearFlush lifts an active suppression on this pool early (R-52's operator
// override). Reports whether a currently-active suppression existed to
// lift.
func (p *Pool) ClearFlush(group string) bool {
	return p.flushes.Clear(group)
}

// Stats returns the dashboard-shaped snapshot of this pool.
func (p *Pool) Stats() PoolStats {
	m := p.metrics.Snapshot()
	return PoolStats{
		PoolCode:           p.cfg.Code,
		Concurrency:        p.Concurrency(),
		ActiveWorkers:      p.ActiveWorkers(),
		OldestMediatingMs:  uint64(p.OldestMediatingAge().Milliseconds()),
		QueueSize:          p.queueSize.Load(),
		QueueCapacity:      p.queueCapacity(),
		MessageGroupCount:  p.MessageGroupCount(),
		RateLimitPerMinute: p.RateLimitPerMinute(),
		IsRateLimited:      p.IsRateLimited(),
		Metrics:            &m,
		Histogram:          p.metrics.HistogramSnapshot(),
	}
}

// queueCapacityMultiplier and minQueueCapacity define the capacity
// derivation: capacity = max(concurrency * 20, 50).
const (
	queueCapacityMultiplier uint32 = 20
	minQueueCapacity        uint32 = 50
)

// queueCapacity is the ceiling on queueSize: how much pre-dispatch backlog the
// pool accepts before submit pushes back on the broker. Derived from the
// concurrency cap, so re-capping a pool re-sizes its buffer with it.
func (p *Pool) queueCapacity() uint32 {
	capacity := p.Concurrency() * queueCapacityMultiplier
	if capacity < minQueueCapacity {
		capacity = minQueueCapacity
	}
	return capacity
}

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
				p.nackMsg(ctx, msg, ptrU32(10), "pool stopped during drain")
				return
			}
			p.clearWorking(group)
			return
		}

		// Release the slot per iteration even if processOne panics past its own
		// recover — a bare deferred release would accumulate across the loop, so
		// scope it to a closure.
		d := func() Disposition {
			defer p.sem.release()
			return p.processOne(ctx, msg)
		}()

		if d.Action == BrokerRelease {
			// Target unreachable, or the in-pipeline retry budget is spent —
			// hand this message AND everything still buffered behind it back to
			// the broker, then exit. releaseGroup clears `working`, so a
			// redelivery spawns a fresh drainer.
			p.releaseGroup(ctx, group, msg, nackDelay(d.RetryAfter), "released to broker")
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
			p.ackBuffered(ctx, group, "head failed under BLOCK_ON_ERROR")
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
				p.nackMsg(ctx, msg, ptrU32(10), "pool stopped during retry")
				return
			}
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
func (p *Pool) releaseGroup(ctx context.Context, group string, inHand common.QueuedMessage, delay *uint32, reason string) {
	p.nackMsg(ctx, inHand, delay, reason)
	released := p.releaseBuffered(ctx, group, reason)
	slog.Info("released message group to broker",
		"group", group, "pool", p.cfg.Code, "message_id", inHand.Message.ID,
		"buffered_released", released, "reason", reason)
}

// takeBuffered empties a group's buffer and clears `working`, returning what
// was in it. Both terminal group outcomes start here and differ only in what
// they then do with the messages — hand them back, or ack them away.
func (p *Pool) takeBuffered(group string) []common.QueuedMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	gq := p.groupQs[group]
	if gq == nil {
		return nil
	}
	buffered := gq.msgs
	gq.msgs = nil
	gq.working = false
	return buffered
}

// releaseBuffered hands back everything still queued behind the head WITHOUT
// touching the head itself, and clears `working` so a redelivery spawns a fresh
// drainer. Used when the TARGET is unreachable: the messages are blameless and
// the broker should redeliver them once it recovers.
//
// Returns how many were released.
func (p *Pool) releaseBuffered(ctx context.Context, group, reason string) int {
	buffered := p.takeBuffered(group)
	for i := range buffered {
		p.queueDec()
		p.nackMsg(ctx, buffered[i], nil, reason)
	}
	if len(buffered) > 0 {
		slog.Info("released buffered group messages to broker",
			"group", group, "pool", p.cfg.Code, "released", len(buffered), "reason", reason)
	}
	return len(buffered)
}

// ackBuffered deletes everything still queued behind a head that failed
// terminally under BLOCK_ON_ERROR, and clears `working`.
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
func (p *Pool) ackBuffered(ctx context.Context, group, reason string) int {
	buffered := p.takeBuffered(group)
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
func (p *Pool) clearWorking(group string) {
	p.mu.Lock()
	if gq := p.groupQs[group]; gq != nil {
		gq.working = false
		gq.parkedAt = time.Now()
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
func (p *Pool) ReleaseParkedGroups(ctx context.Context, minAge time.Duration) int {
	now := time.Now()
	p.mu.Lock()
	var parked []string
	for group, gq := range p.groupQs {
		if gq.working || gq.empty() || gq.parkedAt.IsZero() {
			continue
		}
		if now.Sub(gq.parkedAt) > minAge {
			parked = append(parked, group)
		}
	}
	p.mu.Unlock()

	released := 0
	for _, group := range parked {
		// releaseBuffered re-takes p.mu and clears working (already false); a
		// drainer that claimed the group in the gap simply finds an empty
		// buffer, which is a normal drain exit.
		if n := p.releaseBuffered(ctx, group, "group parked with no drainer"); n > 0 {
			slog.Warn("released a parked message group to the broker; nothing had resumed it",
				"group", group, "pool", p.cfg.Code, "released", n, "parked_for", minAge)
			released += n
		}
	}
	return released
}

// BrokerAction is what a Disposition does to a message at the broker — one
// of the three broker actions the router's design supports (see
// docs/router-architecture.md, "Delivery outcomes"): acknowledge, retry in
// place, or release.
type BrokerAction int

const (
	// BrokerAck settles the message — it is gone from the broker, whether
	// because it succeeded or because it failed in a way retrying cannot fix.
	// processOne has already ACKed it (or, for a deduplicated copy, ACKed it
	// under its own receipt handle) by the time a Disposition carrying this
	// is returned; the caller has nothing further to do at the broker.
	BrokerAck BrokerAction = iota
	// BrokerRetry keeps the message in-flight; the caller re-dispatches after
	// Disposition.RetryAfter (front of the group for ordered, delayed
	// re-spawn for IMMEDIATE). Never touches the broker.
	BrokerRetry
	// BrokerRelease hands the message back to the broker — the TARGET is
	// unreachable (transport failure, 502/503/504, or an open breaker), so
	// nothing about the message is wrong and retrying it in-process would pin
	// it, and its whole group, in memory for the duration of an outage. This
	// is what makes "retry until the broker expires it" true: an in-pipeline
	// retry never returns the message, so the broker's expiry and DLQ can
	// never act on it. Disposition.RetryAfter (when non-zero) becomes the
	// requested redelivery delay.
	BrokerRelease
)

// GroupEffect is what a Disposition means for the REST of an ordered message
// group — meaningless for IMMEDIATE dispatch, which has no group buffer, and
// for BrokerRetry, which blocks the group on this same message until it
// resolves rather than doing anything to the others.
type GroupEffect int

const (
	// GroupContinue: the drainer moves on to the next buffered message (or
	// exits idle if there is none). Covers a success, a discarded failure
	// under NEXT_ON_ERROR/IMMEDIATE, and a discarded deduplicated copy.
	GroupContinue GroupEffect = iota
	// GroupBlock is BLOCK_ON_ERROR's defining behaviour: the head failed
	// terminally, so the drainer stops advancing and ACKs away everything
	// still buffered behind it rather than delivering successors past a
	// failure the mode exists to stop at.
	GroupBlock
	// GroupRelease: the drainer hands the whole group — this message plus
	// everything still buffered behind it — back to the broker. Releasing
	// only the head while successors stayed buffered would put the head
	// behind them on redelivery, reordering a group whose entire purpose is
	// ordering.
	GroupRelease
)

// DispositionMetric names which PoolMetricsCollector method a Disposition's
// outcome records, as DATA rather than a call — the reason DispositionOf can
// stay pure. recordMetric is the single place that turns this into an actual
// Record* call.
type DispositionMetric int

const (
	// MetricNone: nothing recorded — a deduplicated copy, a suppressed-flush
	// ACK, a circuit-open release (no delivery was attempted, so there is
	// nothing to measure), or a retry/release from a path with no mediation
	// outcome at all (a cancelled rate-limiter wait, a recovered panic).
	MetricNone DispositionMetric = iota
	MetricSuccess
	MetricFailure
	MetricTransient
	MetricRateLimited
)

// Disposition is processOne's pure verdict for a mediation outcome: what
// happens to THIS message at the broker (Action), what that means for the
// rest of an ordered group (Group), the metric to record, and the backoff to
// apply for a retry or the redelivery hint to carry on a release
// (RetryAfter).
//
// It replaces the old flat processResult enum, which encoded broker action
// and group effect in the SAME five values positionally — see
// docs/router-go-idiom-plan.md item 5 ("Restructure the outcome enum before
// it grows again"). DispositionOf is exported specifically so a test can
// call it directly: mediation_conformance_test.go used to be unable to
// assert message fate at all, because "the decision lives in an inline
// switch inside pool.go's delivery loop with nothing a test can call".
type Disposition struct {
	Action     BrokerAction
	Group      GroupEffect
	Metric     DispositionMetric
	RetryAfter time.Duration

	// budgetExhausted marks a BrokerRelease produced by retryOrRelease when
	// the in-pipeline retry budget (maxInPipelineAttempts) is spent, as
	// opposed to one produced directly by DispositionOf because the target is
	// unreachable. Unexported: it exists only so settleRetry can log the
	// distinct "budget exhausted" warning in one place: the mediation-outcome
	// path (DispositionOf) and the two paths that retry without ever
	// reaching a mediation outcome (a cancelled rate-limiter wait, a
	// recovered panic) all funnel through it.
	budgetExhausted bool
}

// maxInPipelineAttempts bounds how many times a message may be retried IN
// PLACE before it is handed back to the broker instead.
//
// It has to be bounded. An in-pipeline retry never returns the message, so
// while it loops the broker's expiry, redelivery count and DLQ can never act on
// it — and the stall detector deliberately leaves retrying entries alone, so
// nothing warns either. An endpoint answering 429 or `ack:false` for ever would
// otherwise pin the message, its whole ordered group, and its tracker entry
// (exempt from reaping) in memory for the life of the process, invisibly.
//
// On exhaustion the message is RELEASED, which is the same treatment an
// unreachable target gets and the only outcome that restores the broker's
// authority over it. The attempts spent here are on top of the mediator's own
// bounded burst inside each attempt.
const maxInPipelineAttempts uint = 10

const (
	// retryMinDelay / retryMaxDelay bound the in-pipeline backoff; panicRetryDelay
	// is the fixed backoff after a recovered panic.
	retryMinDelay   = 100 * time.Millisecond
	retryMaxDelay   = 5 * time.Minute
	panicRetryDelay = 10 * time.Second

	// Deferred (2xx + ack=false) gets its own backoff curve: the target is
	// healthy and answering cheap 200s, so recovery latency matters more
	// than politeness — shorter start, 1-minute ceiling.
	deferredMinDelay = 5 * time.Second
	deferredMaxDelay = time.Minute
)

// retryDelay computes the in-pipeline backoff before the next attempt:
// exponential in the attempt count (starting at retryMinDelay), with any
// server-requested delay (Retry-After on 429, the breaker reset on circuit-open,
// the 5xx retry hint) applied as a floor, capped at retryMaxDelay.
func retryDelay(attempts uint, outcomeDelaySec int) time.Duration {
	return backoffDelay(attempts, outcomeDelaySec, retryMinDelay, retryMaxDelay)
}

// deferredDelay is the backoff for 200+ack=false deferrals — same shape as
// retryDelay but on the deferred curve (5s start, 60s cap), still flooring at
// whatever delaySeconds the target requested.
func deferredDelay(attempts uint, outcomeDelaySec int) time.Duration {
	return backoffDelay(attempts, outcomeDelaySec, deferredMinDelay, deferredMaxDelay)
}

func backoffDelay(attempts uint, floorSec int, minDelay, maxDelay time.Duration) time.Duration {
	shift := attempts
	if shift > 12 { // cap the shift so the bit-shift can't overflow
		shift = 12
	}
	d := minDelay << shift
	if floor := time.Duration(floorSec) * time.Second; d < floor {
		d = floor
	}
	if d > maxDelay {
		d = maxDelay
	}
	return d
}

// DispositionOf maps a mediation outcome to its Disposition. Pure: no I/O, no
// metrics, no ack/flush/tracker calls — everything needed is passed in, so a
// test can call it directly instead of simulating a broker and a group
// buffer to observe the decision.
//
// attempts is qm.Attempts, the count of PRIOR in-pipeline attempts: it gates
// maxInPipelineAttempts for the two retryable outcomes (429, ack=false),
// past which they convert to a release instead of retrying forever — see
// maxInPipelineAttempts' doc comment.
//
// mode is the message's DispatchMode: only BLOCK_ON_ERROR changes the Group
// effect of a discarded failure, from GroupContinue to GroupBlock.
func DispositionOf(outcome common.MediationOutcome, attempts uint, mode common.DispatchMode) Disposition {
	switch outcome.Result {
	case common.MediationSuccess:
		return Disposition{Action: BrokerAck, Group: GroupContinue, Metric: MetricSuccess}

	case common.MediationErrorConfig:
		// Permanent ACK-drop: a 4xx, or (R-57) any 5xx the mediator has
		// already classified as "the app answered" rather than "unavailable"
		// (see mediator.go's status-code switch — every 5xx other than
		// 502/503/504 lands here). The mediator already recorded the breaker
		// success (reachable). BLOCK_ON_ERROR must stop the group at this
		// failure rather than deliver successors past it; every other mode
		// moves on to the next buffered message.
		group := GroupContinue
		if mode == common.DispatchBlockOnError {
			group = GroupBlock
		}
		return Disposition{Action: BrokerAck, Group: group, Metric: MetricFailure}

	case common.MediationErrorProcess:
		// Pre-classified by the mediator (R-57): reaching this case at all
		// means 502/503/504 — "never reached a working app", verdict AFTER
		// the mediator's bounded retry burst — so release rather than retry
		// in place. The status code is deliberately not inspected here: the
		// allowlist decision already happened once, in mediator.go, and a
		// second check here would just be a second place for it to drift
		// from the ruling.
		return Disposition{Action: BrokerRelease, Group: GroupRelease, Metric: MetricTransient}

	case common.MediationErrorConnection:
		// Transport failure / unreachable host / timeout — the target is down.
		return Disposition{Action: BrokerRelease, Group: GroupRelease, Metric: MetricFailure}

	case common.MediationRateLimited:
		// 429 — retry in-pipeline honouring Retry-After; NOT a breaker failure.
		return retryOrRelease(attempts, retryDelay(attempts, outcome.DelaySeconds), MetricRateLimited)

	case common.MediationDeferred:
		// 2xx + ack=false — the target explicitly deferred this message
		// (e.g. a blocked record). Not a failure, and the mediator skipped
		// its in-pipeline retries; requeue on the deferred curve (5s start,
		// 60s cap) flooring at any delay the target requested.
		return retryOrRelease(attempts, deferredDelay(attempts, outcome.DelaySeconds), MetricTransient)

	case common.MediationCircuitOpen:
		// Breaker open (decided by the mediator): no delivery was attempted,
		// so there is nothing to measure and the target is by definition
		// unavailable — same class as a transport failure, released to the
		// broker for the same reason.
		//
		// This MUST match the ErrorConnection disposition. The breaker opens
		// almost immediately during a sustained outage, so retrying
		// circuit-open in-pipeline would mean: first failure releases the
		// group, the broker redelivers, the redelivery finds an open breaker
		// and is pinned in-process again — reinstating the memory pinning
		// this release exists to prevent, in exactly the scenario it exists
		// for.
		return Disposition{Action: BrokerRelease, Group: GroupRelease, Metric: MetricNone}
	}
	// Unreachable: MediationResult is a closed enum handled exhaustively
	// above. Same silent fallback the original inline switch had.
	return Disposition{Action: BrokerAck, Group: GroupContinue}
}

// retryOrRelease applies the maxInPipelineAttempts budget: within budget,
// retry after delay; past it, release with delay as the broker's redelivery
// hint instead — the only way "retry until the broker expires it" stays true
// when a target answers 429/ack=false forever. Shared by DispositionOf (the
// two outcome-driven retryable results) and Pool.retryAfter (a cancelled
// rate-limiter wait, a recovered panic — paths that retry without ever
// reaching a mediation outcome, and so share the same budget and the same
// pure decision).
func retryOrRelease(attempts uint, delay time.Duration, metric DispositionMetric) Disposition {
	if attempts+1 >= maxInPipelineAttempts {
		return Disposition{Action: BrokerRelease, Group: GroupRelease, Metric: metric, RetryAfter: delay, budgetExhausted: true}
	}
	return Disposition{Action: BrokerRetry, Group: GroupContinue, Metric: metric, RetryAfter: delay}
}

// recordMetric applies a Disposition's Metric — data computed by
// DispositionOf/retryOrRelease — to the PoolMetricsCollector. The single
// place that turns "which metric" into an actual Record* call, so
// DispositionOf itself never touches p.metrics.
func (p *Pool) recordMetric(m DispositionMetric, durationMs uint64) {
	switch m {
	case MetricSuccess:
		p.metrics.RecordSuccess(durationMs)
	case MetricFailure:
		p.metrics.RecordFailure(durationMs)
	case MetricTransient:
		p.metrics.RecordTransient(durationMs)
	case MetricRateLimited:
		p.metrics.RecordRateLimited()
	case MetricNone:
		// Nothing to record — see MetricNone's doc comment.
	}
}

// processOne runs the per-message pipeline: track (first dispatch only), rate
// limit, mediate, and resolve by disposition. It does NOT release messages to
// the broker on failure — a retryable disposition keeps the in-flight entry
// and returns BrokerRetry so the caller retries in-pipeline (preserving order
// for grouped messages). Only a terminal 2xx/4xx ACKs and clears the entry.
//
// The decision itself lives in DispositionOf (pure); everything here is the
// side effects around it — tracking, rate limiting, mediating, metrics,
// ack/flush.
func (p *Pool) processOne(ctx context.Context, qm common.QueuedMessage) (result Disposition) {
	worker := p.beginMediating(qm)
	defer p.endMediating(worker)

	// Panic isolation: a panic mid-mediation must not crash the process (an
	// unrecovered panic in a goroutine takes down the program) or strand the
	// message. Recover and retry in-pipeline — the in-flight entry is kept, so
	// the redelivery dedup stays intact and the worker survives. Named returns
	// let the deferred recover set the verdict.
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in processOne; retrying in-pipeline",
				"message_id", qm.Message.ID, "panic", r)
			// Through the same funnel as every other retry: a target that
			// panics us on every attempt must not be retried for ever either.
			result = p.retryAfter(qm, panicRetryDelay)
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
				if err := c.Ack(ctx, qm.ReceiptHandle, qm.BrokerMessageID); err != nil {
					slog.Warn("ack (requeue duplicate) failed", "message_id", qm.Message.ID, "err", err)
				}
			}
			return Disposition{Action: BrokerAck, Group: GroupContinue}
		}
	}

	// Flushed group: the target already told us it cannot take this group
	// right now and asked us to ACK the siblings rather than deliver them.
	// Checked BEFORE the rate limiter so a flushed group spends neither a
	// token nor a concurrency slot — that saving is the whole point. Safe
	// only because the target asked: it owns the records and re-drives them
	// itself. Suppression is TTL-bounded, so once the window lapses the next
	// message probes the target again.
	if group := qm.Message.GroupID(); group != "" && p.flushes.Suppressed(group) {
		slog.Debug("message group flushed; ACKing without delivery",
			"message_id", qm.Message.ID, "group", group, "pool", p.cfg.Code)
		// R-53: a suppressed ACK previously left no pool metric at all, so a
		// pool suppressing a whole flushed group read idle rather than
		// busy-but-suppressed. No mediation happened (no HTTP call, no
		// duration), so this bypasses the DispositionOf/recordMetric funnel
		// that ties a metric to a mediation outcome — same reason the
		// duplicate-copy path above records nothing either.
		p.metrics.RecordSuppressed()
		p.ackTracked(ctx, qm)
		return Disposition{Action: BrokerAck, Group: GroupContinue}
	}

	// Rate limit (per-pool token bucket). Record a rate-limited event when the
	// limiter actually held us back (current tokens exhausted).
	if p.limiter.IsLimited() {
		p.metrics.RecordRateLimited()
	}
	if err := p.limiter.Wait(ctx); err != nil {
		// Context cancelled mid-wait — keep the entry and retry in-pipeline.
		return p.retry(qm, 5)
	}

	start := time.Now()
	outcome := p.mediator.Mediate(ctx, &qm.Message)
	durationMs := uint64(time.Since(start).Milliseconds())

	d := p.settleRetry(qm, DispositionOf(outcome, qm.Attempts, qm.Message.DispatchMode))
	p.recordMetric(d.Metric, durationMs)

	if d.Action == BrokerAck {
		// A 2xx carrying {"flushGroup": true} delivered normally but asks us
		// to suppress the rest of its group. DelaySeconds (when the target
		// sent one) sizes the window; the registry clamps it. Only reachable
		// alongside MediationSuccess — a target that wants the message back
		// (a discarded failure) cannot also discard the group.
		if outcome.Result == common.MediationSuccess && outcome.FlushGroup {
			if group := qm.Message.GroupID(); group != "" {
				ttl := time.Duration(outcome.DelaySeconds) * time.Second
				if p.flushes.Flush(group, ttl) {
					slog.Info("message group flushed by target",
						"group", group, "pool", p.cfg.Code,
						"message_id", qm.Message.ID, "delay_seconds", outcome.DelaySeconds)
				}
			} else {
				slog.Warn("flushGroup ignored: message has no message group",
					"message_id", qm.Message.ID, "pool", p.cfg.Code)
			}
		}
		p.ackTracked(ctx, qm)
	}
	return d
}

// retry marks the in-flight entry as retrying (so the stall detector / reaper
// skip it) and returns the retry-or-release Disposition for the computed
// backoff. Metric is always MetricNone: neither caller (a cancelled
// rate-limiter wait, a recovered panic) reflects a mediation outcome, so
// there is nothing to record.
func (p *Pool) retry(qm common.QueuedMessage, outcomeDelaySec int) Disposition {
	return p.retryAfter(qm, retryDelay(qm.Attempts, outcomeDelaySec))
}

// retryAfter is retry with a pre-computed backoff (used by the deferred path,
// which runs on its own delay curve, and by panic recovery). It funnels
// through retryOrRelease — the same budget DispositionOf's two retryable
// outcomes use — via settleRetry, which applies the side effects the pure
// decision can't make itself.
func (p *Pool) retryAfter(qm common.QueuedMessage, delay time.Duration) Disposition {
	return p.settleRetry(qm, retryOrRelease(qm.Attempts, delay, MetricNone))
}

// settleRetry applies retryOrRelease's side effects to a Disposition however
// it was produced — from DispositionOf's own retryable outcomes, or from
// retryAfter's two paths that retry without a mediation outcome — so the
// budget-exhausted warning and the tracker's retrying mark happen in exactly
// one place. A Disposition that isn't a retry-or-release verdict (success,
// discard, a release the target itself caused) passes through untouched.
func (p *Pool) settleRetry(qm common.QueuedMessage, d Disposition) Disposition {
	if d.budgetExhausted {
		slog.Warn("in-pipeline retry budget exhausted; releasing to broker",
			"message_id", qm.Message.ID, "pool", p.cfg.Code,
			"attempts", qm.Attempts+1, "max_attempts", maxInPipelineAttempts,
			"redelivery_delay", d.RetryAfter)
		return d
	}
	if d.Action == BrokerRetry && p.tracker != nil {
		p.tracker.MarkRetrying(qm.Message.ID, qm.BrokerMessageID)
	}
	return d
}

// nackDelay turns a computed backoff into a broker redelivery delay: whole
// seconds, at least one, or nil when there is nothing to wait out.
func nackDelay(d time.Duration) *uint32 {
	if d <= 0 {
		return nil
	}
	secs := uint32(d / time.Second)
	if secs == 0 {
		secs = 1
	}
	return &secs
}

func ptrU32(v uint32) *uint32 { return &v }
