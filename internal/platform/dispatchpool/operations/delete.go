package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteDispatchPool removes a dispatch pool and atomically emits
// [DispatchPoolDeleted].
func DeleteDispatchPool(repo *dispatchpool.Repository) usecaseop.Operation[DeleteCommand, DispatchPoolDeleted] {
	return usecaseop.Operation[DeleteCommand, DispatchPoolDeleted]{
		Name: "DeleteDispatchPool",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Per-resource authz runs post-load in Execute; the coarse "may delete
		// dispatch pools" permission is on the controller.
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[DispatchPoolDeleted], error) {
			p, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if p == nil {
				return nil, httperror.NotFound("DispatchPool", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), p.ClientID); err != nil {
				return nil, err
			}

			event := DispatchPoolDeleted{
				Metadata: usecase.NewEventMetadata(ec, DispatchPoolDeletedType, Source, subjectFor(p.ID)),
				PoolID:   p.ID,
				Code:     p.Code,
			}
			return usecaseop.Delete(p, repo, event), nil
		},
	}
}
