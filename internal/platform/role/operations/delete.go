package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteRole removes a role and emits RoleDeleted. Roles with
// source=CODE cannot be deleted.
func DeleteRole(repo *role.Repository) usecaseop.Operation[DeleteCommand, RoleDeleted] {
	return usecaseop.Operation[DeleteCommand, RoleDeleted]{
		Name: "DeleteRole",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Roles are global (no per-client resource dimension), so there is no
		// use-case-level authz; the coarse CanDeleteRoles permission is enforced
		// at the controller.
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[RoleDeleted], error) {
			r, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if r == nil {
				return nil, httperror.NotFound("Role", cmd.ID)
			}
			if r.Source == role.SourceCode {
				return nil, usecase.Conflict("CODE_ROLE_IMMUTABLE", "Roles with source=CODE cannot be deleted")
			}
			event := RoleDeleted{
				Metadata: usecase.NewEventMetadata(ec, RoleDeletedType, Source, subjectFor(r.ID)),
				RoleID:   r.ID,
				Name:     r.Name,
			}
			return usecaseop.Delete(r, repo, event), nil
		},
	}
}
