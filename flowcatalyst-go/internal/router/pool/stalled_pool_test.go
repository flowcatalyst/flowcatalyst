package pool

import (
	"sync"
	"testing"
	"time"
)

/*
THREAT MODEL: Stalled Pool Detection

Process pools can become stalled due to:
1. Infrastructure failures (queue not reachable)
2. Deadlocked worker goroutines
3. Misconfigured rate limiting
4. External mediator unavailability

Detection requirements:
- Pool activity must be tracked with timestamps
- Inactivity threshold: 120 seconds (2 minutes)
- System should be HEALTHY if at least one pool is active
- System should be UNHEALTHY if ALL pools are stalled
- Startup state (no activity yet) should be treated as HEALTHY

Attack vectors being tested:
- Slow poison: gradual degradation of pools
- Complete blackout: all pools suddenly inactive
- Recovery timing: how quickly recovered state is detected
- Edge cases: exact timeout boundary conditions
*/

// PoolStats represents health statistics for a pool
type PoolStats struct {
	PoolCode         string
	LastActivityTime *time.Time
	IsActive         bool
	QueueSize        int
	ActiveWorkers    int
}

// InfrastructureHealthService tracks pool health
type InfrastructureHealthService struct {
	mu               sync.RWMutex
	poolStats        map[string]*PoolStats
	stalledThreshold time.Duration
}

// NewInfrastructureHealthService creates a new health service
func NewInfrastructureHealthService(stalledThreshold time.Duration) *InfrastructureHealthService {
	return &InfrastructureHealthService{
		poolStats:        make(map[string]*PoolStats),
		stalledThreshold: stalledThreshold,
	}
}

// RecordActivity records activity for a pool
func (s *InfrastructureHealthService) RecordActivity(poolCode string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	stats, exists := s.poolStats[poolCode]
	if !exists {
		stats = &PoolStats{PoolCode: poolCode}
		s.poolStats[poolCode] = stats
	}
	stats.LastActivityTime = &now
	stats.IsActive = true
}

// SetPoolStats sets pool statistics
func (s *InfrastructureHealthService) SetPoolStats(poolCode string, stats *PoolStats) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.poolStats[poolCode] = stats
}

// ClearPools clears all pool statistics
func (s *InfrastructureHealthService) ClearPools() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.poolStats = make(map[string]*PoolStats)
}

// IsHealthy returns true if the infrastructure is healthy
func (s *InfrastructureHealthService) IsHealthy() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// No pools = unhealthy (nothing to process)
	if len(s.poolStats) == 0 {
		return false
	}

	now := time.Now()
	hasActivePool := false

	for _, stats := range s.poolStats {
		// Startup state (null timestamp) is considered healthy
		if stats.LastActivityTime == nil {
			hasActivePool = true
			continue
		}

		// Check if pool activity is within threshold
		timeSinceActivity := now.Sub(*stats.LastActivityTime)
		if timeSinceActivity <= s.stalledThreshold {
			hasActivePool = true
		}
	}

	return hasActivePool
}

// GetStalledPools returns list of stalled pool codes
func (s *InfrastructureHealthService) GetStalledPools() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var stalled []string
	now := time.Now()

	for poolCode, stats := range s.poolStats {
		// Startup state is not stalled
		if stats.LastActivityTime == nil {
			continue
		}

		timeSinceActivity := now.Sub(*stats.LastActivityTime)
		if timeSinceActivity > s.stalledThreshold {
			stalled = append(stalled, poolCode)
		}
	}

	return stalled
}

// === Tests ===

const stalledThreshold = 120 * time.Second // 2 minutes

func TestStalledPool_DetectStalledPoolAfterTimeout(t *testing.T) {
	service := NewInfrastructureHealthService(stalledThreshold)

	// Setup pool with old activity
	oldTime := time.Now().Add(-3 * time.Minute) // 3 minutes ago
	service.SetPoolStats("pool-1", &PoolStats{
		PoolCode:         "pool-1",
		LastActivityTime: &oldTime,
		IsActive:         false,
	})

	// Should detect as stalled
	stalledPools := service.GetStalledPools()
	if len(stalledPools) != 1 {
		t.Errorf("Expected 1 stalled pool, got %d", len(stalledPools))
	}
	if len(stalledPools) > 0 && stalledPools[0] != "pool-1" {
		t.Errorf("Expected pool-1 to be stalled, got %s", stalledPools[0])
	}

	// System should be unhealthy
	if service.IsHealthy() {
		t.Error("System should be unhealthy when only pool is stalled")
	}
}

