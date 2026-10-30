package operations

import (
	"context"
	"slices"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// CancelManyCommand is the bulk id-list command for cancelling (the UI's
// "Ignore selected") dispatch jobs.
type CancelManyCommand struct {
	IDs []string `json:"ids"`
}

// CancelDispatchJobs cancels a batch of jobs: each PENDING or FAILED one
// (dispatchjob.CancellableStatuses) becomes CANCELLED, in one transaction with
// one rollup DispatchJobsCancelled event and audit row. A cancelled job is
// never delivered (claim-for-delivery refuses it) and stops holding its
// message group; a requeue brings it back.
//
// It follows ResendDispatchJobs' bulk contract rather than CancelDispatchJob's:
// ids outside the caller's scope and unknown ids are dropped silently, a job
// in any other status is skipped with reason NOT_CANCELLABLE and reported on
// the event, and a job that left the cancellable statuses after it was read
// (a PENDING job the scheduler queued meanwhile) is refused by the lifecycle's
// SQL and left alone, not an error for the batch — it is still listed in the
// event, as resend's are.
func CancelDispatchJobs(repo *dispatchjob.Repository) usecaseop.Operation[CancelManyCommand, DispatchJobsCancelled] {
	return usecaseop.Operation[CancelManyCommand, DispatchJobsCancelled]{
		Name: "CancelDispatchJobs",
		Validate: func(_ context.Context, cmd CancelManyCommand) error {
			if len(cmd.IDs) == 0 {
				return usecase.Validation("IDS_REQUIRED", "at least one id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[CancelManyCommand],
		Execute: func(ctx context.Context, cmd CancelManyCommand, ec usecase.ExecutionContext) (usecaseop.Plan[DispatchJobsCancelled], error) {
			ac := auth.FromContext(ctx)
			jobs, err := repo.FindByIDs(ctx, cmd.IDs)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_ids failed", err)
			}
			cancel := make([]dispatchjob.DispatchJob, 0, len(jobs))
			var skipped []SkippedJob
			for _, j := range jobs {
				if !auth.CanAccessScope(ac, ids.StringPtr(j.ClientID)) {
					continue
				}
				if !slices.Contains(dispatchjob.CancellableStatuses, j.Status) {
					skipped = append(skipped, SkippedJob{ID: j.ID, Status: string(j.Status), Reason: dispatchjob.SkipNotCancellable})
					continue
				}
				cancel = append(cancel, j)
			}
			cancelled := make([]string, 0, len(cancel))
			for i := range cancel {
				cancel[i].Cancel()
				cancelled = append(cancelled, cancel[i].ID)
			}
			event := DispatchJobsCancelled{
				Metadata: usecase.NewEventMetadata(ec, DispatchJobsCancelledType, Source, "platform.dispatchjobs.cancelled"),
				IDs:      cancelled,
				Skipped:  skipped,
			}
			return usecaseop.SaveAll(cancel, repo.BulkCancelPersister(), event), nil
		},
	}
}
