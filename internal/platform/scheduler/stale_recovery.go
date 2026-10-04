package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StaleQueuedJobPoller recovers dispatch jobs stuck in QUEUED (and in
// PROCESSING; see recoverOnce). When a message is lost on the queue side after
// its job was marked QUEUED, the row stays QUEUED indefinitely. This loop reverts such rows to PENDING after StaleAfter elapses since the row's
// updated_at.
//
// The poller publishes BEFORE it commits QUEUED (see pollOnce), so a scheduler
// that crashes mid-publish rolls its claim back to PENDING and no longer
// strands rows here; recovery is the backstop for messages lost after the
// broker accepted them, and for the rare commit that fails after a publish.
type StaleQueuedJobPoller struct {
	pool         *pgxpool.Pool
	staleAfter   time.Duration
	scanInterval time.Duration
	// IsLeader gates recovery: when non-nil and false, the loop idles so
	// only the single active scheduler reclaims stuck QUEUED jobs. nil =
	// always run. Set by Scheduler.Run.
	IsLeader func() bool
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
			if p.IsLeader != nil && !p.IsLeader() {
				continue // only the leader reclaims
			}
			if n, err := p.recoverOnce(ctx); err != nil {
				slog.Warn("stale recovery error", "err", err)
			} else if n > 0 {
				slog.Info("stale-queued jobs reverted", "count", n)
			}
		}
	}
}

// StaleProcessingReason is recorded in last_error on a PROCESSING job this
// loop returns to PENDING, so an operator can tell why it was re-dispatched.
const StaleProcessingReason = "stale recovery: PROCESSING with no outcome recorded; returned to PENDING"

// recoverOnce reverts stale QUEUED and PROCESSING jobs to PENDING. Returns
// the count.
//
// PROCESSING is recovered too. A job whose attempt died with its process is
// normally taken over by the next copy of its queue message once the claim's
// lease runs out (/api/dispatch/process); but when no copy is coming — the
// message went to the DLQ, or was acked away by an older router — nothing
// else would ever move it, and it stayed PROCESSING for good. After
// StaleAfter (far past any lease: the delivery client's ceiling is two
// minutes) it goes back to PENDING and the poller dispatches it again.
// At-least-once: the dead attempt may have reached the subscriber.
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
	processing, err := p.pool.Exec(ctx,
		`UPDATE msg_dispatch_jobs
		    SET status = 'PENDING', last_error = $2, updated_at = NOW()
		  WHERE status = 'PROCESSING' AND updated_at < $1`,
		cutoff, StaleProcessingReason)
	if err != nil {
		return tag.RowsAffected(), err
	}
	return tag.RowsAffected() + processing.RowsAffected(), nil
}
