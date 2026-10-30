package operations

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// AddSchemaCommand is the input DTO for AddSchema.
//
// SchemaType (JSON_SCHEMA, XSD, PROTO; legacy XML_SCHEMA and PROTOBUF accepted)
// and MimeType are optional: empty means JSON_SCHEMA and the schema type's
// default MIME type. A JSON Schema is stored as the JSON document; a schema
// sent as a JSON string holding the document (the SPA sends its textarea's
// text this way) is stored as the document it holds. XSD and PROTO schemas are
// text, stored as a JSON string.
type AddSchemaCommand struct {
	EventTypeID string          `json:"eventTypeId"`
	Version     string          `json:"version"`
	Schema      json.RawMessage `json:"schema"`
	MimeType    string          `json:"mimeType,omitempty"`
	SchemaType  string          `json:"schemaType,omitempty"`
}

// maxMimeTypeLen is msg_event_type_spec_versions.mime_type's width.
const maxMimeTypeLen = 100

// AddSchema appends a new schema version to an event type and atomically
// emits an [EventTypeSchemaAdded] event. The (id, version) pair must be
// unique.
func AddSchema(repo *eventtype.Repository) usecaseop.Operation[AddSchemaCommand, EventTypeSchemaAdded] {
	return usecaseop.Operation[AddSchemaCommand, EventTypeSchemaAdded]{
		Name: "AddSchema",
		Validate: func(_ context.Context, cmd AddSchemaCommand) error {
			if strings.TrimSpace(cmd.EventTypeID) == "" {
				return usecase.Validation("ID_REQUIRED", "eventTypeId is required")
			}
			if strings.TrimSpace(cmd.Version) == "" {
				return usecase.Validation("VERSION_REQUIRED", "version is required")
			}
			if len(cmd.Schema) == 0 {
				return usecase.Validation("SCHEMA_REQUIRED", "schema payload is required")
			}
			if len(strings.TrimSpace(cmd.MimeType)) > maxMimeTypeLen {
				return usecase.Validation("INVALID_MIME_TYPE", "mimeType is longer than 100 characters")
			}
			st, err := schemaTypeOf(cmd.SchemaType)
			if err != nil {
				return err
			}
			if _, err := eventtype.NormaliseSchemaContent(st, cmd.Schema); err != nil {
				return usecase.Validation("INVALID_SCHEMA", err.Error())
			}
			return nil
		},
		// Per-resource authz runs post-load in Execute; the coarse write
		// permission is on the controller.
		Authorize: usecaseop.Public[AddSchemaCommand],
		Execute: func(ctx context.Context, cmd AddSchemaCommand, ec usecase.ExecutionContext) (usecaseop.Plan[EventTypeSchemaAdded], error) {
			et, err := repo.FindByID(ctx, cmd.EventTypeID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if et == nil {
				return nil, httperror.NotFound("EventType", cmd.EventTypeID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), ids.StringPtr(et.ClientID)); err != nil {
				return nil, err
			}

			for _, sv := range et.SpecVersions {
				if sv.Version == cmd.Version {
					return nil, usecase.Conflict("VERSION_EXISTS",
						"Schema version '"+cmd.Version+"' already exists for this event type")
				}
			}

			st, err := schemaTypeOf(cmd.SchemaType)
			if err != nil {
				return nil, err
			}
			content, err := eventtype.NormaliseSchemaContent(st, cmd.Schema)
			if err != nil {
				return nil, usecase.Validation("INVALID_SCHEMA", err.Error())
			}
			sv := eventtype.NewSpecVersion(et.ID, cmd.Version, content)
			sv.SchemaType = st
			sv.MimeType = eventtype.DefaultMimeType(st)
			if m := strings.TrimSpace(cmd.MimeType); m != "" {
				sv.MimeType = m
			}
			et.AddSchemaVersion(sv)

			event := EventTypeSchemaAdded{
				Metadata:    usecase.NewEventMetadata(ec, EventTypeSchemaAddedType, EventTypeSourceConst, subjectFor(et.ID)),
				EventTypeID: et.ID,
				Version:     sv.Version,
			}
			return usecaseop.Save(et, repo, event), nil
		},
	}
}

// schemaTypeOf parses the command's schema type; empty is JSON_SCHEMA.
func schemaTypeOf(v string) (eventtype.SchemaType, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return eventtype.SchemaJSON, nil
	}
	st, ok := eventtype.ParseSchemaType(strings.ToUpper(v))
	if !ok {
		return "", usecase.Validation("INVALID_SCHEMA_TYPE", "schemaType must be JSON_SCHEMA, XSD or PROTO")
	}
	return st, nil
}
