package operations

import (
	"context"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// DeprecateSchemaCommand is the input DTO for DeprecateEventTypeSchema.
type DeprecateSchemaCommand struct {
	EventTypeID string `json:"eventTypeId"`
	Version     string `json:"version"`
}

// DeprecateEventTypeSchema transitions a spec version → DEPRECATED
// and emits an [EventTypeSchemaDeprecated] event. Refuses to deprecate
// a version that is still FINALISING (use the finalise endpoint first
// or rely on its auto-deprecate side effect) or already DEPRECATED.
func DeprecateEventTypeSchema(
	ctx context.Context,
	repo *eventtype.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd DeprecateSchemaCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[EventTypeSchemaDeprecated], error) {
	var zero commit.Committed[EventTypeSchemaDeprecated]

	if strings.TrimSpace(cmd.EventTypeID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "eventTypeId is required")
	}
	if strings.TrimSpace(cmd.Version) == "" {
		return zero, usecase.Validation("VERSION_REQUIRED", "version is required")
	}

	et, err := repo.FindByID(ctx, cmd.EventTypeID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if et == nil {
		return zero, httperror.NotFound("EventType", cmd.EventTypeID)
	}

	targetIdx := -1
	for i := range et.SpecVersions {
		if et.SpecVersions[i].Version == cmd.Version {
			targetIdx = i
			break
		}
	}
	if targetIdx == -1 {
		return zero, httperror.NotFound("SpecVersion", cmd.Version)
	}
	switch et.SpecVersions[targetIdx].Status {
	case eventtype.SpecFinalising:
		return zero, usecase.Conflict("STILL_FINALISING",
			"Schema version '"+cmd.Version+"' is still in FINALISING state and cannot be deprecated directly")
	case eventtype.SpecDeprecated:
		return zero, usecase.Conflict("ALREADY_DEPRECATED",
			"Schema version '"+cmd.Version+"' is already deprecated")
	}

	now := time.Now().UTC()
	et.SpecVersions[targetIdx].Status = eventtype.SpecDeprecated
	et.SpecVersions[targetIdx].UpdatedAt = now
	et.UpdatedAt = now

	event := EventTypeSchemaDeprecated{
		Metadata:    usecase.NewEventMetadata(ec, EventTypeSchemaDeprecatedType, EventTypeSourceConst, subjectFor(et.ID)),
		EventTypeID: et.ID,
		Version:     cmd.Version,
	}
	return commit.Save(ctx, uow, et, repo, event, cmd)
}
