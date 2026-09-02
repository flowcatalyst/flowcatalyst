package operations

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// CancelDispatchJob flips a FAILED job to CANCELLED — an operator override
// for a stuck/failed job that isn't worth retrying. Only valid from FAILED;
// any other source status is a 409 (see statusFlip). Flipping the job off
// FAILED is also what unblocks the rest of its BLOCK_ON_ERROR message group:
// GroupHoldingStatusSQL stops holding the group the moment the head is no
// longer FAILED, and the scheduler's next poll re-admits the siblings in
// order (see docs/owner-rulings-plan.md T3).
func CancelDispatchJob(repo *dispatchjob.Repository) usecaseop.Operation[CancelCommand, DispatchJobCancelled] {
	return statusFlip("CancelDispatchJob", repo,
		func(j *dispatchjob.DispatchJob) { j.Cancel() },
		func(j *dispatchjob.DispatchJob, ec usecase.ExecutionContext) DispatchJobCancelled {
			return DispatchJobCancelled{commonEvent{
				Metadata:      usecase.NewEventMetadata(ec, DispatchJobCancelledType, Source, subjectFor(j.ID)),
				DispatchJobID: j.ID,
			}}
		})
}
