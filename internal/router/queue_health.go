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

// DefaultQueueHealthConfig matches the Rust defaults.
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
	notifier *Notifier

	mu      sync.Mutex
	history map[string]*queueSizeHistory
}

type queueSizeHistory struct {
	lastSize                  *uint64
	consecutiveGrowthPeriods  uint32
}

// NewQueueHealthMonitor wires a monitor. notifier may be nil (warnings → log only).
func NewQueueHealthMonitor(cfg QueueHealthConfig, notifier *Notifier) *QueueHealthMonitor {
	return &QueueHealthMonitor{
		cfg:      cfg,
		notifier: notifier,
		history:  make(map[string]*queueSizeHistory),
	}
}

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
	if m.notifier != nil {
		m.notifier.Add(Warning{
			Category: WarningCategoryStall,
			Severity: WarningWarning,
			Message:  msg,
			Source:   "QueueHealthMonitor",
		})
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
					if m.notifier != nil {
						m.notifier.Add(Warning{
							Category: WarningCategoryStall,
							Severity: WarningWarning,
							Message:  msg,
							Source:   "QueueHealthMonitor",
						})
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
