package operations

import (
	"context"
	"time"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/principal"
)

// RoleInput represents a role to assign
type RoleInput struct {
	RoleID   string `json:"roleId"`
	RoleName string `json:"roleName"`
}

// AssignRolesCommand contains the data needed to assign roles to a user
type AssignRolesCommand struct {
	PrincipalID string      `json:"principalId"`
	Roles       []RoleInput `json:"roles"`
}

// AssignRolesUseCase handles assigning roles to a principal
type AssignRolesUseCase struct {
	repo       principal.Repository
	unitOfWork common.UnitOfWork
}

// NewAssignRolesUseCase creates a new AssignRolesUseCase
func NewAssignRolesUseCase(repo principal.Repository, uow common.UnitOfWork) *AssignRolesUseCase {
	return &AssignRolesUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute assigns roles to a principal
func (uc *AssignRolesUseCase) Execute(
	ctx context.Context,
	cmd AssignRolesCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.PrincipalID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_PRINCIPAL_ID", "Principal ID is required", nil),
		)
	}

	// Fetch existing principal
	existing, err := uc.repo.FindByID(ctx, cmd.PrincipalID)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to find principal", map[string]any{"error": err.Error()}),
		)
	}
	if existing == nil {
		return common.Failure[common.DomainEvent](
			common.NotFoundError("PRINCIPAL_NOT_FOUND", "Principal not found", map[string]any{"id": cmd.PrincipalID}),
		)
	}

	// Build role assignments
	now := time.Now()
	assignments := make([]principal.RoleAssignment, len(cmd.Roles))
	roleIDs := make([]string, len(cmd.Roles))
	roleNames := make([]string, len(cmd.Roles))

	for i, r := range cmd.Roles {
		assignments[i] = principal.RoleAssignment{
			RoleID:           r.RoleID,
			RoleName:         r.RoleName,
			AssignmentSource: "API",
			AssignedAt:       now,
		}
		roleIDs[i] = r.RoleID
		roleNames[i] = r.RoleName
	}

	// Update roles
	existing.Roles = assignments

	// Create domain event
	event := events.NewPrincipalRolesAssigned(execCtx, existing, roleIDs, roleNames)

	// Atomic commit
	if existing.ClientID != "" {
		return uc.unitOfWork.CommitWithClientID(ctx, existing, event, cmd, existing.ClientID)
	}
	return uc.unitOfWork.Commit(ctx, existing, event, cmd)
}
