package router

import (
	"testing"
	"time"
)

func TestPoolMetricsCollector_Empty(t *testing.T) {
	c := NewPoolMetricsCollector()
	m := c.Snapshot()

	if m.TotalSuccess != 0 || m.TotalFailure != 0 {
		t.Fatalf("expected zero totals, got %+v", m)
	}
	if m.SuccessRate != 1.0 {
		t.Fatalf("empty success rate should be 1.0, got %v", m.SuccessRate)
	}
	if m.ProcessingTime.SampleCount != 0 {
		t.Fatalf("empty histogram should have 0 samples, got %d", m.ProcessingTime.SampleCount)
	}
}

func TestPoolMetricsCollector_SuccessRecording(t *testing.T) {
	c := NewPoolMetricsCollector()

	c.RecordSuccess(100)
	c.RecordSuccess(200)
	c.RecordSuccess(300)

	m := c.Snapshot()

	if m.TotalSuccess != 3 {
		t.Fatalf("TotalSuccess=%d, want 3", m.TotalSuccess)
	}
	if m.TotalFailure != 0 {
		t.Fatalf("TotalFailure=%d, want 0", m.TotalFailure)
	}
	if m.SuccessRate != 1.0 {
		t.Fatalf("SuccessRate=%v, want 1.0", m.SuccessRate)
	}
	if m.ProcessingTime.SampleCount != 3 {
		t.Fatalf("SampleCount=%d, want 3", m.ProcessingTime.SampleCount)
	}
	// Avg of 100/200/300 = 200; allow 1ms slack for rounding.
	if got := m.ProcessingTime.AvgMs; got < 199.0 || got > 201.0 {
		t.Fatalf("AvgMs=%v, want ~200", got)
	}
}

func TestPoolMetricsCollector_FailureRecording(t *testing.T) {
	c := NewPoolMetricsCollector()

	c.RecordSuccess(100)
	c.RecordFailure(500)

	m := c.Snapshot()
	if m.TotalSuccess != 1 || m.TotalFailure != 1 {
		t.Fatalf("got %+v, want 1/1", m)
	}
	if m.SuccessRate != 0.5 {
		t.Fatalf("SuccessRate=%v, want 0.5", m.SuccessRate)
	}
}

func TestPoolMetricsCollector_TransientNotCounted(t *testing.T) {
	c := NewPoolMetricsCollector()

	c.RecordSuccess(100)
	c.RecordTransient(200)

	m := c.Snapshot()
	// Transient should NOT bump TotalFailure (will be retried), but it
	// IS reflected in the windowed success-rate.
	if m.TotalSuccess != 1 || m.TotalFailure != 0 {
		t.Fatalf("totals=%+v, want 1/0", m)
	}
	// Windowed: 1 success + 1 failure(transient) → 0.5
	if m.Last5Min.SuccessRate != 0.5 {
		t.Fatalf("Last5Min.SuccessRate=%v, want 0.5 (transient counted as window non-success)",
			m.Last5Min.SuccessRate)
	}
}

func TestPoolMetricsCollector_Percentiles(t *testing.T) {
	c := NewPoolMetricsCollector()

	for i := uint64(1); i <= 100; i++ {
		c.RecordSuccess(i)
	}
	m := c.Snapshot()

	// Nearest-rank: p50 of 1..100 = 50, p95 = 95, p99 = 99.
	if got := m.ProcessingTime.P50Ms; got < 49 || got > 51 {
		t.Errorf("P50Ms=%d, want ~50", got)
	}
	if got := m.ProcessingTime.P95Ms; got < 94 || got > 96 {
		t.Errorf("P95Ms=%d, want ~95", got)
	}
	if got := m.ProcessingTime.P99Ms; got < 98 || got > 100 {
		t.Errorf("P99Ms=%d, want ~99", got)
	}
	if m.ProcessingTime.MinMs != 1 {
		t.Errorf("MinMs=%d, want 1", m.ProcessingTime.MinMs)
	}
	if m.ProcessingTime.MaxMs != 100 {
		t.Errorf("MaxMs=%d, want 100", m.ProcessingTime.MaxMs)
	}
}

