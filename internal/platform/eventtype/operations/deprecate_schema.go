package operations

import (
	"context"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
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
func DeprecateEventTypeSchema(repo *eventtype.Repository) usecaseop.Operation[DeprecateSchemaCommand, EventTypeSchemaDeprecated] {
	return usecaseop.Operation[DeprecateSchemaCommand, EventTypeSchemaDeprecated]{
		Name: "DeprecateEventTypeSchema",
		Validate: func(_ context.Context, cmd DeprecateSchemaCommand) error {
			if strings.TrimSpace(cmd.EventTypeID) == "" {
				return usecase.Validation("ID_REQUIRED", "eventTypeId is required")
			}
			if strings.TrimSpace(cmd.Version) == "" {
				return usecase.Validation("VERSION_REQUIRED", "version is required")
			}
			return nil
		},
		// Per-resource authz runs post-load in Execute; the coarse write
		// permission is on the controller (deprecate is reached only via bff).
		Authorize: usecaseop.Public[DeprecateSchemaCommand],
		Execute: func(ctx context.Context, cmd DeprecateSchemaCommand, ec usecase.ExecutionContext) (usecaseop.Plan[EventTypeSchemaDeprecated], error) {
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

			targetIdx := -1
			for i := range et.SpecVersions {
				if et.SpecVersions[i].Version == cmd.Version {
					targetIdx = i
					break
				}
			}
			if targetIdx == -1 {
				return nil, httperror.NotFound("SpecVersion", cmd.Version)
			}
			switch et.SpecVersions[targetIdx].Status {
			case eventtype.SpecFinalising:
				return nil, usecase.Conflict("STILL_FINALISING",
					"Schema version '"+cmd.Version+"' is still in FINALISING state and cannot be deprecated directly")
			case eventtype.SpecDeprecated:
				return nil, usecase.Conflict("ALREADY_DEPRECATED",
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
			return usecaseop.Save(et, repo, event), nil
		},
	}
}
