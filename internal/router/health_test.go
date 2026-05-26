package router

import (
	"testing"
	"time"
)

func TestHealthService_PoolSuccessRate(t *testing.T) {
	s := NewHealthService(DefaultHealthServiceConfig(), nil)
	for i := 0; i < 10; i++ {
		s.RecordPoolResult("pool-1", true)
	}
	rate, ok := s.PoolSuccessRate("pool-1")
	if !ok || rate != 1.0 {
		t.Fatalf("PoolSuccessRate: got (%v, %v) want (1.0, true)", rate, ok)
	}
	for i := 0; i < 10; i++ {
		s.RecordPoolResult("pool-1", false)
	}
	rate, ok = s.PoolSuccessRate("pool-1")
	if !ok || rate != 0.5 {
		t.Fatalf("PoolSuccessRate: got (%v, %v) want (0.5, true)", rate, ok)
	}
}

func TestHealthService_PoolSuccessRateAbsent(t *testing.T) {
	s := NewHealthService(DefaultHealthServiceConfig(), nil)
	if _, ok := s.PoolSuccessRate("never-seen"); ok {
		t.Fatal("PoolSuccessRate: returned ok=true for unseen pool")
	}
}

func TestHealthService_ConsumerHealth(t *testing.T) {
	s := NewHealthService(DefaultHealthServiceConfig(), nil)
	s.SetConsumerRunning("c1", true)
	s.RecordConsumerPoll("c1")
	if !s.IsConsumerHealthy("c1") {
		t.Fatal("IsConsumerHealthy: want true after fresh poll on running consumer")
	}
	s.SetConsumerRunning("c1", false)
	if s.IsConsumerHealthy("c1") {
		t.Fatal("IsConsumerHealthy: want false after stop")
	}
}

func TestHealthService_StallDetection(t *testing.T) {
	cfg := DefaultHealthServiceConfig()
	cfg.ConsumerStallThreshold = 10 * time.Millisecond
	s := NewHealthService(cfg, nil)
	s.SetConsumerRunning("c1", true)
	s.RecordConsumerPoll("c1")
	time.Sleep(20 * time.Millisecond)
	stalled := s.StalledConsumers()
	if len(stalled) != 1 || stalled[0] != "c1" {
		t.Fatalf("StalledConsumers: got %+v want [c1]", stalled)
	}
}

func TestHealthService_HealthReport_Healthy(t *testing.T) {
	s := NewHealthService(DefaultHealthServiceConfig(), nil)
	s.SetConsumerRunning("c1", true)
	s.RecordConsumerPoll("c1")
	s.RecordPoolResult("p1", true)

	report := s.HealthReport([]PoolStats{{PoolCode: "p1"}})
	if report.Status != HealthHealthy {
		t.Fatalf("Status: got %v want Healthy (report=%+v)", report.Status, report)
	}
	if report.PoolsHealthy != 1 || report.PoolsUnhealthy != 0 {
		t.Fatalf("Pool counts: got healthy=%d unhealthy=%d want 1/0", report.PoolsHealthy, report.PoolsUnhealthy)
	}
}

func TestHealthService_HealthReport_DegradesOnCritical(t *testing.T) {
	ws := NewWarningService(WarningServiceConfig{})
	s := NewHealthService(DefaultHealthServiceConfig(), ws)
	s.SetConsumerRunning("c1", true)
	s.RecordConsumerPoll("c1")
	ws.Add(WarningCategoryConnection, WarningCritical, "oh no", "test")

	report := s.HealthReport(nil)
	if report.Status != HealthDegraded {
		t.Fatalf("Status: got %v want Degraded (report=%+v)", report.Status, report)
	}
	if report.CriticalWarnings != 1 {
		t.Fatalf("CriticalWarnings: got %d want 1", report.CriticalWarnings)
	}
}

func TestHealthService_HealthReport_WarnsOnCount(t *testing.T) {
	ws := NewWarningService(WarningServiceConfig{})
	cfg := DefaultHealthServiceConfig()
	cfg.MaxWarningsHealthy = 2
	cfg.MaxWarningsWarning = 5
	s := NewHealthService(cfg, ws)
	s.SetConsumerRunning("c1", true)
	s.RecordConsumerPoll("c1")

	for i := 0; i < 3; i++ {
		ws.Add(WarningCategoryConnection, WarningError, "x", "t")
	}
	if got := s.HealthReport(nil).Status; got != HealthWarning {
		t.Fatalf("3 warnings (>2 healthy): got %v want Warning", got)
	}
	for i := 0; i < 4; i++ {
		ws.Add(WarningCategoryConnection, WarningError, "x", "t")
	}
	if got := s.HealthReport(nil).Status; got != HealthDegraded {
		t.Fatalf("7 warnings (>5 warning): got %v want Degraded", got)
	}
}

func TestHealthService_RemoveStaleEntries(t *testing.T) {
	s := NewHealthService(DefaultHealthServiceConfig(), nil)
	s.RecordPoolResult("p1", true)
	s.RecordPoolResult("p2", true)
	s.SetConsumerRunning("c1", true)
	s.SetConsumerRunning("c2", true)

	s.RemoveStaleEntries([]string{"p1"}, []string{"c1"})
	if _, ok := s.PoolSuccessRate("p2"); ok {
		t.Fatal("RemoveStaleEntries: p2 should be gone")
	}
	if s.IsConsumerHealthy("c2") {
		t.Fatal("RemoveStaleEntries: c2 should be gone")
	}
}
