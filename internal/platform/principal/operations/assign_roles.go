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
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// AssignRolesCommand sets the user's full role list (replaces, not appends).
type AssignRolesCommand struct {
	UserID string   `json:"userId"`
	Roles  []string `json:"roles"`
}

// AssignRolesUseCase implements UseCase.
type AssignRolesUseCase struct {
	principals *principal.Repository
	roles      *role.Repository
	uow        *usecasepgx.UnitOfWork
}

// NewAssignRolesUseCase wires the use case.
func NewAssignRolesUseCase(principals *principal.Repository, roles *role.Repository, uow *usecasepgx.UnitOfWork) *AssignRolesUseCase {
	return &AssignRolesUseCase{principals: principals, roles: roles, uow: uow}
}

func (uc *AssignRolesUseCase) Validate(_ context.Context, cmd AssignRolesCommand) error {
	if strings.TrimSpace(cmd.UserID) == "" {
		return usecase.Validation("USER_ID_REQUIRED", "User ID is required")
	}
	return nil
}

func (uc *AssignRolesUseCase) Authorize(_ context.Context, _ AssignRolesCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *AssignRolesUseCase) Execute(ctx context.Context, cmd AssignRolesCommand, ec usecase.ExecutionContext) usecase.Result[RolesAssigned] {
	p, err := uc.principals.FindByID(ctx, cmd.UserID)
	if err != nil {
		return usecase.Failure[RolesAssigned](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if p == nil {
		return usecase.Failure[RolesAssigned](httperror.NotFound("User", cmd.UserID))
	}
	if p.Type != principal.TypeUser {
		return usecase.Failure[RolesAssigned](usecase.BusinessRule("NOT_A_USER",
			"Roles can only be assigned to USER type principals"))
	}

	// Validate each role exists by name.
	for _, name := range cmd.Roles {
		r, err := uc.roles.FindByName(ctx, name)
		if err != nil {
			return usecase.Failure[RolesAssigned](usecase.Internal("REPO", "validate role failed", err))
		}
		if r == nil {
			return usecase.Failure[RolesAssigned](usecase.Validation("ROLE_NOT_FOUND",
				"Role not found: "+name))
		}
	}

	// Compute delta against the existing role names.
	previous := make([]string, 0, len(p.Roles))
	for _, ra := range p.Roles {
		previous = append(previous, ra.Role)
	}
	added := stringDifference(cmd.Roles, previous)
	removed := stringDifference(previous, cmd.Roles)

	// Replace assignments wholesale, source ADMIN_ASSIGNED.
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
	return usecasepgx.Commit[principal.Principal, RolesAssigned, AssignRolesCommand](
		ctx, uc.uow, p, uc.principals, event, cmd,
	)
}

var _ usecase.UseCase[AssignRolesCommand, RolesAssigned] = (*AssignRolesUseCase)(nil)

// stringDifference returns a − b preserving a's order.
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
