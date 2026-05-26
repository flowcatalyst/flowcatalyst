package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteUseCase implements UseCase.
type DeleteUseCase struct {
	repo *application.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewDeleteUseCase wires the use case.
func NewDeleteUseCase(repo *application.Repository, uow *usecasepgx.UnitOfWork) *DeleteUseCase {
	return &DeleteUseCase{repo: repo, uow: uow}
}

func (uc *DeleteUseCase) Validate(_ context.Context, cmd DeleteCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}

func (uc *DeleteUseCase) Authorize(_ context.Context, _ DeleteCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *DeleteUseCase) Execute(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) usecase.Result[ApplicationDeleted] {
	a, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[ApplicationDeleted](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if a == nil {
		return usecase.Failure[ApplicationDeleted](httperror.NotFound("Application", cmd.ID))
	}
	event := ApplicationDeleted{
		Metadata:      usecase.NewEventMetadata(ec, ApplicationDeletedType, Source, subjectFor(a.ID)),
		ApplicationID: a.ID,
		Code:          a.Code,
	}
	return usecasepgx.CommitDelete[application.Application, ApplicationDeleted, DeleteCommand](
		ctx, uc.uow, a, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeleteCommand, ApplicationDeleted] = (*DeleteUseCase)(nil)
