package router

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// Pool is a per-pool drain that respects:
//   - configured concurrency (semaphore-style worker cap),
//   - configured rate limit (per-pool token bucket),
//   - per-endpoint circuit breakers,
//   - FIFO ordering within message groups (when DispatchMode requires it).
//
// One Pool services exactly one queue. Multiple queues fan into multiple Pools.
type Pool struct {
	cfg       common.PoolConfig
	consumer  queue.Consumer
	mediator  Mediator
	breakers  *BreakerRegistry
	limiter   *RateLimiter
	tracker   *InFlightTracker
	metrics   *PoolMetricsCollector

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

	stopped atomic.Bool

	// Batch+group FIFO cascade tracking. Mirrors Rust pool.rs
	// failed_batch_groups / batch_group_message_count: once any message in a
	// (batch_id, message_group) fails, the rest of that batch+group is NACKed
	// un-attempted to preserve FIFO ordering. The state clears once all of the
	// batch+group's messages have drained (count → 0). Only messages carrying
	// a BatchID participate.
	bgMu              sync.Mutex
	failedBatchGroups map[string]struct{}
	batchGroupCount   map[string]int
	batchCounter      atomic.Uint64 // monotonic per-poll-batch id (Rust batch_counter)
}

// groupQueue is the per-message-group buffer. High-priority messages
// (Message.HighPriority=true) drain ahead of regular messages within
// the same group; ordering within each priority class is FIFO. Mirrors
// the Rust MessageGroupHandler in crates/fc-router/src/pool.rs:99-140
// where high_priority and regular sit in separate VecDeques and the
// drain loop pops high_priority first.
type groupQueue struct {
	highPriority []common.QueuedMessage
	regular      []common.QueuedMessage
	working      bool
}

// pop returns the next message to dispatch (high-priority first) and
// whether the queue is now empty. Caller holds p.mu.
func (gq *groupQueue) pop() (common.QueuedMessage, bool) {
	if len(gq.highPriority) > 0 {
		m := gq.highPriority[0]
		gq.highPriority = gq.highPriority[1:]
		return m, len(gq.highPriority) == 0 && len(gq.regular) == 0
	}
	m := gq.regular[0]
	gq.regular = gq.regular[1:]
	return m, len(gq.highPriority) == 0 && len(gq.regular) == 0
}

// empty reports whether the queue holds no pending messages. Caller
// holds p.mu.
func (gq *groupQueue) empty() bool {
	return len(gq.highPriority) == 0 && len(gq.regular) == 0
}

// NewPool wires a pool. tracker may be nil; if so, in-flight tracking
// (and consequently stall detection + duplicate filtering) is disabled
// for messages handled by this pool.
func NewPool(cfg common.PoolConfig, consumer queue.Consumer, mediator Mediator, breakers *BreakerRegistry, tracker *InFlightTracker) *Pool {
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
		cfg:      cfg,
		consumer: consumer,
		mediator: mediator,
		breakers: breakers,
		limiter:  NewRateLimiter(rate),
		tracker:  tracker,
		metrics:  NewPoolMetricsCollector(),
		groupQs:  make(map[string]*groupQueue),

		failedBatchGroups: make(map[string]struct{}),
		batchGroupCount:   make(map[string]int),
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

// Consumer exposes the underlying queue consumer (for metrics aggregation).
func (p *Pool) Consumer() queue.Consumer { return p.consumer }

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

// Run starts the drain loop. Owns the polling cadence; spawns one
// drainGroup goroutine per active message group via tryDrainGroup.
//
// Exit conditions (any one returns):
//   - ctx is cancelled (graceful shutdown via Manager).
//   - p.Stop() was called (sets stopped=true; observed at top of loop).
//   - p.consumer.Poll returns an error AND ctx is already cancelled.
//
// Run does NOT wait for in-flight drainGroup goroutines to finish.
// Manager.Shutdown is responsible for joining workers via the wait group.
func (p *Pool) Run(ctx context.Context) {
	const maxPoll = 10
	pollInterval := 100 * time.Millisecond

	for {
		if p.stopped.Load() {
			return
		}
		// Non-blocking ctx check before poll — exits without paying the
		// poll round-trip on shutdown.
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgs, err := p.consumer.Poll(ctx, maxPoll)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("pool poll error", "pool", p.cfg.Code, "err", err)
			// Backoff 1s on transient poll failure. Wakeup conditions:
			//   <-ctx.Done()       — shutdown; exit immediately.
			//   <-time.After(1s)   — backoff elapsed; retry the poll.
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}

		if len(msgs) == 0 {
			// Empty poll — sleep pollInterval before next poll. Wakeup:
			//   <-ctx.Done()                  — shutdown; exit.
			//   <-time.After(pollInterval)    — go poll again.
			select {
			case <-ctx.Done():
				return
			case <-time.After(pollInterval):
				continue
			}
		}

		// Assign one batch id to every message in this poll batch (Rust
		// manager.rs batch_counter) so a failure preserves FIFO order across
		// the messages received together.
		batchID := strconv.FormatUint(p.batchCounter.Add(1), 10)

		// Submit each message. IMMEDIATE-mode messages (the default) dispatch
		// concurrently; ordered modes serialize per message group.
		for _, m := range msgs {
			m.BatchID = batchID
			p.submit(ctx, m)
		}
	}
}

