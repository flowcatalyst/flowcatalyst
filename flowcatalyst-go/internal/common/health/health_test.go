package health

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewChecker(t *testing.T) {
	checker := NewChecker()
	if checker == nil {
		t.Fatal("NewChecker returned nil")
	}

	if len(checker.livenessChecks) != 0 {
		t.Errorf("Expected 0 liveness checks, got %d", len(checker.livenessChecks))
	}

	if len(checker.readinessChecks) != 0 {
		t.Errorf("Expected 0 readiness checks, got %d", len(checker.readinessChecks))
	}
}

func TestAddLivenessCheck(t *testing.T) {
	checker := NewChecker()

	checker.AddLivenessCheck(func() Check {
		return Check{Name: "test", Status: StatusUp}
	})

	if len(checker.livenessChecks) != 1 {
		t.Errorf("Expected 1 liveness check, got %d", len(checker.livenessChecks))
	}
}

func TestAddReadinessCheck(t *testing.T) {
	checker := NewChecker()

	checker.AddReadinessCheck(func() Check {
		return Check{Name: "test", Status: StatusUp}
	})

	if len(checker.readinessChecks) != 1 {
		t.Errorf("Expected 1 readiness check, got %d", len(checker.readinessChecks))
	}
}

func TestGetLiveness_AllHealthy(t *testing.T) {
	checker := NewChecker()

	checker.AddLivenessCheck(func() Check {
		return Check{Name: "check1", Status: StatusUp}
	})
	checker.AddLivenessCheck(func() Check {
		return Check{Name: "check2", Status: StatusUp}
	})

	response := checker.GetLiveness()

	if response.Status != StatusUp {
		t.Errorf("Expected status UP, got %s", response.Status)
	}

	if len(response.Checks) != 2 {
		t.Errorf("Expected 2 checks, got %d", len(response.Checks))
	}
}

func TestGetLiveness_OneUnhealthy(t *testing.T) {
	checker := NewChecker()

	checker.AddLivenessCheck(func() Check {
		return Check{Name: "healthy", Status: StatusUp}
	})
	checker.AddLivenessCheck(func() Check {
		return Check{Name: "unhealthy", Status: StatusDown}
	})

	response := checker.GetLiveness()

	if response.Status != StatusDown {
		t.Errorf("Expected status DOWN when one check fails, got %s", response.Status)
	}
}

func TestGetReadiness_AllHealthy(t *testing.T) {
	checker := NewChecker()

	checker.AddReadinessCheck(func() Check {
		return Check{Name: "db", Status: StatusUp}
	})
	checker.AddReadinessCheck(func() Check {
		return Check{Name: "queue", Status: StatusUp}
	})

	response := checker.GetReadiness()

	if response.Status != StatusUp {
		t.Errorf("Expected status UP, got %s", response.Status)
	}
}

func TestGetHealth_CombinesChecks(t *testing.T) {
	checker := NewChecker()

	checker.AddLivenessCheck(func() Check {
		return Check{Name: "liveness", Status: StatusUp}
	})
	checker.AddReadinessCheck(func() Check {
		return Check{Name: "readiness", Status: StatusUp}
	})

	response := checker.GetHealth()

	if response.Status != StatusUp {
		t.Errorf("Expected status UP, got %s", response.Status)
	}

	if len(response.Checks) != 2 {
		t.Errorf("Expected 2 combined checks, got %d", len(response.Checks))
	}
}

func TestHandleHealth_Returns200WhenHealthy(t *testing.T) {
	checker := NewChecker()
	checker.AddReadinessCheck(func() Check {
		return Check{Name: "db", Status: StatusUp}
	})

	req := httptest.NewRequest(http.MethodGet, "/q/health", nil)
	w := httptest.NewRecorder()

	checker.HandleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != StatusUp {
		t.Errorf("Expected status UP in response, got %s", response.Status)
	}
}

