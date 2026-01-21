package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/subscription"
)

// DeleteSubscriptionCommand contains the data needed to delete a subscription
type DeleteSubscriptionCommand struct {
	ID string `json:"id"`
}

// DeleteSubscriptionUseCase handles deleting a subscription
type DeleteSubscriptionUseCase struct {
	repo       subscription.Repository
	unitOfWork common.UnitOfWork
}

// NewDeleteSubscriptionUseCase creates a new DeleteSubscriptionUseCase
func NewDeleteSubscriptionUseCase(repo subscription.Repository, uow common.UnitOfWork) *DeleteSubscriptionUseCase {
	return &DeleteSubscriptionUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute deletes a subscription
func (uc *DeleteSubscriptionUseCase) Execute(
	ctx context.Context,
	cmd DeleteSubscriptionCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.ID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_ID", "Subscription ID is required", nil),
		)
	}

	// Fetch existing subscription
	existing, err := uc.repo.FindSubscriptionByID(ctx, cmd.ID)
	if err != nil {
		if err == subscription.ErrNotFound {
			return common.Failure[common.DomainEvent](
				common.NotFoundError("SUBSCRIPTION_NOT_FOUND", "Subscription not found", map[string]any{"id": cmd.ID}),
			)
		}
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to find subscription", map[string]any{"error": err.Error()}),
		)
	}

	// Create domain event before deletion
	event := events.NewSubscriptionDeleted(execCtx, existing)

	// Atomic commit with delete
	return uc.unitOfWork.CommitDelete(ctx, existing, event, cmd)
}
