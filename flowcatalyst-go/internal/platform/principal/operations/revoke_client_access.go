package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/client"
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/principal"
)

// RevokeClientAccessCommand contains the data needed to revoke client access
type RevokeClientAccessCommand struct {
	PrincipalID string `json:"principalId"`
	ClientID    string `json:"clientId"`
}

// RevokeClientAccessUseCase handles revoking a principal's access to a client
type RevokeClientAccessUseCase struct {
	principalRepo principal.Repository
	clientRepo    client.Repository
	unitOfWork    common.UnitOfWork
}

// NewRevokeClientAccessUseCase creates a new RevokeClientAccessUseCase
func NewRevokeClientAccessUseCase(
	principalRepo principal.Repository,
	clientRepo client.Repository,
	uow common.UnitOfWork,
) *RevokeClientAccessUseCase {
	return &RevokeClientAccessUseCase{
		principalRepo: principalRepo,
		clientRepo:    clientRepo,
		unitOfWork:    uow,
	}
}

// Execute revokes a principal's access to a client
func (uc *RevokeClientAccessUseCase) Execute(
	ctx context.Context,
	cmd RevokeClientAccessCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.PrincipalID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_PRINCIPAL_ID", "Principal ID is required", nil),
		)
	}

	if cmd.ClientID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_CLIENT_ID", "Client ID is required", nil),
		)
	}

	// Verify principal exists
	_, err := uc.principalRepo.FindByID(ctx, cmd.PrincipalID)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to find principal", map[string]any{"error": err.Error()}),
		)
	}

	// Check if has access
	hasAccess, err := uc.clientRepo.HasAccess(ctx, cmd.PrincipalID, cmd.ClientID)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to check access", map[string]any{"error": err.Error()}),
		)
	}
	if !hasAccess {
		return common.Failure[common.DomainEvent](
			common.NotFoundError("ACCESS_NOT_FOUND",
				"Principal does not have access to this client",
				map[string]any{"principalId": cmd.PrincipalID, "clientId": cmd.ClientID}),
		)
	}

	// Create a placeholder for the delete operation
	grant := &client.ClientAccessGrant{
		PrincipalID: cmd.PrincipalID,
		ClientID:    cmd.ClientID,
	}

	// Create domain event
	event := events.NewPrincipalClientAccessRevoked(execCtx, cmd.PrincipalID, cmd.ClientID)

	// Atomic commit with delete
	return uc.unitOfWork.CommitDelete(ctx, grant, event, cmd)
}