func TestHandleHealth_Returns503WhenUnhealthy(t *testing.T) {
	checker := NewChecker()
	checker.AddReadinessCheck(func() Check {
		return Check{
			Name:   "db",
			Status: StatusDown,
			Data:   map[string]interface{}{"error": "connection refused"},
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/q/health", nil)
	w := httptest.NewRecorder()

	checker.HandleHealth(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}

	var response HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != StatusDown {
		t.Errorf("Expected status DOWN in response, got %s", response.Status)
	}

	if len(response.Checks) != 1 {
		t.Fatalf("Expected 1 check, got %d", len(response.Checks))
	}

	if response.Checks[0].Data["error"] != "connection refused" {
		t.Errorf("Expected error message in check data")
	}
}

func TestHandleLive_Returns200WhenNoChecks(t *testing.T) {
	checker := NewChecker()

	req := httptest.NewRequest(http.MethodGet, "/q/health/live", nil)
	w := httptest.NewRecorder()

	checker.HandleLive(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != StatusUp {
		t.Errorf("Expected status UP when no checks, got %s", response.Status)
	}
}

func TestHandleReady_Returns200WhenNoChecks(t *testing.T) {
	checker := NewChecker()

	req := httptest.NewRequest(http.MethodGet, "/q/health/ready", nil)
	w := httptest.NewRecorder()

	checker.HandleReady(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandleReady_Returns503WhenUnhealthy(t *testing.T) {
	checker := NewChecker()
	checker.AddReadinessCheck(func() Check {
		return Check{Name: "queue", Status: StatusDown}
	})

	req := httptest.NewRequest(http.MethodGet, "/q/health/ready", nil)
	w := httptest.NewRecorder()

	checker.HandleReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}

func TestMongoDBCheck_Healthy(t *testing.T) {
	pingFunc := func() error {
		return nil
	}

	check := MongoDBCheck(pingFunc)()

	if check.Name != "MongoDB" {
		t.Errorf("Expected name 'MongoDB', got '%s'", check.Name)
	}

	if check.Status != StatusUp {
		t.Errorf("Expected status UP, got %s", check.Status)
	}
}

func TestMongoDBCheck_Unhealthy(t *testing.T) {
	pingFunc := func() error {
		return errors.New("connection refused")
	}

	check := MongoDBCheck(pingFunc)()

	if check.Status != StatusDown {
		t.Errorf("Expected status DOWN, got %s", check.Status)
	}

	if check.Data["error"] != "connection refused" {
		t.Errorf("Expected error in data, got %v", check.Data)
	}
}

func TestNATSCheck_Connected(t *testing.T) {
	isConnected := func() bool {
		return true
	}

	check := NATSCheck(isConnected)()

	if check.Name != "NATS" {
		t.Errorf("Expected name 'NATS', got '%s'", check.Name)
	}

	if check.Status != StatusUp {
		t.Errorf("Expected status UP, got %s", check.Status)
	}
}

func TestNATSCheck_Disconnected(t *testing.T) {
	isConnected := func() bool {
		return false
	}

	check := NATSCheck(isConnected)()

	if check.Status != StatusDown {
		t.Errorf("Expected status DOWN, got %s", check.Status)
	}
}

func TestSQSCheck_Healthy(t *testing.T) {
	checkFunc := func() error {
		return nil
	}

	check := SQSCheck(checkFunc)()

	if check.Name != "SQS" {
		t.Errorf("Expected name 'SQS', got '%s'", check.Name)
	}

	if check.Status != StatusUp {
		t.Errorf("Expected status UP, got %s", check.Status)
	}
}

func TestSQSCheck_Unhealthy(t *testing.T) {
	checkFunc := func() error {
		return errors.New("queue not accessible")
	}

	check := SQSCheck(checkFunc)()

	if check.Status != StatusDown {
		t.Errorf("Expected status DOWN, got %s", check.Status)
	}

	if check.Data["error"] != "queue not accessible" {
		t.Errorf("Expected error in data, got %v", check.Data)
	}
}

func TestContentTypeHeader(t *testing.T) {
	checker := NewChecker()

	req := httptest.NewRequest(http.MethodGet, "/q/health", nil)
	w := httptest.NewRecorder()

	checker.HandleHealth(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}

func TestCheckWithData(t *testing.T) {
	checker := NewChecker()
	checker.AddReadinessCheck(func() Check {
		return Check{
			Name:   "pool",
			Status: StatusUp,
			Data: map[string]interface{}{
				"active_pools":     5,
				"messages_pending": 100,
			},
		}
	})

	response := checker.GetReadiness()

	if len(response.Checks) != 1 {
		t.Fatalf("Expected 1 check, got %d", len(response.Checks))
	}

	check := response.Checks[0]
	if check.Data["active_pools"] != 5 {
		t.Errorf("Expected active_pools=5, got %v", check.Data["active_pools"])
	}
}

func TestMultipleIssues(t *testing.T) {
	checker := NewChecker()

	checker.AddReadinessCheck(func() Check {
		return Check{Name: "db", Status: StatusDown, Data: map[string]interface{}{"error": "connection timeout"}}
	})
	checker.AddReadinessCheck(func() Check {
		return Check{Name: "queue", Status: StatusDown, Data: map[string]interface{}{"error": "not reachable"}}
	})
	checker.AddReadinessCheck(func() Check {
		return Check{Name: "cache", Status: StatusUp}
	})

	req := httptest.NewRequest(http.MethodGet, "/q/health/ready", nil)
	w := httptest.NewRecorder()

	checker.HandleReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}

	var response HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(response.Checks) != 3 {
		t.Errorf("Expected 3 checks, got %d", len(response.Checks))
	}

	// Count failed checks
	failedCount := 0
	for _, check := range response.Checks {
		if check.Status == StatusDown {
			failedCount++
		}
	}

	if failedCount != 2 {
		t.Errorf("Expected 2 failed checks, got %d", failedCount)
	}
}

func TestHealthResponseJSONStructure(t *testing.T) {
	checker := NewChecker()
	checker.AddReadinessCheck(func() Check {
		return Check{Name: "db", Status: StatusUp}
	})

	req := httptest.NewRequest(http.MethodGet, "/q/health", nil)
	w := httptest.NewRecorder()

	checker.HandleHealth(w, req)

	// Verify JSON structure matches expected format
	var rawJSON map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&rawJSON); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if _, ok := rawJSON["status"]; !ok {
		t.Error("Expected 'status' field in response")
	}

	if _, ok := rawJSON["checks"]; !ok {
		t.Error("Expected 'checks' field in response")
	}
}

func TestConcurrentChecks(t *testing.T) {
	checker := NewChecker()

	// Add checks that we'll access concurrently
	for i := 0; i < 10; i++ {
		checker.AddReadinessCheck(func() Check {
			return Check{Name: "check", Status: StatusUp}
		})
	}

	// Run concurrent health checks
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			checker.GetHealth()
			done <- true
		}()
	}

	// Wait for all to complete
	for i := 0; i < 100; i++ {
		<-done
	}
}
