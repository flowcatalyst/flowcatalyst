package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/dispatchpool"
	"go.flowcatalyst.tech/internal/platform/events"
)

// ArchiveDispatchPoolCommand contains the data needed to archive a dispatch pool
type ArchiveDispatchPoolCommand struct {
	ID string `json:"id"`
}

// ArchiveDispatchPoolUseCase handles archiving a dispatch pool
type ArchiveDispatchPoolUseCase struct {
	repo       dispatchpool.Repository
	unitOfWork common.UnitOfWork
}

// NewArchiveDispatchPoolUseCase creates a new ArchiveDispatchPoolUseCase
func NewArchiveDispatchPoolUseCase(repo dispatchpool.Repository, uow common.UnitOfWork) *ArchiveDispatchPoolUseCase {
	return &ArchiveDispatchPoolUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute archives a dispatch pool
func (uc *ArchiveDispatchPoolUseCase) Execute(
	ctx context.Context,
	cmd ArchiveDispatchPoolCommand,
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

	// Check if already archived
	if existing.IsArchived() {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("ALREADY_ARCHIVED", "Dispatch pool is already archived", map[string]any{"id": cmd.ID}),
		)
	}

	// Archive the dispatch pool
	existing.Status = dispatchpool.DispatchPoolStatusArchived

	// Create domain event
	event := events.NewDispatchPoolArchived(execCtx, existing)

	// Atomic commit
	if existing.ClientID != "" {
		return uc.unitOfWork.CommitWithClientID(ctx, existing, event, cmd, existing.ClientID)
	}
	return uc.unitOfWork.Commit(ctx, existing, event, cmd)
}
