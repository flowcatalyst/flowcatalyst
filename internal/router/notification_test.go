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
