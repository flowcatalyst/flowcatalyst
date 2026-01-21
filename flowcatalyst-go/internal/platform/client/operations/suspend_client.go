package operations

import (
	"context"
	"time"

	"go.flowcatalyst.tech/internal/platform/client"
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
)

// SuspendClientCommand contains the data needed to suspend a client
type SuspendClientCommand struct {
	ID     string `json:"id"`
	Reason string `json:"reason,omitempty"`
}

// SuspendClientUseCase handles suspending a client
type SuspendClientUseCase struct {
	repo       client.Repository
	unitOfWork common.UnitOfWork
}

// NewSuspendClientUseCase creates a new SuspendClientUseCase
func NewSuspendClientUseCase(repo client.Repository, uow common.UnitOfWork) *SuspendClientUseCase {
	return &SuspendClientUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute suspends a client
func (uc *SuspendClientUseCase) Execute(
	ctx context.Context,
	cmd SuspendClientCommand,
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

	// Check if already suspended
	if existing.IsSuspended() {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("ALREADY_SUSPENDED", "Client is already suspended", map[string]any{"id": cmd.ID}),
		)
	}

	// Suspend the client
	existing.Status = client.ClientStatusSuspended
	existing.StatusReason = cmd.Reason
	existing.StatusChangedAt = time.Now()

	// Create domain event
	event := events.NewClientSuspended(execCtx, existing, cmd.Reason)

	// Atomic commit
	return uc.unitOfWork.Commit(ctx, existing, event, cmd)
}
