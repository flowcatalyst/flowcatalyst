package router

// Shared types referenced by HealthService + the deferred /monitoring/*
// HTTP surface. Mirrors `fc_common::{HealthStatus, HealthReport,
// ConsumerHealth, PoolStats}`. Field names use Rust-camelCase JSON tags
// so the eventual API surface lands without translation.
//
// PoolStats today carries only the fields HealthService reads. The
// EnhancedPoolMetrics / ProcessingTimeMetrics / WindowedMetrics
// sub-structs land alongside the metrics port (HANDOFF §4 #7 follow-up).

// HealthStatus is the coarse system-health verdict.
type HealthStatus string

const (
	HealthHealthy  HealthStatus = "Healthy"
	HealthWarning  HealthStatus = "Warning"
	HealthDegraded HealthStatus = "Degraded"
)

// HealthReport is the JSON shape returned by /monitoring/health.
type HealthReport struct {
	Status             HealthStatus `json:"status"`
	PoolsHealthy       uint32       `json:"poolsHealthy"`
	PoolsUnhealthy     uint32       `json:"poolsUnhealthy"`
	ConsumersHealthy   uint32       `json:"consumersHealthy"`
	ConsumersUnhealthy uint32       `json:"consumersUnhealthy"`
	ActiveWarnings     uint32       `json:"activeWarnings"`
	CriticalWarnings   uint32       `json:"criticalWarnings"`
	Issues             []string     `json:"issues"`
}

// ConsumerHealth is the per-consumer snapshot returned by
// /monitoring/consumers/{id}.
type ConsumerHealth struct {
	QueueIdentifier      string `json:"queueIdentifier"`
	IsHealthy            bool   `json:"isHealthy"`
	LastPollTimeMs       *int64 `json:"lastPollTimeMs,omitempty"`
	TimeSinceLastPollMs  *int64 `json:"timeSinceLastPollMs,omitempty"`
	IsRunning            bool   `json:"isRunning"`
}

// PoolStats is the per-pool snapshot returned by /monitoring/pools.
// The metrics-heavy fields land later — HealthService only reads PoolCode.
type PoolStats struct {
	PoolCode            string  `json:"poolCode"`
	Concurrency         uint32  `json:"concurrency"`
	ActiveWorkers       uint32  `json:"activeWorkers"`
	QueueSize           uint32  `json:"queueSize"`
	QueueCapacity       uint32  `json:"queueCapacity"`
	MessageGroupCount   uint32  `json:"messageGroupCount"`
	RateLimitPerMinute  *uint32 `json:"rateLimitPerMinute,omitempty"`
	IsRateLimited       bool    `json:"isRateLimited"`
}
