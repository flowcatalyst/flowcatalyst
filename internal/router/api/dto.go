// Package api wires the router's HTTP surface (health probes, monitoring
// reads, warnings management, config read) against the shared router
// services. Sibling of crates/fc-router/src/api/mod.rs.
//
// Wire DTOs live here. The internal router types (HealthReport,
// ConsumerHealth, Warning, etc.) use Go-idiomatic camelCase JSON tags;
// many Rust endpoints emit snake_case (default serde) because the
// original Java dashboard expects it. The DTOs in this file translate
// internal → wire so the API parity contract holds without polluting
// the package-internal types.
//
// Per-endpoint JSON-case decisions:
//
//   - /monitoring          → snake_case (Rust MonitoringResponse default)
//   - /monitoring/health   → camelCase  (DashboardHealthDetails has explicit renames)
//   - /monitoring/pools    → snake_case (Rust PoolStats default)
//   - /monitoring/warnings → snake_case (Rust Warning default)
//   - /warnings/*          → snake_case (Rust Warning default)
//   - /monitoring/consumer-health → camelCase (handler builds inline JSON with camelCase keys)
//   - /api/config          → snake_case (Rust default)
package api

import (
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
)

// ── Probe responses (used by /health, /health/live, /health/ready) ───────

// probeResponse matches Rust's ProbeResponse: `{"status": "LIVE"|"READY"|"NOT_READY"}`.
type probeResponse struct {
	Status string `json:"status"`
}

// ── Monitoring overview (/monitoring) ────────────────────────────────────

// monitoringResponse matches Rust's MonitoringResponse — snake_case
// per Rust's default serde.
type monitoringResponse struct {
	Status           string        `json:"status"`
	Version          string        `json:"version"`
	HealthReport     wireHealthReport `json:"health_report"`
	PoolStats        []wirePoolStats  `json:"pool_stats"`
	ActiveWarnings   uint32        `json:"active_warnings"`
	CriticalWarnings uint32        `json:"critical_warnings"`
}

// wireHealthReport mirrors Rust's HealthReport — snake_case JSON.
type wireHealthReport struct {
	Status             router.HealthStatus `json:"status"`
	PoolsHealthy       uint32              `json:"pools_healthy"`
	PoolsUnhealthy     uint32              `json:"pools_unhealthy"`
	ConsumersHealthy   uint32              `json:"consumers_healthy"`
	ConsumersUnhealthy uint32              `json:"consumers_unhealthy"`
	ActiveWarnings     uint32              `json:"active_warnings"`
	CriticalWarnings   uint32              `json:"critical_warnings"`
	Issues             []string            `json:"issues"`
}

func fromHealthReport(r router.HealthReport) wireHealthReport {
	if r.Issues == nil {
		r.Issues = []string{}
	}
	return wireHealthReport{
		Status:             r.Status,
		PoolsHealthy:       r.PoolsHealthy,
		PoolsUnhealthy:     r.PoolsUnhealthy,
		ConsumersHealthy:   r.ConsumersHealthy,
		ConsumersUnhealthy: r.ConsumersUnhealthy,
		ActiveWarnings:     r.ActiveWarnings,
		CriticalWarnings:   r.CriticalWarnings,
		Issues:             r.Issues,
	}
}

// wirePoolStats mirrors Rust's PoolStats — snake_case JSON.
type wirePoolStats struct {
	PoolCode           string  `json:"pool_code"`
	Concurrency        uint32  `json:"concurrency"`
	ActiveWorkers      uint32  `json:"active_workers"`
	QueueSize          uint32  `json:"queue_size"`
	QueueCapacity      uint32  `json:"queue_capacity"`
	MessageGroupCount  uint32  `json:"message_group_count"`
	RateLimitPerMinute *uint32 `json:"rate_limit_per_minute,omitempty"`
	IsRateLimited      bool    `json:"is_rate_limited"`
}

func fromPoolStats(s []router.PoolStats) []wirePoolStats {
	out := make([]wirePoolStats, len(s))
	for i, p := range s {
		out[i] = wirePoolStats{
			PoolCode:           p.PoolCode,
			Concurrency:        p.Concurrency,
			ActiveWorkers:      p.ActiveWorkers,
			QueueSize:          p.QueueSize,
			QueueCapacity:      p.QueueCapacity,
			MessageGroupCount:  p.MessageGroupCount,
			RateLimitPerMinute: p.RateLimitPerMinute,
			IsRateLimited:      p.IsRateLimited,
		}
	}
	return out
}

// ── Dashboard health (/monitoring/health) ────────────────────────────────

// dashboardHealthResponse mirrors Rust's DashboardHealthResponse —
// top-level snake_case ('status', 'timestamp') with the details object
// using explicit camelCase renames.
type dashboardHealthResponse struct {
	Status       string                  `json:"status"`
	Timestamp    string                  `json:"timestamp"`
	UptimeMillis int64                   `json:"uptimeMillis"`
	Details      *dashboardHealthDetails `json:"details,omitempty"`
}

// dashboardHealthDetails has explicit camelCase per-field renames in Rust.
type dashboardHealthDetails struct {
	TotalQueues         uint32  `json:"totalQueues"`
	HealthyQueues       uint32  `json:"healthyQueues"`
	TotalPools          uint32  `json:"totalPools"`
	HealthyPools        uint32  `json:"healthyPools"`
	ActiveWarnings      uint32  `json:"activeWarnings"`
	CriticalWarnings    uint32  `json:"criticalWarnings"`
	CircuitBreakersOpen uint32  `json:"circuitBreakersOpen"`
	DegradationReason   *string `json:"degradationReason,omitempty"`
}

// ── Warnings (/warnings, /monitoring/warnings, /warnings/{id}/...) ───────

// wireWarning mirrors Rust's Warning struct — snake_case JSON tags.
// The internal router.Warning uses camelCase; the api package
// translates on the way out.
type wireWarning struct {
	ID             string    `json:"id"`
	Category       string    `json:"category"`
	Severity       string    `json:"severity"`
	Message        string    `json:"message"`
	Source         string    `json:"source"`
	CreatedAt      time.Time `json:"created_at"`
	Acknowledged   bool      `json:"acknowledged"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
}

func fromWarning(w router.Warning) wireWarning {
	return wireWarning{
		ID:             w.ID,
		Category:       string(w.Category),
		Severity:       string(w.Severity),
		Message:        w.Message,
		Source:         w.Source,
		CreatedAt:      w.CreatedAt,
		Acknowledged:   w.Acknowledged,
		AcknowledgedAt: w.AcknowledgedAt,
	}
}

func fromWarnings(ws []router.Warning) []wireWarning {
	out := make([]wireWarning, len(ws))
	for i, w := range ws {
		out[i] = fromWarning(w)
	}
	return out
}
