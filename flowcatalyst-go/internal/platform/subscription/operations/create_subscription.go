package operations

import (
	"context"
	"regexp"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/subscription"
)

// Code format: lowercase alphanumeric with hyphens
var subscriptionCodePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)

// EventTypeBindingInput represents an event type binding in a command
type EventTypeBindingInput struct {
	EventTypeID   string `json:"eventTypeId"`
	EventTypeCode string `json:"eventTypeCode"`
	SpecVersion   string `json:"specVersion,omitempty"`
}

// CreateSubscriptionCommand contains the data needed to create a subscription
type CreateSubscriptionCommand struct {
	Code             string                  `json:"code"`
	Name             string                  `json:"name"`
	Description      string                  `json:"description,omitempty"`
	ClientID         string                  `json:"clientId"`
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

// CreateSubscriptionUseCase handles creating a new subscription
type CreateSubscriptionUseCase struct {
	repo       subscription.Repository
	unitOfWork common.UnitOfWork
}

// NewCreateSubscriptionUseCase creates a new CreateSubscriptionUseCase
func NewCreateSubscriptionUseCase(repo subscription.Repository, uow common.UnitOfWork) *CreateSubscriptionUseCase {
	return &CreateSubscriptionUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute creates a new subscription
func (uc *CreateSubscriptionUseCase) Execute(
	ctx context.Context,
	cmd CreateSubscriptionCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.Code == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_CODE", "Subscription code is required", nil),
		)
	}

	if !subscriptionCodePattern.MatchString(cmd.Code) {
		return common.Failure[common.DomainEvent](
			common.ValidationError("INVALID_CODE_FORMAT",
				"Subscription code must be lowercase alphanumeric with hyphens",
				map[string]any{"code": cmd.Code}),
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

	if len(cmd.EventTypes) == 0 {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_EVENT_TYPES", "At least one event type is required", nil),
		)
	}

	// Check for duplicate code
	existing, err := uc.repo.FindSubscriptionByCode(ctx, cmd.Code)
	if err != nil && err != subscription.ErrNotFound {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to check for existing subscription", map[string]any{"error": err.Error()}),
		)
	}
	if existing != nil {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("CODE_EXISTS",
				"A subscription with this code already exists",
				map[string]any{"code": cmd.Code}),
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

	// Set defaults
	mode := subscription.DispatchMode(cmd.Mode)
	if mode == "" {
		mode = subscription.DispatchModeImmediate
	}

	maxRetries := cmd.MaxRetries
	if maxRetries == 0 {
		maxRetries = subscription.DefaultMaxRetries
	}

	timeoutSeconds := cmd.TimeoutSeconds
	if timeoutSeconds == 0 {
		timeoutSeconds = subscription.DefaultTimeoutSeconds
	}

	// Create subscription
	sub := &subscription.Subscription{
		Code:             cmd.Code,
		Name:             cmd.Name,
		Description:      cmd.Description,
		ClientID:         cmd.ClientID,
		Target:           cmd.Target,
		EventTypes:       bindings,
		DispatchPoolID:   cmd.DispatchPoolID,
		DispatchPoolCode: cmd.DispatchPoolCode,
		Mode:             mode,
		MaxRetries:       maxRetries,
		TimeoutSeconds:   timeoutSeconds,
		ServiceAccountID: cmd.ServiceAccountID,
		DataOnly:         cmd.DataOnly,
		Source:           subscription.SubscriptionSourceAPI,
		Status:           subscription.SubscriptionStatusActive,
	}

	// Create domain event
	event := events.NewSubscriptionCreated(execCtx, sub)

	// Atomic commit
	return uc.unitOfWork.CommitWithClientID(ctx, sub, event, cmd, cmd.ClientID)
}
