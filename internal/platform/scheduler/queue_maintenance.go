package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
)

// Queue maintenance, leader only. msg_dispatch_queue is kept exact by the
// dispatch-job lifecycle and claimed (rows deleted) by the poller; two small jobs
// keep it honest and visible:
//
//   - every sweepInterval, reconcile the queue against the job table (see
//     Lifecycle.Reconcile): normally a no-op; it re-creates the row of a PENDING
//     job that has none (the job of a claimer that died), excluding the jobs in
//     THIS process's in-flight set (they are being published). The start-of-
//     leadership pass, with no age guard, lives in the poller;
//   - every backlogInterval, sample the backlog gauges.
//
// Tuning is in vars only so tests can shorten it.
var (
	sweepInterval   = 60 * time.Second
	backlogInterval = 15 * time.Second
	// reconcileMinAge is how long a job (or queue row) must have been unchanged
	// before the reconcile sweep treats a mismatch as drift rather than a
	// transition in progress.
	reconcileMinAge = 60 * time.Second
	// reconcileLimit bounds the rows one reconcile pass repairs, per kind.
	reconcileLimit = 5000
)

type queueMaintainer struct {
	lc *dispatchjob.Lifecycle
	// inflightIDs lists the jobs this process holds in flight.
	inflightIDs func() []string
	// gate is the poller's claim mutex: held while a claim runs, so a job that is
	// claimed and not yet in flight is never seen by a reconcile as lost.
	gate sync.Locker
	// IsLeader gates every pass: nil = always run. Set by Scheduler.Run.
	IsLeader func() bool
}

func newQueueMaintainer(pool *pgxpool.Pool, inflightIDs func() []string, gate sync.Locker) *queueMaintainer {
	return &queueMaintainer{lc: dispatchjob.NewLifecycle(pool), inflightIDs: inflightIDs, gate: gate}
}

func (m *queueMaintainer) leader() bool { return m.IsLeader == nil || m.IsLeader() }

// Run drives the passes until ctx is cancelled.
func (m *queueMaintainer) Run(ctx context.Context) {
	sweep := time.NewTicker(sweepInterval)
	backlog := time.NewTicker(backlogInterval)
	defer sweep.Stop()
	defer backlog.Stop()
	slog.Info("dispatch queue maintenance starting",
		"sweep_interval", sweepInterval, "backlog_interval", backlogInterval)
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
				m.reconcile(ctx)
			}
		}
	}
}

// reconcile runs one reconcile pass.
func (m *queueMaintainer) reconcile(ctx context.Context) dispatchjob.ReconcileResult {
	ctx, cancel := context.WithTimeout(ctx, claimTimeout)
	defer cancel()
	m.gate.Lock()
	defer m.gate.Unlock()
	res, err := m.lc.Reconcile(ctx, reconcileMinAge, reconcileLimit, m.inflightIDs())
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
