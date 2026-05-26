package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
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
	repo *serviceaccount.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewDeleteUseCase wires the use case.
func NewDeleteUseCase(repo *serviceaccount.Repository, uow *usecasepgx.UnitOfWork) *DeleteUseCase {
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

func (uc *DeleteUseCase) Execute(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) usecase.Result[ServiceAccountDeleted] {
	sa, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[ServiceAccountDeleted](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if sa == nil {
		return usecase.Failure[ServiceAccountDeleted](httperror.NotFound("ServiceAccount", cmd.ID))
	}
	event := ServiceAccountDeleted{
		Metadata:         usecase.NewEventMetadata(ec, ServiceAccountDeletedType, Source, subjectFor(sa.ID)),
		ServiceAccountID: sa.ID,
		Code:             sa.Code,
	}
	return usecasepgx.CommitDelete[serviceaccount.ServiceAccount, ServiceAccountDeleted, DeleteCommand](
		ctx, uc.uow, sa, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeleteCommand, ServiceAccountDeleted] = (*DeleteUseCase)(nil)