func TestStalledPool_DetectMultipleStalledPools(t *testing.T) {
	service := NewInfrastructureHealthService(stalledThreshold)

	// Setup multiple stalled pools
	oldTime := time.Now().Add(-3 * time.Minute)
	service.SetPoolStats("pool-1", &PoolStats{
		PoolCode:         "pool-1",
		LastActivityTime: &oldTime,
	})
	service.SetPoolStats("pool-2", &PoolStats{
		PoolCode:         "pool-2",
		LastActivityTime: &oldTime,
	})
	service.SetPoolStats("pool-3", &PoolStats{
		PoolCode:         "pool-3",
		LastActivityTime: &oldTime,
	})

	// All pools should be stalled
	stalledPools := service.GetStalledPools()
	if len(stalledPools) != 3 {
		t.Errorf("Expected 3 stalled pools, got %d", len(stalledPools))
	}

	// System should be unhealthy
	if service.IsHealthy() {
		t.Error("System should be unhealthy when ALL pools are stalled")
	}
}

func TestStalledPool_RecoverWhenPoolBecomesActive(t *testing.T) {
	service := NewInfrastructureHealthService(stalledThreshold)

	// Start with stalled pool
	oldTime := time.Now().Add(-3 * time.Minute)
	service.SetPoolStats("pool-1", &PoolStats{
		PoolCode:         "pool-1",
		LastActivityTime: &oldTime,
	})

	// Verify unhealthy
	if service.IsHealthy() {
		t.Error("Should be unhealthy with stalled pool")
	}

	// Record new activity
	service.RecordActivity("pool-1")

	// Should now be healthy
	if !service.IsHealthy() {
		t.Error("Should be healthy after pool becomes active")
	}

	// Should have no stalled pools
	stalledPools := service.GetStalledPools()
	if len(stalledPools) != 0 {
		t.Errorf("Expected 0 stalled pools, got %d", len(stalledPools))
	}
}

func TestStalledPool_HealthyWhenSomePoolsActive(t *testing.T) {
	service := NewInfrastructureHealthService(stalledThreshold)

	// Mix of stalled and active pools
	oldTime := time.Now().Add(-3 * time.Minute)
	recentTime := time.Now().Add(-30 * time.Second)

	service.SetPoolStats("pool-stalled", &PoolStats{
		PoolCode:         "pool-stalled",
		LastActivityTime: &oldTime,
	})
	service.SetPoolStats("pool-active", &PoolStats{
		PoolCode:         "pool-active",
		LastActivityTime: &recentTime,
	})

	// System should still be healthy (at least one pool active)
	if !service.IsHealthy() {
		t.Error("System should be healthy when at least one pool is active")
	}

	// But should still report stalled pool
	stalledPools := service.GetStalledPools()
	if len(stalledPools) != 1 {
		t.Errorf("Expected 1 stalled pool, got %d", len(stalledPools))
	}
}

func TestStalledPool_HealthyDuringStartup(t *testing.T) {
	service := NewInfrastructureHealthService(stalledThreshold)

	// Pool with no activity yet (startup state)
	service.SetPoolStats("pool-1", &PoolStats{
		PoolCode:         "pool-1",
		LastActivityTime: nil, // No activity yet
	})

	// Should be healthy during startup
	if !service.IsHealthy() {
		t.Error("System should be healthy during startup (null timestamps)")
	}

	// Should have no stalled pools
	stalledPools := service.GetStalledPools()
	if len(stalledPools) != 0 {
		t.Errorf("Expected 0 stalled pools during startup, got %d", len(stalledPools))
	}
}

