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

// ArchiveCommand is the input DTO.
type ArchiveCommand struct {
	ID string `json:"id"`
}

// ArchiveDispatchPool archives a dispatch pool and atomically emits
// [DispatchPoolArchived].
func ArchiveDispatchPool(repo *dispatchpool.Repository) usecaseop.Operation[ArchiveCommand, DispatchPoolArchived] {
	return usecaseop.Operation[ArchiveCommand, DispatchPoolArchived]{
		Name: "ArchiveDispatchPool",
		Validate: func(_ context.Context, cmd ArchiveCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Per-resource authz needs the loaded row, so it runs post-load in
		// Execute; the coarse "may write dispatch pools" permission is on the
		// controller.
		Authorize: usecaseop.Public[ArchiveCommand],
		Execute: func(ctx context.Context, cmd ArchiveCommand, ec usecase.ExecutionContext) (usecaseop.Plan[DispatchPoolArchived], error) {
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

			p.Archive()
			event := DispatchPoolArchived{
				Metadata: usecase.NewEventMetadata(ec, DispatchPoolArchivedType, Source, subjectFor(p.ID)),
				PoolID:   p.ID,
				Code:     p.Code,
			}
			return usecaseop.Save(p, repo, event), nil
		},
	}
}
