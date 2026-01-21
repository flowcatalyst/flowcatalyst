package operations

import (
	"context"
	"time"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/eventtype"
)

// AddSchemaCommand contains the data needed to add a schema version to an event type
type AddSchemaCommand struct {
	EventTypeID string `json:"eventTypeId"`
	Version     string `json:"version"`
	MimeType    string `json:"mimeType"`
	Schema      string `json:"schema"`
	SchemaType  string `json:"schemaType"` // JSON_SCHEMA, PROTO, XSD
}

// AddSchemaUseCase handles adding a new schema version to an event type
type AddSchemaUseCase struct {
	repo       eventtype.Repository
	unitOfWork common.UnitOfWork
}

// NewAddSchemaUseCase creates a new AddSchemaUseCase
func NewAddSchemaUseCase(repo eventtype.Repository, uow common.UnitOfWork) *AddSchemaUseCase {
	return &AddSchemaUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute adds a new schema version to an event type
func (uc *AddSchemaUseCase) Execute(
	ctx context.Context,
	cmd AddSchemaCommand,
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

	if cmd.Schema == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_SCHEMA", "Schema content is required", nil),
		)
	}

	schemaType := eventtype.SchemaType(cmd.SchemaType)
	if schemaType == "" {
		schemaType = eventtype.SchemaTypeJSONSchema // Default
	}

	// Validate schema type
	switch schemaType {
	case eventtype.SchemaTypeJSONSchema, eventtype.SchemaTypeProto, eventtype.SchemaTypeXSD:
		// Valid
	default:
		return common.Failure[common.DomainEvent](
			common.ValidationError("INVALID_SCHEMA_TYPE",
				"Schema type must be JSON_SCHEMA, PROTO, or XSD",
				map[string]any{"schemaType": cmd.SchemaType}),
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

	// Cannot add schema to archived event types
	if et.IsArchived() {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("EVENT_TYPE_ARCHIVED",
				"Cannot add schema to an archived event type",
				map[string]any{"id": cmd.EventTypeID}),
		)
	}

	// Check if version already exists
	if et.HasVersion(cmd.Version) {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("VERSION_EXISTS",
				"Schema version already exists for this event type",
				map[string]any{"version": cmd.Version}),
		)
	}

	// Create new spec version (starts as FINALISING)
	now := time.Now()
	sv := eventtype.SpecVersion{
		Version:    cmd.Version,
		MimeType:   cmd.MimeType,
		Schema:     cmd.Schema,
		SchemaType: schemaType,
		Status:     eventtype.SpecVersionStatusFinalising,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// Add to event type
	et.AddSpecVersion(sv)

	// Create domain event
	event := events.NewEventTypeSchemaAdded(execCtx, et, &sv)

	// Atomic commit - ONLY way to return success
	return uc.unitOfWork.Commit(ctx, et, event, cmd)
}
