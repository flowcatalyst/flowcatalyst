package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ActivateCommand is the input DTO.
type ActivateCommand struct {
	ID string `json:"id"`
}

// ActivateUseCase implements UseCase.
type ActivateUseCase struct {
	repo *principal.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewActivateUseCase wires the use case.
func NewActivateUseCase(repo *principal.Repository, uow *usecasepgx.UnitOfWork) *ActivateUseCase {
	return &ActivateUseCase{repo: repo, uow: uow}
}

func (uc *ActivateUseCase) Validate(_ context.Context, cmd ActivateCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}

func (uc *ActivateUseCase) Authorize(_ context.Context, _ ActivateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *ActivateUseCase) Execute(ctx context.Context, cmd ActivateCommand, ec usecase.ExecutionContext) usecase.Result[UserActivated] {
	p, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[UserActivated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if p == nil {
		return usecase.Failure[UserActivated](httperror.NotFound("Principal", cmd.ID))
	}
	p.Activate()
	event := UserActivated{
		Metadata: usecase.NewEventMetadata(ec, UserActivatedType, Source, subjectFor(p.ID)),
		UserID:   p.ID,
	}
	return usecasepgx.Commit[principal.Principal, UserActivated, ActivateCommand](
		ctx, uc.uow, p, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[ActivateCommand, UserActivated] = (*ActivateUseCase)(nil)
