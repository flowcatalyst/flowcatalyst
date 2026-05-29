package outbox

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// Config tunes the outbox processor.
type Config struct {
	PollInterval time.Duration
	BatchSize    int
	MaxInFlight  int64
	HTTPTimeout  time.Duration
	PlatformURL  string
	AuthToken    string
}

// DefaultConfig matches the Rust outbox defaults.
func DefaultConfig() Config {
	return Config{
		PollInterval: 1 * time.Second,
		BatchSize:    100,
		MaxInFlight:  1000,
		HTTPTimeout:  30 * time.Second,
	}
}

// Processor wires the outbox pipeline:
//
//	repo.ClaimPending → groupDistributor → httpDispatcher → repo.MarkSuccess/Failed
//
// Mirrors fc-outbox/src/enhanced_processor.rs.
type Processor struct {
	cfg          Config
	repo         Repository
	dispatcher   *HTTPDispatcher
	distributor  *GroupDistributor
	inFlight     atomic.Int64
	totalSucceed atomic.Uint64
	totalFailed  atomic.Uint64

	// IsLeader gates polling; nil means always-leader (single instance /
	// standby disabled). When standby is enabled only the leader polls — the
	// Mongo backend has no atomic claim, so a single active poller avoids
	// double-claims. Mirrors the Rust outbox leadership gate.
	IsLeader func() bool
}

// NewProcessor wires a processor.
func NewProcessor(cfg Config, repo Repository) *Processor {
	d := NewHTTPDispatcher(cfg.PlatformURL, cfg.AuthToken, cfg.HTTPTimeout)
	return &Processor{
		cfg:         cfg,
		repo:        repo,
		dispatcher:  d,
		distributor: NewGroupDistributor(),
	}
}

// Run drives the processor until ctx is cancelled.
func (p *Processor) Run(ctx context.Context) {
	tick := time.NewTicker(p.cfg.PollInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("outbox processor stopped")
			return
		case <-tick.C:
			if p.IsLeader != nil && !p.IsLeader() {
				continue // only the leader polls
			}
			if p.inFlight.Load() >= p.cfg.MaxInFlight {
				continue // backpressure
			}
			p.tick(ctx)
		}
	}
}

func (p *Processor) tick(ctx context.Context) {
	items, err := p.repo.ClaimPending(ctx, p.cfg.BatchSize)
	if err != nil {
		slog.Warn("outbox claim failed", "err", err)
		return
	}
	if len(items) == 0 {
		return
	}

	// Route through the group distributor: items with the same
	// message_group execute serially (FIFO); items without a group
	// or in different groups execute concurrently.
	for _, item := range items {
		item := item
		p.inFlight.Add(1)
		p.distributor.Submit(item, func() {
			defer p.inFlight.Add(-1)
			p.dispatch(ctx, item)
		})
	}
}

func (p *Processor) dispatch(ctx context.Context, item Item) {
	out := p.dispatcher.Send(ctx, item)
	if out.Status == common.OutboxSuccess {
		if err := p.repo.MarkSuccess(ctx, []string{item.ID}); err != nil {
			slog.Warn("outbox mark success failed", "id", item.ID, "err", err)
			return
		}
		p.totalSucceed.Add(1)
		return
	}
	// Failed. The repository bumps retry_count + records the error; retryable
	// statuses are returned to PENDING and re-claimed on the next poll, while
	// terminal statuses stop here. The in-memory max-retries cap (to avoid a
	// hot retry loop on a persistently-failing row) and stuck-item recovery
	// land in Phase 8 (OB6/OB3).
	if err := p.repo.MarkFailed(ctx, []string{item.ID}, out.Status, out.Message); err != nil {
		slog.Warn("outbox mark failed", "id", item.ID, "err", err)
	}
	p.totalFailed.Add(1)
}

// InFlight returns the count of items currently in dispatch.
func (p *Processor) InFlight() int64 { return p.inFlight.Load() }

// Totals returns (success, failure) counters since process start.
func (p *Processor) Totals() (uint64, uint64) {
	return p.totalSucceed.Load(), p.totalFailed.Load()
}
