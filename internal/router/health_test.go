package router

import (
	"strings"
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

// fakeConsumerStats stands in for the Manager, which owns consumer liveness.
type fakeConsumerStats struct{ stats []ConsumerStat }

func (f *fakeConsumerStats) ConsumerStats() []ConsumerStat { return f.stats }

func healthWithConsumers(cfg HealthServiceConfig, ws *WarningService, stats ...ConsumerStat) *HealthService {
	s := NewHealthService(cfg, ws)
	s.SetConsumerStats(&fakeConsumerStats{stats: stats})
	return s
}

func polling(name string) ConsumerStat {
	return ConsumerStat{QueueName: name, Running: true, LastPoll: time.Now()}
}

func TestHealthService_ConsumerHealth(t *testing.T) {
	s := healthWithConsumers(DefaultHealthServiceConfig(), nil, polling("c1"))
	if !s.IsConsumerHealthy("c1") {
		t.Fatal("IsConsumerHealthy: want true after fresh poll on running consumer")
	}
	// Gone from the manager's set entirely.
	s.SetConsumerStats(&fakeConsumerStats{})
	if s.IsConsumerHealthy("c1") {
		t.Fatal("IsConsumerHealthy: want false once the consumer is no longer running")
	}
}

// A consumer that has never completed a poll is stalled, not healthy — that is
// the state a rebuilt-but-wedged consumer sits in.
func TestHealthService_ConsumerNeverPolledIsStalled(t *testing.T) {
	s := healthWithConsumers(DefaultHealthServiceConfig(), nil,
		ConsumerStat{QueueName: "c1", Running: true})
	if s.IsConsumerHealthy("c1") {
		t.Fatal("a consumer that has never polled must not be healthy")
	}
	if got := s.StalledConsumers(); len(got) != 1 || got[0] != "c1" {
		t.Fatalf("StalledConsumers: got %+v want [c1]", got)
	}
}

func TestHealthService_StallDetection(t *testing.T) {
	cfg := DefaultHealthServiceConfig()
	cfg.ConsumerStallThreshold = 10 * time.Millisecond
	s := healthWithConsumers(cfg, nil, polling("c1"))
	time.Sleep(20 * time.Millisecond)
	stalled := s.StalledConsumers()
	if len(stalled) != 1 || stalled[0] != "c1" {
		t.Fatalf("StalledConsumers: got %+v want [c1]", stalled)
	}
}

func TestHealthService_HealthReport_Healthy(t *testing.T) {
	s := healthWithConsumers(DefaultHealthServiceConfig(), nil, polling("c1"))
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
	s := healthWithConsumers(DefaultHealthServiceConfig(), ws, polling("c1"))
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
	s := healthWithConsumers(cfg, ws, polling("c1"))

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

	s.RemoveStaleEntries([]string{"p1"}, []string{"c1"})
	if _, ok := s.PoolSuccessRate("p2"); ok {
		t.Fatal("RemoveStaleEntries: p2 should be gone")
	}
	// Consumers need no pruning — they are read live from the Manager, so one
	// that is gone from its set simply stops appearing.
	if s.IsConsumerHealthy("c2") {
		t.Fatal("a consumer absent from the provider must not read as healthy")
	}
}

// TestHealthReportExplainsAWarningCountDegradation pins the reporting gap that
// made a DEGRADED router say nothing about why. HealthReport appended issues
// for unhealthy pools, stalled consumers and CRITICAL warnings — but the
// warning-COUNT branch, which is a degradation cause in its own right,
// appended none. A router degraded purely by warning volume therefore
// reported an empty reason and the dashboard printed "No details available",
// leaving the one cause that explains nothing as the hardest to diagnose.
func TestHealthReportExplainsAWarningCountDegradation(t *testing.T) {
	ws := NewWarningService(WarningServiceConfig{})
	cfg := DefaultHealthServiceConfig()
	cfg.MaxWarningsHealthy = 2
	cfg.MaxWarningsWarning = 5
	s := healthWithConsumers(cfg, ws, polling("c1"))
	for range 7 {
		ws.Add(WarningCategoryConnection, WarningError, "x", "t")
	}

	report := s.HealthReport(nil)
	if report.Status != HealthDegraded {
		t.Fatalf("Status: got %v want Degraded", report.Status)
	}
	if len(report.Issues) == 0 {
		t.Fatal("a degraded report must say why; this is the empty-reason bug")
	}
	joined := strings.Join(report.Issues, "; ")
	if !strings.Contains(joined, "7 active warnings") {
		t.Fatalf("issues %q must name the warning count", joined)
	}
}

// Consumer health reaches the report for the first time here: it used to be
// read from maps nothing wrote, so every count was zero and a wedged consumer
// was invisible. Note the degrade rule — ALL consumers stalled, not one — is
// what keeps /health/ready (503 on DEGRADED) from pulling a router out of
// service over a single bad queue.
func TestHealthReportCountsConsumersFromTheProvider(t *testing.T) {
	stale := ConsumerStat{QueueName: "wedged", Running: true, LastPoll: time.Now().Add(-time.Hour)}

	s := healthWithConsumers(DefaultHealthServiceConfig(), nil, polling("ok"), stale)
	report := s.HealthReport(nil)
	if report.ConsumersHealthy != 1 || report.ConsumersUnhealthy != 1 {
		t.Fatalf("counts: healthy=%d unhealthy=%d want 1/1",
			report.ConsumersHealthy, report.ConsumersUnhealthy)
	}
	if report.Status != HealthWarning {
		t.Fatalf("one stalled consumer of two: got %v want Warning", report.Status)
	}
	if !strings.Contains(strings.Join(report.Issues, "; "), "wedged") {
		t.Fatalf("issues %v must name the stalled consumer", report.Issues)
	}

	// Every consumer stalled: the router genuinely cannot work, so it degrades
	// and readiness starts failing.
	all := healthWithConsumers(DefaultHealthServiceConfig(), nil, stale)
	if got := all.HealthReport(nil).Status; got != HealthDegraded {
		t.Fatalf("all consumers stalled: got %v want Degraded", got)
	}
}

// The Manager is the source, so what it reports must match what the restart
// watchdog judges consumers by — the same lastPoll, not a second copy.
func TestManagerConsumerStatsMirrorsTheWatchdogHeartbeat(t *testing.T) {
	m := NewManager(nil, nil)
	rc := &runningConsumer{consumer: &pollErrConsumer{id: "q"}, cancel: func() {}}
	beat := time.Now().Add(-90 * time.Second)
	rc.lastPoll.Store(beat.UnixNano())
	m.consumers["q"] = rc

	stats := m.ConsumerStats()
	if len(stats) != 1 || stats[0].QueueName != "q" || !stats[0].Running {
		t.Fatalf("ConsumerStats: got %+v", stats)
	}
	if !stats[0].LastPoll.Equal(beat) {
		t.Fatalf("LastPoll = %v, want the watchdog's own heartbeat %v", stats[0].LastPoll, beat)
	}

	// A consumer that has never polled reports a zero time, not "now" — the
	// difference between "just built" and "healthy".
	fresh := &runningConsumer{consumer: &pollErrConsumer{id: "n"}, cancel: func() {}}
	m.consumers["n"] = fresh
	for _, c := range m.ConsumerStats() {
		if c.QueueName == "n" && !c.LastPoll.IsZero() {
			t.Fatalf("never-polled consumer reported LastPoll %v", c.LastPoll)
		}
	}
}
