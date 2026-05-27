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

// DeleteCommand is the input DTO for DeleteEventType.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteEventType removes an event type and atomically emits an
// [EventTypeDeleted] event.
func DeleteEventType(
	ctx context.Context,
	repo *eventtype.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd DeleteCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[EventTypeDeleted], error) {
	var zero commit.Committed[EventTypeDeleted]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "Event type id is required")
	}

	et, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if et == nil {
		return zero, httperror.NotFound("EventType", cmd.ID)
	}

	event := EventTypeDeleted{
		Metadata:    usecase.NewEventMetadata(ec, EventTypeDeletedType, EventTypeSourceConst, subjectFor(et.ID)),
		EventTypeID: et.ID,
		Code:        et.Code,
	}
	return commit.Delete(ctx, uow, et, repo, event, cmd)
}
