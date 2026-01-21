package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"go.flowcatalyst.tech/internal/common/leader"
	"go.flowcatalyst.tech/internal/common/metrics"
)

// ProcessorConfig holds configuration for the outbox processor
type ProcessorConfig struct {
	// Enabled controls whether the processor is active
	Enabled bool

	// PollInterval is how often to poll for pending items
	PollInterval time.Duration

	// PollBatchSize is the maximum items to fetch per poll
	PollBatchSize int

	// APIBatchSize is the maximum items per API call
	APIBatchSize int

	// MaxConcurrentGroups limits parallel message group processing
	MaxConcurrentGroups int

	// MaxInFlight is the maximum items in the pipeline (buffer + processing queues)
	// Poller checks this before polling to implement backpressure
	MaxInFlight int

	// MaxRetries is the maximum retry attempts before marking as failed
	MaxRetries int

	// RecoveryInterval is how often to run periodic recovery
	RecoveryInterval time.Duration

	// ProcessingTimeoutSeconds is how long items can be in error status before recovery
	ProcessingTimeoutSeconds int

	// LeaderElection enables distributed leader election
	LeaderElection LeaderElectionConfig
}

// LeaderElectionConfig holds leader election settings
type LeaderElectionConfig struct {
	Enabled         bool
	LockName        string
	LeaseDuration   time.Duration
	RefreshInterval time.Duration
	// RedisURL is the Redis connection URL (e.g., "redis://localhost:6379")
	// If empty, leader election is disabled even if Enabled is true
	RedisURL string
}

// DefaultLeaderElectionConfig returns sensible defaults for leader election
func DefaultLeaderElectionConfig() LeaderElectionConfig {
	return LeaderElectionConfig{
		Enabled:         false, // Disabled by default (single-instance mode)
		LockName:        "outbox-processor-leader",
		LeaseDuration:   30 * time.Second,
		RefreshInterval: 10 * time.Second,
	}
}

// DefaultProcessorConfig returns sensible defaults
func DefaultProcessorConfig() *ProcessorConfig {
	return &ProcessorConfig{
		Enabled:                  true,
		PollInterval:             time.Second,
		PollBatchSize:            500,
		APIBatchSize:             100,
		MaxConcurrentGroups:      10,
		MaxInFlight:              2500, // 5x PollBatchSize
		MaxRetries:               3,
		RecoveryInterval:         60 * time.Second,
		ProcessingTimeoutSeconds: 300, // 5 minutes
	}
}

// Processor implements the Outbox Pattern for reliable message publishing.
// Uses a single-poller, status-based architecture with NO row locking.
//
// Architecture:
//   1. Single poller fetches items WHERE status = 0 (PENDING)
//   2. Items are marked status = 9 (IN_PROGRESS) immediately after fetch
//   3. Distributor routes items to message group processors (maintains FIFO per group)
//   4. On completion, status is updated to reflect outcome (1=success, 2-6=error types)
//   5. Crash recovery: on startup, reset status = 9 back to 0
//
// This approach avoids row locking (FOR UPDATE SKIP LOCKED) and works
// identically across PostgreSQL, MySQL, and MongoDB.
type Processor struct {
	config    *ProcessorConfig
	repo      Repository
	apiClient *APIClient

	// Global buffer for items waiting to be distributed
	buffer     chan *OutboxItem
	bufferSize int32 // Atomic counter for current buffer occupancy

	// In-flight tracking: buffer + items in message group queues
	inFlightCount int32 // Atomic counter

	// Group distributor
	groupProcessors sync.Map // map[groupKey]*MessageGroupProcessor
	groupSemaphore  chan struct{}

	// Leader election (Redis-based for multi-instance deployments)
	redisLeaderElector *leader.RedisLeaderElector
	mongoLeaderElector *leader.LeaderElector
	isPrimary          atomic.Bool

	// Lifecycle
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	running   bool
	runningMu sync.Mutex
	pollMu    sync.Mutex // Prevent overlapping polls
}

