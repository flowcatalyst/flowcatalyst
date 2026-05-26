package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
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
	repo *dispatchpool.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewDeleteUseCase wires the use case.
func NewDeleteUseCase(repo *dispatchpool.Repository, uow *usecasepgx.UnitOfWork) *DeleteUseCase {
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

func (uc *DeleteUseCase) Execute(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) usecase.Result[DispatchPoolDeleted] {
	p, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[DispatchPoolDeleted](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if p == nil {
		return usecase.Failure[DispatchPoolDeleted](httperror.NotFound("DispatchPool", cmd.ID))
	}
	event := DispatchPoolDeleted{
		Metadata: usecase.NewEventMetadata(ec, DispatchPoolDeletedType, Source, subjectFor(p.ID)),
		PoolID:   p.ID,
		Code:     p.Code,
	}
	return usecasepgx.CommitDelete[dispatchpool.DispatchPool, DispatchPoolDeleted, DeleteCommand](
		ctx, uc.uow, p, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeleteCommand, DispatchPoolDeleted] = (*DeleteUseCase)(nil)
