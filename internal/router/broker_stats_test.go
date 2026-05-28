package router

import (
	"context"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// fakeMetricsSource lets tests drive QueueMetrics / QueueCounters
// independently — Metrics simulates what a fresh broker fetch returns,
// Counters simulates what live atomics report.
type fakeMetricsSource struct {
	metrics  func() []queue.Metrics
	counters func() []queue.Metrics
}

func (f *fakeMetricsSource) QueueMetrics(_ context.Context) []queue.Metrics {
	return f.metrics()
}
func (f *fakeMetricsSource) QueueCounters() []queue.Metrics { return f.counters() }

func TestCachedBrokerStats_OverlayAttrsOnLiveCounters(t *testing.T) {
	src := &fakeMetricsSource{
		metrics: func() []queue.Metrics {
			return []queue.Metrics{{
				QueueIdentifier:  "q1",
				PendingMessages:  100,
				InFlightMessages: 5,
				TotalPolled:      10,
				TotalAcked:       8,
			}}
		},
		counters: func() []queue.Metrics {
			return []queue.Metrics{{
				QueueIdentifier: "q1",
				// Live counters are newer than the snapshot.
				TotalPolled: 50,
				TotalAcked:  48,
			}}
		},
	}
	c := NewCachedBrokerStats(src)
	c.Refresh(context.Background())

	out := c.GetWindowed(0) // no window → all-time
	if len(out) != 1 {
		t.Fatalf("expected 1 queue, got %d", len(out))
	}
	got := out[0]
	if got.PendingMessages != 100 || got.InFlightMessages != 5 {
		t.Errorf("attrs not overlaid: got pending=%d inflight=%d",
			got.PendingMessages, got.InFlightMessages)
	}
	if got.TotalPolled != 50 || got.TotalAcked != 48 {
		t.Errorf("counters should be live, got polled=%d acked=%d",
			got.TotalPolled, got.TotalAcked)
	}
}

func TestCachedBrokerStats_WindowedDelta(t *testing.T) {
	// Snapshot at t0: totals are baseline.
	pollSeq := []uint64{10, 50}
	ackSeq := []uint64{5, 45}
	step := 0

	src := &fakeMetricsSource{
		metrics: func() []queue.Metrics {
			m := queue.Metrics{
				QueueIdentifier: "q1",
				PendingMessages: 1,
				TotalPolled:     pollSeq[step],
				TotalAcked:      ackSeq[step],
			}
			return []queue.Metrics{m}
		},
		counters: func() []queue.Metrics {
			return []queue.Metrics{{
				QueueIdentifier: "q1",
				TotalPolled:     pollSeq[step],
				TotalAcked:      ackSeq[step],
			}}
		},
	}
	c := NewCachedBrokerStats(src)

	// First refresh: baseline (polled=10, acked=5).
	c.Refresh(context.Background())
	step = 1
	// Hop the clock so the baseline is older than the requested window.
	// We force this by waiting 20ms and asking for a 10ms window.
	time.Sleep(20 * time.Millisecond)

	out := c.GetWindowed(10 * time.Millisecond)
	if len(out) != 1 {
		t.Fatalf("expected 1 queue, got %d", len(out))
	}
	// Live = 50/45, baseline = 10/5, delta = 40/40.
	if out[0].TotalPolled != 40 || out[0].TotalAcked != 40 {
		t.Errorf("expected delta 40/40, got polled=%d acked=%d",
			out[0].TotalPolled, out[0].TotalAcked)
	}
}

func TestCachedBrokerStats_AgeSeconds(t *testing.T) {
	src := &fakeMetricsSource{
		metrics:  func() []queue.Metrics { return nil },
		counters: func() []queue.Metrics { return nil },
	}
	c := NewCachedBrokerStats(src)
	if got := c.AgeSeconds(); got != -1 {
		t.Fatalf("AgeSeconds before refresh should be -1, got %d", got)
	}
	c.Refresh(context.Background())
	if got := c.AgeSeconds(); got < 0 {
		t.Fatalf("AgeSeconds after refresh should be >= 0, got %d", got)
	}
}

func TestSaturatingSub(t *testing.T) {
	if got := saturatingSub(10, 3); got != 7 {
		t.Errorf("saturatingSub(10,3)=%d, want 7", got)
	}
	if got := saturatingSub(3, 10); got != 0 {
		t.Errorf("saturatingSub(3,10)=%d, want 0", got)
	}
}
