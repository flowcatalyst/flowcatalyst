package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/principal"
)

// DeleteUserCommand contains the data needed to delete a user
type DeleteUserCommand struct {
	ID string `json:"id"`
}

// DeleteUserUseCase handles deleting a user
type DeleteUserUseCase struct {
	repo       principal.Repository
	unitOfWork common.UnitOfWork
}

// NewDeleteUserUseCase creates a new DeleteUserUseCase
func NewDeleteUserUseCase(repo principal.Repository, uow common.UnitOfWork) *DeleteUserUseCase {
	return &DeleteUserUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute deletes a user
func (uc *DeleteUserUseCase) Execute(
	ctx context.Context,
	cmd DeleteUserCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.ID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_ID", "User ID is required", nil),
		)
	}

	// Fetch existing user
	existing, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to find user", map[string]any{"error": err.Error()}),
		)
	}
	if existing == nil {
		return common.Failure[common.DomainEvent](
			common.NotFoundError("USER_NOT_FOUND", "User not found", map[string]any{"id": cmd.ID}),
		)
	}

	// Verify this is a user
	if existing.Type != principal.PrincipalTypeUser {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("NOT_A_USER", "Principal is not a user", map[string]any{"id": cmd.ID}),
		)
	}

	// Create domain event before deletion
	event := events.NewPrincipalUserDeleted(execCtx, existing)

	// Atomic commit with delete
	return uc.unitOfWork.CommitDelete(ctx, existing, event, cmd)
}
