// Package pool provides the message processing pool implementation
package pool

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"go.flowcatalyst.tech/internal/common/metrics"
)

// MessagePointer represents a message to be processed
// This struct is used internally within the router/pool and contains all
// the information needed for mediation.
type MessagePointer struct {
	ID              string // Application message ID (JobID)
	SQSMessageID    string // Broker message ID for deduplication (new - like Java)
	BatchID         string
	MessageGroupID  string
	MediationTarget string            // URL to POST to for mediation
	MediationType   string            // Type of mediation (HTTP, etc.)
	AuthToken       string            // HMAC auth token for Bearer authentication
	Payload         []byte            // Original payload (for non-pointer mode)
	Headers         map[string]string // Additional headers
	TimeoutSeconds  int
	AckFunc         func() error
	NakFunc         func() error
	NakDelayFunc    func(time.Duration) error
	InProgressFunc  func() error
}

// MediationResult represents the result of mediation
type MediationResult string

const (
	MediationResultSuccess         MediationResult = "SUCCESS"
	MediationResultErrorConfig     MediationResult = "ERROR_CONFIG"     // 4xx - don't retry
	MediationResultErrorProcess    MediationResult = "ERROR_PROCESS"    // 5xx or ack=false - retry
	MediationResultErrorConnection MediationResult = "ERROR_CONNECTION" // Connection error - retry
)

// MediationOutcome represents the outcome of mediation including optional delay
type MediationOutcome struct {
	Result      MediationResult
	Delay       *time.Duration
	Error       error
	StatusCode  int
	ResponseAck *bool
}

// HasCustomDelay returns true if a custom delay is set
func (o *MediationOutcome) HasCustomDelay() bool {
	return o.Delay != nil
}

// GetEffectiveDelaySeconds returns the delay in seconds
func (o *MediationOutcome) GetEffectiveDelaySeconds() int {
	if o.Delay == nil {
		return 0
	}
	return int(o.Delay.Seconds())
}

// Mediator processes messages
type Mediator interface {
	Process(msg *MessagePointer) *MediationOutcome
}

// MessageCallback handles ack/nack operations
type MessageCallback interface {
	Ack(msg *MessagePointer)
	Nack(msg *MessagePointer)
	SetVisibilityDelay(msg *MessagePointer, seconds int)
	SetFastFailVisibility(msg *MessagePointer)
	ResetVisibilityToDefault(msg *MessagePointer)
}

// Pool represents a message processing pool
type Pool interface {
	Start()
	Drain()
	Submit(msg *MessagePointer) bool
	GetPoolCode() string
	GetConcurrency() int
	GetRateLimitPerMinute() *int
	IsFullyDrained() bool
	Shutdown()
	GetQueueSize() int
	GetActiveWorkers() int
	GetQueueCapacity() int
	IsRateLimited() bool
	UpdateConcurrency(newLimit int, timeoutSeconds int) bool
	UpdateRateLimit(newRateLimitPerMinute *int)
}

// ProcessPool implements Pool with per-message-group FIFO ordering
type ProcessPool struct {
	poolCode      string
	concurrency   int32 // Use atomic for thread-safe reads
	queueCapacity int
	semaphore     chan struct{} // Buffered channel as semaphore

	running    atomic.Bool
	rateLimiter *rate.Limiter
	rateLimitMu sync.RWMutex
	rateLimitPerMinute *int

	mediator        Mediator
	messageCallback MessageCallback
	inPipelineMap   sync.Map // map[string]*MessagePointer

	// Per-message-group queues for FIFO ordering
	messageGroupQueues sync.Map // map[string]chan *MessagePointer
	activeGroupThreads sync.Map // map[string]bool

	// Total messages across all group queues
	totalQueuedMessages atomic.Int32

	// Batch+Group FIFO tracking
	failedBatchGroups      sync.Map // map[string]bool - "batchId|groupId" -> failed
	batchGroupMessageCount sync.Map // map[string]*atomic.Int32

	// Shutdown coordination
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	shutdownMu sync.Mutex

	// Gauge update scheduling (like Java's 500ms gaugeUpdater)
	gaugeCtx    context.Context
	gaugeCancel context.CancelFunc
	gaugeWg     sync.WaitGroup
}