// submit routes one polled message, 1:1 with Rust ProcessPool::submit. It
// runs the shared bookkeeping — capacity backpressure, batch+group FIFO
// count, and the early failed-group NACK — then branches on DispatchMode:
// IMMEDIATE-mode messages (the default, RequiresOrdering()==false) dispatch
// concurrently via runImmediate (one worker per message, bounded only by
// the pool semaphore), while ordered modes enqueue into the per-group FIFO
// buffer and drain serially.
func (p *Pool) submit(ctx context.Context, m common.QueuedMessage) {
	// Capacity backpressure: NACK (delay 10) when the pre-dispatch buffer is
	// already at capacity = max(concurrency*20, 50). Mirrors Rust's submit.
	capacity := p.concurrency.Load() * queueCapacityMultiplier
	if capacity < minQueueCapacity {
		capacity = minQueueCapacity
	}
	if p.queueSize.Load() >= capacity {
		delay := uint32(10)
		if err := p.consumer.Nack(ctx, m.ReceiptHandle, &delay); err != nil {
			slog.Warn("nack (pool at capacity) failed", "msg", m.Message.ID, "err", err)
		}
		return
	}

	// Batch+group FIFO cascade (Rust pool.rs submit): count the message, and
	// if its batch+group already failed, NACK it now so ordering is preserved.
	if key, ok := p.batchKey(m); ok {
		p.bgIncrement(key)
		if p.bgFailed(key) {
			p.bgDecrementAndCleanup(key)
			delay := uint32(10)
			if err := p.consumer.Nack(ctx, m.ReceiptHandle, &delay); err != nil {
				slog.Warn("nack (batch+group failed) failed", "msg", m.Message.ID, "err", err)
			}
			return
		}
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
	p.enqueue(group, m)
	p.tryDrainGroup(ctx, group)
}

// runImmediate dispatches a single IMMEDIATE-mode message concurrently:
// acquire a pool semaphore slot, then process it. Unlike the ordered drain,
// an IMMEDIATE message does NOT mark its batch+group failed on error
// (cascade=false) — each message is independent. 1:1 with Rust
// spawn_immediate_task.
func (p *Pool) runImmediate(ctx context.Context, m common.QueuedMessage) {
	sem := p.loadSem()
	select {
	case <-ctx.Done():
		// Shutdown before we could start — release the bookkeeping and NACK
		// for prompt redelivery (mirrors Rust's semaphore-closed path).
		p.queueSize.Add(^uint32(0))
		if key, ok := p.batchKey(m); ok {
			p.bgDecrementAndCleanup(key)
		}
		delay := uint32(10)
		_ = p.consumer.Nack(ctx, m.ReceiptHandle, &delay)
		return
	case sem <- struct{}{}:
	}
	p.queueSize.Add(^uint32(0)) // now active, not queued
	p.processOne(ctx, m, false)
	<-sem
}

// Stop signals the pool to exit. Run will return on its next loop.
func (p *Pool) Stop() { p.stopped.Store(true) }

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
	}
}

// queueCapacityMultiplier and minQueueCapacity mirror the Java/Rust
// derivation: capacity = max(concurrency * 20, 50). Used by Stats() so
// the dashboard's "queue capacity" matches the reference implementations.
const (
	queueCapacityMultiplier uint32 = 20
	minQueueCapacity        uint32 = 50
)

// batchKey returns the batch+group tracking key for m and whether m
// participates in batch+group FIFO tracking (only messages with a BatchID
// do). The key is internal-only and never crosses the wire.
func (p *Pool) batchKey(m common.QueuedMessage) (string, bool) {
	if m.BatchID == "" {
		return "", false
	}
	group := ""
	if m.Message.MessageGroupID != nil {
		group = *m.Message.MessageGroupID
	}
	return m.BatchID + "\x00" + group, true
}

