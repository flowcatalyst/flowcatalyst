package operations

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// AddSchemaCommand is the input DTO for AddSchema.
type AddSchemaCommand struct {
	EventTypeID string          `json:"eventTypeId"`
	Version     string          `json:"version"`
	Schema      json.RawMessage `json:"schema"`
}

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
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), et.ClientID); err != nil {
				return nil, err
			}

			for _, sv := range et.SpecVersions {
				if sv.Version == cmd.Version {
					return nil, usecase.Conflict("VERSION_EXISTS",
						"Schema version '"+cmd.Version+"' already exists for this event type")
				}
			}

			sv := eventtype.NewSpecVersion(et.ID, cmd.Version, cmd.Schema)
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
