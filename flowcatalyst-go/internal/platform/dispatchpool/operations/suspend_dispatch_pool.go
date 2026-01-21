package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/dispatchpool"
	"go.flowcatalyst.tech/internal/platform/events"
)

// SuspendDispatchPoolCommand contains the data needed to suspend a dispatch pool
type SuspendDispatchPoolCommand struct {
	ID string `json:"id"`
}

// SuspendDispatchPoolUseCase handles suspending a dispatch pool
type SuspendDispatchPoolUseCase struct {
	repo       dispatchpool.Repository
	unitOfWork common.UnitOfWork
}

// NewSuspendDispatchPoolUseCase creates a new SuspendDispatchPoolUseCase
func NewSuspendDispatchPoolUseCase(repo dispatchpool.Repository, uow common.UnitOfWork) *SuspendDispatchPoolUseCase {
	return &SuspendDispatchPoolUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute suspends a dispatch pool
func (uc *SuspendDispatchPoolUseCase) Execute(
	ctx context.Context,
	cmd SuspendDispatchPoolCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.ID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_ID", "Dispatch pool ID is required", nil),
		)
	}

	// Fetch existing dispatch pool
	existing, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		if err == dispatchpool.ErrNotFound {
			return common.Failure[common.DomainEvent](
				common.NotFoundError("DISPATCH_POOL_NOT_FOUND", "Dispatch pool not found", map[string]any{"id": cmd.ID}),
			)
		}
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to find dispatch pool", map[string]any{"error": err.Error()}),
		)
	}

	// Check if already suspended or archived
	if existing.IsSuspended() {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("ALREADY_SUSPENDED", "Dispatch pool is already suspended", map[string]any{"id": cmd.ID}),
		)
	}
	if existing.IsArchived() {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("ALREADY_ARCHIVED", "Cannot suspend an archived dispatch pool", map[string]any{"id": cmd.ID}),
		)
	}

	// Suspend the dispatch pool
	existing.Status = dispatchpool.DispatchPoolStatusSuspended

	// Create domain event
	event := events.NewDispatchPoolSuspended(execCtx, existing)

	// Atomic commit
	if existing.ClientID != "" {
		return uc.unitOfWork.CommitWithClientID(ctx, existing, event, cmd, existing.ClientID)
	}
	return uc.unitOfWork.Commit(ctx, existing, event, cmd)
}
