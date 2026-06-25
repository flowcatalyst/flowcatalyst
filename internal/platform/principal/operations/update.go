package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

type UpdateCommand struct {
	ID     string  `json:"id"`
	Name   *string `json:"name,omitempty"`
	Active *bool   `json:"active,omitempty"`
	Email  *string `json:"email,omitempty"`
}

// UpdateUser mutates the supplied mutable fields and emits [UserUpdated].
//
// Resource-level authorization (requireUserResourceAccess against the loaded
// principal — the use-case move of the controller's requireScopeByID) runs
// post-load in Execute; the coarse "may write principals" permission is
// enforced at the controller.
func UpdateUser(repo *principal.Repository) usecaseop.Operation[UpdateCommand, UserUpdated] {
	return usecaseop.Operation[UpdateCommand, UserUpdated]{
		Name: "UpdateUser",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name cannot be empty")
			}
			return nil
		},
		Authorize: usecaseop.Public[UpdateCommand],
		Execute: func(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[UserUpdated], error) {
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
			if cmd.Name != nil {
				p.Name = strings.TrimSpace(*cmd.Name)
			}
			if cmd.Active != nil {
				p.Active = *cmd.Active
			}
			if cmd.Email != nil {
				// Email is the identity key (login, scope derivation, and the sync's own
				// findByEmail match key). Accept it so callers can PUT a full object, but
				// only as a stable assertion: a different value is an identity change this
				// endpoint deliberately does not perform.
				got := strings.ToLower(strings.TrimSpace(*cmd.Email))
				cur := ""
				if p.UserIdentity != nil {
					cur = strings.ToLower(strings.TrimSpace(p.UserIdentity.Email))
				}
				if got != "" && got != cur {
					return nil, usecase.Validation("EMAIL_IMMUTABLE",
						"email cannot be changed here; it is the principal's identity")
				}
			}

			event := UserUpdated{
				Metadata: usecase.NewEventMetadata(ec, UserUpdatedType, Source, subjectFor(p.ID)),
				UserID:   p.ID,
				Name:     p.Name,
			}
			return usecaseop.Save(p, repo, event), nil
		},
	}
}
