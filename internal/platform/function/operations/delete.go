package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteFunction removes a function — cascading to its versions, aliases,
// and settings rows via the fn_versions/fn_aliases/fn_settings FK ON DELETE
// CASCADE (migration 059) — and atomically emits [FunctionDeleted].
//
// TODO(WP4): delete the version artifacts (by digest) from the artifact
// store once versions are read back here; WP3 has no publish path yet, so
// there is nothing in the store to reclaim.
func DeleteFunction(repo *function.Repository) usecaseop.Operation[DeleteCommand, FunctionDeleted] {
	return usecaseop.Operation[DeleteCommand, FunctionDeleted]{
		Name: "DeleteFunction",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Per-resource authz runs post-load in Execute; the coarse "may
		// manage functions" permission is on the controller.
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[FunctionDeleted], error) {
			f, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if f == nil {
				return nil, httperror.NotFound("Function", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
				return nil, err
			}

			event := FunctionDeleted{
				Metadata:   usecase.NewEventMetadata(ec, FunctionDeletedType, Source, subjectFor(f.ID)),
				FunctionID: f.ID,
				Address:    f.Address,
			}
			return usecaseop.Delete(f, repo, event), nil
		},
	}
}
