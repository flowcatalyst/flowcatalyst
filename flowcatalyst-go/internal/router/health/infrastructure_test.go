package health

import (
	"testing"
	"time"
)

// MockPoolMetricsProvider implements PoolMetricsProvider for testing
type MockPoolMetricsProvider struct {
	stats        map[string]*PoolStats
	lastActivity map[string]*time.Time
}

func NewMockPoolMetricsProvider() *MockPoolMetricsProvider {
	return &MockPoolMetricsProvider{
		stats:        make(map[string]*PoolStats),
		lastActivity: make(map[string]*time.Time),
	}
}

func (m *MockPoolMetricsProvider) GetAllPoolStats() map[string]*PoolStats {
	return m.stats
}

func (m *MockPoolMetricsProvider) GetLastActivityTimestamp(poolCode string) *time.Time {
	return m.lastActivity[poolCode]
}

func (m *MockPoolMetricsProvider) AddPool(poolCode string, stats *PoolStats, lastActivity *time.Time) {
	m.stats[poolCode] = stats
	m.lastActivity[poolCode] = lastActivity
}

func TestNewInfrastructureHealthService(t *testing.T) {
	provider := NewMockPoolMetricsProvider()
	svc := NewInfrastructureHealthService(true, provider)

	if svc == nil {
		t.Fatal("NewInfrastructureHealthService returned nil")
	}

	if !svc.enabled {
		t.Error("Service should be enabled")
	}
}

func TestInfrastructureHealthService_DisabledReturnsHealthy(t *testing.T) {
	svc := NewInfrastructureHealthService(false, nil)
	health := svc.CheckHealth()

	if !health.Healthy {
		t.Error("Disabled service should report healthy")
	}

	if health.Message != "Message router is disabled" {
		t.Errorf("Expected disabled message, got: %s", health.Message)
	}
}

func TestInfrastructureHealthService_NilPoolMetrics(t *testing.T) {
	svc := NewInfrastructureHealthService(true, nil)
	health := svc.CheckHealth()

	if health.Healthy {
		t.Error("Service without pool metrics should be unhealthy")
	}

	if len(health.Issues) == 0 {
		t.Error("Should have issues when pool metrics is nil")
	}
}

func TestInfrastructureHealthService_NoActivePools(t *testing.T) {
	provider := NewMockPoolMetricsProvider()
	svc := NewInfrastructureHealthService(true, provider)

	health := svc.CheckHealth()

	if health.Healthy {
		t.Error("Service with no pools should be unhealthy")
	}

	found := false
	for _, issue := range health.Issues {
		if issue == "No active process pools" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Should report 'No active process pools' issue")
	}
}

func TestInfrastructureHealthService_HealthyWithActivePools(t *testing.T) {
	provider := NewMockPoolMetricsProvider()
	recentActivity := time.Now()
	provider.AddPool("pool1", &PoolStats{PoolCode: "pool1"}, &recentActivity)

	svc := NewInfrastructureHealthService(true, provider)
	health := svc.CheckHealth()

	if !health.Healthy {
		t.Errorf("Service with active pool should be healthy, got issues: %v", health.Issues)
	}
}

func TestInfrastructureHealthService_StalledPools(t *testing.T) {
	provider := NewMockPoolMetricsProvider()
	// Activity more than 2 minutes ago
	oldActivity := time.Now().Add(-3 * time.Minute)
	provider.AddPool("pool1", &PoolStats{PoolCode: "pool1"}, &oldActivity)

	svc := NewInfrastructureHealthService(true, provider)
	health := svc.CheckHealth()

	if health.Healthy {
		t.Error("Service with all stalled pools should be unhealthy")
	}
}

func TestInfrastructureHealthService_SomePoolsActive(t *testing.T) {
	provider := NewMockPoolMetricsProvider()
	oldActivity := time.Now().Add(-3 * time.Minute)
	recentActivity := time.Now()

	// One stalled, one active
	provider.AddPool("pool1", &PoolStats{PoolCode: "pool1"}, &oldActivity)
	provider.AddPool("pool2", &PoolStats{PoolCode: "pool2"}, &recentActivity)

	svc := NewInfrastructureHealthService(true, provider)
	health := svc.CheckHealth()

	// Should be healthy because not ALL pools are stalled
	if !health.Healthy {
		t.Error("Service should be healthy when at least one pool is active")
	}
}

func TestInfrastructureHealthService_CachedHealth(t *testing.T) {
	provider := NewMockPoolMetricsProvider()
	recentActivity := time.Now()
	provider.AddPool("pool1", &PoolStats{PoolCode: "pool1"}, &recentActivity)

	svc := NewInfrastructureHealthService(true, provider)

	// First check
	health1 := svc.CheckHealth()

	// Get cached
	cached := svc.GetCachedHealth()

	if cached == nil {
		t.Fatal("Cached health should not be nil after check")
	}

	if cached.Healthy != health1.Healthy {
		t.Error("Cached health should match last check")
	}
}

func TestInfrastructureHealthService_LastHealthCheck(t *testing.T) {
	provider := NewMockPoolMetricsProvider()
	svc := NewInfrastructureHealthService(true, provider)

	before := time.Now()
	svc.CheckHealth()
	after := time.Now()

	lastCheck := svc.GetLastHealthCheck()

	if lastCheck.Before(before) || lastCheck.After(after) {
		t.Error("Last health check time should be between before and after")
	}
}

func TestInfrastructureHealthService_SetQueueManagerStatus(t *testing.T) {
	provider := NewMockPoolMetricsProvider()
	svc := NewInfrastructureHealthService(true, provider)

	svc.SetQueueManagerStatus(true)

	if !svc.queueManagerOK {
		t.Error("Queue manager status should be true")
	}

	svc.SetQueueManagerStatus(false)

	if svc.queueManagerOK {
		t.Error("Queue manager status should be false")
	}
}
