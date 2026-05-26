package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// UpdateCommand is the input DTO for the UpdateEventType use case.
type UpdateCommand struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

// UpdateUseCase implements UseCase[UpdateCommand, EventTypeUpdated].
type UpdateUseCase struct {
	repo *eventtype.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewUpdateUseCase wires the use case.
func NewUpdateUseCase(repo *eventtype.Repository, uow *usecasepgx.UnitOfWork) *UpdateUseCase {
	return &UpdateUseCase{repo: repo, uow: uow}
}

func (uc *UpdateUseCase) Validate(_ context.Context, cmd UpdateCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "Event type id is required")
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return usecase.Validation("NAME_REQUIRED", "Event type name is required")
	}
	return nil
}

func (uc *UpdateUseCase) Authorize(_ context.Context, _ UpdateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *UpdateUseCase) Execute(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) usecase.Result[EventTypeUpdated] {
	et, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[EventTypeUpdated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if et == nil {
		return usecase.Failure[EventTypeUpdated](httperror.NotFound("EventType", cmd.ID))
	}

	et.Name = cmd.Name
	et.Description = cmd.Description

	event := EventTypeUpdated{
		Metadata:    usecase.NewEventMetadata(ec, EventTypeUpdatedType, EventTypeSourceConst, subjectFor(et.ID)),
		EventTypeID: et.ID,
		Name:        et.Name,
		Description: et.Description,
	}
	return usecasepgx.Commit[eventtype.EventType, EventTypeUpdated, UpdateCommand](
		ctx, uc.uow, et, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[UpdateCommand, EventTypeUpdated] = (*UpdateUseCase)(nil)