// NewProcessor creates a new outbox processor
func NewProcessor(repo Repository, apiClient *APIClient, config *ProcessorConfig) *Processor {
	if config == nil {
		config = DefaultProcessorConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	p := &Processor{
		config:         config,
		repo:           repo,
		apiClient:      apiClient,
		buffer:         make(chan *OutboxItem, config.MaxInFlight),
		groupSemaphore: make(chan struct{}, config.MaxConcurrentGroups),
		ctx:            ctx,
		cancel:         cancel,
	}

	// Default to primary if leader election is disabled
	p.isPrimary.Store(true)

	return p
}

// WithRedisLeaderElection enables Redis-based leader election for multi-instance deployments.
// The Redis client is used for distributed lock acquisition.
func (p *Processor) WithRedisLeaderElection(redisClient *redis.Client) *Processor {
	if redisClient == nil || !p.config.LeaderElection.Enabled {
		return p
	}

	cfg := leader.DefaultRedisElectorConfig(p.config.LeaderElection.LockName)
	if p.config.LeaderElection.LeaseDuration > 0 {
		cfg.TTL = p.config.LeaderElection.LeaseDuration
	}
	if p.config.LeaderElection.RefreshInterval > 0 {
		cfg.RefreshInterval = p.config.LeaderElection.RefreshInterval
	}

	p.redisLeaderElector = leader.NewRedisLeaderElector(redisClient, cfg)

	// Set up callbacks to update isPrimary
	p.redisLeaderElector.OnBecomeLeader(func() {
		p.isPrimary.Store(true)
		metrics.OutboxLeaderElectionState.Set(1) // Leader
		slog.Info("Outbox processor became primary via Redis leader election")
	})
	p.redisLeaderElector.OnLoseLeadership(func() {
		p.isPrimary.Store(false)
		metrics.OutboxLeaderElectionState.Set(0) // Follower
		slog.Warn("Outbox processor lost primary status via Redis leader election")
	})

	// Start with non-primary until we acquire leadership
	p.isPrimary.Store(false)

	return p
}

// Start starts the outbox processor
func (p *Processor) Start() {
	p.runningMu.Lock()
	defer p.runningMu.Unlock()

	if p.running {
		return
	}
	p.running = true

	if !p.config.Enabled {
		slog.Info("Outbox processor is disabled")
		return
	}

	// Perform crash recovery FIRST (reset stuck items from previous run)
	p.doCrashRecovery()

	// Start leader election if configured
	if p.redisLeaderElector != nil {
		if err := p.redisLeaderElector.Start(p.ctx); err != nil {
			slog.Error("Failed to start Redis leader election", "error", err)
		} else {
			slog.Info("Redis leader election started for outbox processor",
				"leaderElectionEnabled", true,
				"lockName", p.config.LeaderElection.LockName)
		}
	}

	// Start the distributor goroutine
	p.wg.Add(1)
	go p.runDistributor()

	// Start the polling goroutine
	p.wg.Add(1)
	go p.runPoller()

	// Start the periodic recovery goroutine
	p.wg.Add(1)
	go p.runPeriodicRecovery()

	slog.Info("Outbox processor started",
		"pollInterval", p.config.PollInterval,
		"batchSize", p.config.PollBatchSize,
		"maxConcurrentGroups", p.config.MaxConcurrentGroups,
		"maxInFlight", p.config.MaxInFlight,
		"isPrimary", p.isPrimary.Load())
}

// Stop stops the outbox processor
func (p *Processor) Stop() {
	p.runningMu.Lock()
	p.running = false
	p.runningMu.Unlock()

	p.cancel()
	p.wg.Wait()

	// Stop leader election if running
	if p.redisLeaderElector != nil {
		p.redisLeaderElector.Stop()
	}

	slog.Info("Outbox processor stopped")
}

// IsPrimary returns whether this processor is the current leader
func (p *Processor) IsPrimary() bool {
	return p.isPrimary.Load()
}

// GetStats returns current processor statistics
func (p *Processor) GetStats() ProcessorStats {
	inFlight := atomic.LoadInt32(&p.inFlightCount)
	return ProcessorStats{
		Status:                "UP",
		Healthy:               p.running && p.isPrimary.Load(),
		LastPollTime:          time.Now(), // TODO: track actual last poll time
		ActiveMessageGroups:   p.countActiveGroups(),
		InFlightPermits:       p.config.MaxInFlight - int(inFlight),
		TotalInFlightCapacity: p.config.MaxInFlight,
		BufferedItems:         int(atomic.LoadInt32(&p.bufferSize)),
	}
}

// countActiveGroups counts active message group processors
func (p *Processor) countActiveGroups() int {
	count := 0
	p.groupProcessors.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// doCrashRecovery resets stuck items (status=9) back to pending (status=0)
// This is called on startup to recover from crashes/restarts
func (p *Processor) doCrashRecovery() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, itemType := range []OutboxItemType{OutboxItemTypeEvent, OutboxItemTypeDispatchJob} {
		stuckItems, err := p.repo.FetchStuckItems(ctx, itemType)
		if err != nil {
			slog.Error("Failed to fetch stuck items during crash recovery",
				"error", err,
				"type", string(itemType))
			continue
		}

		if len(stuckItems) == 0 {
			continue
		}

		ids := make([]string, len(stuckItems))
		for i, item := range stuckItems {
			ids[i] = item.ID
		}

		if err := p.repo.ResetStuckItems(ctx, itemType, ids); err != nil {
			slog.Error("Failed to reset stuck items during crash recovery",
				"error", err,
				"type", string(itemType),
				"count", len(ids))
			continue
		}

		metrics.OutboxRecoveredItems.WithLabelValues(string(itemType)).Add(float64(len(ids)))
		slog.Info("Reset stuck outbox items during crash recovery",
			"type", string(itemType),
			"count", len(ids))
	}
}

// runPeriodicRecovery runs the periodic recovery loop
func (p *Processor) runPeriodicRecovery() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.RecoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			if !p.isPrimary.Load() {
				continue
			}
			p.doPeriodicRecovery()
		}
	}
}

