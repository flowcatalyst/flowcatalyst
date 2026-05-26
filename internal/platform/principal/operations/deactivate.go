package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
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
	repo *principal.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewDeactivateUseCase wires the use case.
func NewDeactivateUseCase(repo *principal.Repository, uow *usecasepgx.UnitOfWork) *DeactivateUseCase {
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

func (uc *DeactivateUseCase) Execute(ctx context.Context, cmd DeactivateCommand, ec usecase.ExecutionContext) usecase.Result[UserDeactivated] {
	p, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[UserDeactivated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if p == nil {
		return usecase.Failure[UserDeactivated](httperror.NotFound("Principal", cmd.ID))
	}
	p.Deactivate()
	event := UserDeactivated{
		Metadata: usecase.NewEventMetadata(ec, UserDeactivatedType, Source, subjectFor(p.ID)),
		UserID:   p.ID,
	}
	return usecasepgx.Commit[principal.Principal, UserDeactivated, DeactivateCommand](
		ctx, uc.uow, p, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeactivateCommand, UserDeactivated] = (*DeactivateUseCase)(nil)
