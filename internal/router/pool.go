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

	// immQ holds unordered (IMMEDIATE) messages waiting for a concurrency
	// slot, oldest first. A message waiting here costs only its own
	// QueuedMessage: it has no goroutine until a slot is free (see
	// enqueueImmediate). The pool's buffer can hold concurrency*40 of them, so
	// a goroutine per waiting message put 256,000 goroutines (1.2GB of stack)
	// behind 100 pools of concurrency 64.
	immMu      sync.Mutex
	immQ       []immediateItem
	immHead    int  // index of the oldest element of immQ
	immRunning bool // a dispatchImmediate goroutine is alive

	// workers runs the IMMEDIATE deliveries that hold a slot (see
	// immediate_workers.go). It reuses goroutines across messages instead of
	// starting one per message, and lets them go when they sit idle.
	workers immediateWorkers

	// stopCh is closed by the first Stop, so idle workers leave at once
	// instead of waiting out their idle timeout. nil on a Pool built as a bare
	// struct literal: a nil channel never fires, and those workers just time out.
	stopCh   chan struct{}
	stopOnce sync.Once

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

	// full is whether queueSize was at or over queueCapacity() as of the
	// last call to capacityChanged — tracked so capacityFreed fires only on
	// the crossing back UNDER capacity, never on every admission or
	// completion (G12, docs/spec/router.md §3.2). See capacityChanged.
	full atomic.Bool

	// capacityFreed runs on that crossing — how a poll loop parked in
	// Manager.awaitCapacity learns there is room again without polling on a
	// fixed interval. nil (checked, not called) until the Manager wires it
	// in via SetCapacityFreed, which every pool-creation site does.
	capacityFreed func()

	// admission is the deferral schedule for messages that arrive while the
	// pool is full — see pool_admission.go.
	admission admissionSchedule

	// handbacks is the hand-back gate (pool_handback_gate.go): the ordered
	// groups with members this pool handed back to a broker that does not
	// keep a group in order itself. Guarded by mu.
	handbacks handbackGate

	// now is the clock the admission schedule and the hand-back gate read.
	// nil (every pool outside a test that sets it) is time.Now; see clock.
	now func() time.Time

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

	// events are the pool's event-time counters (event_counters.go), read by
	// the Prometheus scrape.
	events poolEventCounters

	// results receives one bool per delivery outcome the pool's success and
	// failure totals count (MetricSuccess, MetricFailure), feeding the health
	// service's rolling success rate. nil is a no-op. Set by Manager.wirePool.
	results func(poolCode string, success bool)

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
		concurrency = max(rate/60, 1)
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
		stopCh:          make(chan struct{}),
	}
}

// queueDec drops the pre-dispatch count by one. (atomic.Uint32 has no Sub;
// adding ^0 is the two's-complement decrement.)
func (p *Pool) queueDec() {
	p.queueSize.Add(^uint32(0))
	p.capacityChanged()
}

// queueInc raises the pre-dispatch count by one — the increment counterpart
// to queueDec, used everywhere queueSize grows (a fresh admission, a
// re-queue for backoff) so capacityChanged sees every mutation, not just
// decrements. Without this, an admission that pushes queueSize AT capacity
// leaves p.full stale at false, and the decrement that later crosses back
// under capacity would find full already false — swap reports "was not
// full" and capacityFreed never fires for a pool that plainly was.
func (p *Pool) queueInc() {
	p.queueSize.Add(1)
	p.capacityChanged()
}

// SetCapacityFreed registers the callback to run on the crossing back under
// capacity (docs/spec/router.md §3.2) — set once, by whatever registers
// this pool with a Manager. A pool built and used before this is called
// (every test that constructs one directly) simply never signals; nil is
// checked, not invoked.
func (p *Pool) SetCapacityFreed(fn func()) { p.capacityFreed = fn }

// capacityChanged re-evaluates queueSize against queueCapacity and runs
// capacityFreed exactly on the transition from full to not-full — never on
// every admission or completion, which at pool throughput would mean a
// wake-up on every single message. Called after every event that can
// change queueSize (queueInc, queueDec, and enqueue / enqueueFront, which
// count under p.mu and call this once they have let it go).
//
// A no-op when sem is nil: queueCapacity derives from Concurrency, which
// reads sem — nil only for a Pool built as a bare struct literal rather
// than via NewPool (whitebox tests exercising groupQs/flushes in
// isolation). Every pool NewPool builds, which is every pool a Manager
// ever routes through, has one.
func (p *Pool) capacityChanged() {
	if p.sem == nil {
		return
	}
	atCapacity := p.queueSize.Load() >= p.queueCapacity()
	wasFull := p.full.Swap(atCapacity)
	if wasFull && !atCapacity && p.capacityFreed != nil {
		p.capacityFreed()
	}
}

