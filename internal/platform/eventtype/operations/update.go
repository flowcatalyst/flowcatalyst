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

// UpdateCommand is the input DTO for UpdateEventType.
type UpdateCommand struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

// UpdateEventType mutates name + description on an existing event type
// and atomically emits an [EventTypeUpdated] event.
func UpdateEventType(repo *eventtype.Repository) usecaseop.Operation[UpdateCommand, EventTypeUpdated] {
	return usecaseop.Operation[UpdateCommand, EventTypeUpdated]{
		Name: "UpdateEventType",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "Event type id is required")
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "Event type name is required")
			}
			return nil
		},
		// Per-resource authz needs the loaded row, so it runs post-load in
		// Execute; the coarse "may update event types" permission is on the
		// controller.
		Authorize: usecaseop.Public[UpdateCommand],
		Execute: func(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[EventTypeUpdated], error) {
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

			et.Name = cmd.Name
			et.Description = cmd.Description

			event := EventTypeUpdated{
				Metadata:    usecase.NewEventMetadata(ec, EventTypeUpdatedType, EventTypeSourceConst, subjectFor(et.ID)),
				EventTypeID: et.ID,
				Name:        et.Name,
				Description: et.Description,
			}
			return usecaseop.Save(et, repo, event), nil
		},
	}
}