const (
	// DefaultGroup for messages without a messageGroupId
	DefaultGroup = "__DEFAULT__"

	// IdleTimeoutMinutes before cleaning up inactive message groups
	IdleTimeoutMinutes = 5
)

// NewProcessPool creates a new process pool
func NewProcessPool(
	poolCode string,
	concurrency int,
	queueCapacity int,
	rateLimitPerMinute *int,
	mediator Mediator,
	messageCallback MessageCallback,
) *ProcessPool {
	ctx, cancel := context.WithCancel(context.Background())
	gaugeCtx, gaugeCancel := context.WithCancel(context.Background())

	pool := &ProcessPool{
		poolCode:           poolCode,
		concurrency:        int32(concurrency),
		queueCapacity:      queueCapacity,
		semaphore:          make(chan struct{}, concurrency),
		mediator:           mediator,
		messageCallback:    messageCallback,
		rateLimitPerMinute: rateLimitPerMinute,
		ctx:                ctx,
		cancel:             cancel,
		gaugeCtx:           gaugeCtx,
		gaugeCancel:        gaugeCancel,
	}

	// Initialize semaphore with permits
	for i := 0; i < concurrency; i++ {
		pool.semaphore <- struct{}{}
	}

	// Create rate limiter if configured
	if rateLimitPerMinute != nil && *rateLimitPerMinute > 0 {
		// rate.Limiter uses per-second rate
		perSecond := float64(*rateLimitPerMinute) / 60.0
		pool.rateLimiter = rate.NewLimiter(rate.Limit(perSecond), *rateLimitPerMinute)
		slog.Info("Created pool-level rate limiter",
			"pool", poolCode,
			"rateLimit", *rateLimitPerMinute)
	}

	return pool
}

// Start begins processing
func (p *ProcessPool) Start() {
	if p.running.CompareAndSwap(false, true) {
		// Start scheduled gauge updates (every 500ms like Java)
		p.gaugeWg.Add(1)
		go p.runGaugeUpdater()

		slog.Info("Starting process pool with per-group goroutines",
			"pool", p.poolCode,
			"concurrency", atomic.LoadInt32(&p.concurrency))
	}
}

// Drain stops accepting new work but finishes processing
func (p *ProcessPool) Drain() {
	slog.Info("Draining process pool",
		"pool", p.poolCode,
		"queued", p.totalQueuedMessages.Load())
	p.running.Store(false)
}

// Submit submits a message for processing
func (p *ProcessPool) Submit(msg *MessagePointer) bool {
	if !p.running.Load() {
		return false
	}

	// Determine message group
	groupID := msg.MessageGroupID
	if groupID == "" {
		groupID = DefaultGroup
	}

	// Track for batch+group FIFO ordering
	batchID := msg.BatchID
	var batchGroupKey string
	if batchID != "" {
		batchGroupKey = batchID + "|" + groupID
		counter, _ := p.batchGroupMessageCount.LoadOrStore(batchGroupKey, &atomic.Int32{})
		counter.(*atomic.Int32).Add(1)
	}

	// Get or create queue for this message group
	queueIface, created := p.messageGroupQueues.LoadOrStore(groupID, make(chan *MessagePointer, p.queueCapacity))
	queue := queueIface.(chan *MessagePointer)

	if created {
		// Start dedicated goroutine for this message group
		p.startGroupGoroutine(groupID, queue)
		slog.Debug("Created new message group with dedicated goroutine",
			"pool", p.poolCode,
			"group", groupID)
	}

	// Check if group goroutine died and needs restart
	if _, active := p.activeGroupThreads.Load(groupID); !active {
		slog.Warn("Goroutine for message group appears to have died - restarting",
			"pool", p.poolCode,
			"group", groupID)
		p.startGroupGoroutine(groupID, queue)
	}

	// Check total capacity
	current := p.totalQueuedMessages.Load()
	if int(current) >= p.queueCapacity {
		slog.Debug("Pool at capacity, rejecting message",
			"pool", p.poolCode,
			"current", current,
			"capacity", p.queueCapacity,
			"messageId", msg.ID)
		// Clean up batch+group tracking
		if batchGroupKey != "" {
			p.decrementAndCleanupBatchGroup(batchGroupKey)
		}
		return false
	}

	// Try to submit to queue
	select {
	case queue <- msg:
		p.totalQueuedMessages.Add(1)
		return true
	default:
		// Queue full
		if batchGroupKey != "" {
			p.decrementAndCleanupBatchGroup(batchGroupKey)
		}
		return false
	}
}

