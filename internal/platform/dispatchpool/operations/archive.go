package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ArchiveCommand is the input DTO.
type ArchiveCommand struct {
	ID string `json:"id"`
}

// ArchiveDispatchPool archives a dispatch pool and emits DispatchPoolArchived.
func ArchiveDispatchPool(
	ctx context.Context,
	repo *dispatchpool.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd ArchiveCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[DispatchPoolArchived], error) {
	var zero commit.Committed[DispatchPoolArchived]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}

	p, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return zero, httperror.NotFound("DispatchPool", cmd.ID)
	}
	p.Archive()
	event := DispatchPoolArchived{
		Metadata: usecase.NewEventMetadata(ec, DispatchPoolArchivedType, Source, subjectFor(p.ID)),
		PoolID:   p.ID,
		Code:     p.Code,
	}
	return commit.Save(ctx, uow, p, repo, event, cmd)
}