func TestPoolMetricsCollector_Windowed(t *testing.T) {
	c := NewPoolMetricsCollector()
	for i := 0; i < 10; i++ {
		if i%3 == 0 {
			c.RecordFailure(uint64(100 + i*10))
		} else {
			c.RecordSuccess(uint64(100 + i*10))
		}
	}
	m := c.Snapshot()

	if got := m.Last5Min.SuccessCount + m.Last5Min.FailureCount; got != 10 {
		t.Fatalf("Last5Min count=%d, want 10", got)
	}
	if m.Last5Min.ThroughputPerSec <= 0 {
		t.Fatalf("Last5Min throughput should be > 0, got %v", m.Last5Min.ThroughputPerSec)
	}
}

func TestPoolMetricsCollector_RateLimitedWindowed(t *testing.T) {
	c := NewPoolMetricsCollector()
	c.RecordRateLimited()
	c.RecordRateLimited()
	c.RecordRateLimited()

	m := c.Snapshot()

	if m.TotalRateLimited != 3 {
		t.Fatalf("TotalRateLimited=%d, want 3", m.TotalRateLimited)
	}
	if m.Last5Min.RateLimitedCount != 3 {
		t.Fatalf("Last5Min.RateLimitedCount=%d, want 3", m.Last5Min.RateLimitedCount)
	}
	if m.Last30Min.RateLimitedCount != 3 {
		t.Fatalf("Last30Min.RateLimitedCount=%d, want 3", m.Last30Min.RateLimitedCount)
	}
}

func TestPoolMetricsCollector_WindowEviction(t *testing.T) {
	// Use a tiny short window so we can verify samples drop out without
	// relying on real time-of-day.
	c := NewPoolMetricsCollectorWithConfig(MetricsConfig{
		MaxSamples:  100,
		ShortWindow: 50 * time.Millisecond,
		LongWindow:  500 * time.Millisecond,
	})
	c.RecordSuccess(10)
	time.Sleep(100 * time.Millisecond)
	c.RecordSuccess(20)

	m := c.Snapshot()
	// First sample should have dropped out of the short window.
	if m.Last5Min.SuccessCount != 1 {
		t.Fatalf("Last5Min.SuccessCount=%d, want 1 (oldest evicted)", m.Last5Min.SuccessCount)
	}
	// Both still in the long window.
	if m.Last30Min.SuccessCount != 2 {
		t.Fatalf("Last30Min.SuccessCount=%d, want 2", m.Last30Min.SuccessCount)
	}
}

func TestPoolMetricsCollector_Reset(t *testing.T) {
	c := NewPoolMetricsCollector()
	c.RecordSuccess(50)
	c.RecordFailure(60)
	c.RecordRateLimited()
	c.Reset()

	m := c.Snapshot()
	if m.TotalSuccess != 0 || m.TotalFailure != 0 || m.TotalRateLimited != 0 {
		t.Fatalf("Reset failed: %+v", m)
	}
	if m.ProcessingTime.SampleCount != 0 {
		t.Fatalf("Reset failed to clear samples: %d", m.ProcessingTime.SampleCount)
	}
}

func TestPoolMetricsCollector_MaxSamplesBound(t *testing.T) {
	c := NewPoolMetricsCollectorWithConfig(MetricsConfig{
		MaxSamples:  3,
		ShortWindow: 5 * time.Minute,
		LongWindow:  30 * time.Minute,
	})
	for i := 1; i <= 10; i++ {
		c.RecordSuccess(uint64(i))
	}
	m := c.Snapshot()
	if m.TotalSuccess != 10 {
		t.Fatalf("TotalSuccess should track all-time even when buffer wraps: %d", m.TotalSuccess)
	}
	if m.ProcessingTime.SampleCount != 3 {
		t.Fatalf("SampleCount should be capped at MaxSamples=3, got %d", m.ProcessingTime.SampleCount)
	}
	// Most-recent 3 are 8, 9, 10.
	if m.ProcessingTime.MinMs != 8 || m.ProcessingTime.MaxMs != 10 {
		t.Fatalf("expected min=8 max=10, got min=%d max=%d",
			m.ProcessingTime.MinMs, m.ProcessingTime.MaxMs)
	}
}
