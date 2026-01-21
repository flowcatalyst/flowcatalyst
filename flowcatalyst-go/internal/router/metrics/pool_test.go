package metrics

import (
	"testing"
	"time"
)

func TestNewInMemoryPoolMetricsService(t *testing.T) {
	svc := NewInMemoryPoolMetricsService()

	if svc == nil {
		t.Fatal("NewInMemoryPoolMetricsService returned nil")
	}

	if svc.metrics == nil {
		t.Error("Metrics map should be initialized")
	}
}

func TestPoolMetricsService_RecordMessageSubmitted(t *testing.T) {
	svc := NewInMemoryPoolMetricsService()

	svc.RecordMessageSubmitted("pool1")
	svc.RecordMessageSubmitted("pool1")
	svc.RecordMessageSubmitted("pool1")

	stats := svc.GetPoolStats("pool1")
	// messagesSubmitted is not directly exposed in PoolStats, but verifies no panic
	if stats == nil {
		t.Error("Should return stats for pool1")
	}
}

func TestPoolMetricsService_RecordProcessingSuccess(t *testing.T) {
	svc := NewInMemoryPoolMetricsService()

	svc.RecordProcessingSuccess("pool1", 100)
	svc.RecordProcessingSuccess("pool1", 200)

	stats := svc.GetPoolStats("pool1")

	if stats.TotalSucceeded != 2 {
		t.Errorf("Expected 2 succeeded, got %d", stats.TotalSucceeded)
	}

	if stats.TotalProcessed != 2 {
		t.Errorf("Expected 2 processed, got %d", stats.TotalProcessed)
	}

	// Average should be 150ms
	if stats.AverageProcessingTimeMs != 150 {
		t.Errorf("Expected avg 150ms, got %f", stats.AverageProcessingTimeMs)
	}
}

func TestPoolMetricsService_RecordProcessingFailure(t *testing.T) {
	svc := NewInMemoryPoolMetricsService()

	svc.RecordProcessingSuccess("pool1", 100)
	svc.RecordProcessingFailure("pool1", 50, "server_error")

	stats := svc.GetPoolStats("pool1")

	if stats.TotalSucceeded != 1 {
		t.Errorf("Expected 1 succeeded, got %d", stats.TotalSucceeded)
	}

	if stats.TotalFailed != 1 {
		t.Errorf("Expected 1 failed, got %d", stats.TotalFailed)
	}

	if stats.TotalProcessed != 2 {
		t.Errorf("Expected 2 processed, got %d", stats.TotalProcessed)
	}

	// Success rate should be 0.5
	if stats.SuccessRate != 0.5 {
		t.Errorf("Expected success rate 0.5, got %f", stats.SuccessRate)
	}
}

func TestPoolMetricsService_RecordRateLimitExceeded(t *testing.T) {
	svc := NewInMemoryPoolMetricsService()

	svc.RecordRateLimitExceeded("pool1")
	svc.RecordRateLimitExceeded("pool1")

	stats := svc.GetPoolStats("pool1")

	if stats.TotalRateLimited != 2 {
		t.Errorf("Expected 2 rate limited, got %d", stats.TotalRateLimited)
	}
}

func TestPoolMetricsService_InitializePoolCapacity(t *testing.T) {
	svc := NewInMemoryPoolMetricsService()

	svc.InitializePoolCapacity("pool1", 10, 100)

	stats := svc.GetPoolStats("pool1")

	if stats.MaxConcurrency != 10 {
		t.Errorf("Expected max concurrency 10, got %d", stats.MaxConcurrency)
	}

	if stats.MaxQueueCapacity != 100 {
		t.Errorf("Expected max queue capacity 100, got %d", stats.MaxQueueCapacity)
	}
}

func TestPoolMetricsService_UpdatePoolGauges(t *testing.T) {
	svc := NewInMemoryPoolMetricsService()

	svc.UpdatePoolGauges("pool1", 5, 3, 10, 2)

	stats := svc.GetPoolStats("pool1")

	if stats.ActiveWorkers != 5 {
		t.Errorf("Expected 5 active workers, got %d", stats.ActiveWorkers)
	}

	if stats.AvailablePermits != 3 {
		t.Errorf("Expected 3 available permits, got %d", stats.AvailablePermits)
	}

	if stats.QueueSize != 10 {
		t.Errorf("Expected queue size 10, got %d", stats.QueueSize)
	}
}

