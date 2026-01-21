package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/subscription"
)

// ResumeSubscriptionCommand contains the data needed to resume a subscription
type ResumeSubscriptionCommand struct {
	ID string `json:"id"`
}

// ResumeSubscriptionUseCase handles resuming a paused subscription
type ResumeSubscriptionUseCase struct {
	repo       subscription.Repository
	unitOfWork common.UnitOfWork
}

// NewResumeSubscriptionUseCase creates a new ResumeSubscriptionUseCase
func NewResumeSubscriptionUseCase(repo subscription.Repository, uow common.UnitOfWork) *ResumeSubscriptionUseCase {
	return &ResumeSubscriptionUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute resumes a paused subscription
func (uc *ResumeSubscriptionUseCase) Execute(
	ctx context.Context,
	cmd ResumeSubscriptionCommand,
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

	// Check if not paused
	if !existing.IsPaused() {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("NOT_PAUSED", "Subscription is not paused", map[string]any{"id": cmd.ID}),
		)
	}

	// Resume the subscription
	existing.Status = subscription.SubscriptionStatusActive

	// Create domain event
	event := events.NewSubscriptionResumed(execCtx, existing)

	// Atomic commit
	return uc.unitOfWork.CommitWithClientID(ctx, existing, event, cmd, existing.ClientID)
}
