package operations

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
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
func AddSchema(
	ctx context.Context,
	repo *eventtype.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd AddSchemaCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[EventTypeSchemaAdded], error) {
	var zero commit.Committed[EventTypeSchemaAdded]

	if strings.TrimSpace(cmd.EventTypeID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "eventTypeId is required")
	}
	if strings.TrimSpace(cmd.Version) == "" {
		return zero, usecase.Validation("VERSION_REQUIRED", "version is required")
	}
	if len(cmd.Schema) == 0 {
		return zero, usecase.Validation("SCHEMA_REQUIRED", "schema payload is required")
	}

	et, err := repo.FindByID(ctx, cmd.EventTypeID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if et == nil {
		return zero, httperror.NotFound("EventType", cmd.EventTypeID)
	}

	for _, sv := range et.SpecVersions {
		if sv.Version == cmd.Version {
			return zero, usecase.Conflict("VERSION_EXISTS",
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
	return commit.Save(ctx, uow, et, repo, event, cmd)
}
