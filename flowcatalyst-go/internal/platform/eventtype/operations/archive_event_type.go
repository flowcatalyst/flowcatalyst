package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/eventtype"
)

// ArchiveEventTypeCommand contains the data needed to archive an event type
type ArchiveEventTypeCommand struct {
	ID string `json:"id"`
}

// ArchiveEventTypeUseCase handles archiving an event type
type ArchiveEventTypeUseCase struct {
	repo       eventtype.Repository
	unitOfWork common.UnitOfWork
}

// NewArchiveEventTypeUseCase creates a new ArchiveEventTypeUseCase
func NewArchiveEventTypeUseCase(repo eventtype.Repository, uow common.UnitOfWork) *ArchiveEventTypeUseCase {
	return &ArchiveEventTypeUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute archives an event type
func (uc *ArchiveEventTypeUseCase) Execute(
	ctx context.Context,
	cmd ArchiveEventTypeCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.ID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_ID", "Event type ID is required", nil),
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

	// Cannot archive already archived event types
	if existing.IsArchived() {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("ALREADY_ARCHIVED",
				"Event type is already archived",
				map[string]any{"id": cmd.ID}),
		)
	}

	// Archive the event type
	existing.Status = eventtype.EventTypeStatusArchived

	// Create domain event
	event := events.NewEventTypeArchived(execCtx, existing)

	// Atomic commit - ONLY way to return success
	return uc.unitOfWork.Commit(ctx, existing, event, cmd)
}
