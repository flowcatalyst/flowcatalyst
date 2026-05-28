package router

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// counterHistoryWindow bounds the rolling baseline used for windowed
// queue stats. Mirrors COUNTER_HISTORY_WINDOW (30 min) in the Rust
// `api::CachedBrokerStats`.
const counterHistoryWindow = 30 * time.Minute

// brokerRefreshInterval is the cadence for fresh SQS attribute fetches.
// Mirrors the 60s ticker in `api::spawn_broker_stats_refresh`.
const brokerRefreshInterval = 60 * time.Second

// queueAttr is the pair of expensive broker attributes that the cache
// refreshes on a slow cadence. Counter fields (polled / acked / nacked
// / deferred) are read live from the consumer atomics on every request.
type queueAttr struct {
	pendingMessages  uint64
	inFlightMessages uint64
}

// counterSnapshot is the per-queue cumulative-counter baseline stored
// for windowed deltas. Identical shape to QueueCounterSnapshot in
// `api::mod.rs`.
type counterSnapshot struct {
	totalPolled   uint64
	totalAcked    uint64
	totalNacked   uint64
	totalDeferred uint64
}

type counterHistoryEntry struct {
	ts       time.Time
	perQueue map[string]counterSnapshot
}

// MetricsSource fans Consumer.Metrics / Consumer.Counters over the live
// pool set. Manager implements this; tests can substitute a fake.
type MetricsSource interface {
	// QueueMetrics fetches live broker attributes (pending/in-flight)
	// PLUS process-local counters across every running consumer.
	QueueMetrics(ctx context.Context) []queue.Metrics
	// QueueCounters returns counters only — no broker round-trip.
	QueueCounters() []queue.Metrics
}

// CachedBrokerStats serves the dashboard's queue-stats endpoint:
//   - expensive SQS attributes (pending / in-flight) refreshed on a 60s
//     cadence by a background goroutine OR on demand via Refresh,
//   - cheap counters (polled/acked/nacked/deferred) read live on every
//     call,
//   - windowed deltas computed against a 30-min counter history.
//
// Mirrors crates/fc-router/src/api/mod.rs::CachedBrokerStats.
type CachedBrokerStats struct {
	source MetricsSource

	mu             sync.RWMutex
	attrs          map[string]queueAttr // last fetched broker attributes
	lastUpdated    time.Time
	counterHistory []counterHistoryEntry // oldest first
}

// NewCachedBrokerStats wires the cache against the supplied source.
// Callers must call Refresh once on startup (or via spawnBrokerStatsRefresh).
func NewCachedBrokerStats(source MetricsSource) *CachedBrokerStats {
	return &CachedBrokerStats{
		source: source,
		attrs:  make(map[string]queueAttr),
	}
}

// Refresh fetches fresh broker attributes and appends a counter snapshot
// to the rolling history. Trims history entries older than the window.
func (c *CachedBrokerStats) Refresh(ctx context.Context) {
	fresh := c.source.QueueMetrics(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.attrs = make(map[string]queueAttr, len(fresh))
	for _, m := range fresh {
		c.attrs[m.QueueIdentifier] = queueAttr{
			pendingMessages:  m.PendingMessages,
			inFlightMessages: m.InFlightMessages,
		}
	}
	c.lastUpdated = time.Now()

	c.snapshotCountersLocked(fresh)
}

func (c *CachedBrokerStats) snapshotCountersLocked(fresh []queue.Metrics) {
	now := time.Now()
	cutoff := now.Add(-counterHistoryWindow)

	perQueue := make(map[string]counterSnapshot, len(fresh))
	for _, m := range fresh {
		perQueue[m.QueueIdentifier] = counterSnapshot{
			totalPolled:   m.TotalPolled,
			totalAcked:    m.TotalAcked,
			totalNacked:   m.TotalNacked,
			totalDeferred: m.TotalDeferred,
		}
	}

	c.counterHistory = append(c.counterHistory, counterHistoryEntry{ts: now, perQueue: perQueue})
	// Trim front entries beyond the window.
	i := 0
	for i < len(c.counterHistory) && c.counterHistory[i].ts.Before(cutoff) {
		i++
	}
	if i > 0 {
		c.counterHistory = c.counterHistory[i:]
	}
}

// GetWindowed returns metrics with cached broker attributes overlaid on
// live counters. When window is non-zero, cumulative counters are
// replaced with deltas (current - newest baseline at or before now-window;
// falls back to oldest baseline when history is shorter than the window).
func (c *CachedBrokerStats) GetWindowed(window time.Duration) []queue.Metrics {
	live := c.source.QueueCounters()

	c.mu.RLock()
	attrs := make(map[string]queueAttr, len(c.attrs))
	for k, v := range c.attrs {
		attrs[k] = v
	}
	var baseline map[string]counterSnapshot
	if window > 0 && len(c.counterHistory) > 0 {
		target := time.Now().Add(-window)
		// Walk newest → oldest, take the newest entry with ts <= target.
		for i := len(c.counterHistory) - 1; i >= 0; i-- {
			if !c.counterHistory[i].ts.After(target) {
				baseline = c.counterHistory[i].perQueue
				break
			}
		}
		// Fallback to oldest entry if history is shorter than the window.
		if baseline == nil {
			baseline = c.counterHistory[0].perQueue
		}
	}
	c.mu.RUnlock()

	for i := range live {
		m := &live[i]
		if a, ok := attrs[m.QueueIdentifier]; ok {
			m.PendingMessages = a.pendingMessages
			m.InFlightMessages = a.inFlightMessages
		}
		if window == 0 {
			continue
		}
		if base, ok := baseline[m.QueueIdentifier]; ok {
			m.TotalPolled = saturatingSub(m.TotalPolled, base.totalPolled)
			m.TotalAcked = saturatingSub(m.TotalAcked, base.totalAcked)
			m.TotalNacked = saturatingSub(m.TotalNacked, base.totalNacked)
			m.TotalDeferred = saturatingSub(m.TotalDeferred, base.totalDeferred)
		} else {
			// No baseline for this queue — zero out so dashboard doesn't
			// misreport all-time counters as window counters.
			m.TotalPolled = 0
			m.TotalAcked = 0
			m.TotalNacked = 0
			m.TotalDeferred = 0
		}
	}
	return live
}

// AgeSeconds returns time since the last Refresh, or -1 if never refreshed.
func (c *CachedBrokerStats) AgeSeconds() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.lastUpdated.IsZero() {
		return -1
	}
	return int64(time.Since(c.lastUpdated).Seconds())
}

func saturatingSub(a, b uint64) uint64 {
	if b > a {
		return 0
	}
	return a - b
}

// SpawnBrokerStatsRefresh kicks off a background goroutine that performs
// an initial refresh and then re-fetches every brokerRefreshInterval
// until ctx is cancelled.
func SpawnBrokerStatsRefresh(ctx context.Context, c *CachedBrokerStats) {
	go func() {
		c.Refresh(ctx)
		t := time.NewTicker(brokerRefreshInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.Refresh(ctx)
			}
		}
	}()
	slog.Debug("broker stats refresh task spawned", "interval", brokerRefreshInterval)
}
