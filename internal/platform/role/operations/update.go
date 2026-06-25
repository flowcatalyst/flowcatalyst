package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// UpdateCommand is the input DTO. Permissions: nil means "don't touch";
// empty slice means "clear".
type UpdateCommand struct {
	ID            string   `json:"id"`
	DisplayName   *string  `json:"displayName,omitempty"`
	Description   *string  `json:"description,omitempty"`
	Permissions   []string `json:"permissions,omitempty"`
	ClientManaged *bool    `json:"clientManaged,omitempty"`
}

// UpdateRole mutates an existing role and emits RoleUpdated. Roles with
// source=CODE are immutable.
func UpdateRole(repo *role.Repository) usecaseop.Operation[UpdateCommand, RoleUpdated] {
	return usecaseop.Operation[UpdateCommand, RoleUpdated]{
		Name: "UpdateRole",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			if cmd.DisplayName != nil && strings.TrimSpace(*cmd.DisplayName) == "" {
				return usecase.Validation("DISPLAY_NAME_REQUIRED", "displayName cannot be empty")
			}
			return nil
		},
		// Roles are global (no per-client resource dimension), so there is no
		// use-case-level authz; the coarse CanWriteRoles permission is enforced
		// at the controller.
		Authorize: usecaseop.Public[UpdateCommand],
		Execute: func(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[RoleUpdated], error) {
			r, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if r == nil {
				return nil, httperror.NotFound("Role", cmd.ID)
			}
			if r.Source == role.SourceCode {
				return nil, usecase.Conflict("CODE_ROLE_IMMUTABLE", "Roles with source=CODE cannot be modified")
			}
			if cmd.DisplayName != nil {
				r.DisplayName = strings.TrimSpace(*cmd.DisplayName)
			}
			if cmd.Description != nil {
				r.Description = cmd.Description
			}
			if cmd.Permissions != nil {
				r.Permissions = cmd.Permissions
			}
			if cmd.ClientManaged != nil {
				r.ClientManaged = *cmd.ClientManaged
			}

			event := RoleUpdated{
				Metadata: usecase.NewEventMetadata(ec, RoleUpdatedType, Source, subjectFor(r.ID)),
				RoleID:   r.ID,
				Name:     r.Name,
			}
			return usecaseop.Save(r, repo, event), nil
		},
	}
}