// startGroupGoroutine starts a dedicated goroutine for a message group
func (p *ProcessPool) startGroupGoroutine(groupID string, queue chan *MessagePointer) {
	p.activeGroupThreads.Store(groupID, true)
	p.wg.Add(1)
	go p.processMessageGroup(groupID, queue)
}

// processMessageGroup processes messages for a single group
func (p *ProcessPool) processMessageGroup(groupID string, queue chan *MessagePointer) {
	defer p.wg.Done()
	defer p.activeGroupThreads.Delete(groupID)

	slog.Debug("Starting message group processor",
		"pool", p.poolCode,
		"group", groupID)

	idleTimeout := time.Duration(IdleTimeoutMinutes) * time.Minute
	timer := time.NewTimer(idleTimeout)
	defer timer.Stop()

	for {
		select {
		case <-p.ctx.Done():
			slog.Debug("Message group processor shutting down",
				"pool", p.poolCode,
				"group", groupID)
			return

		case msg := <-queue:
			if msg == nil {
				continue
			}

			// Reset idle timer
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idleTimeout)

			p.totalQueuedMessages.Add(-1)
			p.processMessage(groupID, msg)

		case <-timer.C:
			// Idle timeout - check if queue is empty and cleanup
			if len(queue) == 0 {
				slog.Debug("Message group idle, cleaning up",
					"pool", p.poolCode,
					"group", groupID,
					"idleMinutes", IdleTimeoutMinutes)
				p.messageGroupQueues.Delete(groupID)
				return
			}
			timer.Reset(idleTimeout)
		}
	}
}

// processMessage processes a single message
func (p *ProcessPool) processMessage(groupID string, msg *MessagePointer) {
	var semaphoreAcquired bool

	defer func() {
		// CRITICAL: Always release semaphore
		if semaphoreAcquired {
			p.semaphore <- struct{}{}
		}

		// Handle panic
		if r := recover(); r != nil {
			slog.Error("Panic during message processing",
				"pool", p.poolCode,
				"messageId", msg.ID,
				"panic", r)
			p.nackSafely(msg)
		}
	}()

	// Check if batch+group has already failed (FIFO enforcement)
	messageGroupID := msg.MessageGroupID
	if messageGroupID == "" {
		messageGroupID = DefaultGroup
	}
	var batchGroupKey string
	if msg.BatchID != "" {
		batchGroupKey = msg.BatchID + "|" + messageGroupID
	}

	if batchGroupKey != "" {
		if _, failed := p.failedBatchGroups.Load(batchGroupKey); failed {
			slog.Warn("Message from failed batch+group, nacking to preserve FIFO ordering",
				"pool", p.poolCode,
				"messageId", msg.ID,
				"batchGroup", batchGroupKey)
			p.messageCallback.SetFastFailVisibility(msg)
			p.nackSafely(msg)
			p.decrementAndCleanupBatchGroup(batchGroupKey)
			return
		}
	}

	// Check rate limiting BEFORE acquiring semaphore
	if p.shouldRateLimit() {
		metrics.PoolRateLimitRejections.WithLabelValues(p.poolCode).Inc()
		metrics.PoolMessagesProcessed.WithLabelValues(p.poolCode, "rate_limited").Inc()
		slog.Warn("Rate limit exceeded, nacking message",
			"pool", p.poolCode,
			"messageId", msg.ID)
		p.messageCallback.SetFastFailVisibility(msg)
		p.nackSafely(msg)
		if batchGroupKey != "" {
			p.decrementAndCleanupBatchGroup(batchGroupKey)
		}
		return
	}

	// Acquire semaphore permit
	select {
	case <-p.semaphore:
		semaphoreAcquired = true
	case <-p.ctx.Done():
		p.nackSafely(msg)
		return
	}

	// Process message through mediator
	slog.Info("Processing message via mediator",
		"pool", p.poolCode,
		"messageId", msg.ID,
		"target", msg.MediationTarget)

	startTime := time.Now()
	outcome := p.mediator.Process(msg)
	duration := time.Since(startTime)

	// Record metrics
	metrics.PoolProcessingDuration.WithLabelValues(p.poolCode).Observe(duration.Seconds())

	slog.Info("Message processing completed",
		"pool", p.poolCode,
		"messageId", msg.ID,
		"result", string(outcome.Result),
		"duration", duration)

	// Handle mediation outcome
	p.handleMediationOutcome(msg, outcome, batchGroupKey)
}

