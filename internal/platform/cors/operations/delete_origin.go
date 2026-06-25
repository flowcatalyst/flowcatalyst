package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	OriginID string `json:"originId"`
}

// DeleteOrigin removes a CORS origin and emits CorsOriginDeleted.
func DeleteOrigin(repo *cors.Repository) usecaseop.Operation[DeleteCommand, CorsOriginDeleted] {
	return usecaseop.Operation[DeleteCommand, CorsOriginDeleted]{
		Name: "DeleteOrigin",
		// The coarse anchor check lives on the controller; CORS origins have no
		// per-client resource dimension, so the use case carries no
		// resource-level authz (Authorize = usecaseop.Public).
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[CorsOriginDeleted], error) {
			origin, err := repo.FindByID(ctx, cmd.OriginID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if origin == nil {
				return nil, httperror.NotFound("CorsOrigin", cmd.OriginID)
			}
			event := CorsOriginDeleted{
				Metadata: usecase.NewEventMetadata(ec, CorsOriginDeletedType, CorsSource, subjectFor(origin.ID)),
				OriginID: origin.ID,
				Origin:   origin.Origin,
			}
			return usecaseop.Delete(origin, repo, event), nil
		},
	}
}
