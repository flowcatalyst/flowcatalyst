package metrics

import (
	"log/slog"
	"sync"
	"time"
)

// PoolStats represents statistics for a processing pool
type PoolStats struct {
	PoolCode                string  `json:"poolCode"`
	TotalProcessed          int64   `json:"totalProcessed"`
	TotalSucceeded          int64   `json:"totalSucceeded"`
	TotalFailed             int64   `json:"totalFailed"`
	TotalRateLimited        int64   `json:"totalRateLimited"`
	SuccessRate             float64 `json:"successRate"`
	ActiveWorkers           int     `json:"activeWorkers"`
	AvailablePermits        int     `json:"availablePermits"`
	MaxConcurrency          int     `json:"maxConcurrency"`
	QueueSize               int     `json:"queueSize"`
	MaxQueueCapacity        int     `json:"maxQueueCapacity"`
	AverageProcessingTimeMs float64 `json:"averageProcessingTimeMs"`
	// 5-minute rolling window
	TotalProcessed5min int64   `json:"totalProcessed5min"`
	Succeeded5min      int64   `json:"succeeded5min"`
	Failed5min         int64   `json:"failed5min"`
	SuccessRate5min    float64 `json:"successRate5min"`
	// 30-minute rolling window
	TotalProcessed30min int64   `json:"totalProcessed30min"`
	Succeeded30min      int64   `json:"succeeded30min"`
	Failed30min         int64   `json:"failed30min"`
	SuccessRate30min    float64 `json:"successRate30min"`
}

// EmptyPoolStats returns empty statistics for a pool
func EmptyPoolStats(poolCode string) *PoolStats {
	return &PoolStats{
		PoolCode:         poolCode,
		SuccessRate:      1.0,
		SuccessRate5min:  1.0,
		SuccessRate30min: 1.0,
	}
}

// PoolMetricsService tracks processing pool metrics
type PoolMetricsService interface {
	RecordMessageSubmitted(poolCode string)
	RecordProcessingStarted(poolCode string)
	RecordProcessingFinished(poolCode string)
	RecordProcessingSuccess(poolCode string, durationMs int64)
	RecordProcessingFailure(poolCode string, durationMs int64, errorType string)
	RecordRateLimitExceeded(poolCode string)
	RecordProcessingTransient(poolCode string, durationMs int64)
	InitializePoolCapacity(poolCode string, maxConcurrency, maxQueueCapacity int)
	UpdatePoolGauges(poolCode string, activeWorkers, availablePermits, queueSize, messageGroupCount int)
	GetPoolStats(poolCode string) *PoolStats
	GetAllPoolStats() map[string]*PoolStats
	GetLastActivityTimestamp(poolCode string) *time.Time
	RemovePoolMetrics(poolCode string)
}

// poolMetricsHolder holds metrics for a single pool
type poolMetricsHolder struct {
	mu                    sync.RWMutex
	messagesSubmitted     int64
	messagesSucceeded     int64
	messagesFailed        int64
	messagesRateLimited   int64
	messagesTransient     int64
	totalProcessingTimeMs int64
	activeWorkers         int
	availablePermits      int
	queueSize             int
	messageGroupCount     int
	maxConcurrency        int
	maxQueueCapacity      int
	lastActivityTimestamp time.Time
	recordedOutcomes      []timestampedOutcome
}

// InMemoryPoolMetricsService is an in-memory implementation of PoolMetricsService
type InMemoryPoolMetricsService struct {
	mu      sync.RWMutex
	metrics map[string]*poolMetricsHolder
}

// NewInMemoryPoolMetricsService creates a new pool metrics service
func NewInMemoryPoolMetricsService() *InMemoryPoolMetricsService {
	return &InMemoryPoolMetricsService{
		metrics: make(map[string]*poolMetricsHolder),
	}
}

// RecordMessageSubmitted records that a message was submitted to a pool
func (s *InMemoryPoolMetricsService) RecordMessageSubmitted(poolCode string) {
	holder := s.getOrCreateMetrics(poolCode)
	holder.mu.Lock()
	defer holder.mu.Unlock()
	holder.messagesSubmitted++
}

