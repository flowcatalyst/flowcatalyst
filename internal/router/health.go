package router

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// HealthServiceConfig tunes thresholds.
type HealthServiceConfig struct {
	// HealthyThreshold is the minimum pool success rate (0..1) that
	// counts as healthy. Default 0.90.
	HealthyThreshold float64
	// WarningThreshold is the minimum success rate for the Warning band
	// (Below this → still counted as unhealthy). Default 0.70. Reserved
	// for future per-band rendering; current report just uses
	// HealthyThreshold to bucket healthy vs unhealthy.
	WarningThreshold float64
	// RollingWindow is the duration over which success rates are computed.
	// Default 30m.
	RollingWindow time.Duration
	// WarningAgeMinutes caps how old a warning can be and still be counted
	// as "active" for status calculation. Default 30.
	WarningAgeMinutes int64
	// ConsumerStallThreshold flags a consumer as stalled when it hasn't
	// polled in this duration. Default 60s.
	ConsumerStallThreshold time.Duration
	// MaxWarningsHealthy — degrade from Healthy → Warning above this.
	// Default 5.
	MaxWarningsHealthy uint32
	// MaxWarningsWarning — degrade from Warning → Degraded above this.
	// Default 20.
	MaxWarningsWarning uint32
}

// DefaultHealthServiceConfig returns the standard defaults.
func DefaultHealthServiceConfig() HealthServiceConfig {
	return HealthServiceConfig{
		HealthyThreshold:       0.90,
		WarningThreshold:       0.70,
		RollingWindow:          30 * time.Minute,
		WarningAgeMinutes:      30,
		ConsumerStallThreshold: 60 * time.Second,
		MaxWarningsHealthy:     5,
		MaxWarningsWarning:     20,
	}
}

// HealthService aggregates pool success rates + consumer liveness +
// warning counts into a HealthReport.
//
// Per-pool counters track outcome events in a rolling window (default
// 30 min). Expired events are evicted from the front of a slice on
// every record() — amortised O(1) per record because eviction only
// chews through the front prefix.
type HealthService struct {
	cfg            HealthServiceConfig
	warningService *WarningService

	mu           sync.RWMutex
	poolCounters map[string]*rollingCounter

	// consumers is the SOURCE of consumer liveness, not a copy of it.
	//
	// This used to be two maps here, written by SetConsumerRunning and
	// RecordConsumerPoll — which nothing but the tests ever called. So the
	// consumer half of every health answer was permanently empty: zero queues
	// on the dashboard, StalledConsumers always nil, /monitoring/consumer-health
	// always {}, and a wedged consumer invisible to the health report.
	//
	// The fix is not to start writing those maps. The Manager already owns this
	// data (membership of its consumer set, and the lastPoll its restart
	// watchdog runs on), so a copy here would be a second heartbeat that can
	// drift from the one the watchdog uses — and the two disagreeing about the
	// same consumer is a worse bug than the one being fixed. Read it instead,
	// exactly as HealthReport already takes pool stats from the pool manager.
	consumersMu sync.RWMutex
	consumers   ConsumerStatsProvider
}

// ConsumerStat is one consumer's liveness as the Manager sees it.
type ConsumerStat struct {
	QueueName string
	// Running reports that the consumer is in the manager's set — i.e. it is
	// meant to be polling. Whether it actually is, is what LastPoll says.
	Running bool
	// LastPoll is when it last completed a poll. Zero if it never has.
	LastPoll time.Time
}

// ConsumerStatsProvider yields the current consumer liveness snapshot. The
// Manager implements it.
type ConsumerStatsProvider interface {
	ConsumerStats() []ConsumerStat
}

// NewHealthService builds a service. Pass nil warningService to use a
// fresh NoopWarningService — useful for tests that don't care about
// warnings.
func NewHealthService(cfg HealthServiceConfig, ws *WarningService) *HealthService {
	if cfg.RollingWindow <= 0 {
		cfg = DefaultHealthServiceConfig()
	}
	if ws == nil {
		ws = NoopWarningService()
	}
	return &HealthService{
		cfg:            cfg,
		warningService: ws,
		poolCounters:   make(map[string]*rollingCounter),
	}
}

// SetConsumerStats wires the source of consumer liveness. Until it is set the
// consumer half of the health report reports nothing, which is what the whole
// surface did before it existed.
func (s *HealthService) SetConsumerStats(p ConsumerStatsProvider) {
	s.consumersMu.Lock()
	s.consumers = p
	s.consumersMu.Unlock()
}

