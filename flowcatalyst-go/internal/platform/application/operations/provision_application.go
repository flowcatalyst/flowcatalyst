package operations

import (
	"context"
	"time"

	"go.flowcatalyst.tech/internal/common/tsid"
	"go.flowcatalyst.tech/internal/platform/application"
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
)

// ProvisionApplicationCommand contains the data needed to provision an application for a client
type ProvisionApplicationCommand struct {
	ApplicationID   string `json:"applicationId"`
	ClientID        string `json:"clientId"`
	BaseURLOverride string `json:"baseUrlOverride,omitempty"`
	ConfigJSON      string `json:"configJson,omitempty"`
}

// ProvisionApplicationUseCase handles provisioning an application for a client
type ProvisionApplicationUseCase struct {
	repo       *application.Repository
	unitOfWork common.UnitOfWork
}

// NewProvisionApplicationUseCase creates a new ProvisionApplicationUseCase
func NewProvisionApplicationUseCase(repo *application.Repository, uow common.UnitOfWork) *ProvisionApplicationUseCase {
	return &ProvisionApplicationUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute provisions an application for a client
func (uc *ProvisionApplicationUseCase) Execute(
	ctx context.Context,
	cmd ProvisionApplicationCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.ApplicationID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_APPLICATION_ID", "Application ID is required", nil),
		)
	}

	if cmd.ClientID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_CLIENT_ID", "Client ID is required", nil),
		)
	}

	// Fetch application
	app, err := uc.repo.FindByID(ctx, cmd.ApplicationID)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to find application", map[string]any{"error": err.Error()}),
		)
	}
	if app == nil {
		return common.Failure[common.DomainEvent](
			common.NotFoundError("APPLICATION_NOT_FOUND", "Application not found", map[string]any{"id": cmd.ApplicationID}),
		)
	}

	// Check if application is active
	if !app.Active {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("APPLICATION_INACTIVE", "Cannot provision an inactive application", map[string]any{"id": cmd.ApplicationID}),
		)
	}

	// Check if already provisioned for this client
	existingConfig, err := uc.repo.FindClientConfig(ctx, cmd.ApplicationID, cmd.ClientID)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to check existing config", map[string]any{"error": err.Error()}),
		)
	}
	if existingConfig != nil {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("ALREADY_PROVISIONED",
				"Application is already provisioned for this client",
				map[string]any{"applicationId": cmd.ApplicationID, "clientId": cmd.ClientID}),
		)
	}

	// Create client config
	now := time.Now()
	config := &application.ApplicationClientConfig{
		ID:              tsid.Generate(),
		ApplicationID:   cmd.ApplicationID,
		ClientID:        cmd.ClientID,
		Enabled:         true,
		BaseURLOverride: cmd.BaseURLOverride,
		ConfigJSON:      cmd.ConfigJSON,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Create domain event
	event := events.NewApplicationProvisioned(execCtx, app, cmd.ClientID, config.ID)

	// Atomic commit
	return uc.unitOfWork.CommitWithClientID(ctx, config, event, cmd, cmd.ClientID)
}
