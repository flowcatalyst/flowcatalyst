package health

import (
	"log/slog"
	"sync"
	"time"
)

const (
	// ActivityTimeoutMs is the timeout for considering infrastructure stalled
	// If no processing activity in last 2 minutes and pools exist, consider stalled
	ActivityTimeoutMs = 120_000 // 2 minutes
)

// PoolMetricsProvider provides pool metrics for health checking
type PoolMetricsProvider interface {
	// GetAllPoolStats returns statistics for all processing pools
	GetAllPoolStats() map[string]*PoolStats
	// GetLastActivityTimestamp returns the last activity time for a pool
	GetLastActivityTimestamp(poolCode string) *time.Time
}

// InfrastructureHealthService checks if the message router infrastructure is healthy.
// Returns unhealthy status only if the message router infrastructure itself is compromised,
// not if downstream services are failing.
type InfrastructureHealthService struct {
	mu sync.RWMutex

	enabled         bool
	poolMetrics     PoolMetricsProvider
	queueManagerOK  bool
	lastHealthCheck time.Time
	cachedHealth    *InfrastructureHealth
}

// NewInfrastructureHealthService creates a new infrastructure health service
func NewInfrastructureHealthService(enabled bool, poolMetrics PoolMetricsProvider) *InfrastructureHealthService {
	return &InfrastructureHealthService{
		enabled:     enabled,
		poolMetrics: poolMetrics,
	}
}

// SetQueueManagerStatus updates the queue manager initialization status
func (s *InfrastructureHealthService) SetQueueManagerStatus(ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queueManagerOK = ok
}

// CheckHealth checks if the message router infrastructure is healthy
func (s *InfrastructureHealthService) CheckHealth() *InfrastructureHealth {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastHealthCheck = time.Now()

	// If message router is disabled, it's healthy (not running = not broken)
	if !s.enabled {
		health := &InfrastructureHealth{
			Healthy: true,
			Message: "Message router is disabled",
			Issues:  nil,
		}
		s.cachedHealth = health
		return health
	}

	var issues []string

	// Check 1: QueueManager initialization and pools exist
	var poolActivity map[string]*time.Time

	if s.poolMetrics != nil {
		allStats := s.poolMetrics.GetAllPoolStats()
		if len(allStats) == 0 {
			issues = append(issues, "No active process pools")
		}
		poolActivity = s.checkProcessPoolActivity()
	} else {
		issues = append(issues, "QueueManager not initialized")
		poolActivity = make(map[string]*time.Time)
	}

	// Check 2: Process pools with activity are not stalled
	if len(poolActivity) > 0 {
		stalledPools := s.checkForStalledPools(poolActivity)
		if len(stalledPools) > 0 && len(stalledPools) == len(poolActivity) {
			// Only fail if ALL pools with activity are stalled
			issues = append(issues, "All process pools appear stalled (no activity in 120s)")
		}
	}

	healthy := len(issues) == 0
	var message string
	if healthy {
		message = "Infrastructure is operational"
	} else {
		message = "Infrastructure issues detected"
	}

	health := &InfrastructureHealth{
		Healthy: healthy,
		Message: message,
		Issues:  issues,
	}

	if len(issues) == 0 {
		health.Issues = nil
	}

	s.cachedHealth = health
	return health
}

// checkProcessPoolActivity gets last activity timestamp for each pool
func (s *InfrastructureHealthService) checkProcessPoolActivity() map[string]*time.Time {
	poolActivity := make(map[string]*time.Time)

	if s.poolMetrics == nil {
		return poolActivity
	}

	allStats := s.poolMetrics.GetAllPoolStats()
	for poolCode := range allStats {
		lastActivity := s.poolMetrics.GetLastActivityTimestamp(poolCode)
		if lastActivity != nil {
			poolActivity[poolCode] = lastActivity
		}
	}

	return poolActivity
}

// checkForStalledPools checks which pools haven't processed messages recently
func (s *InfrastructureHealthService) checkForStalledPools(poolActivity map[string]*time.Time) []string {
	var stalledPools []string
	currentTime := time.Now()

	for poolCode, lastActivity := range poolActivity {
		if lastActivity == nil {
			// Pool exists but has never processed anything
			// This is OK during startup or if no messages have arrived
			continue
		}

		timeSinceActivity := currentTime.Sub(*lastActivity)
		if timeSinceActivity.Milliseconds() > ActivityTimeoutMs {
			stalledPools = append(stalledPools, poolCode)
			slog.Warn("Pool has not processed messages recently",
				"poolCode", poolCode,
				"secondsSinceActivity", int64(timeSinceActivity.Seconds()))
		}
	}

	return stalledPools
}

// GetLastHealthCheck returns the time of the last health check
func (s *InfrastructureHealthService) GetLastHealthCheck() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastHealthCheck
}

// GetCachedHealth returns the last health check result
func (s *InfrastructureHealthService) GetCachedHealth() *InfrastructureHealth {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cachedHealth
}
