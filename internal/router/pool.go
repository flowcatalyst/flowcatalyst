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

	sem chan struct{} // concurrency semaphore

	mu         sync.Mutex
	groupQs    map[string]*groupQueue // ordered FIFO queues per message-group
	inFlight   atomic.Int64
	stopped    atomic.Bool
}

// groupQueue is the per-message-group buffer.
type groupQueue struct {
	msgs    []common.QueuedMessage
	working bool
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
		concurrency = 1
	}
	return &Pool{
		cfg:      cfg,
		consumer: consumer,
		mediator: mediator,
		breakers: breakers,
		limiter:  NewRateLimiter(rate),
		tracker:  tracker,
		sem:      make(chan struct{}, concurrency),
		groupQs:  make(map[string]*groupQueue),
	}
}

// Consumer exposes the underlying queue consumer (for metrics aggregation).
func (p *Pool) Consumer() queue.Consumer { return p.consumer }

// Identifier is the pool code.
func (p *Pool) Identifier() string { return p.cfg.Code }

// SetRateLimit hot-swaps the rate-limit-per-minute value.
func (p *Pool) SetRateLimit(perMinute uint32) { p.limiter.SetRate(perMinute) }

// Run starts the drain loop. Returns when ctx is cancelled.
func (p *Pool) Run(ctx context.Context) {
	const maxPoll = 10
	pollInterval := 100 * time.Millisecond

	for {
		if p.stopped.Load() {
			return
		}
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
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}

		if len(msgs) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(pollInterval):
				continue
			}
		}

		// Enqueue messages into per-group buffers and kick off drains.
		for _, m := range msgs {
			group := ""
			if m.Message.MessageGroupID != nil {
				group = *m.Message.MessageGroupID
			}
			p.enqueue(group, m)
			p.tryDrainGroup(ctx, group)
		}
	}
}

// Stop signals the pool to exit. Run will return on its next loop.
func (p *Pool) Stop() { p.stopped.Store(true) }

// InFlight returns the count of messages currently in worker goroutines.
func (p *Pool) InFlight() int64 { return p.inFlight.Load() }

func (p *Pool) enqueue(group string, m common.QueuedMessage) {
	p.mu.Lock()
	gq, ok := p.groupQs[group]
	if !ok {
		gq = &groupQueue{}
		p.groupQs[group] = gq
	}
	gq.msgs = append(gq.msgs, m)
	p.mu.Unlock()
}

// tryDrainGroup starts a drainer for the group if none is running.
// Group ordering: only one outstanding message per group at a time when
// DispatchMode requires ordering. For Immediate mode each message can
// be processed concurrently (but we still single-thread per group for
// simplicity; the concurrency budget across groups is the pool's `sem`).
func (p *Pool) tryDrainGroup(ctx context.Context, group string) {
	p.mu.Lock()
	gq := p.groupQs[group]
	if gq == nil || gq.working || len(gq.msgs) == 0 {
		p.mu.Unlock()
		return
	}
	gq.working = true
	p.mu.Unlock()

	go p.drainGroup(ctx, group)
}

func (p *Pool) drainGroup(ctx context.Context, group string) {
	for {
		p.mu.Lock()
		gq := p.groupQs[group]
		if gq == nil || len(gq.msgs) == 0 {
			if gq != nil {
				gq.working = false
			}
			p.mu.Unlock()
			return
		}
		msg := gq.msgs[0]
		gq.msgs = gq.msgs[1:]
		p.mu.Unlock()

		// Acquire concurrency token (pool-wide).
		select {
		case <-ctx.Done():
			return
		case p.sem <- struct{}{}:
		}

		p.processOne(ctx, msg)
		<-p.sem
	}
}

func (p *Pool) processOne(ctx context.Context, qm common.QueuedMessage) {
	p.inFlight.Add(1)
	defer p.inFlight.Add(-1)

	// Record in-flight (and short-circuit on duplicate redelivery).
	var imRef *common.InFlightMessage
	if p.tracker != nil {
		im := common.NewInFlightMessage(&qm.Message, qm.BrokerMessageID, qm.QueueIdentifier, "", qm.ReceiptHandle)
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

	// Rate limit (per-pool token bucket).
	if err := p.limiter.Wait(ctx); err != nil {
		// Context cancelled mid-wait — defer the message and exit.
		_ = p.consumer.Defer(ctx, qm.ReceiptHandle, ptrU32(5))
		return
	}

	// Circuit breaker per target URL.
	cb := p.breakers.Get(qm.Message.MediationTarget)
	if err := cb.Allow(); err != nil {
		// Defer until the breaker's open timeout elapses.
		_ = p.consumer.Defer(ctx, qm.ReceiptHandle, ptrU32(uint32(DefaultBreakerConfig().OpenTimeout.Seconds())))
		return
	}

	outcome := p.mediator.Mediate(ctx, &qm.Message)
	switch outcome.Result {
	case common.MediationSuccess:
		cb.RecordSuccess()
		if err := p.consumer.Ack(ctx, qm.ReceiptHandle); err != nil {
			slog.Warn("ack failed", "msg", qm.Message.ID, "err", err)
		}

	case common.MediationErrorConfig:
		// 4xx — ACK to avoid infinite retries. Do NOT trip the breaker.
		// (The destination is "healthy" in the sense that it responded.)
		if err := p.consumer.Ack(ctx, qm.ReceiptHandle); err != nil {
			slog.Warn("ack (config error) failed", "msg", qm.Message.ID, "err", err)
		}

	case common.MediationErrorProcess:
		cb.RecordFailure()
		delay := uint32(outcome.DelaySeconds)
		if err := p.consumer.Nack(ctx, qm.ReceiptHandle, &delay); err != nil {
			slog.Warn("nack (process error) failed", "msg", qm.Message.ID, "err", err)
		}

	case common.MediationErrorConnection:
		cb.RecordFailure()
		delay := uint32(outcome.DelaySeconds)
		if err := p.consumer.Nack(ctx, qm.ReceiptHandle, &delay); err != nil {
			slog.Warn("nack (connection error) failed", "msg", qm.Message.ID, "err", err)
		}

	case common.MediationRateLimited:
		// 429 — defer with Retry-After; NOT a breaker failure.
		delay := uint32(outcome.DelaySeconds)
		if err := p.consumer.Defer(ctx, qm.ReceiptHandle, &delay); err != nil {
			slog.Warn("defer (rate limited) failed", "msg", qm.Message.ID, "err", err)
		}
	}
}

func ptrU32(v uint32) *uint32 { return &v }
