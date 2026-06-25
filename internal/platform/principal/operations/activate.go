package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

type ActivateCommand struct {
	ID string `json:"id"`
}

// ActivateUser flips a user principal active and emits [UserActivated].
//
// Resource-level authorization (requireUserResourceAccess against the loaded
// principal — the use-case move of the controller's requireScopeByID) runs
// post-load in Execute: anchors manage any user, a client-admin only their own
// client's CLIENT-scope users. The coarse "may write principals" permission is
// enforced at the controller.
func ActivateUser(repo *principal.Repository) usecaseop.Operation[ActivateCommand, UserActivated] {
	return usecaseop.Operation[ActivateCommand, UserActivated]{
		Name: "ActivateUser",
		Validate: func(_ context.Context, cmd ActivateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[ActivateCommand],
		Execute: func(ctx context.Context, cmd ActivateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[UserActivated], error) {
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
			p.Activate()
			event := UserActivated{
				Metadata: usecase.NewEventMetadata(ec, UserActivatedType, Source, subjectFor(p.ID)),
				UserID:   p.ID,
			}
			return usecaseop.Save(p, repo, event), nil
		},
	}
}
