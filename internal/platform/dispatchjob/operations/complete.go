package operations

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// CompleteDispatchJob flips a FAILED job to COMPLETED — an operator override
// recording that the job was actually delivered/handled out of band
// (verified manually, or via a side channel) and should stop reading as a
// failure. Only valid from FAILED; any other source status is a 409 (see
// statusFlip). Like CancelDispatchJob, this also unblocks the rest of the
// job's BLOCK_ON_ERROR message group.
func CompleteDispatchJob(repo *dispatchjob.Repository) usecaseop.Operation[CompleteCommand, DispatchJobCompleted] {
	return statusFlip("CompleteDispatchJob", repo,
		func(j *dispatchjob.DispatchJob) { j.Complete() },
		func(j *dispatchjob.DispatchJob, ec usecase.ExecutionContext) DispatchJobCompleted {
			return DispatchJobCompleted{commonEvent{
				Metadata:      usecase.NewEventMetadata(ec, DispatchJobCompletedType, Source, subjectFor(j.ID)),
				DispatchJobID: j.ID,
			}}
		})
}
