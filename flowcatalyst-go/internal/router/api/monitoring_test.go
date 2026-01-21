package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.flowcatalyst.tech/internal/router/health"
)

// MockHealthStatusService implements health status for testing
type MockHealthStatusService struct {
	status *health.HealthStatus
}

func (m *MockHealthStatusService) GetHealthStatus() *health.HealthStatus {
	if m.status != nil {
		return m.status
	}
	return &health.HealthStatus{
		Status:             "UP",
		ActivePoolCount:    2,
		TotalActiveWorkers: 10,
	}
}

// MockPoolMetricsProvider implements pool metrics for testing
type MockPoolMetricsProvider struct {
	stats        map[string]*health.PoolStats
	lastActivity map[string]*time.Time
}

func (m *MockPoolMetricsProvider) GetAllPoolStats() map[string]*health.PoolStats {
	if m.stats != nil {
		return m.stats
	}
	return map[string]*health.PoolStats{
		"pool1": {PoolCode: "pool1", TotalProcessed: 100},
	}
}

func (m *MockPoolMetricsProvider) GetLastActivityTimestamp(poolCode string) *time.Time {
	if m.lastActivity != nil {
		return m.lastActivity[poolCode]
	}
	return nil
}

// MockQueueStatsGetter implements queue stats for testing
type MockQueueStatsGetter struct {
	stats map[string]*health.QueueStats
}

func (m *MockQueueStatsGetter) GetAllQueueStats() map[string]*health.QueueStats {
	if m.stats != nil {
		return m.stats
	}
	return map[string]*health.QueueStats{
		"queue1": {Name: "queue1", TotalMessages: 50},
	}
}

func (m *MockQueueStatsGetter) GetTotalQueueDepth() int64 {
	return 0
}

func (m *MockQueueStatsGetter) GetThroughput() float64 {
	return 0.0
}

// MockWarningGetter implements warning getter for testing
type MockWarningGetter struct {
	warnings []*health.Warning
}

func (m *MockWarningGetter) GetAllWarnings() []*health.Warning {
	return m.warnings
}

func (m *MockWarningGetter) GetUnacknowledgedWarnings() []*health.Warning {
	var result []*health.Warning
	for _, w := range m.warnings {
		if !w.Acknowledged {
			result = append(result, w)
		}
	}
	return result
}

// MockStandbyService implements StandbyStatusGetter for testing
type MockStandbyService struct {
	enabled bool
	status  *health.StandbyStatus
}

func (m *MockStandbyService) IsEnabled() bool {
	return m.enabled
}

func (m *MockStandbyService) GetStatus() *health.StandbyStatus {
	return m.status
}

// MockTrafficService implements TrafficStatusGetter for testing
type MockTrafficService struct {
	enabled bool
	status  *health.TrafficStatus
}

func (m *MockTrafficService) IsEnabled() bool {
	return m.enabled
}

func (m *MockTrafficService) GetStatus() *health.TrafficStatus {
	return m.status
}

func TestNewMonitoringHandler(t *testing.T) {
	healthSvc := &health.HealthStatusService{}
	poolMetrics := &MockPoolMetricsProvider{}

	handler := NewMonitoringHandler(healthSvc, poolMetrics)

	if handler == nil {
		t.Fatal("NewMonitoringHandler returned nil")
	}
}

func TestMonitoringHandler_GetPoolStats(t *testing.T) {
	poolMetrics := &MockPoolMetricsProvider{
		stats: map[string]*health.PoolStats{
			"pool1": {PoolCode: "pool1", TotalProcessed: 100},
			"pool2": {PoolCode: "pool2", TotalProcessed: 200},
		},
	}

	handler := &MonitoringHandler{
		poolMetrics: poolMetrics,
	}

	req := httptest.NewRequest(http.MethodGet, "/monitoring/pool-stats", nil)
	w := httptest.NewRecorder()

	handler.GetPoolStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result map[string]*health.PoolStats
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 pools, got %d", len(result))
	}
}

func TestMonitoringHandler_GetQueueStats(t *testing.T) {
	queueMetrics := &MockQueueStatsGetter{
		stats: map[string]*health.QueueStats{
			"queue1": {Name: "queue1", TotalMessages: 50},
		},
	}

	handler := &MonitoringHandler{
		queueMetrics: queueMetrics,
	}

	req := httptest.NewRequest(http.MethodGet, "/monitoring/queue-stats", nil)
	w := httptest.NewRecorder()

	handler.GetQueueStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result map[string]*health.QueueStats
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 queue, got %d", len(result))
	}
}

