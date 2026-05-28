package common

import "time"

// ProcessingTimeMetrics summarises latency in milliseconds with the
// percentiles the dashboard renders. Mirrors `fc_common::ProcessingTimeMetrics`
// (crates/fc-common/src/lib.rs:850).
type ProcessingTimeMetrics struct {
	AvgMs       float64 `json:"avgMs"`
	MinMs       uint64  `json:"minMs"`
	MaxMs       uint64  `json:"maxMs"`
	P50Ms       uint64  `json:"p50Ms"`
	P95Ms       uint64  `json:"p95Ms"`
	P99Ms       uint64  `json:"p99Ms"`
	SampleCount uint64  `json:"sampleCount"`
}

// WindowedMetrics is the per-window slice of pool throughput. Mirrors
// `fc_common::WindowedMetrics` (crates/fc-common/src/lib.rs:884).
type WindowedMetrics struct {
	SuccessCount       uint64                `json:"successCount"`
	FailureCount       uint64                `json:"failureCount"`
	RateLimitedCount   uint64                `json:"rateLimitedCount"`
	SuccessRate        float64               `json:"successRate"`
	ThroughputPerSec   float64               `json:"throughputPerSec"`
	ProcessingTime     ProcessingTimeMetrics `json:"processingTime"`
	WindowStart        time.Time             `json:"windowStart"`
	WindowDurationSecs uint64                `json:"windowDurationSecs"`
}

// EnhancedPoolMetrics is the rolling-window snapshot embedded in
// PoolStats.Metrics. Mirrors `fc_common::EnhancedPoolMetrics`
// (crates/fc-common/src/lib.rs:832).
type EnhancedPoolMetrics struct {
	TotalSuccess     uint64                `json:"totalSuccess"`
	TotalFailure     uint64                `json:"totalFailure"`
	TotalRateLimited uint64                `json:"totalRateLimited"`
	SuccessRate      float64               `json:"successRate"`
	ProcessingTime   ProcessingTimeMetrics `json:"processingTime"`
	Last5Min         WindowedMetrics       `json:"last5Min"`
	Last30Min        WindowedMetrics       `json:"last30Min"`
}
