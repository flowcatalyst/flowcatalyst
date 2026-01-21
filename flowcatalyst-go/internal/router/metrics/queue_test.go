package metrics

import (
	"testing"
	"time"
)

func TestNewInMemoryQueueMetricsService(t *testing.T) {
	svc := NewInMemoryQueueMetricsService()

	if svc == nil {
		t.Fatal("NewInMemoryQueueMetricsService returned nil")
	}

	if svc.metrics == nil {
		t.Error("Metrics map should be initialized")
	}
}

func TestQueueMetricsService_RecordMessageReceived(t *testing.T) {
	svc := NewInMemoryQueueMetricsService()

	svc.RecordMessageReceived("queue1")
	svc.RecordMessageReceived("queue1")
	svc.RecordMessageReceived("queue1")

	stats := svc.GetQueueStats("queue1")

	if stats.TotalMessages != 3 {
		t.Errorf("Expected 3 total messages, got %d", stats.TotalMessages)
	}
}

func TestQueueMetricsService_RecordMessageProcessed(t *testing.T) {
	svc := NewInMemoryQueueMetricsService()

	svc.RecordMessageReceived("queue1")
	svc.RecordMessageReceived("queue1")
	svc.RecordMessageReceived("queue1")

	svc.RecordMessageProcessed("queue1", true)
	svc.RecordMessageProcessed("queue1", true)
	svc.RecordMessageProcessed("queue1", false)

	stats := svc.GetQueueStats("queue1")

	if stats.TotalConsumed != 2 {
		t.Errorf("Expected 2 consumed, got %d", stats.TotalConsumed)
	}

	if stats.TotalFailed != 1 {
		t.Errorf("Expected 1 failed, got %d", stats.TotalFailed)
	}
}

func TestQueueMetricsService_SuccessRate(t *testing.T) {
	svc := NewInMemoryQueueMetricsService()

	svc.RecordMessageReceived("queue1")
	svc.RecordMessageReceived("queue1")
	svc.RecordMessageReceived("queue1")
	svc.RecordMessageReceived("queue1")

	svc.RecordMessageProcessed("queue1", true)
	svc.RecordMessageProcessed("queue1", true)
	svc.RecordMessageProcessed("queue1", true)
	svc.RecordMessageProcessed("queue1", false)

	stats := svc.GetQueueStats("queue1")

	// 3 consumed out of 4 total = 0.75
	if stats.SuccessRate != 0.75 {
		t.Errorf("Expected success rate 0.75, got %f", stats.SuccessRate)
	}
}

func TestQueueMetricsService_RecordQueueDepth(t *testing.T) {
	svc := NewInMemoryQueueMetricsService()

	svc.RecordQueueDepth("queue1", 100)

	stats := svc.GetQueueStats("queue1")

	if stats.CurrentSize != 100 {
		t.Errorf("Expected current size 100, got %d", stats.CurrentSize)
	}

	// Update depth
	svc.RecordQueueDepth("queue1", 50)

	stats = svc.GetQueueStats("queue1")

	if stats.CurrentSize != 50 {
		t.Errorf("Expected current size 50, got %d", stats.CurrentSize)
	}
}

func TestQueueMetricsService_RecordQueueMetrics(t *testing.T) {
	svc := NewInMemoryQueueMetricsService()

	svc.RecordQueueMetrics("queue1", 100, 25)

	stats := svc.GetQueueStats("queue1")

	if stats.PendingMessages != 100 {
		t.Errorf("Expected pending messages 100, got %d", stats.PendingMessages)
	}

	if stats.MessagesNotVisible != 25 {
		t.Errorf("Expected messages not visible 25, got %d", stats.MessagesNotVisible)
	}
}

func TestQueueMetricsService_GetAllQueueStats(t *testing.T) {
	svc := NewInMemoryQueueMetricsService()

	svc.RecordMessageReceived("queue1")
	svc.RecordMessageReceived("queue2")
	svc.RecordMessageReceived("queue3")

	allStats := svc.GetAllQueueStats()

	if len(allStats) != 3 {
		t.Errorf("Expected 3 queues, got %d", len(allStats))
	}

	if _, ok := allStats["queue1"]; !ok {
		t.Error("Should have stats for queue1")
	}
	if _, ok := allStats["queue2"]; !ok {
		t.Error("Should have stats for queue2")
	}
	if _, ok := allStats["queue3"]; !ok {
		t.Error("Should have stats for queue3")
	}
}

func TestQueueMetricsService_Throughput(t *testing.T) {
	svc := NewInMemoryQueueMetricsService()

	// Record messages processed over time
	svc.RecordMessageProcessed("queue1", true)
	svc.RecordMessageProcessed("queue1", true)
	svc.RecordMessageProcessed("queue1", true)

	// Wait a bit to get measurable throughput
	time.Sleep(100 * time.Millisecond)

	stats := svc.GetQueueStats("queue1")

	// Throughput should be positive (messages per second)
	if stats.Throughput <= 0 {
		t.Errorf("Expected positive throughput, got %f", stats.Throughput)
	}
}

func TestQueueMetricsService_EmptyQueueStats(t *testing.T) {
	stats := EmptyQueueStats("test-queue")

	if stats.Name != "test-queue" {
		t.Errorf("Expected name 'test-queue', got %s", stats.Name)
	}

	if stats.SuccessRate != 1.0 {
		t.Errorf("Expected default success rate 1.0, got %f", stats.SuccessRate)
	}

	if stats.SuccessRate5min != 1.0 {
		t.Errorf("Expected default 5min success rate 1.0, got %f", stats.SuccessRate5min)
	}

	if stats.SuccessRate30min != 1.0 {
		t.Errorf("Expected default 30min success rate 1.0, got %f", stats.SuccessRate30min)
	}
}

func TestQueueMetricsService_NonExistentQueue(t *testing.T) {
	svc := NewInMemoryQueueMetricsService()

	stats := svc.GetQueueStats("non-existent")

	if stats == nil {
		t.Fatal("Should return empty stats, not nil")
	}

	if stats.Name != "non-existent" {
		t.Errorf("Expected name 'non-existent', got %s", stats.Name)
	}

	if stats.TotalMessages != 0 {
		t.Error("Non-existent queue should have 0 messages")
	}
}

func TestQueueMetricsService_RollingWindowMetrics(t *testing.T) {
	svc := NewInMemoryQueueMetricsService()

	// Record some successes and failures
	svc.RecordMessageProcessed("queue1", true)
	svc.RecordMessageProcessed("queue1", true)
	svc.RecordMessageProcessed("queue1", false)

	stats := svc.GetQueueStats("queue1")

	// 5-minute window should include all
	if stats.TotalMessages5min != 3 {
		t.Errorf("Expected 3 messages in 5min window, got %d", stats.TotalMessages5min)
	}

	if stats.Consumed5min != 2 {
		t.Errorf("Expected 2 consumed in 5min window, got %d", stats.Consumed5min)
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

func TestQueueMetricsService_ConcurrentAccess(t *testing.T) {
	svc := NewInMemoryQueueMetricsService()

	done := make(chan bool, 10)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				svc.RecordMessageReceived("queue1")
				svc.RecordMessageProcessed("queue1", true)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	stats := svc.GetQueueStats("queue1")

	if stats.TotalMessages != 1000 {
		t.Errorf("Expected 1000 total messages, got %d", stats.TotalMessages)
	}

	if stats.TotalConsumed != 1000 {
		t.Errorf("Expected 1000 consumed, got %d", stats.TotalConsumed)
	}
}
