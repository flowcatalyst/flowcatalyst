package router

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// LifecycleConfig tunes the cadence of background tasks.
type LifecycleConfig struct {
	// WarningCleanupInterval drives WarningService.Cleanup.
	WarningCleanupInterval time.Duration
	// HealthReportInterval logs the current HealthReport at this cadence.
	HealthReportInterval time.Duration
	// ConsumerHealthInterval drives HealthService.Cleanup (logs stalled
	// consumers, runs the warning cleanup as a side effect — but
	// dedicated WarningCleanupInterval ensures cleanup happens even when
	// HealthService.Cleanup runs less often). The consumer-restart watchdog
	// runs on this same tick.
	ConsumerHealthInterval time.Duration
	// ConsumerStallThreshold is how long a consumer poll loop may go without
	// completing a poll before the restart watchdog re-spawns it. Set to 60s
	// to match the HealthService stall threshold (DefaultHealthServiceConfig)
	// — a single, consistent notion of
	// "stalled" across restart + health reporting.
	ConsumerStallThreshold time.Duration
}

// ConsumerRestarter re-spawns consumer poll loops that have stalled. The
// router Manager implements it; LifecycleManager calls it on the
// consumer-health tick when set.
type ConsumerRestarter interface {
	RestartStalledConsumers(ctx context.Context, threshold time.Duration) int
}

// DefaultLifecycleConfig returns the standard defaults.
func DefaultLifecycleConfig() LifecycleConfig {
	return LifecycleConfig{
		WarningCleanupInterval: 5 * time.Minute,
		HealthReportInterval:   1 * time.Minute,
		ConsumerHealthInterval: 30 * time.Second,
		ConsumerStallThreshold: 60 * time.Second,
	}
}

// PoolStatsProvider yields the current pool stats snapshot. The router
// Manager will implement this once /monitoring/pools lands; the
// LifecycleManager skips the health-report logger when nil.
type PoolStatsProvider interface {
	PoolStats() []PoolStats
}

// LifecycleManager owns the background tasks that maintain
// WarningService + HealthService. Wraps both services so callers have
// a single shutdown handle and a single place to register optional
// hooks. The manager-coupled tasks (memory health monitor, consumer
// auto-restart, stale-entry reaper) are wired through the Manager and
// Server.
type LifecycleManager struct {
	cfg            LifecycleConfig
	warningService *WarningService
	healthService  *HealthService

	// saturatedPools remembers which pools have already been reported as
	// wedged, so the health tick reports each episode once instead of every
	// minute. Touched only by healthReportLoop (single goroutine).
	saturatedPools map[string]struct{}

	poolStatsProviderMu sync.RWMutex
	poolStatsProvider   PoolStatsProvider

	consumerRestarterMu sync.RWMutex
	consumerRestarter   ConsumerRestarter

	startOnce sync.Once
	stopOnce  sync.Once
	cancelFn  context.CancelFunc
	wg        sync.WaitGroup
}

// NewLifecycleManager builds a manager. Pass nil for either service to
// use a default. Call Start to spawn the background tasks; Shutdown to
// stop them.
func NewLifecycleManager(cfg LifecycleConfig, ws *WarningService, hs *HealthService) *LifecycleManager {
	if cfg.WarningCleanupInterval <= 0 {
		cfg.WarningCleanupInterval = 5 * time.Minute
	}
	if cfg.HealthReportInterval <= 0 {
		cfg.HealthReportInterval = 1 * time.Minute
	}
	if cfg.ConsumerHealthInterval <= 0 {
		cfg.ConsumerHealthInterval = 30 * time.Second
	}
	if cfg.ConsumerStallThreshold <= 0 {
		cfg.ConsumerStallThreshold = 90 * time.Second
	}
	if ws == nil {
		ws = NoopWarningService()
	}
	if hs == nil {
		hs = NewHealthService(DefaultHealthServiceConfig(), ws)
	}
	return &LifecycleManager{
		cfg:            cfg,
		warningService: ws,
		healthService:  hs,
		saturatedPools: make(map[string]struct{}),
	}
}

// WarningService returns the bound warning service.
func (l *LifecycleManager) WarningService() *WarningService { return l.warningService }

