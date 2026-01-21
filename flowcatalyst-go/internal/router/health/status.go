package health

import (
	"sync"
	"time"
)

// HealthStatusService provides aggregated health status for the monitoring dashboard
type HealthStatusService struct {
	mu sync.RWMutex

	startTime            time.Time
	infraHealthService   *InfrastructureHealthService
	brokerHealthService  *BrokerHealthService
	poolMetrics          PoolMetricsProvider
	circuitBreakerGetter CircuitBreakerGetter
	warningGetter        WarningGetter
	queueStatsGetter     QueueStatsGetter
}

// CircuitBreakerGetter provides circuit breaker statistics
type CircuitBreakerGetter interface {
	GetAllCircuitBreakerStats() map[string]*CircuitBreakerStats
	GetOpenCircuitBreakerCount() int
}

// WarningGetter provides warning statistics
type WarningGetter interface {
	GetUnacknowledgedWarnings() []*Warning
	GetAllWarnings() []*Warning
}

// QueueStatsGetter provides queue statistics
type QueueStatsGetter interface {
	GetAllQueueStats() map[string]*QueueStats
	GetTotalQueueDepth() int64
	GetThroughput() float64
}

// NewHealthStatusService creates a new health status service
func NewHealthStatusService(
	infraHealth *InfrastructureHealthService,
	brokerHealth *BrokerHealthService,
	poolMetrics PoolMetricsProvider,
) *HealthStatusService {
	return &HealthStatusService{
		startTime:           time.Now(),
		infraHealthService:  infraHealth,
		brokerHealthService: brokerHealth,
		poolMetrics:         poolMetrics,
	}
}

// SetCircuitBreakerGetter sets the circuit breaker stats provider
func (s *HealthStatusService) SetCircuitBreakerGetter(getter CircuitBreakerGetter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.circuitBreakerGetter = getter
}

// SetWarningGetter sets the warning provider
func (s *HealthStatusService) SetWarningGetter(getter WarningGetter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warningGetter = getter
}

// SetQueueStatsGetter sets the queue stats provider
func (s *HealthStatusService) SetQueueStatsGetter(getter QueueStatsGetter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queueStatsGetter = getter
}

// GetHealthStatus returns the aggregated health status
func (s *HealthStatusService) GetHealthStatus() *HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := &HealthStatus{
		Status:                  "UNKNOWN",
		UpSince:                 s.startTime,
		LastInfrastructureCheck: time.Now(),
	}

	// Get infrastructure health
	if s.infraHealthService != nil {
		infraHealth := s.infraHealthService.CheckHealth()
		if infraHealth.Healthy {
			status.InfrastructureHealth = "HEALTHY"
		} else {
			status.InfrastructureHealth = "UNHEALTHY"
		}
		status.LastInfrastructureCheck = s.infraHealthService.GetLastHealthCheck()
	}

	// Get broker health
	if s.brokerHealthService != nil {
		status.BrokerType = string(s.brokerHealthService.GetBrokerType())
		status.BrokerConnected = s.brokerHealthService.IsAvailable()
	}

	// Get pool metrics
	if s.poolMetrics != nil {
		poolStats := s.poolMetrics.GetAllPoolStats()
		status.ActivePoolCount = len(poolStats)

		var totalProcessed, totalSucceeded, totalFailed int64
		var totalActiveWorkers int
		var poolHealth []PoolHealth

		for poolCode, stats := range poolStats {
			totalProcessed += stats.TotalProcessed
			totalSucceeded += stats.TotalSucceeded
			totalFailed += stats.TotalFailed
			totalActiveWorkers += stats.ActiveWorkers

			ph := PoolHealth{
				PoolCode:      poolCode,
				Status:        "HEALTHY",
				ActiveWorkers: stats.ActiveWorkers,
				QueueSize:     stats.QueueSize,
			}

			lastActivity := s.poolMetrics.GetLastActivityTimestamp(poolCode)
			if lastActivity != nil {
				ph.LastActivityAt = *lastActivity
			}

			// Check for stalled pool
			if lastActivity != nil {
				if time.Since(*lastActivity).Milliseconds() > ActivityTimeoutMs {
					ph.Status = "STALLED"
				}
			}

			poolHealth = append(poolHealth, ph)
		}

		status.TotalMessagesProcessed = totalProcessed
		status.TotalMessagesSucceeded = totalSucceeded
		status.TotalMessagesFailed = totalFailed
		status.TotalActiveWorkers = totalActiveWorkers
		status.PoolHealth = poolHealth

		if totalProcessed > 0 {
			status.OverallSuccessRate = float64(totalSucceeded) / float64(totalProcessed)
		}
	}

	// Get circuit breaker stats
	if s.circuitBreakerGetter != nil {
		status.CircuitBreakersOpen = s.circuitBreakerGetter.GetOpenCircuitBreakerCount()
	}

	// Get warning stats
	if s.warningGetter != nil {
		status.UnacknowledgedWarnings = len(s.warningGetter.GetUnacknowledgedWarnings())
	}

	// Get queue stats
	if s.queueStatsGetter != nil {
		status.CurrentQueueDepth = s.queueStatsGetter.GetTotalQueueDepth()
		status.Throughput = s.queueStatsGetter.GetThroughput()
	}

	// Determine overall status
	if status.InfrastructureHealth == "HEALTHY" && status.BrokerConnected {
		if status.CircuitBreakersOpen > 0 {
			status.Status = "DEGRADED"
		} else {
			status.Status = "HEALTHY"
		}
	} else {
		status.Status = "UNHEALTHY"
	}

	return status
}

// GetUptime returns the uptime duration
func (s *HealthStatusService) GetUptime() time.Duration {
	return time.Since(s.startTime)
}
