package operations

import (
	"context"
	"slices"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// StatusFlipCommand is the shared id-only command for Cancel/Complete.
type StatusFlipCommand struct {
	ID string `json:"id"`
}

type (
	CancelCommand   = StatusFlipCommand
	CompleteCommand = StatusFlipCommand
)

// statusFlip builds an operator-override Operation: load the job, enforce
// per-resource scope, require the source status is one of from (this is NOT
// a general state machine — without this guard an operator could "cancel" an
// already-COMPLETED job), apply the mutator, emit the typed event. A refusal
// is a 409 with conflictCode and conflictWhat ("only <conflictWhat> can be
// ..."). Shared body for CancelDispatchJob / CompleteDispatchJob.
func statusFlip[E usecase.DomainEvent](
	name string,
	from []common.DispatchStatus,
	conflictCode, conflictWhat string,
	repo *dispatchjob.Repository,
	persist usecasepgx.Persist[dispatchjob.DispatchJob],
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
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), ids.StringPtr(j.ClientID)); err != nil {
				return nil, err
			}
			if !slices.Contains(from, j.Status) {
				return nil, usecase.Conflict(conflictCode,
					"dispatch job is "+string(j.Status)+"; only "+conflictWhat)
			}
			apply(j)
			return usecaseop.Save(j, persist, event(j, ec)), nil
		},
	}
}
