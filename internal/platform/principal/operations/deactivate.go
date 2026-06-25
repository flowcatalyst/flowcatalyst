package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

type DeactivateCommand struct {
	ID string `json:"id"`
}

// DeactivateUser flips a user principal inactive and emits [UserDeactivated].
//
// Resource-level authorization (requireUserResourceAccess against the loaded
// principal — the use-case move of the controller's requireScopeByID) runs
// post-load in Execute; the coarse "may write principals" permission is
// enforced at the controller.
func DeactivateUser(repo *principal.Repository) usecaseop.Operation[DeactivateCommand, UserDeactivated] {
	return usecaseop.Operation[DeactivateCommand, UserDeactivated]{
		Name: "DeactivateUser",
		Validate: func(_ context.Context, cmd DeactivateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeactivateCommand],
		Execute: func(ctx context.Context, cmd DeactivateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[UserDeactivated], error) {
			p, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if p == nil {
				return nil, httperror.NotFound("Principal", cmd.ID)
			}
			if err := requireUserResourceAccess(ctx, p); err != nil {
				return nil, err
			}
			p.Deactivate()
			event := UserDeactivated{
				Metadata: usecase.NewEventMetadata(ec, UserDeactivatedType, Source, subjectFor(p.ID)),
				UserID:   p.ID,
			}
			return usecaseop.Save(p, repo, event), nil
		},
	}
}
