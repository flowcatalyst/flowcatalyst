package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeleteCommand is the input DTO for DeleteEventType.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteEventType removes an event type and atomically emits an
// [EventTypeDeleted] event.
func DeleteEventType(repo *eventtype.Repository) usecaseop.Operation[DeleteCommand, EventTypeDeleted] {
	return usecaseop.Operation[DeleteCommand, EventTypeDeleted]{
		Name: "DeleteEventType",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "Event type id is required")
			}
			return nil
		},
		// Per-resource authz runs post-load in Execute; the coarse "may
		// delete event types" permission is on the controller.
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[EventTypeDeleted], error) {
			et, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if et == nil {
				return nil, httperror.NotFound("EventType", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), et.ClientID); err != nil {
				return nil, err
			}

			event := EventTypeDeleted{
				Metadata:    usecase.NewEventMetadata(ec, EventTypeDeletedType, EventTypeSourceConst, subjectFor(et.ID)),
				EventTypeID: et.ID,
				Code:        et.Code,
			}
			return usecaseop.Delete(et, repo, event), nil
		},
	}
}
