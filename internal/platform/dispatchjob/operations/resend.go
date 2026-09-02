package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// ResendCommand is the bulk id-list command for POST /api/dispatch-jobs/requeue
// (the wire route/shape predates the envelope migration and is preserved
// unchanged — see operations/resend.go's doc and api/api.go's requeue
// handler).
type ResendCommand struct {
	IDs []string `json:"ids"`
}

// ResendDispatchJobs resets a batch of jobs to PENDING for a fresh delivery
// cycle — the operator recovery action that used to be a bare UPDATE
// straight from the handler (Repository.Requeue) before this tranche, which
// contradicted entity.go's own doc comment that human-initiated actions go
// through use cases. Migrated onto usecaseop.SaveAll: one Plan, one
// transaction, one rollup DispatchJobsResent event/audit row covering
// however many jobs were actually reset.
//
// Authorization deliberately does NOT use CheckScopeAccess-and-fail like the
// single-resource ops (Cancel/Complete): this is a preserved bulk endpoint
// whose established security contract is "silently drop ids outside the
// caller's scope, reset the rest" (see the original Requeue's doc comment —
// "a view grant can't be used to requeue another tenant's jobs"), not "fail
// the whole batch on the first out-of-scope id". Non-existent ids are
// dropped the same way (FindByIDs simply doesn't return them). The wire
// response (`{requeued: n}`) reports the actual count, so a caller can tell
// the difference between "nothing to do" and "some/all ids were out of
// scope or unknown" only by comparing n to len(request.ids) — same as
// before.
func ResendDispatchJobs(repo *dispatchjob.Repository) usecaseop.Operation[ResendCommand, DispatchJobsResent] {
	return usecaseop.Operation[ResendCommand, DispatchJobsResent]{
		Name: "ResendDispatchJobs",
		Validate: func(_ context.Context, cmd ResendCommand) error {
			if len(cmd.IDs) == 0 {
				return usecase.Validation("IDS_REQUIRED", "at least one id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[ResendCommand],
		Execute: func(ctx context.Context, cmd ResendCommand, ec usecase.ExecutionContext) (usecaseop.Plan[DispatchJobsResent], error) {
			ac := auth.FromContext(ctx)
			jobs, err := repo.FindByIDs(ctx, cmd.IDs)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_ids failed", err)
			}

			accessible := make([]dispatchjob.DispatchJob, 0, len(jobs))
			for _, j := range jobs {
				if auth.CanAccessScope(ac, j.ClientID) {
					accessible = append(accessible, j)
				}
			}
			ids := make([]string, 0, len(accessible))
			for i := range accessible {
				accessible[i].ResetToPending(nil)
				ids = append(ids, accessible[i].ID)
			}

			event := DispatchJobsResent{
				Metadata: usecase.NewEventMetadata(ec, DispatchJobsResentType, Source, "platform.dispatchjobs.resent"),
				IDs:      ids,
			}
			return usecaseop.SaveAll(accessible, repo, event), nil
		},
	}
}
