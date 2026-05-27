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

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteDispatchPool removes a dispatch pool and emits DispatchPoolDeleted.
func DeleteDispatchPool(
	ctx context.Context,
	repo *dispatchpool.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd DeleteCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[DispatchPoolDeleted], error) {
	var zero commit.Committed[DispatchPoolDeleted]

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
	event := DispatchPoolDeleted{
		Metadata: usecase.NewEventMetadata(ec, DispatchPoolDeletedType, Source, subjectFor(p.ID)),
		PoolID:   p.ID,
		Code:     p.Code,
	}
	return commit.Delete(ctx, uow, p, repo, event, cmd)
}
