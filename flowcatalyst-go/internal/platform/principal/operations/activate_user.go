package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/principal"
)

// ActivateUserCommand contains the data needed to activate a user
type ActivateUserCommand struct {
	ID string `json:"id"`
}

// ActivateUserUseCase handles activating a deactivated user
type ActivateUserUseCase struct {
	repo       principal.Repository
	unitOfWork common.UnitOfWork
}

// NewActivateUserUseCase creates a new ActivateUserUseCase
func NewActivateUserUseCase(repo principal.Repository, uow common.UnitOfWork) *ActivateUserUseCase {
	return &ActivateUserUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute activates a deactivated user
func (uc *ActivateUserUseCase) Execute(
	ctx context.Context,
	cmd ActivateUserCommand,
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

	// Check if already active
	if existing.Active {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("ALREADY_ACTIVE", "User is already active", map[string]any{"id": cmd.ID}),
		)
	}

	// Activate the user
	existing.Active = true

	// Create domain event
	event := events.NewPrincipalUserActivated(execCtx, existing)

	// Atomic commit
	if existing.ClientID != "" {
		return uc.unitOfWork.CommitWithClientID(ctx, existing, event, cmd, existing.ClientID)
	}
	return uc.unitOfWork.Commit(ctx, existing, event, cmd)
}