func TestMonitoringHandler_GetAllWarnings(t *testing.T) {
	warningGetter := &MockWarningGetter{
		warnings: []*health.Warning{
			{ID: "w1", Severity: "ERROR", Message: "Test error"},
			{ID: "w2", Severity: "WARNING", Message: "Test warning"},
		},
	}

	handler := &MonitoringHandler{
		warningService: warningGetter,
	}

	req := httptest.NewRequest(http.MethodGet, "/monitoring/warnings", nil)
	w := httptest.NewRecorder()

	handler.GetAllWarnings(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result []*health.Warning
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 warnings, got %d", len(result))
	}
}

func TestMonitoringHandler_GetStandbyStatus_Disabled(t *testing.T) {
	standbySvc := &MockStandbyService{
		enabled: false,
	}

	handler := &MonitoringHandler{
		standbyService: standbySvc,
	}

	req := httptest.NewRequest(http.MethodGet, "/monitoring/standby-status", nil)
	w := httptest.NewRecorder()

	handler.GetStandbyStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result["standbyEnabled"] != false {
		t.Error("Expected standbyEnabled to be false")
	}
}

func TestMonitoringHandler_GetStandbyStatus_Enabled(t *testing.T) {
	standbySvc := &MockStandbyService{
		enabled: true,
		status: &health.StandbyStatus{
			StandbyEnabled: true,
			InstanceID:     "instance-123",
			Role:           "PRIMARY",
			RedisAvailable: true,
		},
	}

	handler := &MonitoringHandler{
		standbyService: standbySvc,
	}

	req := httptest.NewRequest(http.MethodGet, "/monitoring/standby-status", nil)
	w := httptest.NewRecorder()

	handler.GetStandbyStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result health.StandbyStatus
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if !result.StandbyEnabled {
		t.Error("Expected standbyEnabled to be true")
	}

	if result.Role != "PRIMARY" {
		t.Errorf("Expected role PRIMARY, got %s", result.Role)
	}
}

func TestMonitoringHandler_GetTrafficStatus_Disabled(t *testing.T) {
	trafficSvc := &MockTrafficService{
		enabled: false,
	}

	handler := &MonitoringHandler{
		trafficService: trafficSvc,
	}

	req := httptest.NewRequest(http.MethodGet, "/monitoring/traffic-status", nil)
	w := httptest.NewRecorder()

	handler.GetTrafficStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result health.TrafficStatus
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result.Enabled {
		t.Error("Expected enabled to be false")
	}
}

func TestMonitoringHandler_GetTrafficStatus_Enabled(t *testing.T) {
	trafficSvc := &MockTrafficService{
		enabled: true,
		status: &health.TrafficStatus{
			Enabled:      true,
			StrategyType: "aws-alb",
			Registered:   true,
		},
	}

	handler := &MonitoringHandler{
		trafficService: trafficSvc,
	}

	req := httptest.NewRequest(http.MethodGet, "/monitoring/traffic-status", nil)
	w := httptest.NewRecorder()

	handler.GetTrafficStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result health.TrafficStatus
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if !result.Enabled {
		t.Error("Expected enabled to be true")
	}

	if result.StrategyType != "aws-alb" {
		t.Errorf("Expected strategy aws-alb, got %s", result.StrategyType)
	}
}

func TestMonitoringHandler_MethodNotAllowed(t *testing.T) {
	handler := &MonitoringHandler{}

	tests := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"GetPoolStats", handler.GetPoolStats},
		{"GetQueueStats", handler.GetQueueStats},
		{"GetAllWarnings", handler.GetAllWarnings},
		{"GetStandbyStatus", handler.GetStandbyStatus},
		{"GetTrafficStatus", handler.GetTrafficStatus},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			w := httptest.NewRecorder()

			tc.handler(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("Expected status 405, got %d", w.Code)
			}
		})
	}
}

func TestMonitoringHandler_NilServices(t *testing.T) {
	handler := &MonitoringHandler{}

	// GetPoolStats with nil poolMetrics
	req := httptest.NewRequest(http.MethodGet, "/monitoring/pool-stats", nil)
	w := httptest.NewRecorder()
	handler.GetPoolStats(w, req)
	if w.Code != http.StatusOK {
		t.Error("Should return 200 with empty map")
	}

	// GetQueueStats with nil queueMetrics
	req = httptest.NewRequest(http.MethodGet, "/monitoring/queue-stats", nil)
	w = httptest.NewRecorder()
	handler.GetQueueStats(w, req)
	if w.Code != http.StatusOK {
		t.Error("Should return 200 with empty map")
	}

	// GetAllWarnings with nil warningService
	req = httptest.NewRequest(http.MethodGet, "/monitoring/warnings", nil)
	w = httptest.NewRecorder()
	handler.GetAllWarnings(w, req)
	if w.Code != http.StatusOK {
		t.Error("Should return 200 with empty array")
	}
}