// doPeriodicRecovery resets items that have been in error states for too long
// Recovers: IN_PROGRESS, BAD_REQUEST, INTERNAL_ERROR, UNAUTHORIZED, FORBIDDEN, GATEWAY_ERROR
func (p *Processor) doPeriodicRecovery() {
	ctx, cancel := context.WithTimeout(p.ctx, 30*time.Second)
	defer cancel()

	for _, itemType := range []OutboxItemType{OutboxItemTypeEvent, OutboxItemTypeDispatchJob} {
		recoverableItems, err := p.repo.FetchRecoverableItems(
			ctx,
			itemType,
			p.config.ProcessingTimeoutSeconds,
			p.config.PollBatchSize,
		)
		if err != nil {
			slog.Error("Failed to fetch recoverable items during periodic recovery",
				"error", err,
				"type", string(itemType))
			continue
		}

		if len(recoverableItems) == 0 {
			continue
		}

		ids := make([]string, len(recoverableItems))
		for i, item := range recoverableItems {
			ids[i] = item.ID
		}

		if err := p.repo.ResetRecoverableItems(ctx, itemType, ids); err != nil {
			slog.Error("Failed to reset recoverable items during periodic recovery",
				"error", err,
				"type", string(itemType),
				"count", len(ids))
			continue
		}

		metrics.OutboxRecoveredItems.WithLabelValues(string(itemType)).Add(float64(len(ids)))
		slog.Info("Periodic recovery: reset items back to PENDING",
			"type", string(itemType),
			"count", len(ids))
	}
}

// runPoller runs the main polling loop
func (p *Processor) runPoller() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			if !p.isPrimary.Load() {
				continue
			}
			p.doPoll()
		}
	}
}