func TestStalledPool_TransitionFromNullToStale(t *testing.T) {
	service := NewInfrastructureHealthService(stalledThreshold)

	// Start with startup state
	service.SetPoolStats("pool-1", &PoolStats{
		PoolCode:         "pool-1",
		LastActivityTime: nil,
	})

	// Verify healthy
	if !service.IsHealthy() {
		t.Error("Should be healthy with null timestamp")
	}

	// Set old activity (simulating time passing without new activity)
	oldTime := time.Now().Add(-3 * time.Minute)
	service.SetPoolStats("pool-1", &PoolStats{
		PoolCode:         "pool-1",
		LastActivityTime: &oldTime,
	})

	// Now should be stalled
	if service.IsHealthy() {
		t.Error("Should be unhealthy after transitioning to stale")
	}
}

func TestStalledPool_ExactTimeoutBoundary(t *testing.T) {
	service := NewInfrastructureHealthService(stalledThreshold)

	// Activity at exactly 115 seconds ago (below 120 threshold)
	belowThreshold := time.Now().Add(-115 * time.Second)
	service.SetPoolStats("pool-1", &PoolStats{
		PoolCode:         "pool-1",
		LastActivityTime: &belowThreshold,
	})

	// Should still be healthy
	if !service.IsHealthy() {
		t.Error("Should be healthy at 115 seconds (below 120 threshold)")
	}
}

func TestStalledPool_JustOverTimeoutBoundary(t *testing.T) {
	service := NewInfrastructureHealthService(stalledThreshold)

	// Activity at just over 120 seconds ago
	overThreshold := time.Now().Add(-121 * time.Second)
	service.SetPoolStats("pool-1", &PoolStats{
		PoolCode:         "pool-1",
		LastActivityTime: &overThreshold,
	})

	// Should be unhealthy
	if service.IsHealthy() {
		t.Error("Should be unhealthy at 121 seconds (over 120 threshold)")
	}
}

func TestStalledPool_EmptyPoolStats(t *testing.T) {
	service := NewInfrastructureHealthService(stalledThreshold)

	// No pools configured
	if service.IsHealthy() {
		t.Error("Should be unhealthy with no pools configured")
	}

	// Should have no stalled pools
	stalledPools := service.GetStalledPools()
	if len(stalledPools) != 0 {
		t.Errorf("Expected 0 stalled pools, got %d", len(stalledPools))
	}
}

func TestStalledPool_MixOfNullAndRecentActivity(t *testing.T) {
	service := NewInfrastructureHealthService(stalledThreshold)

	// Mix of startup pools and recently active pools
	recentTime := time.Now().Add(-30 * time.Second)

	service.SetPoolStats("pool-startup", &PoolStats{
		PoolCode:         "pool-startup",
		LastActivityTime: nil, // Startup
	})
	service.SetPoolStats("pool-active", &PoolStats{
		PoolCode:         "pool-active",
		LastActivityTime: &recentTime,
	})

	// Should be healthy
	if !service.IsHealthy() {
		t.Error("Should be healthy with mix of startup and active pools")
	}
}

func TestStalledPool_ConcurrentActivityRecording(t *testing.T) {
	service := NewInfrastructureHealthService(stalledThreshold)

	// Setup initial state
	service.SetPoolStats("pool-1", &PoolStats{
		PoolCode: "pool-1",
	})

	// Concurrent activity recording
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			service.RecordActivity("pool-1")
		}()
	}
	wg.Wait()

	// Should be healthy after activity
	if !service.IsHealthy() {
		t.Error("Should be healthy after concurrent activity")
	}
}

func TestStalledPool_MultipleStatusChanges(t *testing.T) {
	service := NewInfrastructureHealthService(stalledThreshold)

	// Simulate multiple status changes
	for i := 0; i < 5; i++ {
		// Make stalled
		oldTime := time.Now().Add(-3 * time.Minute)
		service.SetPoolStats("pool-1", &PoolStats{
			PoolCode:         "pool-1",
			LastActivityTime: &oldTime,
		})
		if service.IsHealthy() {
			t.Errorf("Iteration %d: Should be unhealthy when stalled", i)
		}

		// Make active
		service.RecordActivity("pool-1")
		if !service.IsHealthy() {
			t.Errorf("Iteration %d: Should be healthy after activity", i)
		}
	}
}