// consumerStats returns the current snapshot, or nil when no provider is set.
func (s *HealthService) consumerStats() []ConsumerStat {
	s.consumersMu.RLock()
	p := s.consumers
	s.consumersMu.RUnlock()
	if p == nil {
		return nil
	}
	return p.ConsumerStats()
}

// healthy reports whether a consumer counts as alive: in the manager's set,
// having polled, and having polled recently enough.
func (s *HealthService) healthy(c ConsumerStat) bool {
	return c.Running && !c.LastPoll.IsZero() && time.Since(c.LastPoll) < s.cfg.ConsumerStallThreshold
}

// RecordPoolResult ticks the rolling counter for the named pool.
func (s *HealthService) RecordPoolResult(poolCode string, success bool) {
	s.mu.Lock()
	c, ok := s.poolCounters[poolCode]
	if !ok {
		c = newRollingCounter(s.cfg.RollingWindow)
		s.poolCounters[poolCode] = c
	}
	s.mu.Unlock()
	c.record(success)
}

// PoolSuccessRate returns the rolling success rate (0..1) for a pool,
// or false if no events have been recorded yet within the window.
func (s *HealthService) PoolSuccessRate(poolCode string) (float64, bool) {
	s.mu.RLock()
	c, ok := s.poolCounters[poolCode]
	s.mu.RUnlock()
	if !ok {
		return 0, false
	}
	return c.successRate()
}

// IsConsumerHealthy reports whether the named consumer is running AND has
// polled within ConsumerStallThreshold.
func (s *HealthService) IsConsumerHealthy(queueName string) bool {
	for _, c := range s.consumerStats() {
		if c.QueueName == queueName {
			return s.healthy(c)
		}
	}
	return false
}

// ConsumerHealth returns the per-consumer snapshot.
func (s *HealthService) ConsumerHealth(queueName string) ConsumerHealth {
	for _, c := range s.consumerStats() {
		if c.QueueName != queueName {
			continue
		}
		out := ConsumerHealth{QueueIdentifier: c.QueueName, IsRunning: c.Running, IsHealthy: s.healthy(c)}
		if !c.LastPoll.IsZero() {
			ms := time.Since(c.LastPoll).Milliseconds()
			out.LastPollTimeMs, out.TimeSinceLastPollMs = &ms, &ms
		}
		return out
	}
	return ConsumerHealth{QueueIdentifier: queueName}
}

// StalledConsumers returns the queue names of consumers that are meant to be
// polling but either never have or have not within ConsumerStallThreshold.
func (s *HealthService) StalledConsumers() []string {
	var out []string
	for _, c := range s.consumerStats() {
		if c.Running && !s.healthy(c) {
			out = append(out, c.QueueName)
		}
	}
	return out
}

// HealthReport assembles the overall verdict. Pass the current pool stats
// snapshot — HealthService doesn't own that data, the pool manager does.
func (s *HealthService) HealthReport(poolStats []PoolStats) HealthReport {
	issues := []string{}

	var poolsHealthy, poolsUnhealthy uint32
	for _, st := range poolStats {
		if rate, ok := s.PoolSuccessRate(st.PoolCode); ok {
			if rate >= s.cfg.HealthyThreshold {
				poolsHealthy++
			} else {
				poolsUnhealthy++
				issues = append(issues, fmt.Sprintf("Pool %s success rate: %.1f%%", st.PoolCode, rate*100))
			}
		} else {
			// No data yet → treat as healthy.
			poolsHealthy++
		}
	}

	consumersTotal := uint32(len(s.consumerStats()))
	stalled := s.StalledConsumers()
	consumersUnhealthy := uint32(len(stalled))
	consumersHealthy := consumersTotal
	if consumersHealthy >= consumersUnhealthy {
		consumersHealthy -= consumersUnhealthy
	} else {
		consumersHealthy = 0
	}
	for _, id := range stalled {
		issues = append(issues, fmt.Sprintf("Consumer %s is stalled", id))
	}

	activeWarnings := uint32(len(s.warningService.Active(s.cfg.WarningAgeMinutes)))
	criticalWarnings := uint32(s.warningService.CriticalCount())
	if criticalWarnings > 0 {
		issues = append(issues, fmt.Sprintf("%d critical warnings", criticalWarnings))
	}
	// The warning COUNT is a degradation cause in its own right (see the switch
	// below) and used to append no issue at all, so a router degraded purely by
	// warning volume reported an empty reason and the dashboard showed "No
	// details available" — the one degradation cause that explained nothing.
	if activeWarnings > s.cfg.MaxWarningsHealthy {
		issues = append(issues, fmt.Sprintf("%d active warnings (degrades above %d)",
			activeWarnings, s.cfg.MaxWarningsWarning))
	}

	status := HealthHealthy
	switch {
	case criticalWarnings > 0,
		poolsUnhealthy > 0 && poolsHealthy == 0,
		consumersUnhealthy > 0 && consumersHealthy == 0,
		activeWarnings > s.cfg.MaxWarningsWarning:
		status = HealthDegraded
	case poolsUnhealthy > 0,
		consumersUnhealthy > 0,
		activeWarnings > s.cfg.MaxWarningsHealthy:
		status = HealthWarning
	}

	if status != HealthHealthy {
		slog.Debug("health report",
			"status", status,
			"poolsHealthy", poolsHealthy,
			"poolsUnhealthy", poolsUnhealthy,
			"consumersHealthy", consumersHealthy,
			"consumersUnhealthy", consumersUnhealthy,
			"activeWarnings", activeWarnings,
			"criticalWarnings", criticalWarnings)
	}

	return HealthReport{
		Status:             status,
		PoolsHealthy:       poolsHealthy,
		PoolsUnhealthy:     poolsUnhealthy,
		ConsumersHealthy:   consumersHealthy,
		ConsumersUnhealthy: consumersUnhealthy,
		ActiveWarnings:     activeWarnings,
		CriticalWarnings:   criticalWarnings,
		Issues:             issues,
	}
}