// bgIncrement records one more in-flight message for the batch+group.
func (p *Pool) bgIncrement(key string) {
	p.bgMu.Lock()
	p.batchGroupCount[key]++
	p.bgMu.Unlock()
}

// bgFailed reports whether the batch+group has already had a failure.
func (p *Pool) bgFailed(key string) bool {
	p.bgMu.Lock()
	_, ok := p.failedBatchGroups[key]
	p.bgMu.Unlock()
	return ok
}

// bgMarkFailed marks the batch+group failed so its remaining messages cascade-NACK.
func (p *Pool) bgMarkFailed(key string) {
	p.bgMu.Lock()
	p.failedBatchGroups[key] = struct{}{}
	p.bgMu.Unlock()
}

// bgDecrementAndCleanup decrements the batch+group's in-flight count and, when
// it reaches zero, drops both the count and failed-state entries so a later
// batch reusing the same key starts clean. Mirrors Rust
// decrement_and_cleanup_batch_group.
func (p *Pool) bgDecrementAndCleanup(key string) {
	p.bgMu.Lock()
	if n, ok := p.batchGroupCount[key]; ok {
		if n <= 1 {
			delete(p.batchGroupCount, key)
			delete(p.failedBatchGroups, key)
		} else {
			p.batchGroupCount[key] = n - 1
		}
	}
	p.bgMu.Unlock()
}

