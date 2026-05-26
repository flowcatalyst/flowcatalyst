package operations

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// RegenerateSigningSecretCommand rotates the HMAC signing secret.
type RegenerateSigningSecretCommand struct {
	ServiceAccountID string `json:"serviceAccountId"`
}

// RegenerateSigningSecretUseCase implements UseCase. Same stash pattern
// as regenerate_token — plaintext lands in the process-local stash for
// the HTTP handler to read once.
type RegenerateSigningSecretUseCase struct {
	repo *serviceaccount.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewRegenerateSigningSecretUseCase wires the use case.
func NewRegenerateSigningSecretUseCase(repo *serviceaccount.Repository, uow *usecasepgx.UnitOfWork) *RegenerateSigningSecretUseCase {
	return &RegenerateSigningSecretUseCase{repo: repo, uow: uow}
}

func (uc *RegenerateSigningSecretUseCase) Validate(_ context.Context, cmd RegenerateSigningSecretCommand) error {
	if strings.TrimSpace(cmd.ServiceAccountID) == "" {
		return usecase.Validation("SERVICE_ACCOUNT_ID_REQUIRED", "Service account ID is required")
	}
	return nil
}

func (uc *RegenerateSigningSecretUseCase) Authorize(_ context.Context, _ RegenerateSigningSecretCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *RegenerateSigningSecretUseCase) Execute(ctx context.Context, cmd RegenerateSigningSecretCommand, ec usecase.ExecutionContext) usecase.Result[ServiceAccountSecretRegenerated] {
	sa, err := uc.repo.FindByID(ctx, cmd.ServiceAccountID)
	if err != nil {
		return usecase.Failure[ServiceAccountSecretRegenerated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if sa == nil {
		return usecase.Failure[ServiceAccountSecretRegenerated](httperror.NotFound("ServiceAccount", cmd.ServiceAccountID))
	}

	secret := generateSigningSecret()
	sa.WebhookCredentials.SigningSecret = &secret
	sa.UpdatedAt = time.Now().UTC()
	stashSecret(sa.ID, "signing_secret", secret)

	event := ServiceAccountSecretRegenerated{
		Metadata:         usecase.NewEventMetadata(ec, ServiceAccountSecretRegeneratedType, Source, subjectFor(sa.ID)),
		ServiceAccountID: sa.ID,
		Code:             sa.Code,
	}
	return usecasepgx.Commit[serviceaccount.ServiceAccount, ServiceAccountSecretRegenerated, RegenerateSigningSecretCommand](
		ctx, uc.uow, sa, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[RegenerateSigningSecretCommand, ServiceAccountSecretRegenerated] = (*RegenerateSigningSecretUseCase)(nil)

// generateSigningSecret returns 32 random bytes URL-safe-base64 encoded
// without padding (matches the Rust port).
func generateSigningSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
