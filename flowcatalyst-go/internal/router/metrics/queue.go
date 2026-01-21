package metrics

import (
	"sync"
	"time"
)

// QueueStats represents statistics for a queue
type QueueStats struct {
	Name               string  `json:"name"`
	TotalMessages      int64   `json:"totalMessages"`
	TotalConsumed      int64   `json:"totalConsumed"`
	TotalFailed        int64   `json:"totalFailed"`
	SuccessRate        float64 `json:"successRate"`
	CurrentSize        int64   `json:"currentSize"`
	Throughput         float64 `json:"throughput"`
	PendingMessages    int64   `json:"pendingMessages"`
	MessagesNotVisible int64   `json:"messagesNotVisible"`
	// 5-minute rolling window
	TotalMessages5min int64   `json:"totalMessages5min"`
	Consumed5min      int64   `json:"consumed5min"`
	Failed5min        int64   `json:"failed5min"`
	SuccessRate5min   float64 `json:"successRate5min"`
	// 30-minute rolling window
	TotalMessages30min int64   `json:"totalMessages30min"`
	Consumed30min      int64   `json:"consumed30min"`
	Failed30min        int64   `json:"failed30min"`
	SuccessRate30min   float64 `json:"successRate30min"`
}

// EmptyQueueStats returns empty statistics for a queue
func EmptyQueueStats(queueID string) *QueueStats {
	return &QueueStats{
		Name:             queueID,
		SuccessRate:      1.0,
		SuccessRate5min:  1.0,
		SuccessRate30min: 1.0,
	}
}

// QueueMetricsService tracks queue-level metrics including message throughput,
// success/failure rates, and queue depth.
type QueueMetricsService interface {
	RecordMessageReceived(queueID string)
	RecordMessageProcessed(queueID string, success bool)
	RecordQueueDepth(queueID string, depth int64)
	RecordQueueMetrics(queueID string, pendingMessages, messagesNotVisible int64)
	GetQueueStats(queueID string) *QueueStats
	GetAllQueueStats() map[string]*QueueStats
}

// timestampedOutcome tracks message outcomes for rolling window calculations
type timestampedOutcome struct {
	timestamp time.Time
	success   bool
}

// queueMetricsHolder holds metrics for a single queue
type queueMetricsHolder struct {
	mu                 sync.RWMutex
	messagesReceived   int64
	messagesConsumed   int64
	messagesFailed     int64
	currentDepth       int64
	pendingMessages    int64
	messagesNotVisible int64
	startTime          time.Time
	lastProcessedTime  time.Time
	recordedOutcomes   []timestampedOutcome
}

// InMemoryQueueMetricsService is an in-memory implementation of QueueMetricsService
type InMemoryQueueMetricsService struct {
	mu      sync.RWMutex
	metrics map[string]*queueMetricsHolder
}

// NewInMemoryQueueMetricsService creates a new queue metrics service
func NewInMemoryQueueMetricsService() *InMemoryQueueMetricsService {
	return &InMemoryQueueMetricsService{
		metrics: make(map[string]*queueMetricsHolder),
	}
}

// RecordMessageReceived records that a message was received from a queue
func (s *InMemoryQueueMetricsService) RecordMessageReceived(queueID string) {
	s.getOrCreateMetrics(queueID).recordReceived()
}

// RecordMessageProcessed records that a message was processed
func (s *InMemoryQueueMetricsService) RecordMessageProcessed(queueID string, success bool) {
	s.getOrCreateMetrics(queueID).recordProcessed(success)
}

// RecordQueueDepth records the current queue depth
func (s *InMemoryQueueMetricsService) RecordQueueDepth(queueID string, depth int64) {
	s.getOrCreateMetrics(queueID).recordDepth(depth)
}

// RecordQueueMetrics records pending messages and messages not visible
func (s *InMemoryQueueMetricsService) RecordQueueMetrics(queueID string, pendingMessages, messagesNotVisible int64) {
	s.getOrCreateMetrics(queueID).recordMetrics(pendingMessages, messagesNotVisible)
}

// GetQueueStats returns statistics for a specific queue
func (s *InMemoryQueueMetricsService) GetQueueStats(queueID string) *QueueStats {
	s.mu.RLock()
	holder, ok := s.metrics[queueID]
	s.mu.RUnlock()

	if !ok {
		return EmptyQueueStats(queueID)
	}

	return holder.buildStats(queueID)
}

