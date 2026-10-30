package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// ArchiveCommand is the input DTO for ArchiveEventType.
type ArchiveCommand struct {
	ID string `json:"id"`
}

// ArchiveEventType transitions an event type from CURRENT → ARCHIVED
// and atomically emits an [EventTypeArchived] event. Re-archiving an
// already-archived event type is rejected with a conflict; an event type with
// a schema version that is not deprecated is refused (SCHEMAS_NOT_DEPRECATED).
func ArchiveEventType(repo *eventtype.Repository) usecaseop.Operation[ArchiveCommand, EventTypeArchived] {
	return usecaseop.Operation[ArchiveCommand, EventTypeArchived]{
		Name: "ArchiveEventType",
		Validate: func(_ context.Context, cmd ArchiveCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Per-resource authz runs post-load in Execute; the coarse write
		// permission is on the controller (archive is reached only via bff).
		Authorize: usecaseop.Public[ArchiveCommand],
		Execute: func(ctx context.Context, cmd ArchiveCommand, ec usecase.ExecutionContext) (usecaseop.Plan[EventTypeArchived], error) {
			et, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if et == nil {
				return nil, httperror.NotFound("EventType", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), ids.StringPtr(et.ClientID)); err != nil {
				return nil, err
			}
			if et.Status == eventtype.StatusArchived {
				return nil, usecase.Conflict("ALREADY_ARCHIVED",
					"Event type '"+et.Code+"' is already archived")
			}
			if !et.CanArchive() {
				return nil, usecase.BusinessRule("SCHEMAS_NOT_DEPRECATED", eventtype.CannotArchiveReason)
			}

			et.Archive()

			event := EventTypeArchived{
				Metadata:    usecase.NewEventMetadata(ec, EventTypeArchivedType, EventTypeSourceConst, subjectFor(et.ID)),
				EventTypeID: et.ID,
				Code:        et.Code,
			}
			return usecaseop.Save(et, repo, event), nil
		},
	}
}
