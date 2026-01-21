package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/client"
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
)

// UpdateClientCommand contains the data needed to update a client
type UpdateClientCommand struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// UpdateClientUseCase handles updating a client
type UpdateClientUseCase struct {
	repo       client.Repository
	unitOfWork common.UnitOfWork
}

// NewUpdateClientUseCase creates a new UpdateClientUseCase
func NewUpdateClientUseCase(repo client.Repository, uow common.UnitOfWork) *UpdateClientUseCase {
	return &UpdateClientUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute updates a client
func (uc *UpdateClientUseCase) Execute(
	ctx context.Context,
	cmd UpdateClientCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.ID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_ID", "Client ID is required", nil),
		)
	}

	if cmd.Name == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_NAME", "Client name is required", nil),
		)
	}

	// Fetch existing client
	existing, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to find client", map[string]any{"error": err.Error()}),
		)
	}
	if existing == nil {
		return common.Failure[common.DomainEvent](
			common.NotFoundError("CLIENT_NOT_FOUND", "Client not found", map[string]any{"id": cmd.ID}),
		)
	}

	// Update fields (identifier is immutable)
	existing.Name = cmd.Name

	// Create domain event
	event := events.NewClientUpdated(execCtx, existing)

	// Atomic commit
	return uc.unitOfWork.Commit(ctx, existing, event, cmd)
}