// consumerFor resolves the source consumer for a message via its origin
// queue (QueueIdentifier); nil when that queue was deregistered between
// routing and processing.
func (p *Pool) consumerFor(qm common.QueuedMessage) queue.Consumer {
	if p.resolveConsumer == nil {
		return nil
	}
	return p.resolveConsumer(qm.QueueIdentifier)
}

// honoursDelayedReturn reports whether qm's SOURCE consumer will actually
// hold the message back for a Nack's delay before redelivering it — R5
// (owner ruling 2026-09-17, docs/spec/router-deferral-handback.md, second
// unit). Gates DispositionOf's R1 hand-back for a delay-bearing
// MediationDeferred.
//
// A queue with no registered consumer (deregistered between routing and
// processing) answers false: nothing there to honour a hand-back, so the
// deferral is safer kept in memory than nacked away.
func (p *Pool) honoursDelayedReturn(qm common.QueuedMessage) bool {
	c := p.consumerFor(qm)
	return c != nil && c.HonoursDelayedReturn()
}

// ackTracked / nackMsg resolve a message's source consumer and apply the
// terminal action there — a pool processes messages routed from many queues, so
// the action must target the queue the message arrived on. A missing consumer
// (deregistered queue) is logged and skipped.

// isOrdered reports whether the message is delivered in per-group order: the
// same rule dispatch uses to choose the group drainer over the immediate path.
func isOrdered(qm common.QueuedMessage) bool {
	return qm.Message.DispatchMode.RequiresOrdering() && qm.Message.GroupID() != ""
}

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
		// An ordered message's group waits for this ack before the next
		// message is delivered, so it must not linger in a batching backend.
		if isOrdered(qm) {
			ctx = queue.WithUrgentAck(ctx)
		}
		if err := c.Ack(ctx, receipt, qm.BrokerMessageID); err != nil {
			p.logger(qm).Warn("ack failed", "err", err)
		}
	} else {
		p.logger(qm).Warn("ack: no consumer for queue")
	}
	if p.tracker != nil {
		p.tracker.Remove(qm.Message.ID, qm.BrokerMessageID)
	}
}

// nackMsg releases a message back to its source broker. It is used only for the
// non-retryable control paths (pool stopped, shutdown before dispatch, a
// released group). A full pool is not one of them — that is deferMsg, which
// is a Defer with a computed delay rather than a Nack. Retryable mediation
// failures do NOT go here; they are retried in-pipeline.
//
// The message is leaving the pipeline, so its in-flight entry (claimed at
// route time) is released first: a lingering entry would classify the coming
// redelivery as a duplicate and drop it — the message would never re-enter.
//
// The nack carries the FRESHEST receipt handle, read from that entry before
// it goes, exactly as ackTracked's ack does. A message held in the pipeline
// past its visibility timeout is claimed again by a later poll under a new
// handle, which the entry adopts (RegisterRedelivery) while qm keeps the one
// it was routed with. On Postgres a nack with the old handle matches no row
// and fails silently: the row stays claimed instead of nacked with a delay,
// a claimed row does not hold back the rest of its group, and a later member
// of the group was delivered before the released one came back.
func (p *Pool) nackMsg(ctx context.Context, qm common.QueuedMessage, delay *uint32, reason string) {
	receipt := qm.ReceiptHandle
	if p.tracker != nil {
		if rh, ok := p.tracker.CurrentReceipt(qm.Message.ID, qm.BrokerMessageID); ok {
			receipt = rh
		}
		p.tracker.Remove(qm.Message.ID, qm.BrokerMessageID)
	}
	c := p.consumerFor(qm)
	if c == nil {
		p.logger(qm).Warn("nack: no consumer for queue", "reason", reason)
		return
	}
	if err := c.Nack(ctx, receipt, delay); err != nil {
		p.logger(qm).Warn("nack failed", "reason", reason, "err", err)
	}
}