// doPoll performs a single poll iteration
func (p *Processor) doPoll() {
	// Prevent overlapping polls
	if !p.pollMu.TryLock() {
		return
	}
	defer p.pollMu.Unlock()

	// Check if there's sufficient capacity BEFORE polling
	// We need space for at least a full batch
	currentInFlight := atomic.LoadInt32(&p.inFlightCount)
	availableSlots := p.config.MaxInFlight - int(currentInFlight)

	if availableSlots < p.config.PollBatchSize {
		slog.Debug("Skipping poll - insufficient in-flight capacity",
			"availableSlots", availableSlots,
			"pollBatchSize", p.config.PollBatchSize)
		return
	}

	startTime := time.Now()
	defer func() {
		metrics.OutboxPollDuration.Observe(time.Since(startTime).Seconds())
	}()

	ctx, cancel := context.WithTimeout(p.ctx, 30*time.Second)
	defer cancel()

	// Poll for events
	p.pollItemType(ctx, OutboxItemTypeEvent)

	// Poll for dispatch jobs
	p.pollItemType(ctx, OutboxItemTypeDispatchJob)
}

// pollItemType polls for items of a specific type
func (p *Processor) pollItemType(ctx context.Context, itemType OutboxItemType) {
	// 1. Fetch pending items (simple SELECT, no locking)
	items, err := p.repo.FetchPending(ctx, itemType, p.config.PollBatchSize)
	if err != nil {
		slog.Error("Failed to fetch pending outbox items",
			"error", err,
			"type", string(itemType))
		return
	}

	if len(items) == 0 {
		return
	}

	// 2. Mark as in-progress IMMEDIATELY (before buffering)
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}

	if err := p.repo.MarkAsInProgress(ctx, itemType, ids); err != nil {
		slog.Error("Failed to mark items as in-progress",
			"error", err,
			"type", string(itemType),
			"count", len(ids))
		// Don't proceed - items will be picked up on next poll
		return
	}

	// 3. Acquire in-flight permits for the actual fetched count
	atomic.AddInt32(&p.inFlightCount, int32(len(items)))
	metrics.OutboxInFlightItems.Set(float64(atomic.LoadInt32(&p.inFlightCount)))

	slog.Debug("Fetched and marked outbox items as in-progress",
		"type", string(itemType),
		"count", len(items))

	// 4. Add to buffer
	for _, item := range items {
		select {
		case p.buffer <- item:
			atomic.AddInt32(&p.bufferSize, 1)
			metrics.OutboxBufferSize.Set(float64(atomic.LoadInt32(&p.bufferSize)))
		case <-ctx.Done():
			// Context cancelled, items are already marked in-progress
			// They will be recovered on next startup
			return
		}
	}
}

// runDistributor runs the distributor loop that routes items to group processors
func (p *Processor) runDistributor() {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			// Drain remaining items
			p.drainBuffer()
			return
		case item := <-p.buffer:
			atomic.AddInt32(&p.bufferSize, -1)
			metrics.OutboxBufferSize.Set(float64(atomic.LoadInt32(&p.bufferSize)))
			p.distributeItem(item)
		}
	}
}

// distributeItem routes an item to the appropriate message group processor
func (p *Processor) distributeItem(item *OutboxItem) {
	groupKey := fmt.Sprintf("%s:%s", item.Type, item.GetEffectiveMessageGroup())

	// Get or create processor for this group
	processorI, _ := p.groupProcessors.LoadOrStore(groupKey, &MessageGroupProcessor{
		groupKey:   groupKey,
		itemType:   item.Type,
		queue:      make(chan *OutboxItem, 1000), // Large queue per group
		processor:  p,
		processing: false,
	})
	processor := processorI.(*MessageGroupProcessor)

	// Add item to group queue (maintains FIFO within group)
	select {
	case processor.queue <- item:
		// Try to start processing if not already running
		processor.tryStart()
	default:
		// Group queue full - this shouldn't happen with 1000 capacity
		slog.Warn("Group queue full",
			"group", groupKey,
			"itemId", item.ID)
	}
}

