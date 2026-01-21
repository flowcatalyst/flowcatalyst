package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/principal"
)

// UpdateUserCommand contains the data needed to update a user
type UpdateUserCommand struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// UpdateUserUseCase handles updating a user
type UpdateUserUseCase struct {
	repo       principal.Repository
	unitOfWork common.UnitOfWork
}

// NewUpdateUserUseCase creates a new UpdateUserUseCase
func NewUpdateUserUseCase(repo principal.Repository, uow common.UnitOfWork) *UpdateUserUseCase {
	return &UpdateUserUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute updates a user
func (uc *UpdateUserUseCase) Execute(
	ctx context.Context,
	cmd UpdateUserCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.ID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_ID", "User ID is required", nil),
		)
	}

	if cmd.Name == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_NAME", "Name is required", nil),
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

	// Verify this is a user, not a service account
	if existing.Type != principal.PrincipalTypeUser {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("NOT_A_USER", "Principal is not a user", map[string]any{"id": cmd.ID}),
		)
	}

	// Update fields
	existing.Name = cmd.Name

	// Create domain event
	event := events.NewPrincipalUserUpdated(execCtx, existing)

	// Atomic commit
	if existing.ClientID != "" {
		return uc.unitOfWork.CommitWithClientID(ctx, existing, event, cmd, existing.ClientID)
	}
	return uc.unitOfWork.Commit(ctx, existing, event, cmd)
}
