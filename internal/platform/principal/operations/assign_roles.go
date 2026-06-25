package operations

import (
	"context"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

type AssignRolesCommand struct {
	UserID string   `json:"userId"`
	Roles  []string `json:"roles"`
}

// AssignRoles replaces a user principal's full role set and emits
// [RolesAssigned].
//
// Resource-level authorization (requireUserAdmin against the loaded principal —
// the use-case move of the controller's auth.RequireUserAdmin +
// blockNonClientTarget) runs post-load in Execute. The application-scoped role
// BOUNDING for non-anchor admins (assertAssignableRoles + preserved roles) is
// command-shaping the controller still performs before building the desired set.
func AssignRoles(repo *principal.Repository, roles *role.Repository) usecaseop.Operation[AssignRolesCommand, RolesAssigned] {
	return usecaseop.Operation[AssignRolesCommand, RolesAssigned]{
		Name: "AssignRoles",
		Validate: func(_ context.Context, cmd AssignRolesCommand) error {
			if strings.TrimSpace(cmd.UserID) == "" {
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
				return nil, httperror.NotFound("User", cmd.UserID)
			}
			if err := requireUserAdmin(ctx, p); err != nil {
				return nil, err
			}
			if p.Type != principal.TypeUser {
				return nil, usecase.BusinessRule("NOT_A_USER",
					"Roles can only be assigned to USER type principals")
			}

			for _, name := range cmd.Roles {
				r, err := roles.FindByName(ctx, name)
				if err != nil {
					return nil, usecase.Internal("REPO", "validate role failed", err)
				}
				if r == nil {
					return nil, usecase.Validation("ROLE_NOT_FOUND", "Role not found: "+name)
				}
			}

			previous := make([]string, 0, len(p.Roles))
			for _, ra := range p.Roles {
				previous = append(previous, ra.Role)
			}
			added := stringDifference(cmd.Roles, previous)
			removed := stringDifference(previous, cmd.Roles)

			now := time.Now().UTC()
			src := "ADMIN_ASSIGNED"
			newAssignments := make([]serviceaccount.RoleAssignment, 0, len(cmd.Roles))
			for _, name := range cmd.Roles {
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
