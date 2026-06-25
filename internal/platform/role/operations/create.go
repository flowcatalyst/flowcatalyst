package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// CreateCommand is the input DTO.
type CreateCommand struct {
	ApplicationCode string   `json:"applicationCode"`
	RoleName        string   `json:"roleName"`
	DisplayName     string   `json:"displayName"`
	Description     *string  `json:"description,omitempty"`
	Permissions     []string `json:"permissions,omitempty"`
	ClientManaged   bool     `json:"clientManaged"`
}

// CreateRole creates a new role and emits RoleCreated.
func CreateRole(repo *role.Repository) usecaseop.Operation[CreateCommand, RoleCreated] {
	return usecaseop.Operation[CreateCommand, RoleCreated]{
		Name: "CreateRole",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			if strings.TrimSpace(cmd.ApplicationCode) == "" {
				return usecase.Validation("APPLICATION_REQUIRED", "applicationCode is required")
			}
			if strings.TrimSpace(cmd.RoleName) == "" {
				return usecase.Validation("ROLE_NAME_REQUIRED", "roleName is required")
			}
			if strings.TrimSpace(cmd.DisplayName) == "" {
				return usecase.Validation("DISPLAY_NAME_REQUIRED", "displayName is required")
			}
			return nil
		},
		// Roles are global (no per-client resource dimension), so there is no
		// use-case-level authz; the coarse CanWriteRoles permission is enforced
		// at the controller.
		Authorize: usecaseop.Public[CreateCommand],
		Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[RoleCreated], error) {
			fullName := cmd.ApplicationCode + ":" + cmd.RoleName
			existing, err := repo.FindByName(ctx, fullName)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_name failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("ROLE_EXISTS", "Role '"+fullName+"' already exists")
			}
			r := role.New(cmd.ApplicationCode, cmd.RoleName, cmd.DisplayName)
			r.Description = cmd.Description
			r.ClientManaged = cmd.ClientManaged
			for _, p := range cmd.Permissions {
				r.GrantPermission(p)
			}

			event := RoleCreated{
				Metadata: usecase.NewEventMetadata(ec, RoleCreatedType, Source, subjectFor(r.ID)),
				RoleID:   r.ID,
				Name:     r.Name,
			}
			return usecaseop.Save(r, repo, event), nil
		},
	}
}