// RecordProcessingStarted records that processing started (no-op for gauge-based tracking)
func (s *InMemoryPoolMetricsService) RecordProcessingStarted(poolCode string) {
	// No-op: activeWorkers is tracked via UpdatePoolGauges()
}

// RecordProcessingFinished records that processing finished (no-op for gauge-based tracking)
func (s *InMemoryPoolMetricsService) RecordProcessingFinished(poolCode string) {
	// No-op: activeWorkers is tracked via UpdatePoolGauges()
}

// RecordProcessingSuccess records successful message processing
func (s *InMemoryPoolMetricsService) RecordProcessingSuccess(poolCode string, durationMs int64) {
	holder := s.getOrCreateMetrics(poolCode)
	holder.mu.Lock()
	defer holder.mu.Unlock()

	holder.messagesSucceeded++
	holder.totalProcessingTimeMs += durationMs
	holder.lastActivityTimestamp = time.Now()
	holder.recordedOutcomes = append(holder.recordedOutcomes, timestampedOutcome{
		timestamp: time.Now(),
		success:   true,
	})
}

// RecordProcessingFailure records failed message processing
func (s *InMemoryPoolMetricsService) RecordProcessingFailure(poolCode string, durationMs int64, errorType string) {
	holder := s.getOrCreateMetrics(poolCode)
	holder.mu.Lock()
	defer holder.mu.Unlock()

	holder.messagesFailed++
	holder.totalProcessingTimeMs += durationMs
	holder.lastActivityTimestamp = time.Now()
	holder.recordedOutcomes = append(holder.recordedOutcomes, timestampedOutcome{
		timestamp: time.Now(),
		success:   false,
	})
}

// RecordRateLimitExceeded records a rate limit rejection
func (s *InMemoryPoolMetricsService) RecordRateLimitExceeded(poolCode string) {
	holder := s.getOrCreateMetrics(poolCode)
	holder.mu.Lock()
	defer holder.mu.Unlock()
	holder.messagesRateLimited++
}

// RecordProcessingTransient records a transient error (will be retried)
func (s *InMemoryPoolMetricsService) RecordProcessingTransient(poolCode string, durationMs int64) {
	holder := s.getOrCreateMetrics(poolCode)
	holder.mu.Lock()
	defer holder.mu.Unlock()
	holder.messagesTransient++
	holder.totalProcessingTimeMs += durationMs
	// Do NOT update lastActivityTimestamp for transient errors
}

// InitializePoolCapacity sets pool capacity settings
func (s *InMemoryPoolMetricsService) InitializePoolCapacity(poolCode string, maxConcurrency, maxQueueCapacity int) {
	holder := s.getOrCreateMetrics(poolCode)
	holder.mu.Lock()
	defer holder.mu.Unlock()
	holder.maxConcurrency = maxConcurrency
	holder.maxQueueCapacity = maxQueueCapacity
}

// UpdatePoolGauges updates gauge metrics for pool state
func (s *InMemoryPoolMetricsService) UpdatePoolGauges(poolCode string, activeWorkers, availablePermits, queueSize, messageGroupCount int) {
	holder := s.getOrCreateMetrics(poolCode)
	holder.mu.Lock()
	defer holder.mu.Unlock()
	holder.activeWorkers = activeWorkers
	holder.availablePermits = availablePermits
	holder.queueSize = queueSize
	holder.messageGroupCount = messageGroupCount
}

// GetPoolStats returns statistics for a specific pool
func (s *InMemoryPoolMetricsService) GetPoolStats(poolCode string) *PoolStats {
	s.mu.RLock()
	holder, ok := s.metrics[poolCode]
	s.mu.RUnlock()

	if !ok {
		return EmptyPoolStats(poolCode)
	}

	return holder.buildStats(poolCode)
}

// GetAllPoolStats returns statistics for all pools
func (s *InMemoryPoolMetricsService) GetAllPoolStats() map[string]*PoolStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*PoolStats)
	for poolCode, holder := range s.metrics {
		result[poolCode] = holder.buildStats(poolCode)
	}
	return result
}

