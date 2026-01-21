package operations

import (
	"context"
	"time"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/serviceaccount"
)

// RotateCredentialsCommand contains the data needed to rotate service account credentials
type RotateCredentialsCommand struct {
	ID               string                           `json:"id"`
	AuthType         serviceaccount.WebhookAuthType   `json:"authType,omitempty"`
	SigningAlgorithm serviceaccount.SigningAlgorithm  `json:"signingAlgorithm,omitempty"`
}

// RotateCredentialsUseCase handles rotating service account credentials
type RotateCredentialsUseCase struct {
	repo       *serviceaccount.Repository
	unitOfWork common.UnitOfWork
}

// NewRotateCredentialsUseCase creates a new RotateCredentialsUseCase
func NewRotateCredentialsUseCase(repo *serviceaccount.Repository, uow common.UnitOfWork) *RotateCredentialsUseCase {
	return &RotateCredentialsUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute rotates the credentials for a service account
func (uc *RotateCredentialsUseCase) Execute(
	ctx context.Context,
	cmd RotateCredentialsCommand,
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

	// Check if active
	if !existing.Active {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("INACTIVE_ACCOUNT", "Cannot rotate credentials for inactive service account", map[string]any{"id": cmd.ID}),
		)
	}

	// Rotate credentials
	now := time.Now()
	authType := cmd.AuthType
	if authType == "" {
		authType = serviceaccount.WebhookAuthTypeBearer
	}
	signingAlg := cmd.SigningAlgorithm
	if signingAlg == "" {
		signingAlg = serviceaccount.SigningAlgorithmHMACSHA256
	}

	if existing.WebhookCredentials == nil {
		existing.WebhookCredentials = &serviceaccount.WebhookCredentials{
			AuthType:         authType,
			SigningAlgorithm: signingAlg,
			CreatedAt:        now,
		}
	} else {
		existing.WebhookCredentials.AuthType = authType
		existing.WebhookCredentials.SigningAlgorithm = signingAlg
	}
	existing.WebhookCredentials.RegeneratedAt = now

	// Note: Actual credential generation (tokens, secrets) should be handled
	// by a secrets manager integration that updates AuthTokenRef and SigningSecretRef

	// Create domain event
	event := events.NewServiceAccountCredentialsRotated(execCtx, existing)

	// Atomic commit
	return uc.unitOfWork.Commit(ctx, existing, event, cmd)
}
