package common

import "time"

// ProcessingTimeMetrics summarises latency in milliseconds with the
// percentiles the dashboard renders.
type ProcessingTimeMetrics struct {
	AvgMs       float64 `json:"avgMs"`
	MinMs       uint64  `json:"minMs"`
	MaxMs       uint64  `json:"maxMs"`
	P50Ms       uint64  `json:"p50Ms"`
	P95Ms       uint64  `json:"p95Ms"`
	P99Ms       uint64  `json:"p99Ms"`
	SampleCount uint64  `json:"sampleCount"`
}

// WindowedMetrics is the per-window slice of pool throughput.
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
// PoolStats.Metrics.
type EnhancedPoolMetrics struct {
	TotalSuccess     uint64 `json:"totalSuccess"`
	TotalFailure     uint64 `json:"totalFailure"`
	TotalRateLimited uint64 `json:"totalRateLimited"`
	// TotalSuppressed counts messages ACKed without delivery because their
	// group was suppressed by the GroupFlushRegistry (R-53). Kept separate
	// from TotalSuccess/TotalFailure: no delivery was attempted, so neither
	// bucket fits, and a pool suppressing a flushed group should read
	// busy-but-suppressed rather than idle.
	TotalSuppressed uint64                `json:"totalSuppressed"`
	SuccessRate     float64               `json:"successRate"`
	ProcessingTime  ProcessingTimeMetrics `json:"processingTime"`
	Last5Min        WindowedMetrics       `json:"last5Min"`
	Last30Min       WindowedMetrics       `json:"last30Min"`
}
