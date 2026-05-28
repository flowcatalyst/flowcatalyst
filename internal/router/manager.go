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

	pubMu      sync.Mutex
	publishers map[string]queue.Publisher // pool code → publisher (lazy)
}

type runningPool struct {
	pool     *Pool
	cancel   context.CancelFunc
	queueCfg common.QueueConfig
}

// NewManager builds a manager. The mediator is shared by all pools; the
// breaker registry is shared so per-URL state survives pool reloads.
// tracker may be nil; if so, pools run without in-flight tracking.
func NewManager(mediator Mediator, breakers *BreakerRegistry, tracker *InFlightTracker) *Manager {
	return &Manager{
		mediator:   mediator,
		breakers:   breakers,
		tracker:    tracker,
		pools:      make(map[string]*runningPool),
		stopCh:     make(chan struct{}),
		publishers: make(map[string]queue.Publisher),
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

// PoolStats returns one snapshot per running pool. Order is unspecified
// (map iteration). Used by the HTTP API (/monitoring/pools,
// /monitoring/pool-stats) and the health service to count
// healthy/unhealthy pools.
func (m *Manager) PoolStats() []PoolStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]PoolStats, 0, len(m.pools))
	for _, rp := range m.pools {
		out = append(out, rp.pool.Stats())
	}
	return out
}

// PoolCodes returns the codes of all currently registered pools.
func (m *Manager) PoolCodes() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.pools))
	for code := range m.pools {
		out = append(out, code)
	}
	return out
}

// Pool returns the running pool with the given code, or nil if absent.
// Held only long enough to copy the reference; the Pool itself uses
// internal locking for its own state.
func (m *Manager) Pool(code string) *Pool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rp, ok := m.pools[code]; ok {
		return rp.pool
	}
	return nil
}

// QueueMetrics returns per-consumer broker attributes and counters for
// every running pool. Calls Consumer.Metrics(ctx) which may perform a
// broker round-trip — keep that in mind on the hot path; the dashboard
// goes through CachedBrokerStats which only calls this every ~60s.
//
// Consumers that fail to report (return an error) are skipped with a
// warn log; this lets one degraded backend not poison the response.
func (m *Manager) QueueMetrics(ctx context.Context) []queue.Metrics {
	consumers := m.Consumers()
	out := make([]queue.Metrics, 0, len(consumers))
	for _, c := range consumers {
		mtr, err := c.Metrics(ctx)
		if err != nil {
			slog.Warn("queue metrics fetch failed", "queue", c.Identifier(), "err", err)
			continue
		}
		if mtr != nil {
			out = append(out, *mtr)
		}
	}
	return out
}

// QueueCounters returns process-local counters only. No broker
// round-trip — safe to call on every request. Used by CachedBrokerStats
// when overlaying live counter values onto cached SQS attributes.
func (m *Manager) QueueCounters() []queue.Metrics {
	consumers := m.Consumers()
	out := make([]queue.Metrics, 0, len(consumers))
	for _, c := range consumers {
		if mtr := c.Counters(); mtr != nil {
			out = append(out, *mtr)
		}
	}
	return out
}

// QueueConfig returns the queue config bound to a running pool, or
// the zero-value with ok=false if the pool isn't registered.
func (m *Manager) QueueConfig(code string) (common.QueueConfig, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rp, ok := m.pools[code]; ok {
		return rp.queueCfg, true
	}
	return common.QueueConfig{}, false
}

// Publisher returns (and lazily caches) a queue.Publisher for the named
// pool. The publisher reuses the pool's queue URI so messages published
// via the API land on the same broker the consumer reads from.
func (m *Manager) Publisher(ctx context.Context, code string) (queue.Publisher, error) {
	m.pubMu.Lock()
	if p, ok := m.publishers[code]; ok {
		m.pubMu.Unlock()
		return p, nil
	}
	m.pubMu.Unlock()

	qc, ok := m.QueueConfig(code)
	if !ok {
		return nil, fmt.Errorf("publisher: pool %q not registered", code)
	}
	pub, err := queue.NewPublisher(ctx, qc)
	if err != nil {
		return nil, fmt.Errorf("publisher: build for %q: %w", code, err)
	}
	m.pubMu.Lock()
	// Double-check after relocking — a concurrent caller may have won.
	if existing, ok := m.publishers[code]; ok {
		m.pubMu.Unlock()
		return existing, nil
	}
	m.publishers[code] = pub
	m.pubMu.Unlock()
	return pub, nil
}

// UpdatePool applies a runtime config update to an existing pool.
// Concurrency=0 (or omitted by the caller) means "leave unchanged";
// rateLimitPerMinute=nil clears the limit, *rateLimitPerMinute=0 also
// clears it (mirrors the Rust API).
// Returns false if the pool isn't registered or concurrency==0 is
// rejected by the pool. Used by PUT /monitoring/pools/{poolCode}.
func (m *Manager) UpdatePool(code string, concurrency uint32, rateLimitPerMinute *uint32, setRateLimit bool) bool {
	pool := m.Pool(code)
	if pool == nil {
		return false
	}
	if concurrency != 0 {
		if !pool.UpdateConcurrency(concurrency) {
			return false
		}
	}
	if setRateLimit {
		pool.UpdateRateLimit(rateLimitPerMinute)
	}
	return true
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
		m.pools[code] = &runningPool{pool: pool, cancel: cancel, queueCfg: qc}
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
