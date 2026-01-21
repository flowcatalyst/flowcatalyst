package operations

import (
	"context"
	"time"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/eventtype"
)

// FinaliseSchemaCommand contains the data needed to finalise a schema version
type FinaliseSchemaCommand struct {
	EventTypeID string `json:"eventTypeId"`
	Version     string `json:"version"`
}

// FinaliseSchemaUseCase handles finalising a schema version (making it current)
type FinaliseSchemaUseCase struct {
	repo       eventtype.Repository
	unitOfWork common.UnitOfWork
}

// NewFinaliseSchemaUseCase creates a new FinaliseSchemaUseCase
func NewFinaliseSchemaUseCase(repo eventtype.Repository, uow common.UnitOfWork) *FinaliseSchemaUseCase {
	return &FinaliseSchemaUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute finalises a schema version, making it current
func (uc *FinaliseSchemaUseCase) Execute(
	ctx context.Context,
	cmd FinaliseSchemaCommand,
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

	// Cannot finalise schema for archived event types
	if et.IsArchived() {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("EVENT_TYPE_ARCHIVED",
				"Cannot finalise schema for an archived event type",
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

	// Can only finalise versions that are in FINALISING status
	if !sv.IsFinalising() {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("INVALID_STATUS",
				"Can only finalise schemas in FINALISING status",
				map[string]any{"version": cmd.Version, "status": sv.Status}),
		)
	}

	// Deprecate any current versions first
	for i := range et.SpecVersions {
		if et.SpecVersions[i].IsCurrent() {
			et.SpecVersions[i].Status = eventtype.SpecVersionStatusDeprecated
			et.SpecVersions[i].UpdatedAt = time.Now()
		}
	}

	// Finalise the requested version (make it current)
	sv.Status = eventtype.SpecVersionStatusCurrent
	sv.UpdatedAt = time.Now()

	// Create domain event
	event := events.NewEventTypeSchemaFinalised(execCtx, et, cmd.Version)

	// Atomic commit - ONLY way to return success
	return uc.unitOfWork.Commit(ctx, et, event, cmd)
}
