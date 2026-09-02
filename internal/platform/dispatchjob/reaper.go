package dispatchjob

// reaper.go implements the T3/A-01 backstop half of BLOCK_ON_ERROR group
// recovery: a periodic sweep that finds dispatch jobs stuck in
// QUEUED/PROCESSING behind a terminally FAILED BLOCK_ON_ERROR head and
// resets them to PENDING.
//
// It exists because the router→platform settled-message hook (see the
// settled package) can miss: internal/router/pool.go's ackBuffered ACKs a
// message group's untried buffered siblings the instant its head fails
// terminally, but if the router crashes (or its call to
// /api/dispatch/settled is dropped) between that ACK and the hook call,
// nothing else ever marks those rows — they sit at QUEUED forever, the
// exact silent data loss this tranche exists to close. The sweep is
// deliberately redundant with the hook on the common path: both write
// through the same "status IN (QUEUED, PROCESSING)" guard, so running both
// costs nothing when the hook already did the job — the sweep just finds
// nothing to reset.
//
// See docs/owner-rulings-plan.md §4 ("(c) Both — hook as the fast path,
// reaper as the backstop") for why both mechanisms are needed rather than
// either alone.

import (
	"context"
	"log/slog"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

// DefaultReaperInterval is a reasonable sweep cadence for RunReaper. This is
// a backstop for a rare failure (a router crash in a narrow window), not a
// hot path, so a low cadence is fine — chosen to match StartPurger's
// once-a-minute housekeeping cadence (internal/server/subsystems.go) as a
// familiar reference point, doubled since this sweep does a cross-partition
// self-join rather than an indexed expires_at lookup.
const DefaultReaperInterval = 2 * time.Minute

// DefaultProcessingLiveAfter is the liveness cutoff for a PROCESSING
// sibling: a row updated more recently than this is presumed to be a
// genuine in-flight delivery and left alone. Sized well above every
// legitimate reason a PROCESSING row stays PROCESSING:
//   - a single delivery attempt is bounded by the job's own timeout
//     (default 30s) and the processing handler's outer HTTP client ceiling
//     of 2 minutes (internal/platform/dispatchjob/processing/processing.go);
//   - the router's own mediator-level timeout for calling back into the
//     platform's processing endpoint is a hard 15-minute-per-attempt,
//     up-to-3-attempts contract that the owner has ruled must not be
//     undercut ("size everything else around it" — see the
//     router-backpressure-restart-deadlock incident notes: one hung
//     delivery can legitimately hold a slot, and its whole pool, for up to
//     ~45 minutes).
//
// 45 minutes keeps the reaper from ever racing a delivery that is
// legitimately still in flight under that contract. The cost of being this
// conservative is only that a genuinely stranded PROCESSING row waits up to
// 45 minutes for the reaper to catch it — QUEUED rows (which, for this
// specific failure mode, can never be a genuine in-flight delivery: a
// BLOCK_ON_ERROR sibling never leaves the router's per-group buffer to
// start one) have no such wait.
const DefaultProcessingLiveAfter = 45 * time.Minute

// reapReason is recorded in LastError on every job the reaper resets, so an
// operator looking at a PENDING job's detail can tell it was the reaper (as
// opposed to the settled hook, or a human Resend) that put it there.
const reapReason = "reaper: sibling of a FAILED BLOCK_ON_ERROR head, stranded QUEUED/PROCESSING"

// SweepStrandedGroupSiblings runs one sweep: it resets to PENDING every
// QUEUED/PROCESSING job whose message group is headed by a terminally
// FAILED job under BLOCK_ON_ERROR (see GroupHoldingStatusSQL and
// pool.go's ackBuffered doc comment for why such rows can exist), subject to
// the processingLiveAfter liveness cutoff on PROCESSING rows (see
// DefaultProcessingLiveAfter for the sizing rationale — QUEUED rows have no
// such cutoff). Once reset, GroupHoldingStatusSQL / the scheduler's
// filterByDispatchMode keep the group visibly held (last_error records why)
// until an operator resolves the head (Cancel/Complete), at which point the
// poller's normal claim query re-admits the group in order.
//
// Returns the ids reset, for logging and tests. Safe to call concurrently
// from multiple instances — see RunReaper's doc comment.
func (r *Repository) SweepStrandedGroupSiblings(ctx context.Context, processingLiveAfter time.Duration) ([]string, error) {
	cutoff := time.Now().Add(-processingLiveAfter).UTC()
	return r.q.DispatchJobSweepStrandedSiblings(ctx, dbq.DispatchJobSweepStrandedSiblingsParams{
		LiveBefore: cutoff,
		Reason:     reapReason,
	})
}

// RunReaper drives SweepStrandedGroupSiblings on a ticker until ctx is
// cancelled. Self-contained and exported so the caller can start it as its
// own goroutine — mirrors StartPurger's loop shape
// (internal/server/subsystems.go). See this package's doc comment (top of
// this file) for where to wire it and why it isn't leader-gated: unlike the
// scheduler's claim/dispatch loops, each sweep is a plain conditional
// UPDATE guarded by "status IN (QUEUED, PROCESSING)" with no risk of
// double-publishing or duplicate delivery if more than one platform
// instance runs it concurrently, so no leadership plumbing is required.
func RunReaper(ctx context.Context, repo *Repository, interval, processingLiveAfter time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	slog.Info("dispatch-job group reaper started",
		"interval", interval, "processing_live_after", processingLiveAfter)
	for {
		select {
		case <-ctx.Done():
			slog.Info("dispatch-job group reaper stopped")
			return
		case <-tick.C:
			ids, err := repo.SweepStrandedGroupSiblings(ctx, processingLiveAfter)
			if err != nil {
				slog.Warn("dispatch-job group reaper sweep failed", "err", err)
				continue
			}
			if len(ids) > 0 {
				slog.Info("dispatch-job group reaper reset stranded siblings", "count", len(ids), "ids", ids)
			}
		}
	}
}
