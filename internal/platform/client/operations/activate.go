package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
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
	repo *client.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewActivateUseCase wires the use case.
func NewActivateUseCase(repo *client.Repository, uow *usecasepgx.UnitOfWork) *ActivateUseCase {
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

func (uc *ActivateUseCase) Execute(ctx context.Context, cmd ActivateCommand, ec usecase.ExecutionContext) usecase.Result[ClientActivated] {
	c, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[ClientActivated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if c == nil {
		return usecase.Failure[ClientActivated](httperror.NotFound("Client", cmd.ID))
	}
	c.Activate()
	event := ClientActivated{
		Metadata: usecase.NewEventMetadata(ec, ClientActivatedType, Source, subjectFor(c.ID)),
		ClientID: c.ID,
	}
	return usecasepgx.Commit[client.Client, ClientActivated, ActivateCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[ActivateCommand, ClientActivated] = (*ActivateUseCase)(nil)
