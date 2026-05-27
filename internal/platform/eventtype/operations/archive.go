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

// ArchiveCommand is the input DTO for ArchiveEventType.
type ArchiveCommand struct {
	ID string `json:"id"`
}

// ArchiveEventType transitions an event type from CURRENT → ARCHIVED
// and atomically emits an [EventTypeArchived] event. Re-archiving an
// already-archived event type is rejected with a conflict.
func ArchiveEventType(
	ctx context.Context,
	repo *eventtype.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd ArchiveCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[EventTypeArchived], error) {
	var zero commit.Committed[EventTypeArchived]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}

	et, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if et == nil {
		return zero, httperror.NotFound("EventType", cmd.ID)
	}
	if et.Status == eventtype.StatusArchived {
		return zero, usecase.Conflict("ALREADY_ARCHIVED",
			"Event type '"+et.Code+"' is already archived")
	}

	et.Archive()

	event := EventTypeArchived{
		Metadata:    usecase.NewEventMetadata(ec, EventTypeArchivedType, EventTypeSourceConst, subjectFor(et.ID)),
		EventTypeID: et.ID,
		Code:        et.Code,
	}
	return commit.Save(ctx, uow, et, repo, event, cmd)
}
