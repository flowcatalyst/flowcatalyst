package operations

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// CancelDispatchJob flips a PENDING or FAILED job to CANCELLED — an operator
// override ("ignore") for a job that isn't worth delivering or retrying. Only
// valid from dispatchjob.CancellableStatuses; any other source status is a 409
// NOT_CANCELLABLE (see statusFlip), and the lifecycle checks again in SQL, so
// a PENDING job the scheduler queued meanwhile is refused (ErrTransitionRefused)
// rather than cancelled mid-delivery. Flipping a FAILED job off FAILED is also
// what unblocks the rest of its BLOCK_ON_ERROR message group:
// GroupHoldersSQL stops holding the group the moment the head is no
// longer FAILED, and the scheduler's next poll re-admits the siblings in
// order (see docs/owner-rulings-plan.md T3).
func CancelDispatchJob(repo *dispatchjob.Repository) usecaseop.Operation[CancelCommand, DispatchJobCancelled] {
	return statusFlip("CancelDispatchJob", dispatchjob.CancellableStatuses,
		"NOT_CANCELLABLE", "a PENDING or FAILED job can be cancelled", repo, repo.CancelPersister(),
		func(j *dispatchjob.DispatchJob) { j.Cancel() },
		func(j *dispatchjob.DispatchJob, ec usecase.ExecutionContext) DispatchJobCancelled {
			return DispatchJobCancelled{commonEvent{
				Metadata:      usecase.NewEventMetadata(ec, DispatchJobCancelledType, Source, subjectFor(j.ID)),
				DispatchJobID: j.ID,
			}}
		})
}