// shouldRateLimit checks if the message should be rate limited
func (p *ProcessPool) shouldRateLimit() bool {
	p.rateLimitMu.RLock()
	limiter := p.rateLimiter
	p.rateLimitMu.RUnlock()

	if limiter == nil {
		return false
	}

	// Non-blocking check
	return !limiter.Allow()
}

// handleMediationOutcome handles the result of mediation
func (p *ProcessPool) handleMediationOutcome(msg *MessagePointer, outcome *MediationOutcome, batchGroupKey string) {
	if outcome == nil {
		outcome = &MediationOutcome{Result: MediationResultErrorProcess}
	}

	switch outcome.Result {
	case MediationResultSuccess:
		metrics.PoolMessagesProcessed.WithLabelValues(p.poolCode, "success").Inc()
		slog.Info("Message processed successfully - ACKing",
			"pool", p.poolCode,
			"messageId", msg.ID)
		p.messageCallback.Ack(msg)
		if batchGroupKey != "" {
			p.decrementAndCleanupBatchGroup(batchGroupKey)
		}

	case MediationResultErrorConfig:
		// Configuration error (4xx) - ACK to prevent infinite retries
		metrics.PoolMessagesProcessed.WithLabelValues(p.poolCode, "failed").Inc()
		slog.Warn("Configuration error - ACKing to prevent retry",
			"pool", p.poolCode,
			"messageId", msg.ID,
			"statusCode", outcome.StatusCode)
		p.messageCallback.Ack(msg)
		if batchGroupKey != "" {
			p.decrementAndCleanupBatchGroup(batchGroupKey)
		}

	case MediationResultErrorProcess:
		// Transient error - NACK for retry
		metrics.PoolMessagesProcessed.WithLabelValues(p.poolCode, "failed").Inc()
		if outcome.HasCustomDelay() {
			delaySeconds := outcome.GetEffectiveDelaySeconds()
			slog.Warn("Transient error with custom delay - NACKing",
				"pool", p.poolCode,
				"messageId", msg.ID,
				"delaySeconds", delaySeconds)
			p.messageCallback.SetVisibilityDelay(msg, delaySeconds)
		} else {
			slog.Warn("Transient error - NACKing for retry",
				"pool", p.poolCode,
				"messageId", msg.ID)
			p.messageCallback.ResetVisibilityToDefault(msg)
		}
		p.messageCallback.Nack(msg)

		// Mark batch+group as failed
		if batchGroupKey != "" {
			p.failedBatchGroups.Store(batchGroupKey, true)
			slog.Warn("Batch+group marked as failed",
				"pool", p.poolCode,
				"batchGroup", batchGroupKey)
			p.decrementAndCleanupBatchGroup(batchGroupKey)
		}

	case MediationResultErrorConnection:
		// Connection error - NACK for retry
		metrics.PoolMessagesProcessed.WithLabelValues(p.poolCode, "failed").Inc()
		slog.Warn("Connection error - NACKing for retry",
			"pool", p.poolCode,
			"messageId", msg.ID)
		p.messageCallback.ResetVisibilityToDefault(msg)
		p.messageCallback.Nack(msg)

		// Mark batch+group as failed
		if batchGroupKey != "" {
			p.failedBatchGroups.Store(batchGroupKey, true)
			p.decrementAndCleanupBatchGroup(batchGroupKey)
		}

	default:
		slog.Warn("Unknown result - NACKing for retry",
			"pool", p.poolCode,
			"messageId", msg.ID,
			"result", string(outcome.Result))
		p.messageCallback.ResetVisibilityToDefault(msg)
		p.messageCallback.Nack(msg)
		if batchGroupKey != "" {
			p.failedBatchGroups.Store(batchGroupKey, true)
			p.decrementAndCleanupBatchGroup(batchGroupKey)
		}
	}
}