// nackSafely is nackMsg for a panic boundary: the nack that hands the
// message back must not itself be able to take the process down (it calls
// into the broker, which may be what panicked in the first place).
func (p *Pool) nackSafely(ctx context.Context, qm common.QueuedMessage, delay *uint32, reason string) {
	safely("pool.nack", func() { p.nackMsg(ctx, qm, delay, reason) }, p.msgAttrs(qm)...)
}

// logger is the per-message logger (see messageLogger).
func (p *Pool) logger(qm common.QueuedMessage) *slog.Logger {
	return messageLogger(p.cfg.Code, qm)
}

// msgAttrs is the correlation set every per-message log line in this pool
// carries: which message, which group, which pool, which queue. The same
// four keys everywhere, so one grep follows a message through the router.
func (p *Pool) msgAttrs(qm common.QueuedMessage) []any {
	return messageAttrs(p.cfg.Code, qm)
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
// after construction (Manager.SetSettledReporter hands every pool it creates
// the reporter internal/server/run.go wires) so every NewPool call site keeps working
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

// Stop signals the pool to exit and hands every buffered (not yet
// dispatched) message back to its broker. The buffer is abandoned on stop —
// nothing will ever drain it — so each message is NACKed at once rather
// than left to reappear only when its visibility timeout lapses (minutes on
// SQS, during which its group is stuck behind it). nackMsg also releases
// its in-flight tracker entry, so the redelivery re-enters the pipeline
// fresh (routed per the new config) instead of being dropped as a
// duplicate. In-flight workers drain out on their own and ack/remove per
// outcome.
//
// A group whose drainer is still delivering its head gets the sibling delay
// (siblingNackDelay): its successors must not surface before that head is
// settled. An idle or parked group comes back straight away.
//
// Stop calls into the broker, so callers must not hold the Manager's
// consumer lock (the nack resolves the consumer through it); Manager.Shutdown
// stops pools before it tears consumers down, so the nacks still reach them.
func (p *Pool) Stop() {
	p.stopped.Store(true)
	p.stopOnce.Do(func() {
		if p.stopCh != nil {
			close(p.stopCh) // idle IMMEDIATE workers exit now
		}
	})
	type flushedGroup struct {
		msgs    []common.QueuedMessage
		working bool
	}
	p.mu.Lock()
	var groups []flushedGroup
	for _, gq := range p.groupQs {
		if len(gq.msgs) > 0 {
			groups = append(groups, flushedGroup{msgs: gq.msgs, working: gq.working})
		}
		gq.msgs = nil
	}
	p.groupQs = make(map[string]*groupQueue)
	p.mu.Unlock()

	if len(groups) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), stopNackTimeout)
	defer cancel()
	flushed := 0
	for _, g := range groups {
		p.events.reject(RejectStopped, len(g.msgs))
		var delay *uint32
		if g.working {
			delay = siblingNackDelay(nil)
		}
		for i := range g.msgs {
			p.queueDec()
			p.nackSafely(ctx, g.msgs[i], delay, "pool stopped")
			flushed++
		}
	}
	slog.Info("pool stopped; nacked buffered messages back to the broker",
		"pool", p.cfg.Code, "count", flushed)
}

// stopNackTimeout bounds the nacks Stop sends for its abandoned buffer, so a
// broker that has gone away cannot hold shutdown up. A nack that does not
// make it simply leaves that message to its visibility timeout, as before.
const stopNackTimeout = 10 * time.Second

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
	defer func() {
		if r := recover(); r != nil {
			logRecovered("pool.awaitDrainAndStop", r, "pool", p.cfg.Code)
		}
	}()
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

// queueCapacityMultiplier and minQueueCapacity define the capacity
// derivation: capacity = max(concurrency * 40, 100). Doubled from 20/50
// (owner ruling 2026-09-22): a buffered message is a pointer-sized struct,
// and twice the buffer is twice the burst a pool absorbs before it starts
// deferring messages to the broker. (It also doubles how long a buffered
// message waits, and so how many visibility lapses the tracker dedups for
// it while it waits — accepted.)
const (
	queueCapacityMultiplier uint32 = 40
	minQueueCapacity        uint32 = 100
)

// queueCapacity is the ceiling on queueSize: how much pre-dispatch backlog the
// pool accepts before submit pushes back on the broker. Derived from the
// concurrency cap, so re-capping a pool re-sizes its buffer with it.
func (p *Pool) queueCapacity() uint32 {
	capacity := max(p.Concurrency()*queueCapacityMultiplier, minQueueCapacity)
	return capacity
}
