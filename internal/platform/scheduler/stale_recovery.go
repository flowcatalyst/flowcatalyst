package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
)

// StaleQueuedJobPoller recovers dispatch jobs stuck in QUEUED (and in
// PROCESSING; see recoverOnce). When a message is lost on the queue side after
// its job was marked QUEUED, the row stays QUEUED indefinitely. This loop reverts
// such rows to PENDING once queuedAfter has elapsed since the row's updated_at
// (15 minutes by default: any duplicate the early revert causes is dropped by
// the router while the original is in its pipeline, or skipped by the delivery
// callback), and PROCESSING rows after processingAfter.
//
// A lane publishes BEFORE it marks QUEUED (see lane.go), so a scheduler that
// crashes mid-publish leaves its claim PENDING and no longer strands rows here;
// recovery is the backstop for messages lost after the broker accepted them, and
// for the rare QUEUED update that fails after a publish.
type StaleQueuedJobPoller struct {
	pool            *pgxpool.Pool
	queuedAfter     time.Duration
	processingAfter time.Duration
	scanInterval    time.Duration
	// IsLeader gates recovery: when non-nil and false, the loop idles so
	// only the single active scheduler reclaims stuck QUEUED jobs. nil =
	// always run. Set by Scheduler.Run.
	IsLeader func() bool
}

// NewStaleQueuedJobPoller wires the recovery loop.
func NewStaleQueuedJobPoller(pool *pgxpool.Pool, queuedAfter, processingAfter, scanInterval time.Duration) *StaleQueuedJobPoller {
	return &StaleQueuedJobPoller{pool: pool, queuedAfter: queuedAfter, processingAfter: processingAfter, scanInterval: scanInterval}
}

// Run drives the loop until ctx is cancelled.
func (p *StaleQueuedJobPoller) Run(ctx context.Context) {
	tick := time.NewTicker(p.scanInterval)
	defer tick.Stop()
	slog.Info("stale-queued recovery starting",
		"queued_after", p.queuedAfter, "processing_after", p.processingAfter, "interval", p.scanInterval)
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
// The sweeps themselves are the dispatch-job lifecycle's (RecoverStaleQueued /
// RecoverStaleProcessing); this loop only decides when to run them.
const StaleProcessingReason = dispatchjob.StaleProcessingReason

// recoverOnce reverts stale QUEUED and PROCESSING jobs to PENDING. Returns
// the count.
//
// PROCESSING is recovered too. A job whose attempt died with its process is
// normally taken over by the next copy of its queue message once the claim's
// lease runs out (/api/dispatch/process); but when no copy is coming — the
// message went to the DLQ, or was acked away by an older router — nothing
// else would ever move it, and it stayed PROCESSING for good. After
// processingAfter (far past any lease: the delivery client's ceiling is two
// minutes) it goes back to PENDING and the poller dispatches it again.
// At-least-once: the dead attempt may have reached the subscriber.
func (p *StaleQueuedJobPoller) recoverOnce(ctx context.Context) (int64, error) {
	now := time.Now()
	lc := dispatchjob.NewLifecycle(p.pool)
	queued, err := lc.RecoverStaleQueued(ctx, now.Add(-p.queuedAfter).UTC())
	schedMetrics.staleJobsRecovered.WithLabelValues("QUEUED").Add(float64(queued))
	if err != nil {
		return 0, err
	}
	processing, err := lc.RecoverStaleProcessing(ctx, now.Add(-p.processingAfter).UTC())
	schedMetrics.staleJobsRecovered.WithLabelValues("PROCESSING").Add(float64(processing))
	if err != nil {
		return queued, err
	}
	return queued + processing, nil
}