// nackSafely safely nacks a message
func (p *ProcessPool) nackSafely(msg *MessagePointer) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Panic during message nack",
				"pool", p.poolCode,
				"messageId", msg.ID,
				"panic", r)
		}
	}()
	p.messageCallback.Nack(msg)
}

// decrementAndCleanupBatchGroup decrements count and cleans up if zero
func (p *ProcessPool) decrementAndCleanupBatchGroup(batchGroupKey string) {
	if counterIface, ok := p.batchGroupMessageCount.Load(batchGroupKey); ok {
		counter := counterIface.(*atomic.Int32)
		remaining := counter.Add(-1)
		if remaining <= 0 {
			p.batchGroupMessageCount.Delete(batchGroupKey)
			p.failedBatchGroups.Delete(batchGroupKey)
			slog.Debug("Batch+group fully processed, cleaned up",
				"pool", p.poolCode,
				"batchGroup", batchGroupKey)
		}
	}
}

// GetPoolCode returns the pool code
func (p *ProcessPool) GetPoolCode() string {
	return p.poolCode
}

// GetConcurrency returns the concurrency limit
func (p *ProcessPool) GetConcurrency() int {
	return int(atomic.LoadInt32(&p.concurrency))
}

// GetRateLimitPerMinute returns the rate limit
func (p *ProcessPool) GetRateLimitPerMinute() *int {
	p.rateLimitMu.RLock()
	defer p.rateLimitMu.RUnlock()
	return p.rateLimitPerMinute
}

// IsFullyDrained returns true if the pool is fully drained
func (p *ProcessPool) IsFullyDrained() bool {
	return p.totalQueuedMessages.Load() == 0 && len(p.semaphore) == int(atomic.LoadInt32(&p.concurrency))
}

// Shutdown shuts down the pool
func (p *ProcessPool) Shutdown() {
	p.shutdownMu.Lock()
	defer p.shutdownMu.Unlock()

	p.running.Store(false)

	// Stop gauge updater first
	p.gaugeCancel()
	p.gaugeWg.Wait()

	p.cancel()

	// Wait for all goroutines with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("Pool shutdown complete", "pool", p.poolCode)
	case <-time.After(10 * time.Second):
		slog.Warn("Pool shutdown timed out", "pool", p.poolCode)
	}
}

// GetQueueSize returns the total queued messages
func (p *ProcessPool) GetQueueSize() int {
	return int(p.totalQueuedMessages.Load())
}

// GetActiveWorkers returns the number of active workers
func (p *ProcessPool) GetActiveWorkers() int {
	return int(atomic.LoadInt32(&p.concurrency)) - len(p.semaphore)
}

// GetQueueCapacity returns the queue capacity
func (p *ProcessPool) GetQueueCapacity() int {
	return p.queueCapacity
}

// HasCapacity returns true if the pool can accept the specified number of messages
func (p *ProcessPool) HasCapacity(needed int) bool {
	return p.GetQueueSize()+needed <= p.queueCapacity
}

