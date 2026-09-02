package router

import (
	"testing"
	"time"
)

// TestNotifierDefaultMinSeverityDropsInfo pins X-04: the webhook notifier
// defaults to a WARNING floor, so an INFO-severity warning is dropped before
// it is ever queued for delivery while WARNING+ still goes through.
func TestNotifierDefaultMinSeverityDropsInfo(t *testing.T) {
	n := NewNotifier("", 1000, time.Hour)
	n.Add(Warning{Category: WarningCategoryQueueHealth, Severity: WarningInfo, Message: "info", Source: "t"})
	n.Add(Warning{Category: WarningCategoryQueueHealth, Severity: WarningWarning, Message: "warn", Source: "t"})

	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.queue) != 1 {
		t.Fatalf("queue: got %d want 1 (INFO must be dropped by the default WARNING floor)", len(n.queue))
	}
	if n.queue[0].Severity != WarningWarning {
		t.Fatalf("queue[0].Severity: got %q want WARNING", n.queue[0].Severity)
	}
}

// TestNotifierSetMinSeverityOverride pins that the floor is configurable:
// explicitly lowering it to INFO lets INFO-severity warnings through too.
func TestNotifierSetMinSeverityOverride(t *testing.T) {
	n := NewNotifier("", 1000, time.Hour)
	n.SetMinSeverity(WarningInfo)
	n.Add(Warning{Category: WarningCategoryQueueHealth, Severity: WarningInfo, Message: "info", Source: "t"})

	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.queue) != 1 {
		t.Fatalf("queue: got %d want 1 (explicit INFO threshold must let INFO through)", len(n.queue))
	}
}

// TestNotifierSetMinSeverityCanRaiseAboveDefault pins the other direction:
// raising the floor (e.g. to ERROR) drops WARNING too, not just INFO.
func TestNotifierSetMinSeverityCanRaiseAboveDefault(t *testing.T) {
	n := NewNotifier("", 1000, time.Hour)
	n.SetMinSeverity(WarningError)
	n.Add(Warning{Category: WarningCategoryQueueHealth, Severity: WarningWarning, Message: "warn", Source: "t"})
	n.Add(Warning{Category: WarningCategoryQueueHealth, Severity: WarningError, Message: "err", Source: "t"})

	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.queue) != 1 {
		t.Fatalf("queue: got %d want 1 (WARNING must be dropped under an ERROR floor)", len(n.queue))
	}
	if n.queue[0].Severity != WarningError {
		t.Fatalf("queue[0].Severity: got %q want ERROR", n.queue[0].Severity)
	}
}

// TestParseWarningSeverity pins the FC_NOTIFY_MIN_SEVERITY parse guard: the
// four known levels parse case-insensitively (with surrounding whitespace
// trimmed), and everything else — including empty — reports false so the
// caller (Server.NewServer) leaves the Notifier's own WARNING default
// standing rather than silently reopening the INFO floodgate on a typo.
func TestParseWarningSeverity(t *testing.T) {
	cases := []struct {
		raw    string
		want   WarningSeverity
		wantOK bool
	}{
		{"INFO", WarningInfo, true},
		{"warning", WarningWarning, true},
		{"  Error  ", WarningError, true},
		{"CRITICAL", WarningCritical, true},
		{"", "", false},
		{"bogus", "", false},
		{"WARN", "", false}, // not one of the four literal levels
	}
	for _, tc := range cases {
		got, ok := parseWarningSeverity(tc.raw)
		if ok != tc.wantOK || (ok && got != tc.want) {
			t.Errorf("parseWarningSeverity(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.wantOK)
		}
	}
}

// TestNewServerAppliesNotifyMinSeverityFromConfig is the end-to-end wiring
// pin for FC_NOTIFY_MIN_SEVERITY (X-04, task 1a): a valid
// ServerConfig.NotifyMinSeverity is applied to the constructed Notifier, and
// an invalid one leaves the Notifier's own WARNING default in place rather
// than falling through to "unknown ranks lowest" and reopening INFO.
func TestNewServerAppliesNotifyMinSeverityFromConfig(t *testing.T) {
	s, err := NewServer(ServerConfig{NotifyMinSeverity: "info"})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	s.Notifier.Add(Warning{Category: WarningCategoryQueueHealth, Severity: WarningInfo, Message: "info", Source: "t"})
	s.Notifier.mu.Lock()
	n := len(s.Notifier.queue)
	s.Notifier.mu.Unlock()
	if n != 1 {
		t.Fatalf("queue: got %d want 1 (explicit info config must lower the floor)", n)
	}

	s2, err := NewServer(ServerConfig{NotifyMinSeverity: "not-a-real-severity"})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	s2.Notifier.Add(Warning{Category: WarningCategoryQueueHealth, Severity: WarningInfo, Message: "info", Source: "t"})
	s2.Notifier.mu.Lock()
	n2 := len(s2.Notifier.queue)
	s2.Notifier.mu.Unlock()
	if n2 != 0 {
		t.Fatalf("queue: got %d want 0 (an invalid config value must NOT lower the WARNING default)", n2)
	}
}

// TestWarningServiceForwardsToNotifierAndAppliesItsFloor is the task-1
// verification: WarningService.Add really does forward every add to its
// attached Notifier (SetNotifier), and the notifier's own severity floor
// (WARNING by default) is what keeps an INFO-severity warning out of the
// webhook while it still lands in the store.
func TestWarningServiceForwardsToNotifierAndAppliesItsFloor(t *testing.T) {
	ws := NewWarningService(WarningServiceConfig{})
	n := NewNotifier("", 1000, time.Hour)
	ws.SetNotifier(n)

	ws.Add(WarningCategoryQueueHealth, WarningInfo, "info", "t")
	ws.Add(WarningCategoryQueueHealth, WarningWarning, "warn", "t")

	if got := ws.Count(); got != 2 {
		t.Fatalf("WarningService.Count: got %d want 2 — both severities must be stored", got)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.queue) != 1 {
		t.Fatalf("notifier queue: got %d want 1 — only the WARNING should have been forwarded through", len(n.queue))
	}
	if n.queue[0].Severity != WarningWarning {
		t.Fatalf("notifier queue[0].Severity: got %q want WARNING", n.queue[0].Severity)
	}
}