func (p *Pool) enqueue(group string, m common.QueuedMessage) {
	p.mu.Lock()
	gq, ok := p.groupQs[group]
	if !ok {
		gq = &groupQueue{}
		p.groupQs[group] = gq
	}
	if m.Message.HighPriority {
		gq.highPriority = append(gq.highPriority, m)
	} else {
		gq.regular = append(gq.regular, m)
	}
	p.mu.Unlock()
	p.queueSize.Add(1)
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
//   - the group buffer is empty (working flag flipped back to false).
//   - ctx is cancelled while waiting for a semaphore slot.
//
// Note: ctx cancellation between processOne calls does NOT stop the loop
// — only the semaphore-acquire select is ctx-aware. This is intentional;
// a cancellation mid-process is handled inside processOne / mediator.
func (p *Pool) drainGroup(ctx context.Context, group string) {
	for {
		p.mu.Lock()
		gq := p.groupQs[group]
		if gq == nil || gq.empty() {
			if gq != nil {
				gq.working = false
			}
			p.mu.Unlock()
			return
		}
		msg, _ := gq.pop()
		p.mu.Unlock()
		// Pop happens under p.mu before any await, so queueSize stays
		// consistent with what's actually buffered in groupQs.
		p.queueSize.Add(^uint32(0)) // atomic decrement

		// Batch+group FIFO cascade re-check (Rust pool.rs drain loop): the
		// group may have failed after this message was enqueued. If so, NACK
		// it instead of dispatching, preserving order.
		if key, ok := p.batchKey(msg); ok && p.bgFailed(key) {
			p.bgDecrementAndCleanup(key)
			delay := uint32(10)
			if err := p.consumer.Nack(ctx, msg.ReceiptHandle, &delay); err != nil {
				slog.Warn("nack (batch+group failed) failed", "msg", msg.Message.ID, "err", err)
			}
			continue
		}

		// Acquire a concurrency slot. Snapshot the channel locally so a
		// resize between acquire and release doesn't cross channels.
		// Wakeup conditions:
		//   <-ctx.Done()         — shutdown; abandon this message.
		//   sem <- struct{}{}    — slot acquired; proceed.
		sem := p.loadSem()
		select {
		case <-ctx.Done():
			return
		case sem <- struct{}{}:
		}

		p.processOne(ctx, msg, true)
		// Release the slot back to the same channel we acquired from.
		<-sem
	}
}

// processOne runs the full per-message pipeline: dedup, rate limit,
// circuit breaker, mediate, and ack/nack/defer by outcome. cascade controls
// whether a transient failure marks this message's batch+group failed so
// later ordered messages cascade-NACK — true for the ordered serial drain,
// false for IMMEDIATE-mode workers (which are independent of each other).
func (p *Pool) processOne(ctx context.Context, qm common.QueuedMessage, cascade bool) {
	p.activeWorkers.Add(1)
	defer p.activeWorkers.Add(^uint32(0)) // atomic decrement

	// Batch+group FIFO cascade: this delivery was counted at enqueue, so
	// release its slot on every exit path. Mirrors Rust's post-process
	// decrement_and_cleanup_batch_group.
	bgKey, hasBatchGroup := p.batchKey(qm)
	if hasBatchGroup {
		defer p.bgDecrementAndCleanup(bgKey)
	}

	// Record in-flight (and short-circuit on duplicate redelivery).
	var imRef *common.InFlightMessage
	if p.tracker != nil {
		im := common.NewInFlightMessage(&qm.Message, qm.BrokerMessageID, qm.QueueIdentifier, qm.BatchID, qm.ReceiptHandle)
		existing, isDuplicate := p.tracker.Insert(im)
		if isDuplicate {
			// Broker redelivered while we're still processing. Swap the
			// receipt handle on the original tracker entry and return —
			// the original goroutine still owns the work.
			slog.Debug("duplicate redelivery; swapped receipt handle",
				"msg", existing.MessageID, "queue", qm.QueueIdentifier)
			return
		}
		imRef = im
		defer p.tracker.Remove(im.MessageID, im.BrokerMessageID)
	}
	_ = imRef // referenced via defer

	// Rate limit (per-pool token bucket). Record a rate-limited event
	// when the limiter actually held us back (current tokens exhausted).
	if p.limiter.IsLimited() {
		p.metrics.RecordRateLimited()
	}
	if err := p.limiter.Wait(ctx); err != nil {
		// Context cancelled mid-wait — defer the message and exit.
		_ = p.consumer.Defer(ctx, qm.ReceiptHandle, ptrU32(5))
		return
	}

	// Circuit breaker per target URL.
	cb := p.breakers.Get(qm.Message.MediationTarget)
	if err := cb.Allow(); err != nil {
		// Breaker open: the message can't be delivered now, so (for ordered
		// messages) mark its batch+group failed to keep later messages in
		// order, then defer until the breaker's open timeout elapses.
		if cascade && hasBatchGroup {
			p.bgMarkFailed(bgKey)
		}
		_ = p.consumer.Defer(ctx, qm.ReceiptHandle, ptrU32(uint32(DefaultBreakerConfig().OpenTimeout.Seconds())))
		return
	}

	start := time.Now()
	outcome := p.mediator.Mediate(ctx, &qm.Message)
	durationMs := uint64(time.Since(start).Milliseconds())

	switch outcome.Result {
	case common.MediationSuccess:
		cb.RecordSuccess()
		p.metrics.RecordSuccess(durationMs)
		if err := p.consumer.Ack(ctx, qm.ReceiptHandle); err != nil {
			slog.Warn("ack failed", "msg", qm.Message.ID, "err", err)
		}

	case common.MediationErrorConfig:
		// 4xx — ACK to avoid infinite retries. Do NOT trip the breaker.
		// (The destination is "healthy" in the sense that it responded.)
		// Rust counts this against total_failure (it was a non-success
		// terminal outcome), so we do the same.
		p.metrics.RecordFailure(durationMs)
		if err := p.consumer.Ack(ctx, qm.ReceiptHandle); err != nil {
			slog.Warn("ack (config error) failed", "msg", qm.Message.ID, "err", err)
		}

	case common.MediationErrorProcess:
		cb.RecordFailure()
		// Transient: message will be redelivered, so don't penalise
		// the all-time failure counter. Matches Rust record_transient.
		p.metrics.RecordTransient(durationMs)
		// FIFO cascade (ordered only): mark the batch+group failed so the
		// remaining ordered messages NACK.
		if cascade && hasBatchGroup {
			p.bgMarkFailed(bgKey)
		}
		delay := uint32(outcome.DelaySeconds)
		if err := p.consumer.Nack(ctx, qm.ReceiptHandle, &delay); err != nil {
			slog.Warn("nack (process error) failed", "msg", qm.Message.ID, "err", err)
		}

	case common.MediationErrorConnection:
		cb.RecordFailure()
		p.metrics.RecordFailure(durationMs)
		// FIFO cascade (ordered only): mark the batch+group failed so the
		// remaining ordered messages NACK.
		if cascade && hasBatchGroup {
			p.bgMarkFailed(bgKey)
		}
		delay := uint32(outcome.DelaySeconds)
		if err := p.consumer.Nack(ctx, qm.ReceiptHandle, &delay); err != nil {
			slog.Warn("nack (connection error) failed", "msg", qm.Message.ID, "err", err)
		}

	case common.MediationRateLimited:
		// 429 — defer with Retry-After; NOT a breaker failure.
		p.metrics.RecordRateLimited()
		delay := uint32(outcome.DelaySeconds)
		if err := p.consumer.Defer(ctx, qm.ReceiptHandle, &delay); err != nil {
			slog.Warn("defer (rate limited) failed", "msg", qm.Message.ID, "err", err)
		}
	}
}

func ptrU32(v uint32) *uint32 { return &v }
