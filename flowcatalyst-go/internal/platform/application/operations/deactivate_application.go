package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/application"
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
)

// DeactivateApplicationCommand contains the data needed to deactivate an application
type DeactivateApplicationCommand struct {
	ID string `json:"id"`
}

// DeactivateApplicationUseCase handles deactivating an application
type DeactivateApplicationUseCase struct {
	repo       *application.Repository
	unitOfWork common.UnitOfWork
}

// NewDeactivateApplicationUseCase creates a new DeactivateApplicationUseCase
func NewDeactivateApplicationUseCase(repo *application.Repository, uow common.UnitOfWork) *DeactivateApplicationUseCase {
	return &DeactivateApplicationUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute deactivates an application
func (uc *DeactivateApplicationUseCase) Execute(
	ctx context.Context,
	cmd DeactivateApplicationCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.ID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_ID", "Application ID is required", nil),
		)
	}

	// Fetch existing application
	existing, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to find application", map[string]any{"error": err.Error()}),
		)
	}
	if existing == nil {
		return common.Failure[common.DomainEvent](
			common.NotFoundError("APPLICATION_NOT_FOUND", "Application not found", map[string]any{"id": cmd.ID}),
		)
	}

	// Check if already inactive
	if !existing.Active {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("ALREADY_INACTIVE", "Application is already inactive", map[string]any{"id": cmd.ID}),
		)
	}

	// Deactivate the application
	existing.Active = false

	// Create domain event
	event := events.NewApplicationDeactivated(execCtx, existing)

	// Atomic commit
	return uc.unitOfWork.Commit(ctx, existing, event, cmd)
}
