package dispatchjob

import "github.com/flowcatalyst/flowcatalyst-go/internal/common"

// What an operator requeue may touch, by status. One statement of the rule for
// the id-list requeue, requeue-by-filter and the preview; the lifecycle
// enforces it again in SQL (TRequeue / TRedrive), so a job that changed status
// after it was read is refused rather than reset.
//
//   - PENDING, FAILED (and its legacy alias ERROR), CANCELLED, EXPIRED: requeued.
//     PENDING is reset too: requeueing a job sitting out a retry backoff makes it
//     due now.
//   - QUEUED and PROCESSING (and the legacy alias IN_PROGRESS): never — a
//     delivery is under way or on the queue; resetting it risks a double delivery.
//   - COMPLETED: only as an explicit, doubly confirmed redrive.
var RequeueableStatuses = []common.DispatchStatus{
	common.DispatchPending, common.DispatchFailed, "ERROR", common.DispatchCancelled, common.DispatchExpired,
}

// RedriveConfirmation is the literal a request sends as
// confirmRedriveCompleted, next to redriveCompleted=true, to redrive COMPLETED
// jobs.
const RedriveConfirmation = "REDRIVE"

// Skip reasons a requeue reports for a job it did not reset.
const (
	SkipInFlight              = "IN_FLIGHT"
	SkipCompletedNotConfirmed = "COMPLETED_REDRIVE_NOT_CONFIRMED"
)

// SkipCompletedMessage is the human wording of SkipCompletedNotConfirmed.
const SkipCompletedMessage = "completed jobs are redriven only with redriveCompleted and confirmRedriveCompleted"

// Verdict is what a requeue does with a job of a given status.
type Verdict int

const (
	// VerdictRequeue: reset to PENDING.
	VerdictRequeue Verdict = iota
	// VerdictCompleted: COMPLETED; reset only for a confirmed redrive.
	VerdictCompleted
	// VerdictInFlight: QUEUED or PROCESSING; never reset.
	VerdictInFlight
)

// ClassifyForRequeue applies the rule above to a status.
func ClassifyForRequeue(s common.DispatchStatus) Verdict {
	switch s { //nolint:exhaustive // anything not named is requeueable
	case common.DispatchCompleted:
		return VerdictCompleted
	case common.DispatchQueued, common.DispatchProcessing, "IN_PROGRESS":
		return VerdictInFlight
	}
	return VerdictRequeue
}

// RedriveConfirmed reports whether a request's two redrive fields together
// authorise redriving COMPLETED jobs.
func RedriveConfirmed(redriveCompleted bool, confirm string) bool {
	return redriveCompleted && confirm == RedriveConfirmation
}
