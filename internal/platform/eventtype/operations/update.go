package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// UpdateCommand is the input DTO for UpdateEventType.
type UpdateCommand struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

// UpdateEventType mutates name + description on an existing event type
// and atomically emits an [EventTypeUpdated] event.
func UpdateEventType(
	ctx context.Context,
	repo *eventtype.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd UpdateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[EventTypeUpdated], error) {
	var zero commit.Committed[EventTypeUpdated]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "Event type id is required")
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return zero, usecase.Validation("NAME_REQUIRED", "Event type name is required")
	}

	et, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if et == nil {
		return zero, httperror.NotFound("EventType", cmd.ID)
	}

	et.Name = cmd.Name
	et.Description = cmd.Description

	event := EventTypeUpdated{
		Metadata:    usecase.NewEventMetadata(ec, EventTypeUpdatedType, EventTypeSourceConst, subjectFor(et.ID)),
		EventTypeID: et.ID,
		Name:        et.Name,
		Description: et.Description,
	}
	return commit.Save(ctx, uow, et, repo, event, cmd)
}
