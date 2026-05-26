package router

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// WarningServiceConfig tunes retention + acknowledgement behaviour.
// Defaults mirror the Rust `WarningServiceConfig::default()`.
type WarningServiceConfig struct {
	// MaxWarningAge auto-clears any warning older than this. Java + Rust
	// default: 8 hours.
	MaxWarningAge time.Duration
	// MaxWarnings caps the in-memory set; when full, the oldest 10% are
	// evicted by created_at order.
	MaxWarnings int
	// AutoAcknowledgeAge auto-acks any warning older than this. Defaults
	// to MaxWarningAge so cleanup() naturally hides stale warnings before
	// dropping them.
	AutoAcknowledgeAge time.Duration
}

// DefaultWarningServiceConfig returns the Rust defaults.
func DefaultWarningServiceConfig() WarningServiceConfig {
	return WarningServiceConfig{
		MaxWarningAge:      8 * time.Hour,
		MaxWarnings:        1000,
		AutoAcknowledgeAge: 8 * time.Hour,
	}
}

// WarningService is the in-memory warning store. Mirrors
// `crates/fc-router/src/warning.rs::WarningService`.
//
// The store is bounded (MaxWarnings) and self-cleaning (cleanup()
// auto-acks aged warnings + drops very old ones). If a Notifier is
// attached, every add fires off a non-blocking webhook send.
//
// Designed to be cheap to read concurrently (RWMutex) and to keep
// add() bounded by O(MaxWarnings) on overflow (the eviction sort
// runs only on overflow, not every add).
type WarningService struct {
	cfg WarningServiceConfig

	mu       sync.RWMutex
	warnings map[string]Warning

	notifyMu sync.RWMutex
	notifier *Notifier
}

// NewWarningService builds a service. Pass a zero-value Config to use defaults.
func NewWarningService(cfg WarningServiceConfig) *WarningService {
	if cfg.MaxWarningAge <= 0 {
		cfg.MaxWarningAge = 8 * time.Hour
	}
	if cfg.MaxWarnings <= 0 {
		cfg.MaxWarnings = 1000
	}
	if cfg.AutoAcknowledgeAge <= 0 {
		cfg.AutoAcknowledgeAge = cfg.MaxWarningAge
	}
	return &WarningService{
		cfg:      cfg,
		warnings: make(map[string]Warning),
	}
}

// NoopWarningService returns a service with default config. Used as the
// default when no explicit service is wired.
func NoopWarningService() *WarningService { return NewWarningService(WarningServiceConfig{}) }

// SetNotifier attaches a Notifier; subsequent Add calls also enqueue a
// webhook send. Pass nil to detach.
func (s *WarningService) SetNotifier(n *Notifier) {
	s.notifyMu.Lock()
	defer s.notifyMu.Unlock()
	s.notifier = n
}

// Add records a new warning and returns its id. Forwards to the
// attached notifier (if any). Evicts the oldest 10% if the store is at
// capacity.
func (s *WarningService) Add(category WarningCategory, severity WarningSeverity, message, source string) string {
	w := NewWarning(category, severity, message, source)

	s.mu.Lock()
	if len(s.warnings) >= s.cfg.MaxWarnings {
		s.evictOldestLocked()
	}
	s.warnings[w.ID] = w
	s.mu.Unlock()

	s.notifyMu.RLock()
	n := s.notifier
	s.notifyMu.RUnlock()
	if n != nil {
		n.Add(w)
	}
	return w.ID
}

// All returns a snapshot of every stored warning, in undefined order.
func (s *WarningService) All() []Warning {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Warning, 0, len(s.warnings))
	for _, w := range s.warnings {
		out = append(out, w)
	}
	return out
}

// BySeverity returns warnings with the given severity.
func (s *WarningService) BySeverity(severity WarningSeverity) []Warning {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Warning
	for _, w := range s.warnings {
		if w.Severity == severity {
			out = append(out, w)
		}
	}
	return out
}

// ByCategory returns warnings with the given category.
func (s *WarningService) ByCategory(category WarningCategory) []Warning {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Warning
	for _, w := range s.warnings {
		if w.Category == category {
			out = append(out, w)
		}
	}
	return out
}

// Unacknowledged returns every warning whose Acknowledged flag is false.
func (s *WarningService) Unacknowledged() []Warning {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Warning
	for _, w := range s.warnings {
		if !w.Acknowledged {
			out = append(out, w)
		}
	}
	return out
}