// IsHealthy is a shortcut for HealthReport(stats).Status == Healthy.
func (s *HealthService) IsHealthy(poolStats []PoolStats) bool {
	return s.HealthReport(poolStats).Status == HealthHealthy
}

// Cleanup runs the warning-service cleanup + logs any stalled consumers.
// Wire it onto a ticker; the LifecycleManager owns the period.
func (s *HealthService) Cleanup() {
	s.warningService.Cleanup()
	if stalled := s.StalledConsumers(); len(stalled) > 0 {
		slog.Warn("detected stalled consumers", "count", len(stalled), "consumers", stalled)
	}
}

// RunCleanupLoop drives Cleanup on a ticker until ctx is cancelled.
func (s *HealthService) RunCleanupLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 1 * time.Minute
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

// RemoveStaleEntries drops pool counters + consumer entries for ids no
// longer present in the supplied active sets. Call after a config
// reload so stale entries don't accumulate forever.
func (s *HealthService) RemoveStaleEntries(activePoolCodes, activeConsumerIDs []string) {
	poolSet := stringSet(activePoolCodes)
	consumerSet := stringSet(activeConsumerIDs)

	s.mu.Lock()
	defer s.mu.Unlock()

	for code := range s.poolCounters {
		if _, ok := poolSet[code]; !ok {
			delete(s.poolCounters, code)
		}
	}
	// Consumer entries need no pruning: they are read live from the Manager,
	// so a removed consumer simply stops appearing.
	_ = consumerSet
}

func stringSet(xs []string) map[string]struct{} {
	out := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		out[x] = struct{}{}
	}
	return out
}

// ── rolling counter ──────────────────────────────────────────────────────

// rollingCounter is a bounded-window success-rate counter. Events are
// recorded with their timestamps; expired entries are popped from the
// front on each record (amortised O(1)).
type rollingCounter struct {
	window time.Duration

	mu     sync.Mutex
	events []rcEvent
}

type rcEvent struct {
	at      time.Time
	success bool
}

func newRollingCounter(window time.Duration) *rollingCounter {
	return &rollingCounter{window: window}
}

func (c *rollingCounter) record(success bool) {
	now := time.Now()
	cutoff := now.Add(-c.window)
	c.mu.Lock()
	defer c.mu.Unlock()
	// Drop expired front entries.
	i := 0
	for i < len(c.events) && !c.events[i].at.After(cutoff) {
		i++
	}
	if i > 0 {
		c.events = c.events[i:]
	}
	c.events = append(c.events, rcEvent{at: now, success: success})
}

func (c *rollingCounter) successRate() (float64, bool) {
	cutoff := time.Now().Add(-c.window)
	c.mu.Lock()
	defer c.mu.Unlock()
	var total, successes int
	for _, e := range c.events {
		if !e.at.After(cutoff) {
			continue
		}
		total++
		if e.success {
			successes++
		}
	}
	if total == 0 {
		return 0, false
	}
	return float64(successes) / float64(total), true
}
