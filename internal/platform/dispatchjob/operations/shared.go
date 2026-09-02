package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// StatusFlipCommand is the shared id-only command for Cancel/Complete.
type StatusFlipCommand struct {
	ID string `json:"id"`
}

type (
	CancelCommand   = StatusFlipCommand
	CompleteCommand = StatusFlipCommand
)

// statusFlip builds a terminal→terminal operator-override Operation: load
// the job, enforce per-resource scope, require the source status is FAILED
// (this is NOT a general state machine — DispatchStatus.IsTerminal() already
// counts FAILED as terminal, so without this guard an operator could "cancel"
// an already-COMPLETED job), apply the mutator, emit the typed event.
// Shared body for CancelDispatchJob / CompleteDispatchJob.
func statusFlip[E usecase.DomainEvent](
	name string,
	repo *dispatchjob.Repository,
	apply func(*dispatchjob.DispatchJob),
	event func(*dispatchjob.DispatchJob, usecase.ExecutionContext) E,
) usecaseop.Operation[StatusFlipCommand, E] {
	return usecaseop.Operation[StatusFlipCommand, E]{
		Name: name,
		Validate: func(_ context.Context, cmd StatusFlipCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Coarse "may write dispatch jobs" permission is the controller's
		// (mirrors requeue's viewPerm-gated write, api/api.go); resource-level
		// scope is enforced here, post-load, per the locked authz model.
		Authorize: usecaseop.Public[StatusFlipCommand],
		Execute: func(ctx context.Context, cmd StatusFlipCommand, ec usecase.ExecutionContext) (usecaseop.Plan[E], error) {
			j, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if j == nil {
				return nil, httperror.NotFound("DispatchJob", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), j.ClientID); err != nil {
				return nil, err
			}
			if j.Status != common.DispatchFailed {
				return nil, usecase.Conflict("NOT_FAILED",
					"dispatch job is not FAILED (current status: "+string(j.Status)+"); only a FAILED job can be overridden")
			}
			apply(j)
			return usecaseop.Save(j, repo, event(j, ec)), nil
		},
	}
}
