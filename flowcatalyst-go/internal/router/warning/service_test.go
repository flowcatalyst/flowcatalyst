package warning

import (
	"testing"
	"time"
)

func TestNewInMemoryService(t *testing.T) {
	svc := NewInMemoryService()

	if svc == nil {
		t.Fatal("NewInMemoryService returned nil")
	}

	if svc.warnings == nil {
		t.Error("Warnings map should be initialized")
	}
}

func TestInMemoryService_AddWarning(t *testing.T) {
	svc := NewInMemoryService()

	svc.AddWarning("SYSTEM", "ERROR", "Test error", "test-source")

	warnings := svc.GetAllWarnings()
	if len(warnings) != 1 {
		t.Fatalf("Expected 1 warning, got %d", len(warnings))
	}

	w := warnings[0]
	if w.Category != "SYSTEM" {
		t.Errorf("Expected category SYSTEM, got %s", w.Category)
	}
	if w.Severity != "ERROR" {
		t.Errorf("Expected severity ERROR, got %s", w.Severity)
	}
	if w.Message != "Test error" {
		t.Errorf("Expected message 'Test error', got %s", w.Message)
	}
	if w.Source != "test-source" {
		t.Errorf("Expected source 'test-source', got %s", w.Source)
	}
	if w.Acknowledged {
		t.Error("New warning should not be acknowledged")
	}
}

func TestInMemoryService_MaxWarningsLimit(t *testing.T) {
	svc := NewInMemoryService()

	// Add more than MaxWarnings
	for i := 0; i < MaxWarnings+10; i++ {
		svc.AddWarning("SYSTEM", "INFO", "Test message", "test")
	}

	warnings := svc.GetAllWarnings()
	if len(warnings) > MaxWarnings {
		t.Errorf("Expected max %d warnings, got %d", MaxWarnings, len(warnings))
	}
}

func TestInMemoryService_GetAllWarnings_SortedByTimestamp(t *testing.T) {
	svc := NewInMemoryService()

	// Add warnings with delays
	svc.AddWarning("SYSTEM", "INFO", "First", "test")
	time.Sleep(10 * time.Millisecond)
	svc.AddWarning("SYSTEM", "INFO", "Second", "test")
	time.Sleep(10 * time.Millisecond)
	svc.AddWarning("SYSTEM", "INFO", "Third", "test")

	warnings := svc.GetAllWarnings()
	if len(warnings) != 3 {
		t.Fatalf("Expected 3 warnings, got %d", len(warnings))
	}

	// Should be sorted newest first
	if warnings[0].Message != "Third" {
		t.Error("First warning should be 'Third' (newest)")
	}
	if warnings[2].Message != "First" {
		t.Error("Last warning should be 'First' (oldest)")
	}
}

func TestInMemoryService_GetWarningsBySeverity(t *testing.T) {
	svc := NewInMemoryService()

	svc.AddWarning("SYSTEM", "ERROR", "Error 1", "test")
	svc.AddWarning("SYSTEM", "WARNING", "Warning 1", "test")
	svc.AddWarning("SYSTEM", "ERROR", "Error 2", "test")
	svc.AddWarning("SYSTEM", "INFO", "Info 1", "test")

	errors := svc.GetWarningsBySeverity("ERROR")
	if len(errors) != 2 {
		t.Errorf("Expected 2 ERROR warnings, got %d", len(errors))
	}

	warnings := svc.GetWarningsBySeverity("WARNING")
	if len(warnings) != 1 {
		t.Errorf("Expected 1 WARNING warning, got %d", len(warnings))
	}

	// Case insensitive
	infos := svc.GetWarningsBySeverity("info")
	if len(infos) != 1 {
		t.Errorf("Expected 1 INFO warning (case insensitive), got %d", len(infos))
	}
}

func TestInMemoryService_GetUnacknowledgedWarnings(t *testing.T) {
	svc := NewInMemoryService()

	svc.AddWarning("SYSTEM", "ERROR", "Error 1", "test")
	svc.AddWarning("SYSTEM", "ERROR", "Error 2", "test")

	warnings := svc.GetAllWarnings()
	if len(warnings) != 2 {
		t.Fatal("Should have 2 warnings")
	}

	// Acknowledge one
	svc.AcknowledgeWarning(warnings[0].ID)

	unacked := svc.GetUnacknowledgedWarnings()
	if len(unacked) != 1 {
		t.Errorf("Expected 1 unacknowledged warning, got %d", len(unacked))
	}
}

func TestInMemoryService_AcknowledgeWarning(t *testing.T) {
	svc := NewInMemoryService()

	svc.AddWarning("SYSTEM", "ERROR", "Test error", "test")
	warnings := svc.GetAllWarnings()
	warningID := warnings[0].ID

	// Acknowledge existing
	result := svc.AcknowledgeWarning(warningID)
	if !result {
		t.Error("Should return true for existing warning")
	}

	// Verify acknowledged
	warnings = svc.GetAllWarnings()
	if !warnings[0].Acknowledged {
		t.Error("Warning should be acknowledged")
	}

	// Acknowledge non-existent
	result = svc.AcknowledgeWarning("non-existent-id")
	if result {
		t.Error("Should return false for non-existent warning")
	}
}

func TestInMemoryService_ClearAllWarnings(t *testing.T) {
	svc := NewInMemoryService()

	svc.AddWarning("SYSTEM", "ERROR", "Error 1", "test")
	svc.AddWarning("SYSTEM", "ERROR", "Error 2", "test")

	if len(svc.GetAllWarnings()) != 2 {
		t.Fatal("Should have 2 warnings before clear")
	}

	svc.ClearAllWarnings()

	if len(svc.GetAllWarnings()) != 0 {
		t.Error("Should have 0 warnings after clear")
	}
}

func TestInMemoryService_ClearOldWarnings(t *testing.T) {
	svc := NewInMemoryService()

	// Add a warning
	svc.AddWarning("SYSTEM", "ERROR", "Recent error", "test")

	// Manually add an old warning
	svc.mu.Lock()
	oldWarning := &Warning{
		ID:        "old-warning",
		Category:  "SYSTEM",
		Severity:  "ERROR",
		Message:   "Old error",
		Timestamp: time.Now().Add(-48 * time.Hour), // 48 hours ago
		Source:    "test",
	}
	svc.warnings["old-warning"] = oldWarning
	svc.mu.Unlock()

	if len(svc.GetAllWarnings()) != 2 {
		t.Fatal("Should have 2 warnings before clearing old")
	}

	// Clear warnings older than 24 hours
	svc.ClearOldWarnings(24)

	warnings := svc.GetAllWarnings()
	if len(warnings) != 1 {
		t.Errorf("Expected 1 warning after clearing old, got %d", len(warnings))
	}

	if warnings[0].Message != "Recent error" {
		t.Error("Remaining warning should be the recent one")
	}
}

func TestInMemoryService_ConcurrentAccess(t *testing.T) {
	svc := NewInMemoryService()

	// Concurrent writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 10; j++ {
				svc.AddWarning("SYSTEM", "INFO", "Concurrent message", "test")
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have exactly 100 warnings (or MaxWarnings if exceeded)
	warnings := svc.GetAllWarnings()
	if len(warnings) != 100 {
		t.Errorf("Expected 100 warnings, got %d", len(warnings))
	}
}
