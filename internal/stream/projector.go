// Package stream implements the CQRS projection processors that move
// rows from write tables (msg_events, msg_dispatch_jobs) into their
// denormalized read counterparts, plus the event fan-out that matches
// subscriptions and emits dispatch jobs.
//
// Ports fc-stream/src/* faithfully. SQL queries match Rust word-for-word
// for parity; only the language plumbing changes.
package stream

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ProjectorConfig is the per-projection knob set.
type ProjectorConfig struct {
	// Enabled toggles the projection on/off.
	Enabled bool
	// BatchSize is the max rows claimed per tick.
	BatchSize int
	// PollInterval is the sleep between empty polls.
	PollInterval time.Duration
	// IdleSleep is the sleep when no rows are claimed.
	IdleSleep time.Duration
}

// DefaultProjectorConfig matches the Rust StreamConfig defaults.
func DefaultProjectorConfig() ProjectorConfig {
	return ProjectorConfig{
		Enabled:      true,
		BatchSize:    500,
		PollInterval: 100 * time.Millisecond,
		IdleSleep:    1 * time.Second,
	}
}

// Projector is the abstract loop: claim → process → loop. Concrete
// projections (events, dispatch_jobs, fan_out) supply the claim+process
// step via the Step function.
type Projector struct {
	Name string
	Pool *pgxpool.Pool
	Cfg  ProjectorConfig
	// Step claims a batch and processes it. Returns rowsProcessed and
	// any error. err is logged, not fatal — the loop continues.
	Step func(ctx context.Context, batchSize int) (rowsProcessed int, err error)
	// Health is an optional tracker. When non-nil the loop toggles
	// Running on entry/exit, bumps AddProcessed per non-empty step, and
	// RecordError per Step failure. nil is fine — the projector then
	// reports no health (the stream HealthService will mark it stopped).
	Health *Health
}

// Run drives the projector until ctx is cancelled.
func (p *Projector) Run(ctx context.Context) {
	if !p.Cfg.Enabled {
		slog.Info("projector disabled", "name", p.Name)
		return
	}
	slog.Info("projector starting", "name", p.Name, "batch_size", p.Cfg.BatchSize)
	if p.Health != nil {
		p.Health.SetRunning(true)
		defer p.Health.SetRunning(false)
	}
	for {
		select {
		case <-ctx.Done():
			slog.Info("projector stopped", "name", p.Name)
			return
		default:
		}

		n, err := p.Step(ctx, p.Cfg.BatchSize)
		if err != nil {
			slog.Warn("projector step error", "name", p.Name, "err", err)
			if p.Health != nil {
				p.Health.RecordError()
			}
			sleep(ctx, p.Cfg.IdleSleep)
			continue
		}
		if n == 0 {
			sleep(ctx, p.Cfg.IdleSleep)
			continue
		}
		if p.Health != nil {
			p.Health.AddProcessed(uint64(n))
		}
		// Got work — go again immediately, with a tiny pause to let
		// transactions on the read side commit.
		sleep(ctx, p.Cfg.PollInterval)
	}
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
