package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
)

// Version is the FlowCatalyst release reported by /monitoring. Override
// at link time with `-ldflags "-X .../api.Version=0.x.y"` once the
// release pipeline exists; until then the binary self-reports "dev".
var Version = "dev"

// startTime is captured at first call to RegisterRoutes so the
// `uptimeMillis` field on /monitoring/health is process-relative.
// Lazy init so package init order doesn't matter.
var startTime time.Time

// PoolStatsProvider is the source of pool stats for /monitoring/pools.
// The router Manager will implement this once the metrics + per-pool
// stats roll-up lands; for now the handler tolerates nil and returns
// an empty slice (matches Rust when no pools are registered).
type PoolStatsProvider interface {
	PoolStats() []router.PoolStats
}

// CircuitBreakerOpenCounter reports the count of currently-open breakers.
// Optional — when nil the dashboard health endpoint reports 0.
type CircuitBreakerOpenCounter interface {
	OpenCount() int
}

// Deps bundles the services every handler needs. Callers (cmd/fc-router,
// fc-server.StartRouter) construct it from their *router.Server.
type Deps struct {
	Warnings       *router.WarningService
	Health         *router.HealthService
	PoolStats      PoolStatsProvider          // optional
	BreakersCount  CircuitBreakerOpenCounter  // optional
}

// FromServer is the convenience constructor. Adapts *router.Server's
// fields into a Deps. Manager doesn't yet expose PoolStats() — passed
// as nil here until the Manager grows that surface (HANDOFF.md §7).
func FromServer(s *router.Server) Deps {
	return Deps{
		Warnings:      s.Warnings,
		Health:        s.Health,
		PoolStats:     nil, // TODO(manager-surface): wire s.Manager.PoolStats()
		BreakersCount: breakersAdapter{breakers: s.Breakers},
	}
}

// breakersAdapter counts open breakers from the registry snapshot.
type breakersAdapter struct {
	breakers *router.BreakerRegistry
}

func (a breakersAdapter) OpenCount() int {
	if a.breakers == nil {
		return 0
	}
	n := 0
	for _, s := range a.breakers.Snapshot() {
		if s.State == router.CircuitOpen {
			n++
		}
	}
	return n
}

// RegisterRoutes mounts the router's HTTP API on r. Safe to call once
// per process — the start-time stamp is process-global.
//
// Routes mounted (matches the high-value subset of
// crates/fc-router/src/api/mod.rs; Prometheus /metrics, test/benchmark
// endpoints, and HTML dashboard are deferred):
//
//	Health probes:
//	  GET  /health                    legacy + dashboard summary
//	  GET  /q/health                  k8s alias
//	  GET  /health/live               liveness
//	  GET  /health/ready              readiness (Health degraded → 503)
//
//	Monitoring reads:
//	  GET  /monitoring                MonitoringResponse (status/version/health/pools)
//	  GET  /monitoring/health         DashboardHealthResponse (Java-shape)
//	  GET  /monitoring/pools          []PoolStats
//	  GET  /monitoring/warnings       []Warning (active only, sorted newest-first)
//	  GET  /monitoring/consumer-health per-consumer health map
//
//	Warnings management:
//	  GET    /warnings                       list (?severity=, ?category=, ?acknowledged=)
//	  DELETE /warnings                       clear all
//	  POST   /warnings/{id}/acknowledge      ack one
//	  POST   /warnings/acknowledge-all       ack every unacked
//	  GET    /warnings/critical              filter shortcut
//	  GET    /warnings/unacknowledged        filter shortcut
//	  DELETE /warnings/old                   purge older than ?hours= (default 8)
//
//	Config:
//	  GET  /api/config                snapshot of current pool config (no secrets)
func RegisterRoutes(r chi.Router, d Deps) {
	if startTime.IsZero() {
		startTime = time.Now()
	}

	// Health probes.
	r.Get("/health", handleHealth(d))
	r.Get("/q/health", handleHealth(d))
	r.Get("/health/live", handleLiveness())
	r.Get("/health/ready", handleReadiness(d))

	// Monitoring reads.
	r.Get("/monitoring", handleMonitoring(d))
	r.Get("/monitoring/health", handleDashboardHealth(d))
	r.Get("/monitoring/pools", handlePoolStats(d))
	r.Get("/monitoring/warnings", handleMonitoringWarnings(d))
	r.Get("/monitoring/consumer-health", handleConsumerHealth(d))

	// Warnings management.
	r.Get("/warnings", handleListWarnings(d))
	r.Delete("/warnings", handleClearAllWarnings(d))
	r.Post("/warnings/{id}/acknowledge", handleAcknowledge(d))
	r.Post("/warnings/acknowledge-all", handleAcknowledgeAll(d))
	r.Get("/warnings/critical", handleCriticalWarnings(d))
	r.Get("/warnings/unacknowledged", handleUnacknowledgedWarnings(d))
	r.Delete("/warnings/old", handleClearOldWarnings(d))

	// Config read.
	r.Get("/api/config", handleConfig(d))
}