// HealthService returns the bound health service.
func (l *LifecycleManager) HealthService() *HealthService { return l.healthService }

// SetPoolStatsProvider wires the source of pool stats for the
// health-report logger. When nil, the logger emits a stats-less report
// (pool counts always zero) and otherwise behaves the same.
func (l *LifecycleManager) SetPoolStatsProvider(p PoolStatsProvider) {
	l.poolStatsProviderMu.Lock()
	l.poolStatsProvider = p
	l.poolStatsProviderMu.Unlock()
}

// SetConsumerRestarter wires the source of consumer-restart (the router
// Manager). When nil, the consumer-health loop only runs HealthService
// cleanup; when set, it also restarts stalled consumer poll loops.
func (l *LifecycleManager) SetConsumerRestarter(r ConsumerRestarter) {
	l.consumerRestarterMu.Lock()
	l.consumerRestarter = r
	l.consumerRestarterMu.Unlock()
}

// Start spawns the background tasks. Idempotent — only spawns on the
// first call (guarded by startOnce).
//
// Three goroutines are spawned, each owning one ticker-driven loop:
//   - warningCleanupLoop  — fires every cfg.WarningCleanupInterval.
//   - consumerHealthLoop  — fires every cfg.ConsumerHealthInterval.
//   - healthReportLoop    — fires every cfg.HealthReportInterval.
//
// Lifecycle contract: each goroutine returns when ctx is cancelled.
// Shutdown cancels that ctx and blocks on wg.Wait() — so spawning a
// loop here also obliges incrementing wg.Add(1) (already done) and
// calling wg.Done() on exit (done via defer inside the closure).
func (l *LifecycleManager) Start(parent context.Context) {
	l.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(parent)
		l.cancelFn = cancel

		l.wg.Add(1)
		go func() {
			defer l.wg.Done()
			l.warningCleanupLoop(ctx)
		}()
		l.wg.Add(1)
		go func() {
			defer l.wg.Done()
			l.consumerHealthLoop(ctx)
		}()
		l.wg.Add(1)
		go func() {
			defer l.wg.Done()
			l.healthReportLoop(ctx)
		}()

		slog.Info("router lifecycle manager started",
			"warning_cleanup", l.cfg.WarningCleanupInterval,
			"consumer_health", l.cfg.ConsumerHealthInterval,
			"health_report", l.cfg.HealthReportInterval)
	})
}

// Shutdown cancels every background task and blocks until they exit
// (or ctx is cancelled). Idempotent — only the first call cancels
// (guarded by stopOnce); subsequent calls still block on wg.
//
// The `done` channel is an unbuffered signalling channel. It is created
// fresh per call (Shutdown can be invoked multiple times concurrently),
// owned by THIS goroutine, closed exactly once by the inner goroutine
// after wg.Wait returns. Close-after-wait is the standard
// "wait-group → channel" bridge — gives us a select-able shutdown.
func (l *LifecycleManager) Shutdown(ctx context.Context) error {
	l.stopOnce.Do(func() {
		if l.cancelFn != nil {
			l.cancelFn()
		}
	})
	done := make(chan struct{})
	go func() { l.wg.Wait(); close(done) }()
	// Wakeup conditions:
	//   <-done       — all loops exited cleanly.
	//   <-ctx.Done() — caller's shutdown budget expired; abandon the wait
	//                  (loops may still be running — caller's choice).
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// warningCleanupLoop runs WarningService.Cleanup on a ticker. The
// ticker's channel (t.C) is owned by `time.Ticker` and stopped by the
// deferred t.Stop(); we never close it.
//
// Wakeup conditions:
//
//	<-ctx.Done() — shutdown; log and exit.
//	<-t.C        — interval elapsed; run Cleanup. NB: t.C is dropped if
//	               Cleanup overruns the interval — by design (no
//	               pile-up).
func (l *LifecycleManager) warningCleanupLoop(ctx context.Context) {
	t := time.NewTicker(l.cfg.WarningCleanupInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("warning cleanup loop stopped")
			return
		case <-t.C:
			l.warningService.Cleanup()
		}
	}
}