// IsRateLimited returns true if currently rate limited
func (p *ProcessPool) IsRateLimited() bool {
	p.rateLimitMu.RLock()
	limiter := p.rateLimiter
	p.rateLimitMu.RUnlock()

	if limiter == nil {
		return false
	}
	return limiter.Tokens() <= 0
}

// UpdateConcurrency updates the concurrency limit
func (p *ProcessPool) UpdateConcurrency(newLimit int, timeoutSeconds int) bool {
	if newLimit <= 0 {
		return false
	}

	current := int(atomic.LoadInt32(&p.concurrency))
	if newLimit == current {
		return true
	}

	if newLimit > current {
		// Increasing - add permits
		diff := newLimit - current
		for i := 0; i < diff; i++ {
			p.semaphore <- struct{}{}
		}
		atomic.StoreInt32(&p.concurrency, int32(newLimit))
		slog.Info("Concurrency increased",
			"pool", p.poolCode,
			"from", current,
			"to", newLimit)
		return true
	}

	// Decreasing - try to acquire permits with timeout
	diff := current - newLimit
	timeout := time.Duration(timeoutSeconds) * time.Second
	deadline := time.Now().Add(timeout)

	acquired := 0
	for acquired < diff {
		select {
		case <-p.semaphore:
			acquired++
		case <-time.After(time.Until(deadline)):
			// Timeout - release acquired permits and fail
			for i := 0; i < acquired; i++ {
				p.semaphore <- struct{}{}
			}
			slog.Warn("Concurrency decrease timed out",
				"pool", p.poolCode,
				"from", current,
				"to", newLimit)
			return false
		}
	}

	atomic.StoreInt32(&p.concurrency, int32(newLimit))
	slog.Info("Concurrency decreased",
		"pool", p.poolCode,
		"from", current,
		"to", newLimit)
	return true
}

// UpdateRateLimit updates the rate limit
func (p *ProcessPool) UpdateRateLimit(newRateLimitPerMinute *int) {
	p.rateLimitMu.Lock()
	defer p.rateLimitMu.Unlock()

	if newRateLimitPerMinute == nil || *newRateLimitPerMinute <= 0 {
		p.rateLimiter = nil
		p.rateLimitPerMinute = nil
		slog.Info("Rate limiting disabled", "pool", p.poolCode)
		return
	}

	perSecond := float64(*newRateLimitPerMinute) / 60.0
	p.rateLimiter = rate.NewLimiter(rate.Limit(perSecond), *newRateLimitPerMinute)
	p.rateLimitPerMinute = newRateLimitPerMinute
	slog.Info("Rate limit updated",
		"pool", p.poolCode,
		"rateLimit", *newRateLimitPerMinute)
}

// runGaugeUpdater runs the scheduled gauge update loop (every 500ms like Java)
func (p *ProcessPool) runGaugeUpdater() {
	defer p.gaugeWg.Done()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Initial update
	p.updateGauges()

	for {
		select {
		case <-p.gaugeCtx.Done():
			return
		case <-ticker.C:
			p.updateGauges()
		}
	}
}

// updateGauges updates all pool gauge metrics
func (p *ProcessPool) updateGauges() {
	activeWorkers := p.GetActiveWorkers()
	queueSize := p.GetQueueSize()
	availablePermits := int(atomic.LoadInt32(&p.concurrency)) - activeWorkers
	messageGroupCount := p.countMessageGroups()

	// Update Prometheus gauges
	metrics.PoolActiveWorkers.WithLabelValues(p.poolCode).Set(float64(activeWorkers))
	metrics.PoolQueueDepth.WithLabelValues(p.poolCode).Set(float64(queueSize))
	metrics.PoolAvailablePermits.WithLabelValues(p.poolCode).Set(float64(availablePermits))
	metrics.PoolMessageGroupCount.WithLabelValues(p.poolCode).Set(float64(messageGroupCount))
}

// countMessageGroups returns the number of active message groups
func (p *ProcessPool) countMessageGroups() int {
	count := 0
	p.messageGroupQueues.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}
