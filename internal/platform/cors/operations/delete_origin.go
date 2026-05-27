package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	OriginID string `json:"originId"`
}

// DeleteOrigin removes a CORS origin and emits CorsOriginDeleted.
func DeleteOrigin(
	ctx context.Context,
	repo *cors.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd DeleteCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[CorsOriginDeleted], error) {
	var zero commit.Committed[CorsOriginDeleted]

	origin, err := repo.FindByID(ctx, cmd.OriginID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if origin == nil {
		return zero, httperror.NotFound("CorsOrigin", cmd.OriginID)
	}
	event := CorsOriginDeleted{
		Metadata: usecase.NewEventMetadata(ec, CorsOriginDeletedType, CorsSource, subjectFor(origin.ID)),
		OriginID: origin.ID,
		Origin:   origin.Origin,
	}
	return commit.Delete(ctx, uow, origin, repo, event, cmd)
}
