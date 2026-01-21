package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/subscription"
)

// UpdateSubscriptionCommand contains the data needed to update a subscription
type UpdateSubscriptionCommand struct {
	ID               string                  `json:"id"`
	Name             string                  `json:"name"`
	Description      string                  `json:"description,omitempty"`
	Target           string                  `json:"target"`
	EventTypes       []EventTypeBindingInput `json:"eventTypes"`
	DispatchPoolID   string                  `json:"dispatchPoolId,omitempty"`
	DispatchPoolCode string                  `json:"dispatchPoolCode,omitempty"`
	Mode             string                  `json:"mode,omitempty"`
	MaxRetries       int                     `json:"maxRetries,omitempty"`
	TimeoutSeconds   int                     `json:"timeoutSeconds,omitempty"`
	ServiceAccountID string                  `json:"serviceAccountId,omitempty"`
	DataOnly         bool                    `json:"dataOnly"`
}

// UpdateSubscriptionUseCase handles updating a subscription
type UpdateSubscriptionUseCase struct {
	repo       subscription.Repository
	unitOfWork common.UnitOfWork
}

// NewUpdateSubscriptionUseCase creates a new UpdateSubscriptionUseCase
func NewUpdateSubscriptionUseCase(repo subscription.Repository, uow common.UnitOfWork) *UpdateSubscriptionUseCase {
	return &UpdateSubscriptionUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute updates a subscription
func (uc *UpdateSubscriptionUseCase) Execute(
	ctx context.Context,
	cmd UpdateSubscriptionCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.ID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_ID", "Subscription ID is required", nil),
		)
	}

	if cmd.Name == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_NAME", "Subscription name is required", nil),
		)
	}

	if cmd.Target == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_TARGET", "Target URL is required", nil),
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

	// Build event type bindings
	bindings := make([]subscription.EventTypeBinding, len(cmd.EventTypes))
	for i, et := range cmd.EventTypes {
		bindings[i] = subscription.EventTypeBinding{
			EventTypeID:   et.EventTypeID,
			EventTypeCode: et.EventTypeCode,
			SpecVersion:   et.SpecVersion,
		}
	}

	// Update fields (code and clientId are immutable)
	existing.Name = cmd.Name
	existing.Description = cmd.Description
	existing.Target = cmd.Target
	existing.EventTypes = bindings
	existing.DispatchPoolID = cmd.DispatchPoolID
	existing.DispatchPoolCode = cmd.DispatchPoolCode
	existing.ServiceAccountID = cmd.ServiceAccountID
	existing.DataOnly = cmd.DataOnly

	if cmd.Mode != "" {
		existing.Mode = subscription.DispatchMode(cmd.Mode)
	}
	if cmd.MaxRetries > 0 {
		existing.MaxRetries = cmd.MaxRetries
	}
	if cmd.TimeoutSeconds > 0 {
		existing.TimeoutSeconds = cmd.TimeoutSeconds
	}

	// Create domain event
	event := events.NewSubscriptionUpdated(execCtx, existing)

	// Atomic commit
	return uc.unitOfWork.CommitWithClientID(ctx, existing, event, cmd, existing.ClientID)
}
