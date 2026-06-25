package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteUser removes a principal and emits [UserDeleted].
//
// Resource-level authorization (requireUserResourceAccess against the loaded
// principal — the use-case move of the controller's requireScopeByID) runs
// post-load in Execute; the coarse "may delete principals" permission is
// enforced at the controller.
func DeleteUser(repo *principal.Repository) usecaseop.Operation[DeleteCommand, UserDeleted] {
	return usecaseop.Operation[DeleteCommand, UserDeleted]{
		Name: "DeleteUser",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[UserDeleted], error) {
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
			email := ""
			if p.UserIdentity != nil {
				email = p.UserIdentity.Email
			}
			event := UserDeleted{
				Metadata: usecase.NewEventMetadata(ec, UserDeletedType, Source, subjectFor(p.ID)),
				UserID:   p.ID,
				Email:    email,
			}
			return usecaseop.Delete(p, repo, event), nil
		},
	}
}
