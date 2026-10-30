package operations

import (
	"context"
	"maps"
	"slices"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// SkippedJob is a job a requeue found and did not reset, with the reason.
type SkippedJob struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// ResendCommand is the bulk id-list command for POST /api/dispatch-jobs/requeue
// (the wire route/shape predates the envelope migration and is preserved
// unchanged — see operations/resend.go's doc and api/api.go's requeue
// handler).
//
// RedriveCompleted and ConfirmRedriveCompleted together authorise re-delivering
// COMPLETED jobs (dispatchjob.RedriveConfirmed); without both, COMPLETED jobs
// are skipped and reported. RequeueInFlight resets QUEUED and PROCESSING jobs
// too (an operator's judgement that they are stuck; a delivery still under way
// may land as well); without it they are skipped and reported. The HTTP API
// does not set it: only the admin UI's confirmed requeue does. The command is
// the audit row's operation JSON, so the flags a caller sent are on the record.
type ResendCommand struct {
	IDs                     []string `json:"ids"`
	RedriveCompleted        bool     `json:"redriveCompleted,omitempty"`
	ConfirmRedriveCompleted string   `json:"confirmRedriveCompleted,omitempty"`
	RequeueInFlight         bool     `json:"requeueInFlight,omitempty"`
}

// ResendDispatchJobs resets a batch of jobs to PENDING for a fresh delivery
// cycle — the operator recovery action that used to be a bare UPDATE
// straight from the handler (Repository.Requeue) before this tranche, which
// contradicted entity.go's own doc comment that human-initiated actions go
// through use cases. Migrated onto usecaseop.SaveAll: one Plan, one
// transaction, one rollup DispatchJobsResent event/audit row covering
// however many jobs were actually reset.
//
// Status guard (dispatchjob/requeue_rules.go): QUEUED and PROCESSING jobs are
// reset only when the command carries RequeueInFlight (otherwise skipped,
// reason IN_FLIGHT); COMPLETED jobs are reset only when
// the command carries both redrive fields (a redrive; the event says so),
// otherwise skipped (reason COMPLETED_REDRIVE_NOT_CONFIRMED). A skipped job is
// reported on the event, never an error for the batch. The lifecycle checks the
// status again in SQL, so a job that moved after it was read is refused too.
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

			redrive := dispatchjob.RedriveConfirmed(cmd.RedriveCompleted, cmd.ConfirmRedriveCompleted)
			accessible := make([]dispatchjob.DispatchJob, 0, len(jobs))
			redriven := map[string]bool{}
			inFlight := map[string]bool{}
			var skipped []SkippedJob
			for _, j := range jobs {
				if !auth.CanAccessScope(ac, ids.StringPtr(j.ClientID)) {
					continue
				}
				switch dispatchjob.ClassifyForRequeue(j.Status) {
				case dispatchjob.VerdictInFlight:
					if !cmd.RequeueInFlight {
						skipped = append(skipped, SkippedJob{ID: j.ID, Status: string(j.Status), Reason: dispatchjob.SkipInFlight})
						continue
					}
					inFlight[j.ID] = true
				case dispatchjob.VerdictCompleted:
					if !redrive {
						skipped = append(skipped, SkippedJob{ID: j.ID, Status: string(j.Status), Reason: dispatchjob.SkipCompletedNotConfirmed})
						continue
					}
					redriven[j.ID] = true
				case dispatchjob.VerdictRequeue:
				}
				accessible = append(accessible, j)
			}
			ids := make([]string, 0, len(accessible))
			for i := range accessible {
				accessible[i].ResetToPending(nil)
				ids = append(ids, accessible[i].ID)
			}

			event := DispatchJobsResent{
				Metadata: usecase.NewEventMetadata(ec, DispatchJobsResentType, Source, "platform.dispatchjobs.resent"),
				IDs:      ids,
				Redrive:  len(redriven) > 0,
				Redriven: slices.Sorted(maps.Keys(redriven)),
				InFlight: slices.Sorted(maps.Keys(inFlight)),
				Skipped:  skipped,
			}
			return usecaseop.SaveAll(accessible, repo.RequeuePersister(redriven, inFlight), event), nil
		},
	}
}
