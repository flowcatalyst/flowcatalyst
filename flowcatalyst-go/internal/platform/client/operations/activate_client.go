package operations

import (
	"context"
	"time"

	"go.flowcatalyst.tech/internal/platform/client"
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
)

// ActivateClientCommand contains the data needed to activate a client
type ActivateClientCommand struct {
	ID string `json:"id"`
}

// ActivateClientUseCase handles activating a suspended client
type ActivateClientUseCase struct {
	repo       client.Repository
	unitOfWork common.UnitOfWork
}

// NewActivateClientUseCase creates a new ActivateClientUseCase
func NewActivateClientUseCase(repo client.Repository, uow common.UnitOfWork) *ActivateClientUseCase {
	return &ActivateClientUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute activates a suspended client
func (uc *ActivateClientUseCase) Execute(
	ctx context.Context,
	cmd ActivateClientCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.ID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_ID", "Client ID is required", nil),
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

	// Check if already active
	if existing.IsActive() {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("ALREADY_ACTIVE", "Client is already active", map[string]any{"id": cmd.ID}),
		)
	}

	// Activate the client
	existing.Status = client.ClientStatusActive
	existing.StatusReason = ""
	existing.StatusChangedAt = time.Now()

	// Create domain event
	event := events.NewClientActivated(execCtx, existing)

	// Atomic commit
	return uc.unitOfWork.Commit(ctx, existing, event, cmd)
}
