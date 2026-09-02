package router

import (
	"testing"
	"time"
)

func TestWarningService_AddAndGet(t *testing.T) {
	s := NewWarningService(WarningServiceConfig{})
	id := s.Add(WarningCategoryConnection, WarningError, "boom", "test")
	if id == "" {
		t.Fatal("expected non-empty id")
	}
	if got := s.Count(); got != 1 {
		t.Fatalf("Count: got %d want 1", got)
	}
	all := s.All()
	if len(all) != 1 || all[0].ID != id {
		t.Fatalf("All: got %+v want one entry with id %q", all, id)
	}
}

func TestWarningService_Acknowledge(t *testing.T) {
	s := NewWarningService(WarningServiceConfig{})
	id := s.Add(WarningCategoryConnection, WarningWarning, "stall", "test")

	if got := s.UnacknowledgedCount(); got != 1 {
		t.Fatalf("UnacknowledgedCount: got %d want 1", got)
	}
	if !s.Acknowledge(id) {
		t.Fatal("Acknowledge: returned false for existing id")
	}
	if got := s.UnacknowledgedCount(); got != 0 {
		t.Fatalf("UnacknowledgedCount after ack: got %d want 0", got)
	}
	if s.Acknowledge("does-not-exist") {
		t.Fatal("Acknowledge: returned true for missing id")
	}
}

func TestWarningService_FilterBySeverity(t *testing.T) {
	s := NewWarningService(WarningServiceConfig{})
	s.Add(WarningCategoryConnection, WarningWarning, "w", "t")
	s.Add(WarningCategoryConnection, WarningCritical, "c", "t")

	crit := s.BySeverity(WarningCritical)
	if len(crit) != 1 || crit[0].Message != "c" {
		t.Fatalf("BySeverity: got %+v want one CRITICAL", crit)
	}
	if got := s.CriticalCount(); got != 1 {
		t.Fatalf("CriticalCount: got %d want 1", got)
	}
}

func TestWarningService_EvictOnCapacity(t *testing.T) {
	// MaxWarnings=10 means eviction triggers when the 10th add lands at
	// capacity; eviction removes the oldest 10% (= 1) and then the
	// 10th add succeeds, leaving 10. Adding an 11th repeats the cycle.
	s := NewWarningService(WarningServiceConfig{MaxWarnings: 10})
	for i := 0; i < 15; i++ {
		s.Add(WarningCategoryConnection, WarningWarning, "msg", "t")
		// Sequential adds within the same nanosecond would race the
		// eviction sort key; sleep a hair so each entry has a distinct
		// created_at.
		time.Sleep(time.Millisecond)
	}
	if got := s.Count(); got > 10 {
		t.Fatalf("Count: got %d, expected <= 10 due to eviction", got)
	}
}

// TestWarningService_InfoAgesOutFasterThanWarning pins X-04's per-severity
// TTL: an INFO-severity warning must age out on the shorter InfoWarningAge
// (default 1h) rather than the general MaxWarningAge (default 8h), so a
// chatty INFO source cannot sit in the bounded store long enough to crowd
// out real WARNING+ entries.
func TestWarningService_InfoAgesOutFasterThanWarning(t *testing.T) {
	s := NewWarningService(WarningServiceConfig{
		MaxWarningAge:      8 * time.Hour,
		InfoWarningAge:     time.Hour,
		AutoAcknowledgeAge: 8 * time.Hour, // keep auto-ack out of the way of this test
	})
	infoID := s.Add(WarningCategoryQueueHealth, WarningInfo, "info", "t")
	warnID := s.Add(WarningCategoryQueueHealth, WarningWarning, "warn", "t")

	// Age both past the INFO threshold but well inside MaxWarningAge.
	s.mu.Lock()
	for _, id := range []string{infoID, warnID} {
		w := s.warnings[id]
		w.CreatedAt = time.Now().UTC().Add(-90 * time.Minute)
		s.warnings[id] = w
	}
	s.mu.Unlock()

	removed := s.ClearAged()
	if removed != 1 {
		t.Fatalf("ClearAged: removed %d want 1 (only the INFO entry)", removed)
	}
	if _, ok := s.warnings[infoID]; ok {
		t.Fatal("INFO warning past its 1h TTL must be gone")
	}
	if _, ok := s.warnings[warnID]; !ok {
		t.Fatal("WARNING warning of the same age must survive under the 8h MaxWarningAge")
	}
}

func TestWarningService_ActiveFiltersOnAcknowledgedAndAge(t *testing.T) {
	s := NewWarningService(WarningServiceConfig{})
	idAck := s.Add(WarningCategoryConnection, WarningError, "a", "t")
	s.Add(WarningCategoryConnection, WarningError, "b", "t")
	s.Acknowledge(idAck)

	active := s.Active(60) // 60-minute window
	if len(active) != 1 || active[0].Message != "b" {
		t.Fatalf("Active: got %+v want only 'b'", active)
	}
}
