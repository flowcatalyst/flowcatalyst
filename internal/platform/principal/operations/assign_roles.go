package operations

import (
	"context"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

type AssignRolesCommand struct {
	UserID ids.PrincipalID `json:"userId"`
	Roles  []string        `json:"roles"`
}

// AssignRoles replaces a user principal's full role set and emits
// [RolesAssigned].
//
// Resource-level authorization (requireRoleAssigner against the loaded
// principal: the controller's auth.RequireUserAdmin + blockNonClientTarget,
// then user:assign-roles) runs post-load in Execute. The permission is checked
// here as well as at the routes' pre-load gate so every caller of the
// operation needs it, the bulk import's rows included (owner ruling
// 2026-10-11: assign-roles, not update, governs a user's roles). The
// application-scoped role BOUNDING for non-anchor admins (assertAssignableRoles
// + preserved roles) is command-shaping the controller still performs before
// building the desired set. The role ceiling (role.RequireRolesWithinCaller,
// F8) then bounds the change for every caller: 403 ROLE_ABOVE_CALLER.
func AssignRoles(repo *principal.Repository, roles *role.Repository) usecaseop.Operation[AssignRolesCommand, RolesAssigned] {
	return usecaseop.Operation[AssignRolesCommand, RolesAssigned]{
		Name: "AssignRoles",
		Validate: func(_ context.Context, cmd AssignRolesCommand) error {
			if strings.TrimSpace(string(cmd.UserID)) == "" {
				return usecase.Validation("USER_ID_REQUIRED", "User ID is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[AssignRolesCommand],
		Execute: func(ctx context.Context, cmd AssignRolesCommand, ec usecase.ExecutionContext) (usecaseop.Plan[RolesAssigned], error) {
			p, err := repo.FindByID(ctx, cmd.UserID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if p == nil {
				return nil, httperror.NotFound("User", string(cmd.UserID))
			}
			if err := requireRoleAssigner(ctx, p); err != nil {
				return nil, err
			}
			if p.Type != principal.TypeUser {
				return nil, usecase.BusinessRule("NOT_A_USER",
					"Roles can only be assigned to USER type principals")
			}

			// Every name is shape-checked (400 INVALID_ROLE_NAME) and resolved
			// to an existing role (400 ROLE_NOT_FOUND); the assignments below
			// are built from the resolved role.Name, never the raw string.
			resolved, err := role.ResolveNames(ctx, roles, cmd.Roles)
			if err != nil {
				return nil, err
			}

			previous := make([]string, 0, len(p.Roles))
			for _, ra := range p.Roles {
				previous = append(previous, ra.Role.String())
			}
			// The role ceiling (F8): only roles whose platform permissions the
			// caller holds may be added or removed; kept roles are not checked.
			if err := role.RequireRolesWithinCaller(ctx, auth.FromContext(ctx),
				role.ChangedRoles(previous, cmd.Roles), role.PermissionsFrom(roles)); err != nil {
				return nil, err
			}
			added := stringDifference(cmd.Roles, previous)
			removed := stringDifference(previous, cmd.Roles)

			now := time.Now().UTC()
			src := "ADMIN_ASSIGNED"
			newAssignments := make([]serviceaccount.RoleAssignment, 0, len(cmd.Roles))
			for _, name := range resolved {
				newAssignments = append(newAssignments, serviceaccount.RoleAssignment{
					Role:             name,
					AssignmentSource: &src,
					AssignedAt:       now,
				})
			}
			p.Roles = newAssignments
			p.UpdatedAt = now

			event := RolesAssigned{
				Metadata: usecase.NewEventMetadata(ec, RolesAssignedType, Source, subjectFor(p.ID)),
				UserID:   p.ID,
				Roles:    cmd.Roles,
				Added:    added,
				Removed:  removed,
			}
			// RolesPersister rewrites the iam_principal_roles junction from p.Roles
			// in the same tx as the event — the base principal Persist writes only
			// the iam_principals row.
			return usecaseop.Save(p, principal.RolesPersister{Repository: repo}, event), nil
		},
	}
}

func stringDifference(a, b []string) []string {
	in := make(map[string]struct{}, len(b))
	for _, x := range b {
		in[x] = struct{}{}
	}
	out := []string{}
	for _, x := range a {
		if _, ok := in[x]; !ok {
			out = append(out, x)
		}
	}
	return out
}
