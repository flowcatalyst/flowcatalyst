package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
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

// crossApplication reports whether an admin-API write may put another
// application's permissions on a role (F12): only a super-admin's, as in Java
// (Access.crossApplication) and Rust (cross_application). The SDK and platform
// role syncs never pass it.
func crossApplication(ctx context.Context) bool { return auth.FromContext(ctx).IsSuperAdmin() }

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
			// F18: rejected as sent, never trimmed. The name is the canonical
			// "{application}:{role}" the roles are assigned and minted by.
			if err := role.ValidateName(cmd.ApplicationCode + ":" + cmd.RoleName); err != nil {
				return err
			}
			if err := role.ValidatePermissions(cmd.Permissions); err != nil {
				return err
			}
			return nil
		},
		// Roles are global (no per-client resource dimension), so there is no
		// per-resource authz; the permission for the operation (CanCreateRoles)
		// is enforced at the controller. In Execute, after the duplicate check:
		// confinement (F12), the role may carry only its own application's
		// permissions unless the caller is a super-admin, 400
		// role.OutsideApplicationCode; then the permission ceiling (F10), only
		// platform permissions the caller holds, 403 role.PermissionCeilingCode.
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
			// F18: the role is written with the id of the application that owns
			// it, and a role for an application that does not exist is refused.
			appID, err := repo.ApplicationIDByCode(ctx, cmd.ApplicationCode)
			if err != nil {
				return nil, usecase.Internal("REPO", "application_by_code failed", err)
			}
			if appID == "" {
				return nil, usecase.Validation("APPLICATION_NOT_FOUND",
					"Application '"+cmd.ApplicationCode+"' does not exist")
			}
			r := role.New(cmd.ApplicationCode, cmd.RoleName, cmd.DisplayName)
			r.ApplicationID = &appID
			r.Description = cmd.Description
			r.ClientManaged = cmd.ClientManaged
			for _, p := range cmd.Permissions {
				r.GrantPermission(p)
			}
			if err := role.RequirePermissionsInApplication(r.OwningApplicationCode(), r.Permissions,
				crossApplication(ctx)); err != nil {
				return nil, err
			}
			if err := role.RequirePermissionsWithinCaller(auth.FromContext(ctx), r.Permissions); err != nil {
				return nil, err
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
