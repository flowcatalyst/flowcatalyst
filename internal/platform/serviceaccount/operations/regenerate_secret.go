package operations

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// RegenerateSigningSecretCommand rotates the HMAC signing secret.
type RegenerateSigningSecretCommand struct {
	ServiceAccountID string `json:"serviceAccountId"`
}

// RegenerateSigningSecret rotates the signing secret. Plaintext lands in
// the process-local stash for the HTTP handler to read once.
func RegenerateSigningSecret(
	ctx context.Context,
	repo *serviceaccount.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd RegenerateSigningSecretCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ServiceAccountSecretRegenerated], error) {
	var zero commit.Committed[ServiceAccountSecretRegenerated]

	if strings.TrimSpace(cmd.ServiceAccountID) == "" {
		return zero, usecase.Validation("SERVICE_ACCOUNT_ID_REQUIRED", "Service account ID is required")
	}

	sa, err := repo.FindByID(ctx, cmd.ServiceAccountID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if sa == nil {
		return zero, httperror.NotFound("ServiceAccount", cmd.ServiceAccountID)
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
	return commit.Save(ctx, uow, sa, repo, event, cmd)
}

// generateSigningSecret returns 32 random bytes URL-safe-base64 encoded
// without padding (matches the Rust port).
func generateSigningSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
