package operations

import (
	"context"
	"regexp"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/eventtype"
)

// Code format: lowercase alphanumeric with hyphens, e.g., "order-created"
var eventTypeCodePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)

// CreateEventTypeCommand contains the data needed to create an event type
type CreateEventTypeCommand struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
}

// CreateEventTypeUseCase handles creating a new event type
type CreateEventTypeUseCase struct {
	repo       eventtype.Repository
	unitOfWork common.UnitOfWork
}

// NewCreateEventTypeUseCase creates a new CreateEventTypeUseCase
func NewCreateEventTypeUseCase(repo eventtype.Repository, uow common.UnitOfWork) *CreateEventTypeUseCase {
	return &CreateEventTypeUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute creates a new event type
func (uc *CreateEventTypeUseCase) Execute(
	ctx context.Context,
	cmd CreateEventTypeCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.Code == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_CODE", "Event type code is required", nil),
		)
	}

	if !eventTypeCodePattern.MatchString(cmd.Code) {
		return common.Failure[common.DomainEvent](
			common.ValidationError("INVALID_CODE_FORMAT",
				"Event type code must be lowercase alphanumeric with hyphens (e.g., 'order-created')",
				map[string]any{"code": cmd.Code}),
		)
	}

	if cmd.Name == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_NAME", "Event type name is required", nil),
		)
	}

	// Check for duplicate code
	existing, err := uc.repo.FindByCode(ctx, cmd.Code)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to check for existing event type", map[string]any{"error": err.Error()}),
		)
	}
	if existing != nil {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("CODE_EXISTS",
				"An event type with this code already exists",
				map[string]any{"code": cmd.Code}),
		)
	}

	// Create aggregate
	et := &eventtype.EventType{
		Code:        cmd.Code,
		Name:        cmd.Name,
		Description: cmd.Description,
		Category:    cmd.Category,
		Status:      eventtype.EventTypeStatusCurrent,
	}

	// Create domain event
	event := events.NewEventTypeCreated(execCtx, et)

	// Atomic commit - ONLY way to return success
	return uc.unitOfWork.Commit(ctx, et, event, cmd)
}