// drainBuffer drains remaining items from the buffer during shutdown
func (p *Processor) drainBuffer() {
	for {
		select {
		case item := <-p.buffer:
			slog.Debug("Draining item during shutdown - will be recovered on restart",
				"itemId", item.ID)
		default:
			return
		}
	}
}

// MessageGroupProcessor processes items for a single message group in FIFO order
type MessageGroupProcessor struct {
	groupKey   string
	itemType   OutboxItemType
	queue      chan *OutboxItem
	processor  *Processor
	processing bool
	mu         sync.Mutex
}

// tryStart attempts to start processing if not already running
func (m *MessageGroupProcessor) tryStart() {
	m.mu.Lock()
	if m.processing {
		m.mu.Unlock()
		return
	}
	m.processing = true
	m.mu.Unlock()

	go m.processLoop()
}

// processLoop processes items in the group queue
func (m *MessageGroupProcessor) processLoop() {
	defer func() {
		m.mu.Lock()
		m.processing = false
		m.mu.Unlock()
	}()

	for {
		// Collect a batch from this group's queue
		batch := m.collectBatch()
		if len(batch) == 0 {
			return // No more items, exit
		}

		// Acquire semaphore for concurrent group limit
		select {
		case m.processor.groupSemaphore <- struct{}{}:
			// Got semaphore
		case <-m.processor.ctx.Done():
			return
		}

		m.processBatch(batch)

		// Release semaphore
		<-m.processor.groupSemaphore
	}
}

// collectBatch collects up to APIBatchSize items from the queue
func (m *MessageGroupProcessor) collectBatch() []*OutboxItem {
	batch := make([]*OutboxItem, 0, m.processor.config.APIBatchSize)

	for i := 0; i < m.processor.config.APIBatchSize; i++ {
		select {
		case item := <-m.queue:
			batch = append(batch, item)
		default:
			return batch
		}
	}

	return batch
}

// processBatch sends a batch to the API and updates item statuses
func (m *MessageGroupProcessor) processBatch(batch []*OutboxItem) {
	if len(batch) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(m.processor.ctx, 30*time.Second)
	defer cancel()

	// Track active processors
	metrics.OutboxActiveProcessors.Inc()
	defer metrics.OutboxActiveProcessors.Dec()

	// Track API call duration
	apiStartTime := time.Now()

	var result *BatchResult
	var err error

	switch m.itemType {
	case OutboxItemTypeEvent:
		result, err = m.processor.apiClient.SendEventBatch(ctx, batch)
	case OutboxItemTypeDispatchJob:
		result, err = m.processor.apiClient.SendDispatchJobBatch(ctx, batch)
	}

	metrics.OutboxAPIDuration.WithLabelValues(string(m.itemType)).Observe(time.Since(apiStartTime).Seconds())

	// Release in-flight permits for this batch
	atomic.AddInt32(&m.processor.inFlightCount, -int32(len(batch)))
	metrics.OutboxInFlightItems.Set(float64(atomic.LoadInt32(&m.processor.inFlightCount)))

	if err != nil {
		slog.Error("Failed to send batch",
			"error", err,
			"group", m.groupKey,
			"batchSize", len(batch))
		m.handleAPIError(ctx, batch, err)
		return
	}

	// Mark successful items
	if len(result.SuccessIDs) > 0 {
		if err := m.processor.repo.MarkWithStatus(ctx, m.itemType, result.SuccessIDs, StatusSuccess); err != nil {
			slog.Error("Failed to mark items as completed", "error", err)
		}
		metrics.OutboxItemsProcessed.WithLabelValues(string(m.itemType), "completed").Add(float64(len(result.SuccessIDs)))
	}

	// Handle failed items (from result.FailedItems map with per-item status)
	if result.FailedItems != nil && len(result.FailedItems) > 0 {
		m.handlePerItemFailures(ctx, batch, result.FailedItems)
	}

	slog.Debug("Batch processed",
		"group", m.groupKey,
		"success", len(result.SuccessIDs),
		"failed", len(result.FailedItems))
}

