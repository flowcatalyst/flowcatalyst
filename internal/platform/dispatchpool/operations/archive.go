package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ArchiveCommand is the input DTO.
type ArchiveCommand struct {
	ID string `json:"id"`
}

// ArchiveUseCase implements UseCase.
type ArchiveUseCase struct {
	repo *dispatchpool.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewArchiveUseCase wires the use case.
func NewArchiveUseCase(repo *dispatchpool.Repository, uow *usecasepgx.UnitOfWork) *ArchiveUseCase {
	return &ArchiveUseCase{repo: repo, uow: uow}
}

func (uc *ArchiveUseCase) Validate(_ context.Context, cmd ArchiveCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}

func (uc *ArchiveUseCase) Authorize(_ context.Context, _ ArchiveCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *ArchiveUseCase) Execute(ctx context.Context, cmd ArchiveCommand, ec usecase.ExecutionContext) usecase.Result[DispatchPoolArchived] {
	p, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[DispatchPoolArchived](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if p == nil {
		return usecase.Failure[DispatchPoolArchived](httperror.NotFound("DispatchPool", cmd.ID))
	}
	p.Archive()
	event := DispatchPoolArchived{
		Metadata: usecase.NewEventMetadata(ec, DispatchPoolArchivedType, Source, subjectFor(p.ID)),
		PoolID:   p.ID,
		Code:     p.Code,
	}
	return usecasepgx.Commit[dispatchpool.DispatchPool, DispatchPoolArchived, ArchiveCommand](
		ctx, uc.uow, p, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[ArchiveCommand, DispatchPoolArchived] = (*ArchiveUseCase)(nil)