// ── helpers ──────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func poolStatsSnapshot(d Deps) []router.PoolStats {
	if d.PoolStats == nil {
		return nil
	}
	return d.PoolStats.PoolStats()
}

func statusString(s router.HealthStatus) string {
	switch s {
	case router.HealthHealthy:
		return "HEALTHY"
	case router.HealthWarning:
		return "WARNING"
	case router.HealthDegraded:
		return "DEGRADED"
	default:
		return "UNKNOWN"
	}
}

// ── Health probes ────────────────────────────────────────────────────────

// handleHealth is the legacy /health endpoint kept for backward
// compatibility — emits a compact JSON summary that pre-dates the
// liveness/readiness split.
func handleHealth(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		report := d.Health.HealthReport(poolStatsSnapshot(d))
		writeJSON(w, http.StatusOK, map[string]any{
			"status":            statusString(report.Status),
			"version":           Version,
			"active_warnings":   report.ActiveWarnings,
			"critical_warnings": report.CriticalWarnings,
		})
	}
}

func handleLiveness() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, probeResponse{Status: "LIVE"})
	}
}

// handleReadiness mirrors Rust's: report Status==Degraded → 503, else 200.
// Broker-connectivity check is deferred (needs Manager surface).
func handleReadiness(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		report := d.Health.HealthReport(poolStatsSnapshot(d))
		if report.Status == router.HealthDegraded {
			writeJSON(w, http.StatusServiceUnavailable, probeResponse{Status: "NOT_READY"})
			return
		}
		writeJSON(w, http.StatusOK, probeResponse{Status: "READY"})
	}
}

// ── Monitoring reads ─────────────────────────────────────────────────────

func handleMonitoring(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		stats := poolStatsSnapshot(d)
		report := d.Health.HealthReport(stats)
		writeJSON(w, http.StatusOK, monitoringResponse{
			Status:           statusString(report.Status),
			Version:          Version,
			HealthReport:     fromHealthReport(report),
			PoolStats:        fromPoolStats(stats),
			ActiveWarnings:   uint32(d.Warnings.UnacknowledgedCount()),
			CriticalWarnings: uint32(d.Warnings.CriticalCount()),
		})
	}
}

func handleDashboardHealth(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		stats := poolStatsSnapshot(d)
		report := d.Health.HealthReport(stats)
		var degradationReason *string
		if len(report.Issues) > 0 {
			s := strings.Join(report.Issues, "; ")
			degradationReason = &s
		}
		breakersOpen := uint32(0)
		if d.BreakersCount != nil {
			breakersOpen = uint32(d.BreakersCount.OpenCount())
		}
		writeJSON(w, http.StatusOK, dashboardHealthResponse{
			Status:       statusString(report.Status),
			Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
			UptimeMillis: time.Since(startTime).Milliseconds(),
			Details: &dashboardHealthDetails{
				TotalQueues:         report.ConsumersHealthy + report.ConsumersUnhealthy,
				HealthyQueues:       report.ConsumersHealthy,
				TotalPools:          report.PoolsHealthy + report.PoolsUnhealthy,
				HealthyPools:        report.PoolsHealthy,
				ActiveWarnings:      report.ActiveWarnings,
				CriticalWarnings:    report.CriticalWarnings,
				CircuitBreakersOpen: breakersOpen,
				DegradationReason:   degradationReason,
			},
		})
	}
}

func handlePoolStats(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, fromPoolStats(poolStatsSnapshot(d)))
	}
}

// handleMonitoringWarnings returns the active (unacknowledged, recent)
// warnings sorted newest-first — the dashboard's preferred shape.
func handleMonitoringWarnings(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		// `Active` filters by HealthServiceConfig.WarningAgeMinutes (default 30).
		warnings := d.Warnings.Active(30)
		sort.Slice(warnings, func(i, j int) bool {
			return warnings[i].CreatedAt.After(warnings[j].CreatedAt)
		})
		writeJSON(w, http.StatusOK, fromWarnings(warnings))
	}
}

// handleConsumerHealth emits the Java-shape camelCase JSON the
// dashboard expects. Consumer list comes from the WarningService's
// running flags; until the Manager exposes a consumer registry this
// uses HealthService directly.
func handleConsumerHealth(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		now := time.Now().UTC()
		consumers := map[string]any{}

		// Stalled consumers are the only ones the HealthService can
		// enumerate today. When the Manager exposes a complete
		// consumer-id list this will switch to that source.
		for _, id := range d.Health.StalledConsumers() {
			h := d.Health.ConsumerHealth(id)
			consumers[id] = consumerDetail(id, h, now)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"currentTimeMs": now.UnixMilli(),
			"currentTime":   now.Format(time.RFC3339Nano),
			"consumers":     consumers,
		})
	}
}

