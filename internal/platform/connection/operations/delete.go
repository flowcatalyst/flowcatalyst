package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
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
	repo *connection.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewDeleteUseCase wires the use case.
func NewDeleteUseCase(repo *connection.Repository, uow *usecasepgx.UnitOfWork) *DeleteUseCase {
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

func (uc *DeleteUseCase) Execute(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) usecase.Result[ConnectionDeleted] {
	c, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[ConnectionDeleted](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if c == nil {
		return usecase.Failure[ConnectionDeleted](httperror.NotFound("Connection", cmd.ID))
	}
	event := ConnectionDeleted{
		Metadata:     usecase.NewEventMetadata(ec, ConnectionDeletedType, Source, subjectFor(c.ID)),
		ConnectionID: c.ID,
		Code:         c.Code,
	}
	return usecasepgx.CommitDelete[connection.Connection, ConnectionDeleted, DeleteCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeleteCommand, ConnectionDeleted] = (*DeleteUseCase)(nil)
