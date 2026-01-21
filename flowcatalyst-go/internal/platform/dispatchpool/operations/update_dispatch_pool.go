package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/dispatchpool"
	"go.flowcatalyst.tech/internal/platform/events"
)

// UpdateDispatchPoolCommand contains the data needed to update a dispatch pool
type UpdateDispatchPoolCommand struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	Concurrency     int    `json:"concurrency"`
	QueueCapacity   int    `json:"queueCapacity"`
	RateLimitPerMin *int   `json:"rateLimitPerMin,omitempty"`
}

// UpdateDispatchPoolUseCase handles updating a dispatch pool
type UpdateDispatchPoolUseCase struct {
	repo       dispatchpool.Repository
	unitOfWork common.UnitOfWork
}

// NewUpdateDispatchPoolUseCase creates a new UpdateDispatchPoolUseCase
func NewUpdateDispatchPoolUseCase(repo dispatchpool.Repository, uow common.UnitOfWork) *UpdateDispatchPoolUseCase {
	return &UpdateDispatchPoolUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute updates a dispatch pool
func (uc *UpdateDispatchPoolUseCase) Execute(
	ctx context.Context,
	cmd UpdateDispatchPoolCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.ID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_ID", "Dispatch pool ID is required", nil),
		)
	}

	if cmd.Concurrency <= 0 {
		return common.Failure[common.DomainEvent](
			common.ValidationError("INVALID_CONCURRENCY", "Concurrency must be greater than 0", nil),
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

	// Cannot update archived pools
	if existing.IsArchived() {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("DISPATCH_POOL_ARCHIVED",
				"Cannot update an archived dispatch pool",
				map[string]any{"id": cmd.ID}),
		)
	}

	// Update fields (code and clientId are immutable)
	existing.Name = cmd.Name
	existing.Description = cmd.Description
	existing.Concurrency = cmd.Concurrency
	if cmd.QueueCapacity > 0 {
		existing.QueueCapacity = cmd.QueueCapacity
	}
	existing.RateLimitPerMin = cmd.RateLimitPerMin

	// Create domain event
	event := events.NewDispatchPoolUpdated(execCtx, existing)

	// Atomic commit
	if existing.ClientID != "" {
		return uc.unitOfWork.CommitWithClientID(ctx, existing, event, cmd, existing.ClientID)
	}
	return uc.unitOfWork.Commit(ctx, existing, event, cmd)
}