// Active returns every unacknowledged warning younger than maxAgeMinutes.
// Used by HealthService for its warning-count thresholds.
func (s *WarningService) Active(maxAgeMinutes int64) []Warning {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Warning
	for _, w := range s.warnings {
		if !w.Acknowledged && w.AgeMinutes() <= maxAgeMinutes {
			out = append(out, w)
		}
	}
	return out
}

// Critical returns warnings with Critical severity (acknowledged or not).
func (s *WarningService) Critical() []Warning { return s.BySeverity(WarningCritical) }

// Acknowledge flips a single warning. Returns false if no warning has that id.
func (s *WarningService) Acknowledge(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.warnings[id]
	if !ok {
		return false
	}
	now := time.Now().UTC()
	w.Acknowledged = true
	w.AcknowledgedAt = &now
	s.warnings[id] = w
	return true
}

// AcknowledgeMatching acks every unacknowledged warning where predicate
// returns true. Returns the count acknowledged.
func (s *WarningService) AcknowledgeMatching(predicate func(Warning) bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	count := 0
	for id, w := range s.warnings {
		if !w.Acknowledged && predicate(w) {
			w.Acknowledged = true
			w.AcknowledgedAt = &now
			s.warnings[id] = w
			count++
		}
	}
	return count
}

// AutoAcknowledgeOld acks any warning older than the configured threshold.
func (s *WarningService) AutoAcknowledgeOld() int {
	limit := int64(s.cfg.AutoAcknowledgeAge.Minutes())
	return s.AcknowledgeMatching(func(w Warning) bool { return w.AgeMinutes() > limit })
}

// ClearOlderThan removes every warning older than `age`. Returns removed count.
func (s *WarningService) ClearOlderThan(age time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	limit := int64(age.Minutes())
	removed := 0
	for id, w := range s.warnings {
		if w.AgeMinutes() > limit {
			delete(s.warnings, id)
			removed++
		}
	}
	if removed > 0 {
		slog.Info("warning service: cleared old warnings", "removed", removed)
	}
	return removed
}

// ClearAcknowledged drops every acknowledged warning.
func (s *WarningService) ClearAcknowledged() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for id, w := range s.warnings {
		if w.Acknowledged {
			delete(s.warnings, id)
			removed++
		}
	}
	return removed
}

// Remove drops a warning by id. Returns true if it was present.
func (s *WarningService) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.warnings[id]; !ok {
		return false
	}
	delete(s.warnings, id)
	return true
}

// Count returns the total number of stored warnings.
func (s *WarningService) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.warnings)
}

// UnacknowledgedCount returns the count of unacknowledged warnings.
func (s *WarningService) UnacknowledgedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, w := range s.warnings {
		if !w.Acknowledged {
			n++
		}
	}
	return n
}

// CriticalCount returns the count of unacknowledged critical warnings.
// HealthService treats any non-zero result as a Degraded trigger.
func (s *WarningService) CriticalCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, w := range s.warnings {
		if w.Severity == WarningCritical && !w.Acknowledged {
			n++
		}
	}
	return n
}

// HasCritical reports whether any unacknowledged critical warning exists.
func (s *WarningService) HasCritical() bool { return s.CriticalCount() > 0 }

// Cleanup auto-acks old warnings and drops very old ones. Idempotent;
// call from a periodic ticker (LifecycleManager or a dedicated goroutine).
func (s *WarningService) Cleanup() {
	s.AutoAcknowledgeOld()
	s.ClearOlderThan(s.cfg.MaxWarningAge)
}

// RunCleanupLoop drives Cleanup on a ticker until ctx is cancelled.
// Convenience wrapper for callers that want the service self-maintaining.
func (s *WarningService) RunCleanupLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Cleanup()
		}
	}
}

// evictOldestLocked removes the oldest 10% of stored warnings. Caller
// must hold s.mu (write).
func (s *WarningService) evictOldestLocked() {
	toRemove := len(s.warnings) / 10
	if toRemove == 0 {
		return
	}
	type kv struct {
		id string
		at time.Time
	}
	all := make([]kv, 0, len(s.warnings))
	for id, w := range s.warnings {
		all = append(all, kv{id: id, at: w.CreatedAt})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].at.Before(all[j].at) })
	for i := 0; i < toRemove; i++ {
		delete(s.warnings, all[i].id)
	}
}