func consumerDetail(id string, h router.ConsumerHealth, now time.Time) map[string]any {
	lastPollMs := int64(0)
	if h.LastPollTimeMs != nil {
		lastPollMs = *h.LastPollTimeMs
	}
	timeSinceMs := int64(-1)
	if h.TimeSinceLastPollMs != nil {
		timeSinceMs = *h.TimeSinceLastPollMs
	}
	lastPollTime := "never"
	if lastPollMs > 0 {
		lastPollTime = now.Add(-time.Duration(timeSinceMs) * time.Millisecond).Format(time.RFC3339Nano)
	}
	timeSinceSeconds := int64(-1)
	if timeSinceMs > 0 {
		timeSinceSeconds = timeSinceMs / 1000
	}
	return map[string]any{
		"mapKey":                   id,
		"queueIdentifier":          id,
		"consumerQueueIdentifier":  id,
		"isHealthy":                h.IsHealthy,
		"lastPollTimeMs":           lastPollMs,
		"lastPollTime":             lastPollTime,
		"timeSinceLastPollMs":      timeSinceMs,
		"timeSinceLastPollSeconds": timeSinceSeconds,
		"isRunning":                h.IsRunning,
	}
}

// ── Warnings management ──────────────────────────────────────────────────

func handleListWarnings(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var warnings []router.Warning
		if q.Get("acknowledged") == "false" {
			warnings = d.Warnings.Unacknowledged()
		} else {
			warnings = d.Warnings.All()
		}
		if sev := strings.ToUpper(q.Get("severity")); sev != "" {
			warnings = filterWarnings(warnings, func(w router.Warning) bool {
				return matchesSeverity(w.Severity, sev)
			})
		}
		if cat := strings.ToUpper(q.Get("category")); cat != "" {
			warnings = filterWarnings(warnings, func(w router.Warning) bool {
				return strings.ToUpper(string(w.Category)) == cat
			})
		}
		sort.Slice(warnings, func(i, j int) bool {
			return warnings[i].CreatedAt.After(warnings[j].CreatedAt)
		})
		writeJSON(w, http.StatusOK, fromWarnings(warnings))
	}
}

// matchesSeverity accepts "WARN" or "WARNING" interchangeably to mirror
// Rust's list_warnings parser.
func matchesSeverity(have router.WarningSeverity, want string) bool {
	if want == "WARN" {
		return have == router.WarningWarning
	}
	return strings.EqualFold(string(have), want)
}

func filterWarnings(in []router.Warning, ok func(router.Warning) bool) []router.Warning {
	out := in[:0]
	for _, w := range in {
		if ok(w) {
			out = append(out, w)
		}
	}
	return out
}

func handleClearAllWarnings(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		// "clear all" semantics in Rust: remove every warning regardless of ack state.
		// Simulate by clearing-acknowledged + clearing-old(0) — but cleaner: walk + remove.
		ids := make([]string, 0)
		for _, x := range d.Warnings.All() {
			ids = append(ids, x.ID)
		}
		for _, id := range ids {
			d.Warnings.Remove(id)
		}
		writeJSON(w, http.StatusOK, map[string]any{"cleared": len(ids)})
	}
}

func handleAcknowledge(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if d.Warnings.Acknowledge(id) {
			slog.Debug("warning acknowledged", "id", id)
			writeJSON(w, http.StatusOK, map[string]any{"acknowledged": true})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "Warning not found"})
	}
}

func handleAcknowledgeAll(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		n := d.Warnings.AcknowledgeMatching(func(router.Warning) bool { return true })
		writeJSON(w, http.StatusOK, map[string]any{"acknowledged": n})
	}
}

func handleCriticalWarnings(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, fromWarnings(d.Warnings.Critical()))
	}
}

func handleUnacknowledgedWarnings(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, fromWarnings(d.Warnings.Unacknowledged()))
	}
}

func handleClearOldWarnings(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hours := parseIntQuery(r, "hours", 8)
		n := d.Warnings.ClearOlderThan(time.Duration(hours) * time.Hour)
		writeJSON(w, http.StatusOK, map[string]any{"cleared": n})
	}
}

func parseIntQuery(r *http.Request, key string, def int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 {
		return def
	}
	return n
}

// ── Config ───────────────────────────────────────────────────────────────

// handleConfig returns a minimal snapshot of router knobs. Sensitive
// values (Redis URLs, webhook secrets) are deliberately omitted —
// matches Rust's get_local_config which only reports pool/queue
// metadata, never credentials.
func handleConfig(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"version":          Version,
			"warnings_total":   d.Warnings.Count(),
			"warnings_critical": d.Warnings.CriticalCount(),
		})
	}
}
