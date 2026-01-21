package operations

import (
	"context"
	"time"

	"go.flowcatalyst.tech/internal/platform/client"
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/principal"
)

// GrantClientAccessCommand contains the data needed to grant client access
type GrantClientAccessCommand struct {
	PrincipalID string     `json:"principalId"`
	ClientID    string     `json:"clientId"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

// GrantClientAccessUseCase handles granting a principal access to a client
type GrantClientAccessUseCase struct {
	principalRepo principal.Repository
	clientRepo    client.Repository
	unitOfWork    common.UnitOfWork
}

// NewGrantClientAccessUseCase creates a new GrantClientAccessUseCase
func NewGrantClientAccessUseCase(
	principalRepo principal.Repository,
	clientRepo client.Repository,
	uow common.UnitOfWork,
) *GrantClientAccessUseCase {
	return &GrantClientAccessUseCase{
		principalRepo: principalRepo,
		clientRepo:    clientRepo,
		unitOfWork:    uow,
	}
}

// Execute grants a principal access to a client
func (uc *GrantClientAccessUseCase) Execute(
	ctx context.Context,
	cmd GrantClientAccessCommand,
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
	p, err := uc.principalRepo.FindByID(ctx, cmd.PrincipalID)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to find principal", map[string]any{"error": err.Error()}),
		)
	}
	if p == nil {
		return common.Failure[common.DomainEvent](
			common.NotFoundError("PRINCIPAL_NOT_FOUND", "Principal not found", map[string]any{"id": cmd.PrincipalID}),
		)
	}

	// Only PARTNER scope users can be granted access to multiple clients
	if p.Scope != principal.UserScopePartner && p.Scope != principal.UserScopeAnchor {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("INVALID_SCOPE",
				"Only PARTNER or ANCHOR scope principals can be granted client access",
				map[string]any{"scope": p.Scope}),
		)
	}

	// Check if already has access
	hasAccess, err := uc.clientRepo.HasAccess(ctx, cmd.PrincipalID, cmd.ClientID)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to check access", map[string]any{"error": err.Error()}),
		)
	}
	if hasAccess {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("ACCESS_EXISTS",
				"Principal already has access to this client",
				map[string]any{"principalId": cmd.PrincipalID, "clientId": cmd.ClientID}),
		)
	}

	// Create access grant
	grant := &client.ClientAccessGrant{
		PrincipalID: cmd.PrincipalID,
		ClientID:    cmd.ClientID,
		GrantedAt:   time.Now(),
	}
	if cmd.ExpiresAt != nil {
		grant.ExpiresAt = *cmd.ExpiresAt
	}

	// Create domain event
	event := events.NewPrincipalClientAccessGranted(execCtx, cmd.PrincipalID, cmd.ClientID)

	// Atomic commit
	return uc.unitOfWork.Commit(ctx, grant, event, cmd)
}
