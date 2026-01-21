package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/subscription"
)

// PauseSubscriptionCommand contains the data needed to pause a subscription
type PauseSubscriptionCommand struct {
	ID string `json:"id"`
}

// PauseSubscriptionUseCase handles pausing a subscription
type PauseSubscriptionUseCase struct {
	repo       subscription.Repository
	unitOfWork common.UnitOfWork
}

// NewPauseSubscriptionUseCase creates a new PauseSubscriptionUseCase
func NewPauseSubscriptionUseCase(repo subscription.Repository, uow common.UnitOfWork) *PauseSubscriptionUseCase {
	return &PauseSubscriptionUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute pauses a subscription
func (uc *PauseSubscriptionUseCase) Execute(
	ctx context.Context,
	cmd PauseSubscriptionCommand,
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

	// Check if already paused
	if existing.IsPaused() {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("ALREADY_PAUSED", "Subscription is already paused", map[string]any{"id": cmd.ID}),
		)
	}

	// Pause the subscription
	existing.Status = subscription.SubscriptionStatusPaused

	// Create domain event
	event := events.NewSubscriptionPaused(execCtx, existing)

	// Atomic commit
	return uc.unitOfWork.CommitWithClientID(ctx, existing, event, cmd, existing.ClientID)
}