func TestPoolMetricsService_GetAllPoolStats(t *testing.T) {
	svc := NewInMemoryPoolMetricsService()

	svc.RecordProcessingSuccess("pool1", 100)
	svc.RecordProcessingSuccess("pool2", 200)
	svc.RecordProcessingSuccess("pool3", 300)

	allStats := svc.GetAllPoolStats()

	if len(allStats) != 3 {
		t.Errorf("Expected 3 pools, got %d", len(allStats))
	}

	if _, ok := allStats["pool1"]; !ok {
		t.Error("Should have stats for pool1")
	}
	if _, ok := allStats["pool2"]; !ok {
		t.Error("Should have stats for pool2")
	}
	if _, ok := allStats["pool3"]; !ok {
		t.Error("Should have stats for pool3")
	}
}

func TestPoolMetricsService_GetLastActivityTimestamp(t *testing.T) {
	svc := NewInMemoryPoolMetricsService()

	// No activity yet
	ts := svc.GetLastActivityTimestamp("pool1")
	if ts != nil {
		t.Error("Should return nil for pool with no activity")
	}

	// Record activity
	before := time.Now()
	svc.RecordProcessingSuccess("pool1", 100)
	after := time.Now()

	ts = svc.GetLastActivityTimestamp("pool1")
	if ts == nil {
		t.Fatal("Should return timestamp after activity")
	}

	if ts.Before(before) || ts.After(after) {
		t.Error("Timestamp should be between before and after")
	}
}

func TestPoolMetricsService_RemovePoolMetrics(t *testing.T) {
	svc := NewInMemoryPoolMetricsService()

	svc.RecordProcessingSuccess("pool1", 100)

	stats := svc.GetPoolStats("pool1")
	if stats.TotalProcessed != 1 {
		t.Fatal("Should have stats for pool1")
	}

	svc.RemovePoolMetrics("pool1")

	stats = svc.GetPoolStats("pool1")
	if stats.TotalProcessed != 0 {
		t.Error("Should return empty stats after removal")
	}
}

func TestPoolMetricsService_EmptyPoolStats(t *testing.T) {
	stats := EmptyPoolStats("test-pool")

	if stats.PoolCode != "test-pool" {
		t.Errorf("Expected pool code 'test-pool', got %s", stats.PoolCode)
	}

	if stats.SuccessRate != 1.0 {
		t.Errorf("Expected default success rate 1.0, got %f", stats.SuccessRate)
	}
}

func TestPoolMetricsService_RollingWindowMetrics(t *testing.T) {
	svc := NewInMemoryPoolMetricsService()

	// Record some successes and failures
	svc.RecordProcessingSuccess("pool1", 100)
	svc.RecordProcessingSuccess("pool1", 100)
	svc.RecordProcessingFailure("pool1", 100, "error")

	stats := svc.GetPoolStats("pool1")

	// 5-minute window should include all
	if stats.TotalProcessed5min != 3 {
		t.Errorf("Expected 3 processed in 5min window, got %d", stats.TotalProcessed5min)
	}

	if stats.Succeeded5min != 2 {
		t.Errorf("Expected 2 succeeded in 5min window, got %d", stats.Succeeded5min)
	}

	if stats.Failed5min != 1 {
		t.Errorf("Expected 1 failed in 5min window, got %d", stats.Failed5min)
	}

	// Success rate should be 2/3
	expectedRate := 2.0 / 3.0
	if stats.SuccessRate5min < expectedRate-0.01 || stats.SuccessRate5min > expectedRate+0.01 {
		t.Errorf("Expected 5min success rate ~0.67, got %f", stats.SuccessRate5min)
	}
}

func TestPoolMetricsService_TransientDoesNotUpdateActivity(t *testing.T) {
	svc := NewInMemoryPoolMetricsService()

	// Record success first
	svc.RecordProcessingSuccess("pool1", 100)
	activityAfterSuccess := svc.GetLastActivityTimestamp("pool1")

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Record transient
	svc.RecordProcessingTransient("pool1", 50)
	activityAfterTransient := svc.GetLastActivityTimestamp("pool1")

	// Activity timestamp should not change for transient
	if activityAfterTransient == nil || activityAfterSuccess == nil {
		t.Fatal("Activity timestamps should not be nil")
	}

	if !activityAfterTransient.Equal(*activityAfterSuccess) {
		t.Error("Transient error should not update last activity timestamp")
	}
}
