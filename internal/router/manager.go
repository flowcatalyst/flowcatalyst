package router

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// Manager owns the set of running pools and orchestrates lifecycle
// (start, stop, config reload).
type Manager struct {
	mediator Mediator
	breakers *BreakerRegistry
	tracker  *InFlightTracker

	mu     sync.Mutex
	pools  map[string]*runningPool // pool code → state
	stopCh chan struct{}
	wg     sync.WaitGroup
}

type runningPool struct {
	pool   *Pool
	cancel context.CancelFunc
}

// NewManager builds a manager. The mediator is shared by all pools; the
// breaker registry is shared so per-URL state survives pool reloads.
// tracker may be nil; if so, pools run without in-flight tracking.
func NewManager(mediator Mediator, breakers *BreakerRegistry, tracker *InFlightTracker) *Manager {
	return &Manager{
		mediator: mediator,
		breakers: breakers,
		tracker:  tracker,
		pools:    make(map[string]*runningPool),
		stopCh:   make(chan struct{}),
	}
}

// Consumers returns the consumer for each running pool (for the
// QueueHealthMonitor to call Metrics on).
func (m *Manager) Consumers() []queue.Consumer {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]queue.Consumer, 0, len(m.pools))
	for _, rp := range m.pools {
		out = append(out, rp.pool.Consumer())
	}
	return out
}

// Reconfigure applies a new RouterConfig: starts new pools, updates
// rate limits on existing ones, stops removed pools. Hot-reloadable.
func (m *Manager) Reconfigure(ctx context.Context, cfg common.RouterConfig) error {
	// Build a quick lookup of incoming pools.
	want := make(map[string]common.PoolConfig, len(cfg.ProcessingPools))
	for _, p := range cfg.ProcessingPools {
		want[p.Code] = p
	}

	// Match queues to pools by name. The Rust router uses pool_code
	// derived from the queue config; here we use the queue's Name
	// matching the pool's Code. This is the standard 1:1 mapping.
	queuesByPool := make(map[string]common.QueueConfig, len(cfg.Queues))
	for _, q := range cfg.Queues {
		queuesByPool[q.Name] = q
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop pools that aren't in the new config.
	for code, rp := range m.pools {
		if _, ok := want[code]; !ok {
			slog.Info("manager: stopping pool", "code", code)
			rp.cancel()
			rp.pool.Stop()
			delete(m.pools, code)
		}
	}

	// Update existing, start new.
	for code, pc := range want {
		if rp, ok := m.pools[code]; ok {
			rate := uint32(0)
			if pc.RateLimitPerMinute != nil {
				rate = *pc.RateLimitPerMinute
			}
			rp.pool.SetRateLimit(rate)
			continue
		}
		qc, ok := queuesByPool[code]
		if !ok {
			slog.Warn("manager: pool has no matching queue", "code", code)
			continue
		}
		consumer, err := queue.NewConsumer(ctx, qc)
		if err != nil {
			return fmt.Errorf("build consumer for pool %s: %w", code, err)
		}
		pool := NewPool(pc, consumer, m.mediator, m.breakers, m.tracker)
		pctx, cancel := context.WithCancel(ctx)
		m.pools[code] = &runningPool{pool: pool, cancel: cancel}
		m.wg.Add(1)
		go func(p *Pool) {
			defer m.wg.Done()
			p.Run(pctx)
		}(pool)
	}
	return nil
}

// Shutdown cancels all pools and waits for them to exit.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	for _, rp := range m.pools {
		rp.cancel()
		rp.pool.Stop()
	}
	m.pools = nil
	m.mu.Unlock()

	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PoolCount returns the count of running pools (for /health or /metrics).
func (m *Manager) PoolCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.pools)
}
