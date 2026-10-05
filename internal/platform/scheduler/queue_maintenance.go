package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
)

// Queue maintenance, leader only. msg_dispatch_queue is kept exact by the
// dispatch-job lifecycle and claimed by the poller; three small jobs keep it
// honest and visible:
//
//   - every sweepInterval, release the claims no live process holds: rows
//     claimed longer ago than staleClaimAfter that are not in THIS process's
//     in-flight set (a process that died between claiming and publishing). The
//     start-of-leadership pass that releases all of them lives in the poller;
//   - every sweepInterval, reconcile the queue against the job table (see
//     Lifecycle.Reconcile): normally a no-op, WARN-logged and counted when not;
//   - every backlogInterval, sample the backlog gauges.
//
// Tuning is in vars only so tests can shorten it.
var (
	sweepInterval   = 60 * time.Second
	backlogInterval = 15 * time.Second
	// staleClaimAfter is how old a claim must be before the periodic sweep
	// releases it. A claim lives from the claim statement to the lane's
	// mark-QUEUED, normally well under a second; a broker stall can stretch a
	// publish to publishTimeout (2 minutes), so 5 minutes is a real death, not a
	// slow publish.
	staleClaimAfter = 5 * time.Minute
	// reconcileMinAge is how long a job (or queue row) must have been unchanged
	// before the reconcile sweep treats a mismatch as drift rather than a
	// transition in progress.
	reconcileMinAge = 60 * time.Second
	// reconcileLimit bounds the rows one reconcile pass repairs, per kind.
	reconcileLimit = 5000
)

type queueMaintainer struct {
	lc *dispatchjob.Lifecycle
	// inflightIDs lists the claims this process holds in memory.
	inflightIDs func() []string
	// IsLeader gates every pass: nil = always run. Set by Scheduler.Run.
	IsLeader func() bool
}

func newQueueMaintainer(pool *pgxpool.Pool, inflightIDs func() []string) *queueMaintainer {
	return &queueMaintainer{lc: dispatchjob.NewLifecycle(pool), inflightIDs: inflightIDs}
}

func (m *queueMaintainer) leader() bool { return m.IsLeader == nil || m.IsLeader() }

// Run drives the passes until ctx is cancelled.
func (m *queueMaintainer) Run(ctx context.Context) {
	sweep := time.NewTicker(sweepInterval)
	backlog := time.NewTicker(backlogInterval)
	defer sweep.Stop()
	defer backlog.Stop()
	slog.Info("dispatch queue maintenance starting",
		"sweep_interval", sweepInterval, "backlog_interval", backlogInterval, "stale_claim_after", staleClaimAfter)
	for {
		select {
		case <-ctx.Done():
			slog.Info("dispatch queue maintenance stopped")
			return
		case <-backlog.C:
			if m.leader() {
				m.sampleBacklog(ctx)
			}
		case <-sweep.C:
			if m.leader() {
				m.sweepClaims(ctx)
				m.reconcile(ctx)
			}
		}
	}
}

// sweepClaims releases the claims older than staleClaimAfter that this process
// does not hold. Returns how many.
func (m *queueMaintainer) sweepClaims(ctx context.Context) int64 {
	ctx, cancel := context.WithTimeout(ctx, claimTimeout)
	defer cancel()
	cutoff := time.Now().Add(-staleClaimAfter)
	n, err := m.lc.ReleaseStaleClaims(ctx, cutoff, m.inflightIDs())
	if err != nil {
		slog.Warn("stale dispatch claim sweep failed", "err", err)
		return 0
	}
	if n > 0 {
		schedMetrics.staleClaimsReleased.WithLabelValues("periodic").Add(float64(n))
		slog.Warn("released stale dispatch claims: claimed more than the stale-claim age ago by a process that no longer holds them",
			"count", n, "older_than", staleClaimAfter)
	}
	return n
}

// reconcile runs one reconcile pass.
func (m *queueMaintainer) reconcile(ctx context.Context) dispatchjob.ReconcileResult {
	ctx, cancel := context.WithTimeout(ctx, claimTimeout)
	defer cancel()
	res, err := m.lc.Reconcile(ctx, reconcileMinAge, staleClaimAfter, reconcileLimit)
	if err != nil {
		slog.Warn("dispatch queue reconcile failed", "err", err)
	}
	return res
}

// sampleBacklog sets the backlog gauges.
func (m *queueMaintainer) sampleBacklog(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, claimTimeout)
	defer cancel()
	rows, oldest, err := m.lc.QueueBacklog(ctx)
	if err != nil {
		slog.Warn("dispatch queue backlog sample failed", "err", err)
		return
	}
	schedMetrics.queueBacklogRows.Set(float64(rows))
	schedMetrics.queueOldestAge.Set(oldest.Seconds())
}
