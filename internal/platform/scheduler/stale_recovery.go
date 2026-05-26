package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StaleQueuedJobPoller recovers dispatch jobs stuck in QUEUED. When the
// scheduler crashes between marking PENDING→QUEUED and successfully
// publishing to the broker, or when the broker drops a message, the
// row stays QUEUED indefinitely. This loop reverts such rows to PENDING
// after StaleAfter elapses since the row's updated_at.
type StaleQueuedJobPoller struct {
	pool         *pgxpool.Pool
	staleAfter   time.Duration
	scanInterval time.Duration
}

// NewStaleQueuedJobPoller wires the recovery loop.
func NewStaleQueuedJobPoller(pool *pgxpool.Pool, staleAfter, scanInterval time.Duration) *StaleQueuedJobPoller {
	return &StaleQueuedJobPoller{pool: pool, staleAfter: staleAfter, scanInterval: scanInterval}
}

// Run drives the loop until ctx is cancelled.
func (p *StaleQueuedJobPoller) Run(ctx context.Context) {
	tick := time.NewTicker(p.scanInterval)
	defer tick.Stop()
	slog.Info("stale-queued recovery starting",
		"stale_after", p.staleAfter, "interval", p.scanInterval)
	for {
		select {
		case <-ctx.Done():
			slog.Info("stale-queued recovery stopped")
			return
		case <-tick.C:
			if n, err := p.recoverOnce(ctx); err != nil {
				slog.Warn("stale recovery error", "err", err)
			} else if n > 0 {
				slog.Info("stale-queued jobs reverted", "count", n)
			}
		}
	}
}

// recoverOnce reverts stale QUEUED jobs to PENDING. Returns the count.
func (p *StaleQueuedJobPoller) recoverOnce(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-p.staleAfter).UTC()
	tag, err := p.pool.Exec(ctx,
		`UPDATE msg_dispatch_jobs
		    SET status = 'PENDING', updated_at = NOW()
		  WHERE status = 'QUEUED' AND updated_at < $1`,
		cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
