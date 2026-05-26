package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	OriginID string `json:"originId"`
}

// DeleteUseCase implements UseCase[DeleteCommand, CorsOriginDeleted].
type DeleteUseCase struct {
	repo *cors.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewDeleteUseCase wires the use case.
func NewDeleteUseCase(repo *cors.Repository, uow *usecasepgx.UnitOfWork) *DeleteUseCase {
	return &DeleteUseCase{repo: repo, uow: uow}
}

func (uc *DeleteUseCase) Validate(_ context.Context, _ DeleteCommand) error { return nil }

func (uc *DeleteUseCase) Authorize(_ context.Context, _ DeleteCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *DeleteUseCase) Execute(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) usecase.Result[CorsOriginDeleted] {
	origin, err := uc.repo.FindByID(ctx, cmd.OriginID)
	if err != nil {
		return usecase.Failure[CorsOriginDeleted](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if origin == nil {
		return usecase.Failure[CorsOriginDeleted](httperror.NotFound("CorsOrigin", cmd.OriginID))
	}
	event := CorsOriginDeleted{
		Metadata: usecase.NewEventMetadata(ec, CorsOriginDeletedType, CorsSource, subjectFor(origin.ID)),
		OriginID: origin.ID,
		Origin:   origin.Origin,
	}
	return usecasepgx.CommitDelete[cors.AllowedOrigin, CorsOriginDeleted, DeleteCommand](
		ctx, uc.uow, origin, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeleteCommand, CorsOriginDeleted] = (*DeleteUseCase)(nil)