// handleAPIError handles an API error for the entire batch
func (m *MessageGroupProcessor) handleAPIError(ctx context.Context, batch []*OutboxItem, apiErr error) {
	// Determine status from error (could parse HTTP status from error message)
	status := StatusInternalError
	if apiErr != nil {
		errStr := apiErr.Error()
		// Simple heuristic - in practice, the API client could return structured errors
		if contains(errStr, "400") {
			status = StatusBadRequest
		} else if contains(errStr, "401") {
			status = StatusUnauthorized
		} else if contains(errStr, "403") {
			status = StatusForbidden
		} else if contains(errStr, "502") || contains(errStr, "503") || contains(errStr, "504") {
			status = StatusGatewayError
		}
	}

	// Separate items into retry vs failed based on status and retry count
	retryIDs := make([]string, 0)
	failIDs := make([]string, 0)

	for _, item := range batch {
		if status.IsRetryable() && item.RetryCount < m.processor.config.MaxRetries {
			retryIDs = append(retryIDs, item.ID)
		} else {
			failIDs = append(failIDs, item.ID)
		}
	}

	// Schedule retries (increment retry count, reset to pending)
	if len(retryIDs) > 0 {
		if err := m.processor.repo.IncrementRetryCount(ctx, m.itemType, retryIDs); err != nil {
			slog.Error("Failed to schedule retry", "error", err)
		}
		metrics.OutboxItemsProcessed.WithLabelValues(string(m.itemType), "retried").Add(float64(len(retryIDs)))
	}

	// Mark permanently failed
	if len(failIDs) > 0 {
		if err := m.processor.repo.MarkWithStatusAndError(ctx, m.itemType, failIDs, status, apiErr.Error()); err != nil {
			slog.Error("Failed to mark items as failed", "error", err)
		}
		metrics.OutboxItemsProcessed.WithLabelValues(string(m.itemType), "failed").Add(float64(len(failIDs)))
		slog.Warn("Items marked as failed",
			"group", m.groupKey,
			"count", len(failIDs),
			"status", status.String())
	}
}

// handlePerItemFailures handles failures with per-item status codes
func (m *MessageGroupProcessor) handlePerItemFailures(ctx context.Context, batch []*OutboxItem, failedItems map[string]OutboxStatus) {
	// Build lookup for items by ID
	itemByID := make(map[string]*OutboxItem)
	for _, item := range batch {
		itemByID[item.ID] = item
	}

	// Group failed items by status
	byStatus := make(map[OutboxStatus][]string)
	retryIDs := make([]string, 0)

	for id, status := range failedItems {
		item := itemByID[id]
		if item == nil {
			continue
		}

		if status.IsRetryable() && item.RetryCount < m.processor.config.MaxRetries {
			retryIDs = append(retryIDs, id)
		} else {
			byStatus[status] = append(byStatus[status], id)
		}
	}

	// Schedule retries
	if len(retryIDs) > 0 {
		if err := m.processor.repo.IncrementRetryCount(ctx, m.itemType, retryIDs); err != nil {
			slog.Error("Failed to schedule retry for failed items", "error", err)
		}
		metrics.OutboxItemsProcessed.WithLabelValues(string(m.itemType), "retried").Add(float64(len(retryIDs)))
	}

	// Mark failed items by status
	for status, ids := range byStatus {
		if err := m.processor.repo.MarkWithStatus(ctx, m.itemType, ids, status); err != nil {
			slog.Error("Failed to mark items with status",
				"error", err,
				"status", status.String())
		}
		metrics.OutboxItemsProcessed.WithLabelValues(string(m.itemType), "failed").Add(float64(len(ids)))
	}
}

// contains is a simple substring check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