// GetLastActivityTimestamp returns the last activity timestamp for a pool
func (s *InMemoryPoolMetricsService) GetLastActivityTimestamp(poolCode string) *time.Time {
	s.mu.RLock()
	holder, ok := s.metrics[poolCode]
	s.mu.RUnlock()

	if !ok {
		return nil
	}

	holder.mu.RLock()
	defer holder.mu.RUnlock()

	if holder.lastActivityTimestamp.IsZero() {
		return nil
	}
	ts := holder.lastActivityTimestamp
	return &ts
}

// RemovePoolMetrics removes all metrics for a pool
func (s *InMemoryPoolMetricsService) RemovePoolMetrics(poolCode string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.metrics[poolCode]; ok {
		delete(s.metrics, poolCode)
		slog.Info("Removed metrics for pool", "poolCode", poolCode)
	}
}

func (s *InMemoryPoolMetricsService) getOrCreateMetrics(poolCode string) *poolMetricsHolder {
	s.mu.RLock()
	holder, ok := s.metrics[poolCode]
	s.mu.RUnlock()

	if ok {
		return holder
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	if holder, ok := s.metrics[poolCode]; ok {
		return holder
	}

	holder = &poolMetricsHolder{
		recordedOutcomes: make([]timestampedOutcome, 0),
	}
	s.metrics[poolCode] = holder
	slog.Info("Creating metrics for pool", "poolCode", poolCode)
	return holder
}

func (h *poolMetricsHolder) buildStats(poolCode string) *PoolStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	totalProcessed := h.messagesSucceeded + h.messagesFailed

	successRate := 1.0
	if totalProcessed > 0 {
		successRate = float64(h.messagesSucceeded) / float64(totalProcessed)
	}

	avgProcessingTime := 0.0
	if totalProcessed > 0 {
		avgProcessingTime = float64(h.totalProcessingTimeMs) / float64(totalProcessed)
	}

	// Calculate rolling window metrics
	now := time.Now()
	fiveMinutesAgo := now.Add(-5 * time.Minute)
	thirtyMinutesAgo := now.Add(-30 * time.Minute)

	var succeeded5min, failed5min, succeeded30min, failed30min int64

	// Clean up old outcomes and count recent ones
	validOutcomes := make([]timestampedOutcome, 0)
	for _, outcome := range h.recordedOutcomes {
		if outcome.timestamp.After(thirtyMinutesAgo) {
			validOutcomes = append(validOutcomes, outcome)
			if outcome.success {
				succeeded30min++
				if outcome.timestamp.After(fiveMinutesAgo) {
					succeeded5min++
				}
			} else {
				failed30min++
				if outcome.timestamp.After(fiveMinutesAgo) {
					failed5min++
				}
			}
		}
	}

	totalProcessed5min := succeeded5min + failed5min
	successRate5min := 1.0
	if totalProcessed5min > 0 {
		successRate5min = float64(succeeded5min) / float64(totalProcessed5min)
	}

	totalProcessed30min := succeeded30min + failed30min
	successRate30min := 1.0
	if totalProcessed30min > 0 {
		successRate30min = float64(succeeded30min) / float64(totalProcessed30min)
	}

	return &PoolStats{
		PoolCode:                poolCode,
		TotalProcessed:          totalProcessed,
		TotalSucceeded:          h.messagesSucceeded,
		TotalFailed:             h.messagesFailed,
		TotalRateLimited:        h.messagesRateLimited,
		SuccessRate:             successRate,
		ActiveWorkers:           h.activeWorkers,
		AvailablePermits:        h.availablePermits,
		MaxConcurrency:          h.maxConcurrency,
		QueueSize:               h.queueSize,
		MaxQueueCapacity:        h.maxQueueCapacity,
		AverageProcessingTimeMs: avgProcessingTime,
		TotalProcessed5min:      totalProcessed5min,
		Succeeded5min:           succeeded5min,
		Failed5min:              failed5min,
		SuccessRate5min:         successRate5min,
		TotalProcessed30min:     totalProcessed30min,
		Succeeded30min:          succeeded30min,
		Failed30min:             failed30min,
		SuccessRate30min:        successRate30min,
	}
}
