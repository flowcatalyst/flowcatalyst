package operations

import (
	"context"
	"time"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/eventtype"
)

// DeprecateSchemaCommand contains the data needed to deprecate a schema version
type DeprecateSchemaCommand struct {
	EventTypeID string `json:"eventTypeId"`
	Version     string `json:"version"`
}

// DeprecateSchemaUseCase handles deprecating a schema version
type DeprecateSchemaUseCase struct {
	repo       eventtype.Repository
	unitOfWork common.UnitOfWork
}

// NewDeprecateSchemaUseCase creates a new DeprecateSchemaUseCase
func NewDeprecateSchemaUseCase(repo eventtype.Repository, uow common.UnitOfWork) *DeprecateSchemaUseCase {
	return &DeprecateSchemaUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute deprecates a schema version
func (uc *DeprecateSchemaUseCase) Execute(
	ctx context.Context,
	cmd DeprecateSchemaCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.EventTypeID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_EVENT_TYPE_ID", "Event type ID is required", nil),
		)
	}

	if cmd.Version == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_VERSION", "Schema version is required", nil),
		)
	}

	// Fetch existing event type
	et, err := uc.repo.FindByID(ctx, cmd.EventTypeID)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to find event type", map[string]any{"error": err.Error()}),
		)
	}
	if et == nil {
		return common.Failure[common.DomainEvent](
			common.NotFoundError("EVENT_TYPE_NOT_FOUND",
				"Event type not found",
				map[string]any{"id": cmd.EventTypeID}),
		)
	}

	// Find the spec version
	sv := et.FindSpecVersion(cmd.Version)
	if sv == nil {
		return common.Failure[common.DomainEvent](
			common.NotFoundError("VERSION_NOT_FOUND",
				"Schema version not found",
				map[string]any{"version": cmd.Version}),
		)
	}

	// Cannot deprecate already deprecated versions
	if sv.IsDeprecated() {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("ALREADY_DEPRECATED",
				"Schema version is already deprecated",
				map[string]any{"version": cmd.Version}),
		)
	}

	// Deprecate the version
	sv.Status = eventtype.SpecVersionStatusDeprecated
	sv.UpdatedAt = time.Now()

	// Create domain event
	event := events.NewEventTypeSchemaDeprecated(execCtx, et, cmd.Version)

	// Atomic commit - ONLY way to return success
	return uc.unitOfWork.Commit(ctx, et, event, cmd)
}
