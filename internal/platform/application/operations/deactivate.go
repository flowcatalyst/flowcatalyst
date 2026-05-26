package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
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
	repo *application.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewDeactivateUseCase wires the use case.
func NewDeactivateUseCase(repo *application.Repository, uow *usecasepgx.UnitOfWork) *DeactivateUseCase {
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

func (uc *DeactivateUseCase) Execute(ctx context.Context, cmd DeactivateCommand, ec usecase.ExecutionContext) usecase.Result[ApplicationDeactivated] {
	a, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[ApplicationDeactivated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if a == nil {
		return usecase.Failure[ApplicationDeactivated](httperror.NotFound("Application", cmd.ID))
	}
	a.Deactivate()
	event := ApplicationDeactivated{
		Metadata:      usecase.NewEventMetadata(ec, ApplicationDeactivatedType, Source, subjectFor(a.ID)),
		ApplicationID: a.ID,
	}
	return usecasepgx.Commit[application.Application, ApplicationDeactivated, DeactivateCommand](
		ctx, uc.uow, a, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeactivateCommand, ApplicationDeactivated] = (*DeactivateUseCase)(nil)
