package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/role"
)

// DeleteRoleCommand contains the data needed to delete a role
type DeleteRoleCommand struct {
	ID string `json:"id"`
}

// DeleteRoleUseCase handles deleting a role
type DeleteRoleUseCase struct {
	repo       role.Repository
	unitOfWork common.UnitOfWork
}

// NewDeleteRoleUseCase creates a new DeleteRoleUseCase
func NewDeleteRoleUseCase(repo role.Repository, uow common.UnitOfWork) *DeleteRoleUseCase {
	return &DeleteRoleUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute deletes a role
func (uc *DeleteRoleUseCase) Execute(
	ctx context.Context,
	cmd DeleteRoleCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.ID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_ID", "Role ID is required", nil),
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

	// Cannot delete built-in roles
	if existing.BuiltIn {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("BUILTIN_ROLE", "Cannot delete built-in roles", map[string]any{"id": cmd.ID}),
		)
	}

	// Create domain event before deletion
	event := events.NewRoleDeleted(execCtx, existing)

	// Atomic commit with delete
	return uc.unitOfWork.CommitDelete(ctx, existing, event, cmd)
}
