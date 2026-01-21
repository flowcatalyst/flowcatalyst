package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/role"
)

// UpdateRoleCommand contains the data needed to update a role
type UpdateRoleCommand struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

// UpdateRoleUseCase handles updating a role
type UpdateRoleUseCase struct {
	repo       role.Repository
	unitOfWork common.UnitOfWork
}

// NewUpdateRoleUseCase creates a new UpdateRoleUseCase
func NewUpdateRoleUseCase(repo role.Repository, uow common.UnitOfWork) *UpdateRoleUseCase {
	return &UpdateRoleUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute updates a role
func (uc *UpdateRoleUseCase) Execute(
	ctx context.Context,
	cmd UpdateRoleCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.ID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_ID", "Role ID is required", nil),
		)
	}

	if cmd.Name == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_NAME", "Role name is required", nil),
		)
	}

	// Fetch existing role
	existing, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to find role", map[string]any{"error": err.Error()}),
		)
	}
	if existing == nil {
		return common.Failure[common.DomainEvent](
			common.NotFoundError("ROLE_NOT_FOUND", "Role not found", map[string]any{"id": cmd.ID}),
		)
	}

	// Cannot update built-in roles
	if existing.BuiltIn {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("BUILTIN_ROLE", "Cannot modify built-in roles", map[string]any{"id": cmd.ID}),
		)
	}

	// Update fields (code is immutable)
	existing.Name = cmd.Name
	existing.Description = cmd.Description
	existing.Permissions = cmd.Permissions

	// Create domain event
	event := events.NewRoleUpdated(execCtx, existing)

	// Atomic commit
	return uc.unitOfWork.Commit(ctx, existing, event, cmd)
}
