package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/eventtype"
)

// UpdateEventTypeCommand contains the data needed to update an event type
type UpdateEventTypeCommand struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
}

// UpdateEventTypeUseCase handles updating an event type
type UpdateEventTypeUseCase struct {
	repo       eventtype.Repository
	unitOfWork common.UnitOfWork
}

// NewUpdateEventTypeUseCase creates a new UpdateEventTypeUseCase
func NewUpdateEventTypeUseCase(repo eventtype.Repository, uow common.UnitOfWork) *UpdateEventTypeUseCase {
	return &UpdateEventTypeUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute updates an event type
func (uc *UpdateEventTypeUseCase) Execute(
	ctx context.Context,
	cmd UpdateEventTypeCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.ID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_ID", "Event type ID is required", nil),
		)
	}

	if cmd.Name == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_NAME", "Event type name is required", nil),
		)
	}

	// Fetch existing event type
	existing, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to find event type", map[string]any{"error": err.Error()}),
		)
	}
	if existing == nil {
		return common.Failure[common.DomainEvent](
			common.NotFoundError("EVENT_TYPE_NOT_FOUND",
				"Event type not found",
				map[string]any{"id": cmd.ID}),
		)
	}

	// Cannot update archived event types
	if existing.IsArchived() {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("EVENT_TYPE_ARCHIVED",
				"Cannot update an archived event type",
				map[string]any{"id": cmd.ID}),
		)
	}

	// Update fields (code is immutable)
	existing.Name = cmd.Name
	existing.Description = cmd.Description
	existing.Category = cmd.Category

	// Create domain event
	event := events.NewEventTypeUpdated(execCtx, existing)

	// Atomic commit - ONLY way to return success
	return uc.unitOfWork.Commit(ctx, existing, event, cmd)
}
