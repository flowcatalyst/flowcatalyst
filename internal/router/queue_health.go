package router

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// QueueHealthConfig configures the backlog + growth detector.
type QueueHealthConfig struct {
	Enabled                bool
	CheckInterval          time.Duration
	BacklogThreshold       uint64
	GrowthThreshold        uint64
	GrowthPeriodsThreshold uint32
}

// DefaultQueueHealthConfig returns the standard defaults.
func DefaultQueueHealthConfig() QueueHealthConfig {
	return QueueHealthConfig{
		Enabled:                true,
		CheckInterval:          30 * time.Second,
		BacklogThreshold:       1000,
		GrowthThreshold:        100,
		GrowthPeriodsThreshold: 3,
	}
}

// QueueHealthMonitor watches per-queue depth and emits warnings for
// backlogs (queue > threshold) and sustained growth (size increased
// for N consecutive periods).
type QueueHealthMonitor struct {
	cfg      QueueHealthConfig
	warnings *WarningService // never nil after NewQueueHealthMonitor; see SetWarnings

	mu      sync.Mutex
	history map[string]*queueSizeHistory
}

type queueSizeHistory struct {
	lastSize                 *uint64
	consecutiveGrowthPeriods uint32
}

// NewQueueHealthMonitor wires a monitor. notifier may be nil (warnings are
// still recorded, just never webhooked).
//
// Backlog/growth warnings are routed through a WarningService (X-04) rather
// than the notifier directly, so they reach /warnings, health counts and
// acknowledgement — not the webhook alone. NewQueueHealthMonitor provisions
// a private WarningService wired to notifier so that holds true even before
// SetWarnings wires in the process-wide store; SetWarnings then swaps in
// that shared instance without changing behaviour otherwise (same warnings,
// same forwarding to notifier — see WarningService.SetNotifier/Add).
func NewQueueHealthMonitor(cfg QueueHealthConfig, notifier *Notifier) *QueueHealthMonitor {
	ws := NewWarningService(WarningServiceConfig{})
	ws.SetNotifier(notifier)
	return &QueueHealthMonitor{
		cfg:      cfg,
		warnings: ws,
		history:  make(map[string]*queueSizeHistory),
	}
}

// SetWarnings swaps in the process-wide WarningService (e.g. Server.Warnings)
// so backlog/growth warnings share the same store — and the same
// acknowledgement/health-count surface — as every other emitter, instead of
// the private one NewQueueHealthMonitor provisions. Call once at startup,
// before Watch; nil detaches (warnings become log-only).
func (m *QueueHealthMonitor) SetWarnings(ws *WarningService) { m.warnings = ws }

// Watch runs the periodic check until ctx is cancelled. consumers is
// snapshotted on every tick.
func (m *QueueHealthMonitor) Watch(ctx context.Context, consumers func() []queue.Consumer) {
	if !m.cfg.Enabled {
		return
	}
	tick := time.NewTicker(m.cfg.CheckInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			m.tick(ctx, consumers())
		}
	}
}

func (m *QueueHealthMonitor) tick(ctx context.Context, cs []queue.Consumer) {
	for _, c := range cs {
		metrics, err := c.Metrics(ctx)
		if err != nil || metrics == nil {
			continue
		}
		m.checkBacklog(metrics.QueueIdentifier, metrics.PendingMessages)
		m.checkGrowth(metrics.QueueIdentifier, metrics.PendingMessages)
	}
}

func (m *QueueHealthMonitor) checkBacklog(name string, size uint64) {
	if size <= m.cfg.BacklogThreshold {
		return
	}
	msg := formatBacklog(name, size, m.cfg.BacklogThreshold)
	slog.Warn("queue backlog", "queue", name, "size", size, "threshold", m.cfg.BacklogThreshold)
	if m.warnings != nil {
		m.warnings.Add(WarningCategoryQueueHealth, WarningWarning, msg, "QueueHealthMonitor")
	}
}

func (m *QueueHealthMonitor) checkGrowth(name string, size uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.history[name]
	if !ok {
		h = &queueSizeHistory{}
		m.history[name] = h
	}
	if h.lastSize != nil {
		if size > *h.lastSize {
			growth := size - *h.lastSize
			if growth >= m.cfg.GrowthThreshold {
				h.consecutiveGrowthPeriods++
				if h.consecutiveGrowthPeriods >= m.cfg.GrowthPeriodsThreshold {
					msg := formatGrowth(name, size, growth, h.consecutiveGrowthPeriods)
					slog.Warn("queue growth", "queue", name, "size", size, "growth", growth,
						"consecutive_periods", h.consecutiveGrowthPeriods)
					if m.warnings != nil {
						m.warnings.Add(WarningCategoryQueueHealth, WarningWarning, msg, "QueueHealthMonitor")
					}
				}
			} else {
				h.consecutiveGrowthPeriods = 0
			}
		} else {
			h.consecutiveGrowthPeriods = 0
		}
	}
	v := size
	h.lastSize = &v
}

func formatBacklog(name string, size, threshold uint64) string {
	return "Queue " + name + " depth is " + itoa(size) + " (threshold: " + itoa(threshold) + ")"
}

func formatGrowth(name string, size, growth uint64, periods uint32) string {
	return "Queue " + name + " has grown by " + itoa(growth) +
		" messages for " + utoa(uint64(periods)) + " consecutive periods (current size: " + itoa(size) + ")"
}

func itoa(v uint64) string { return utoa(v) }
func utoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	b := [20]byte{}
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
