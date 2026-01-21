package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/serviceaccount"
)

// DeleteServiceAccountCommand contains the data needed to delete a service account
type DeleteServiceAccountCommand struct {
	ID string `json:"id"`
}

// DeleteServiceAccountUseCase handles deleting a service account
type DeleteServiceAccountUseCase struct {
	repo       *serviceaccount.Repository
	unitOfWork common.UnitOfWork
}

// NewDeleteServiceAccountUseCase creates a new DeleteServiceAccountUseCase
func NewDeleteServiceAccountUseCase(repo *serviceaccount.Repository, uow common.UnitOfWork) *DeleteServiceAccountUseCase {
	return &DeleteServiceAccountUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute deletes a service account
func (uc *DeleteServiceAccountUseCase) Execute(
	ctx context.Context,
	cmd DeleteServiceAccountCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.ID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_ID", "Service account ID is required", nil),
		)
	}

	// Fetch existing service account
	existing, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to find service account", map[string]any{"error": err.Error()}),
		)
	}
	if existing == nil {
		return common.Failure[common.DomainEvent](
			common.NotFoundError("SERVICE_ACCOUNT_NOT_FOUND", "Service account not found", map[string]any{"id": cmd.ID}),
		)
	}

	// Create domain event before deletion
	event := events.NewServiceAccountDeleted(execCtx, existing)

	// Atomic commit with delete
	return uc.unitOfWork.CommitDelete(ctx, existing, event, cmd)
}
