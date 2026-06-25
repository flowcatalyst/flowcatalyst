package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// SuspendCommand pauses dispatch into the pool. Pool keeps existing
// in-flight messages — `Suspend` only flips the routing-eligibility flag.
type SuspendCommand struct {
	ID string `json:"id"`
}

// SuspendDispatchPool flips status to SUSPENDED and atomically emits
// [DispatchPoolSuspended].
func SuspendDispatchPool(repo *dispatchpool.Repository) usecaseop.Operation[SuspendCommand, DispatchPoolSuspended] {
	return usecaseop.Operation[SuspendCommand, DispatchPoolSuspended]{
		Name: "SuspendDispatchPool",
		Validate: func(_ context.Context, cmd SuspendCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Per-resource authz needs the loaded row, so it runs post-load in
		// Execute; the coarse "may write dispatch pools" permission is on the
		// controller.
		Authorize: usecaseop.Public[SuspendCommand],
		Execute: func(ctx context.Context, cmd SuspendCommand, ec usecase.ExecutionContext) (usecaseop.Plan[DispatchPoolSuspended], error) {
			p, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if p == nil {
				return nil, httperror.NotFound("DispatchPool", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), p.ClientID); err != nil {
				return nil, err
			}

			p.Suspend()
			event := DispatchPoolSuspended{
				Metadata: usecase.NewEventMetadata(ec, DispatchPoolSuspendedType, Source, subjectFor(p.ID)),
				PoolID:   p.ID,
				Code:     p.Code,
			}
			return usecaseop.Save(p, repo, event), nil
		},
	}
}

// ActivateCommand restores a suspended pool to ACTIVE.
type ActivateCommand struct {
	ID string `json:"id"`
}

// ActivateDispatchPool flips status to ACTIVE and atomically emits
// [DispatchPoolActivated].
func ActivateDispatchPool(repo *dispatchpool.Repository) usecaseop.Operation[ActivateCommand, DispatchPoolActivated] {
	return usecaseop.Operation[ActivateCommand, DispatchPoolActivated]{
		Name: "ActivateDispatchPool",
		Validate: func(_ context.Context, cmd ActivateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Per-resource authz needs the loaded row, so it runs post-load in
		// Execute; the coarse "may write dispatch pools" permission is on the
		// controller.
		Authorize: usecaseop.Public[ActivateCommand],
		Execute: func(ctx context.Context, cmd ActivateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[DispatchPoolActivated], error) {
			p, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if p == nil {
				return nil, httperror.NotFound("DispatchPool", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), p.ClientID); err != nil {
				return nil, err
			}

			p.Activate()
			event := DispatchPoolActivated{
				Metadata: usecase.NewEventMetadata(ec, DispatchPoolActivatedType, Source, subjectFor(p.ID)),
				PoolID:   p.ID,
				Code:     p.Code,
			}
			return usecaseop.Save(p, repo, event), nil
		},
	}
}
