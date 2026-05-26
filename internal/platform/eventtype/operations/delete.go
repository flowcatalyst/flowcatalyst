package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// DeleteCommand is the input DTO for the DeleteEventType use case.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteUseCase implements UseCase[DeleteCommand, EventTypeDeleted].
type DeleteUseCase struct {
	repo *eventtype.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewDeleteUseCase wires the use case.
func NewDeleteUseCase(repo *eventtype.Repository, uow *usecasepgx.UnitOfWork) *DeleteUseCase {
	return &DeleteUseCase{repo: repo, uow: uow}
}

func (uc *DeleteUseCase) Validate(_ context.Context, cmd DeleteCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "Event type id is required")
	}
	return nil
}

func (uc *DeleteUseCase) Authorize(_ context.Context, _ DeleteCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *DeleteUseCase) Execute(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) usecase.Result[EventTypeDeleted] {
	et, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[EventTypeDeleted](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if et == nil {
		return usecase.Failure[EventTypeDeleted](httperror.NotFound("EventType", cmd.ID))
	}

	event := EventTypeDeleted{
		Metadata:    usecase.NewEventMetadata(ec, EventTypeDeletedType, EventTypeSourceConst, subjectFor(et.ID)),
		EventTypeID: et.ID,
		Code:        et.Code,
	}
	return usecasepgx.CommitDelete[eventtype.EventType, EventTypeDeleted, DeleteCommand](
		ctx, uc.uow, et, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeleteCommand, EventTypeDeleted] = (*DeleteUseCase)(nil)