// consumerHealthLoop runs HealthService.Cleanup on a ticker.
//
// Wakeup conditions:
//
//	<-ctx.Done() — shutdown.
//	<-t.C        — interval elapsed; run Cleanup (also runs
//	               WarningService.Cleanup as a side effect).
func (l *LifecycleManager) consumerHealthLoop(ctx context.Context) {
	t := time.NewTicker(l.cfg.ConsumerHealthInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("consumer health loop stopped")
			return
		case <-t.C:
			l.healthService.Cleanup()
			l.consumerRestarterMu.RLock()
			r := l.consumerRestarter
			l.consumerRestarterMu.RUnlock()
			if r != nil {
				if n := r.RestartStalledConsumers(ctx, l.cfg.ConsumerStallThreshold); n > 0 {
					slog.Warn("restarted stalled consumers", "count", n)
				}
			}
		}
	}
}

// healthReportLoop logs the HealthReport on a ticker. Reads
// poolSaturationGrace is how long a pool may sit with NO free slots before it
// is reported. It is deliberately one full mediation attempt plus slack: a
// delivery cannot legitimately exceed DefaultMediatorConfig().Timeout, because
// that is the client timeout, so a pool still blocked past it is blocked by
// something the timeout was supposed to have ended. Below that, a saturated
// pool may simply be doing long but contractual work, and warning on it would
// be noise.
//
// The dashboard flags a saturated pool from 60s (amber) — the cheap, glanceable
// signal. This is the alertable one.
var poolSaturationGrace = DefaultMediatorConfig().Timeout + time.Minute

// reportSaturatedPools warns once per episode for any pool holding zero free
// slots with a delivery older than poolSaturationGrace.
//
// This is the condition that had no signal at all: a pool at 1/1 whose single
// slot has been held for half an hour is, at low concurrency, blocking every
// message group it owns — and nothing in /warnings, the health report or the
// dashboard said so. ActiveWorkers alone cannot show it, because a healthy busy
// pool looks identical.
func (l *LifecycleManager) reportSaturatedPools(stats []PoolStats) {
	live := make(map[string]struct{}, len(stats))
	for _, s := range stats {
		saturated := s.Concurrency > 0 && s.ActiveWorkers >= s.Concurrency &&
			time.Duration(s.OldestMediatingMs)*time.Millisecond > poolSaturationGrace
		if !saturated {
			continue
		}
		live[s.PoolCode] = struct{}{}
		if _, seen := l.saturatedPools[s.PoolCode]; seen {
			continue
		}
		l.saturatedPools[s.PoolCode] = struct{}{}
		msg := fmt.Sprintf(
			"Pool %s has had no free slots for %s (oldest delivery); every message group in it is blocked",
			s.PoolCode, (time.Duration(s.OldestMediatingMs) * time.Millisecond).Round(time.Second))
		slog.Warn("pool saturated with no free slots",
			"pool", s.PoolCode, "active_workers", s.ActiveWorkers, "concurrency", s.Concurrency,
			"oldest_mediating_ms", s.OldestMediatingMs)
		if l.warningService != nil {
			l.warningService.Add(WarningCategoryPoolCapacity, WarningError, msg, "router")
		}
	}
	// A pool that recovered can report again next time.
	for code := range l.saturatedPools {
		if _, ok := live[code]; !ok {
			delete(l.saturatedPools, code)
		}
	}
}

// poolStatsProvider under RLock — the field is swapped via
// SetPoolStatsProvider, so reads must be guarded.
//
// Wakeup conditions:
//
//	<-ctx.Done() — shutdown.
//	<-t.C        — interval elapsed; gather stats, emit log line.
func (l *LifecycleManager) healthReportLoop(ctx context.Context) {
	t := time.NewTicker(l.cfg.HealthReportInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("health report loop stopped")
			return
		case <-t.C:
			l.poolStatsProviderMu.RLock()
			provider := l.poolStatsProvider
			l.poolStatsProviderMu.RUnlock()
			var stats []PoolStats
			if provider != nil {
				stats = provider.PoolStats()
			}
			l.reportSaturatedPools(stats)
			report := l.healthService.HealthReport(stats)
			if len(report.Issues) > 0 {
				slog.Warn("router health report",
					"status", report.Status,
					"issues", report.Issues)
			} else {
				slog.Debug("router health report", "status", report.Status)
			}
		}
	}
}
