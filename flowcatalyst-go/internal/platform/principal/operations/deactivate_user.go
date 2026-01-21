package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/principal"
)

// DeactivateUserCommand contains the data needed to deactivate a user
type DeactivateUserCommand struct {
	ID string `json:"id"`
}

// DeactivateUserUseCase handles deactivating an active user
type DeactivateUserUseCase struct {
	repo       principal.Repository
	unitOfWork common.UnitOfWork
}

// NewDeactivateUserUseCase creates a new DeactivateUserUseCase
func NewDeactivateUserUseCase(repo principal.Repository, uow common.UnitOfWork) *DeactivateUserUseCase {
	return &DeactivateUserUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute deactivates an active user
func (uc *DeactivateUserUseCase) Execute(
	ctx context.Context,
	cmd DeactivateUserCommand,
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

	// Check if already inactive
	if !existing.Active {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("ALREADY_INACTIVE", "User is already inactive", map[string]any{"id": cmd.ID}),
		)
	}

	// Deactivate the user
	existing.Active = false

	// Create domain event
	event := events.NewPrincipalUserDeactivated(execCtx, existing)

	// Atomic commit
	if existing.ClientID != "" {
		return uc.unitOfWork.CommitWithClientID(ctx, existing, event, cmd, existing.ClientID)
	}
	return uc.unitOfWork.Commit(ctx, existing, event, cmd)
}