// GetAllQueueStats returns statistics for all queues
func (s *InMemoryQueueMetricsService) GetAllQueueStats() map[string]*QueueStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*QueueStats)
	for queueID, holder := range s.metrics {
		result[queueID] = holder.buildStats(queueID)
	}
	return result
}

func (s *InMemoryQueueMetricsService) getOrCreateMetrics(queueID string) *queueMetricsHolder {
	s.mu.RLock()
	holder, ok := s.metrics[queueID]
	s.mu.RUnlock()

	if ok {
		return holder
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	if holder, ok := s.metrics[queueID]; ok {
		return holder
	}

	holder = &queueMetricsHolder{
		startTime:         time.Now(),
		lastProcessedTime: time.Now(),
		recordedOutcomes:  make([]timestampedOutcome, 0),
	}
	s.metrics[queueID] = holder
	return holder
}

func (h *queueMetricsHolder) recordReceived() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.messagesReceived++
}

func (h *queueMetricsHolder) recordProcessed(success bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if success {
		h.messagesConsumed++
	} else {
		h.messagesFailed++
	}
	h.lastProcessedTime = time.Now()
	h.recordedOutcomes = append(h.recordedOutcomes, timestampedOutcome{
		timestamp: time.Now(),
		success:   success,
	})
}

func (h *queueMetricsHolder) recordDepth(depth int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.currentDepth = depth
}

func (h *queueMetricsHolder) recordMetrics(pendingMessages, messagesNotVisible int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pendingMessages = pendingMessages
	h.messagesNotVisible = messagesNotVisible
}

func (h *queueMetricsHolder) buildStats(queueID string) *QueueStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	now := time.Now()
	totalMessages := h.messagesReceived
	totalConsumed := h.messagesConsumed
	totalFailed := h.messagesFailed

	successRate := 1.0
	if totalMessages > 0 {
		successRate = float64(totalConsumed) / float64(totalMessages)
	}

	// Calculate throughput (messages per second)
	elapsed := now.Sub(h.startTime).Seconds()
	throughput := 0.0
	if elapsed > 0 {
		throughput = float64(totalConsumed) / elapsed
	}

	// Calculate rolling window metrics
	fiveMinutesAgo := now.Add(-5 * time.Minute)
	thirtyMinutesAgo := now.Add(-30 * time.Minute)

	var consumed5min, failed5min, consumed30min, failed30min int64

	// Clean up old outcomes and count recent ones
	validOutcomes := make([]timestampedOutcome, 0)
	for _, outcome := range h.recordedOutcomes {
		if outcome.timestamp.After(thirtyMinutesAgo) {
			validOutcomes = append(validOutcomes, outcome)
			if outcome.success {
				consumed30min++
				if outcome.timestamp.After(fiveMinutesAgo) {
					consumed5min++
				}
			} else {
				failed30min++
				if outcome.timestamp.After(fiveMinutesAgo) {
					failed5min++
				}
			}
		}
	}

	totalMessages5min := consumed5min + failed5min
	successRate5min := 1.0
	if totalMessages5min > 0 {
		successRate5min = float64(consumed5min) / float64(totalMessages5min)
	}

	totalMessages30min := consumed30min + failed30min
	successRate30min := 1.0
	if totalMessages30min > 0 {
		successRate30min = float64(consumed30min) / float64(totalMessages30min)
	}

	return &QueueStats{
		Name:               queueID,
		TotalMessages:      totalMessages,
		TotalConsumed:      totalConsumed,
		TotalFailed:        totalFailed,
		SuccessRate:        successRate,
		CurrentSize:        h.currentDepth,
		Throughput:         throughput,
		PendingMessages:    h.pendingMessages,
		MessagesNotVisible: h.messagesNotVisible,
		TotalMessages5min:  totalMessages5min,
		Consumed5min:       consumed5min,
		Failed5min:         failed5min,
		SuccessRate5min:    successRate5min,
		TotalMessages30min: totalMessages30min,
		Consumed30min:      consumed30min,
		Failed30min:        failed30min,
		SuccessRate30min:   successRate30min,
	}
}
