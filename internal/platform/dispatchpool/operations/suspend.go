package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// SuspendCommand pauses dispatch into the pool. Pool keeps existing
// in-flight messages — `Suspend` only flips the routing-eligibility flag.
type SuspendCommand struct {
	ID string `json:"id"`
}

// SuspendDispatchPool flips status to SUSPENDED and emits the event.
func SuspendDispatchPool(
	ctx context.Context,
	repo *dispatchpool.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd SuspendCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[DispatchPoolSuspended], error) {
	var zero commit.Committed[DispatchPoolSuspended]
	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	p, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return zero, httperror.NotFound("DispatchPool", cmd.ID)
	}
	p.Suspend()
	event := DispatchPoolSuspended{
		Metadata: usecase.NewEventMetadata(ec, DispatchPoolSuspendedType, Source, subjectFor(p.ID)),
		PoolID:   p.ID,
		Code:     p.Code,
	}
	return commit.Save(ctx, uow, p, repo, event, cmd)
}

// ActivateCommand restores a suspended pool to ACTIVE.
type ActivateCommand struct {
	ID string `json:"id"`
}

// ActivateDispatchPool flips status to ACTIVE and emits the event.
func ActivateDispatchPool(
	ctx context.Context,
	repo *dispatchpool.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd ActivateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[DispatchPoolActivated], error) {
	var zero commit.Committed[DispatchPoolActivated]
	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	p, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return zero, httperror.NotFound("DispatchPool", cmd.ID)
	}
	p.Activate()
	event := DispatchPoolActivated{
		Metadata: usecase.NewEventMetadata(ec, DispatchPoolActivatedType, Source, subjectFor(p.ID)),
		PoolID:   p.ID,
		Code:     p.Code,
	}
	return commit.Save(ctx, uow, p, repo, event, cmd)
}
