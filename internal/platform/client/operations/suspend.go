package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// SuspendCommand is the input DTO.
type SuspendCommand struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// SuspendUseCase implements UseCase.
type SuspendUseCase struct {
	repo *client.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewSuspendUseCase wires the use case.
func NewSuspendUseCase(repo *client.Repository, uow *usecasepgx.UnitOfWork) *SuspendUseCase {
	return &SuspendUseCase{repo: repo, uow: uow}
}

func (uc *SuspendUseCase) Validate(_ context.Context, cmd SuspendCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	if strings.TrimSpace(cmd.Reason) == "" {
		return usecase.Validation("REASON_REQUIRED", "reason is required")
	}
	return nil
}

func (uc *SuspendUseCase) Authorize(_ context.Context, _ SuspendCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *SuspendUseCase) Execute(ctx context.Context, cmd SuspendCommand, ec usecase.ExecutionContext) usecase.Result[ClientSuspended] {
	c, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[ClientSuspended](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if c == nil {
		return usecase.Failure[ClientSuspended](httperror.NotFound("Client", cmd.ID))
	}
	c.Suspend(cmd.Reason)
	event := ClientSuspended{
		Metadata: usecase.NewEventMetadata(ec, ClientSuspendedType, Source, subjectFor(c.ID)),
		ClientID: c.ID,
		Reason:   cmd.Reason,
	}
	return usecasepgx.Commit[client.Client, ClientSuspended, SuspendCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[SuspendCommand, ClientSuspended] = (*SuspendUseCase)(nil)
