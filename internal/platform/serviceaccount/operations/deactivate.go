package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// DeactivateCommand is the input DTO.
type DeactivateCommand struct {
	ID string `json:"id"`
}

// DeactivateUseCase implements UseCase.
type DeactivateUseCase struct {
	repo *serviceaccount.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewDeactivateUseCase wires the use case.
func NewDeactivateUseCase(repo *serviceaccount.Repository, uow *usecasepgx.UnitOfWork) *DeactivateUseCase {
	return &DeactivateUseCase{repo: repo, uow: uow}
}

func (uc *DeactivateUseCase) Validate(_ context.Context, cmd DeactivateCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}

func (uc *DeactivateUseCase) Authorize(_ context.Context, _ DeactivateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *DeactivateUseCase) Execute(ctx context.Context, cmd DeactivateCommand, ec usecase.ExecutionContext) usecase.Result[ServiceAccountDeactivated] {
	sa, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[ServiceAccountDeactivated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if sa == nil {
		return usecase.Failure[ServiceAccountDeactivated](httperror.NotFound("ServiceAccount", cmd.ID))
	}
	sa.Deactivate()
	event := ServiceAccountDeactivated{
		Metadata:         usecase.NewEventMetadata(ec, ServiceAccountDeactivatedType, Source, subjectFor(sa.ID)),
		ServiceAccountID: sa.ID,
	}
	return usecasepgx.Commit[serviceaccount.ServiceAccount, ServiceAccountDeactivated, DeactivateCommand](
		ctx, uc.uow, sa, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeactivateCommand, ServiceAccountDeactivated] = (*DeactivateUseCase)(nil)
