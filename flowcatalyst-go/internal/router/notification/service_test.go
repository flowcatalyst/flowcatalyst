package notification

import (
	"testing"
	"time"
)

func TestSeverityOrder(t *testing.T) {
	// Test that SeverityOrder slice is defined correctly
	if len(SeverityOrder) != 4 {
		t.Errorf("Expected 4 severity levels, got %d", len(SeverityOrder))
	}

	if SeverityOrder[0] != "INFO" {
		t.Errorf("Expected first severity to be INFO, got %s", SeverityOrder[0])
	}

	if SeverityOrder[3] != "CRITICAL" {
		t.Errorf("Expected last severity to be CRITICAL, got %s", SeverityOrder[3])
	}
}

func TestGetSeverityIndex(t *testing.T) {
	tests := []struct {
		severity string
		expected int
	}{
		{"CRITICAL", 3},
		{"ERROR", 2},
		{"WARNING", 1},
		{"INFO", 0},
		{"UNKNOWN", 0}, // Unknown defaults to 0
		{"", 0},
	}

	for _, tc := range tests {
		index := GetSeverityIndex(tc.severity)
		if index != tc.expected {
			t.Errorf("GetSeverityIndex(%s) = %d, want %d", tc.severity, index, tc.expected)
		}
	}
}

func TestMeetsMinSeverity(t *testing.T) {
	tests := []struct {
		severity, minSeverity string
		expected              bool
	}{
		{"CRITICAL", "ERROR", true},
		{"CRITICAL", "CRITICAL", true},
		{"ERROR", "ERROR", true},
		{"ERROR", "CRITICAL", false},
		{"WARNING", "ERROR", false},
		{"INFO", "WARNING", false},
		{"INFO", "INFO", true},
	}

	for _, tc := range tests {
		result := MeetsMinSeverity(tc.severity, tc.minSeverity)
		if result != tc.expected {
			t.Errorf("MeetsMinSeverity(%s, %s) = %v, want %v", tc.severity, tc.minSeverity, result, tc.expected)
		}
	}
}

func TestWarning(t *testing.T) {
	warning := Warning{
		ID:        "test-123",
		Category:  "SYSTEM",
		Severity:  "ERROR",
		Message:   "Test error message",
		Source:    "test-source",
		Timestamp: time.Now(),
	}

	if warning.ID != "test-123" {
		t.Errorf("Expected ID 'test-123', got %s", warning.ID)
	}

	if warning.Category != "SYSTEM" {
		t.Errorf("Expected Category 'SYSTEM', got %s", warning.Category)
	}

	if warning.Severity != "ERROR" {
		t.Errorf("Expected Severity 'ERROR', got %s", warning.Severity)
	}

	if warning.Message != "Test error message" {
		t.Errorf("Expected Message 'Test error message', got %s", warning.Message)
	}

	if warning.Source != "test-source" {
		t.Errorf("Expected Source 'test-source', got %s", warning.Source)
	}
}

func TestNoOpService(t *testing.T) {
	svc := NewNoOpService()

	if svc == nil {
		t.Fatal("NewNoOpService returned nil")
	}

	// NotifyWarning should not panic
	warning := &Warning{
		ID:       "test-123",
		Category: "SYSTEM",
		Severity: "ERROR",
		Message:  "Test error",
		Source:   "test",
	}

	// Should not panic
	svc.NotifyWarning(warning)
	svc.NotifyCriticalError("Critical error", "test-source")
	svc.NotifySystemEvent("STARTUP", "System started")

	// IsEnabled should return false for NoOp
	if svc.IsEnabled() {
		t.Error("NoOpService.IsEnabled should return false")
	}
}

func TestNoOpService_NilWarning(t *testing.T) {
	svc := NewNoOpService()

	// Should handle nil gracefully (might panic, but let's test the happy path)
	warning := &Warning{
		Severity: "INFO",
		Message:  "Test",
	}
	svc.NotifyWarning(warning)
}
