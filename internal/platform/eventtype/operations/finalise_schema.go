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

// FinaliseSchemaCommand is the input DTO for FinaliseEventTypeSchema.
type FinaliseSchemaCommand struct {
	EventTypeID string `json:"eventTypeId"`
	Version     string `json:"version"`
}

// FinaliseEventTypeSchema transitions a spec version FINALISING →
// CURRENT and emits an [EventTypeSchemaFinalised] event. If another
// version with the same major prefix is already CURRENT, it is
// auto-deprecated in the SAME transaction; the deprecated version
// string is carried on the emitted event.
//
// Rejects: missing event type, missing version, version not in the
// FINALISING state.
func FinaliseEventTypeSchema(repo *eventtype.Repository) usecaseop.Operation[FinaliseSchemaCommand, EventTypeSchemaFinalised] {
	return usecaseop.Operation[FinaliseSchemaCommand, EventTypeSchemaFinalised]{
		Name: "FinaliseEventTypeSchema",
		Validate: func(_ context.Context, cmd FinaliseSchemaCommand) error {
			if strings.TrimSpace(cmd.EventTypeID) == "" {
				return usecase.Validation("ID_REQUIRED", "eventTypeId is required")
			}
			if strings.TrimSpace(cmd.Version) == "" {
				return usecase.Validation("VERSION_REQUIRED", "version is required")
			}
			return nil
		},
		// Per-resource authz runs post-load in Execute; the coarse write
		// permission is on the controller (finalise is reached only via bff).
		Authorize: usecaseop.Public[FinaliseSchemaCommand],
		Execute: func(ctx context.Context, cmd FinaliseSchemaCommand, ec usecase.ExecutionContext) (usecaseop.Plan[EventTypeSchemaFinalised], error) {
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
			if et.SpecVersions[targetIdx].Status != eventtype.SpecFinalising {
				return nil, usecase.Conflict("NOT_FINALISING",
					"Schema version '"+cmd.Version+"' is not in FINALISING state")
			}

			now := time.Now().UTC()
			target := &et.SpecVersions[targetIdx]
			target.Status = eventtype.SpecCurrent
			target.UpdatedAt = now

			var deprecatedVersion *string
			targetMajor := target.Major()
			for i := range et.SpecVersions {
				if i == targetIdx {
					continue
				}
				sibling := &et.SpecVersions[i]
				if sibling.Status != eventtype.SpecCurrent {
					continue
				}
				if sibling.Major() != targetMajor {
					continue
				}
				sibling.Status = eventtype.SpecDeprecated
				sibling.UpdatedAt = now
				v := sibling.Version
				deprecatedVersion = &v
				break
			}
			et.UpdatedAt = now

			event := EventTypeSchemaFinalised{
				Metadata:          usecase.NewEventMetadata(ec, EventTypeSchemaFinalisedType, EventTypeSourceConst, subjectFor(et.ID)),
				EventTypeID:       et.ID,
				Version:           cmd.Version,
				DeprecatedVersion: deprecatedVersion,
			}
			return usecaseop.Save(et, repo, event), nil
		},
	}
}
