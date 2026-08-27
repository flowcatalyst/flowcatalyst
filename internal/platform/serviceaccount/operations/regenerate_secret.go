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
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// RegenerateSigningSecretCommand rotates the HMAC signing secret.
type RegenerateSigningSecretCommand struct {
	ServiceAccountID string `json:"serviceAccountId"`
	// Disclose receives the freshly minted plaintext, once, during Execute.
	// See RegenerateAuthTokenCommand.Disclose.
	Disclose func(plaintext string) `json:"-"`
}

// RegenerateSigningSecret rotates the signing secret, disclosing the plaintext
// through cmd.Disclose — a caller-owned sink. See RegenerateAuthToken for why
// the process-wide stash it replaced was the wrong shape.
func RegenerateSigningSecret(repo *serviceaccount.Repository) usecaseop.Operation[RegenerateSigningSecretCommand, ServiceAccountSecretRegenerated] {
	return usecaseop.Operation[RegenerateSigningSecretCommand, ServiceAccountSecretRegenerated]{
		Name: "RegenerateSigningSecret",
		Validate: func(_ context.Context, cmd RegenerateSigningSecretCommand) error {
			if strings.TrimSpace(cmd.ServiceAccountID) == "" {
				return usecase.Validation("SERVICE_ACCOUNT_ID_REQUIRED", "Service account ID is required")
			}
			return nil
		},
		// The coarse anchor-only permission is enforced at the controller; this
		// admin-managed rotation has no per-client resource check, so the
		// operation is intentionally open.
		Authorize: usecaseop.Public[RegenerateSigningSecretCommand],
		Execute: func(ctx context.Context, cmd RegenerateSigningSecretCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ServiceAccountSecretRegenerated], error) {
			sa, err := repo.FindByID(ctx, cmd.ServiceAccountID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if sa == nil {
				return nil, httperror.NotFound("ServiceAccount", cmd.ServiceAccountID)
			}

			secret := generateSigningSecret()
			sa.WebhookCredentials.SigningSecret = &secret
			sa.UpdatedAt = time.Now().UTC()
			if cmd.Disclose != nil {
				cmd.Disclose(secret)
			}

			event := ServiceAccountSecretRegenerated{
				Metadata:         usecase.NewEventMetadata(ec, ServiceAccountSecretRegeneratedType, Source, subjectFor(sa.ID)),
				ServiceAccountID: sa.ID,
				Code:             sa.Code,
			}
			return usecaseop.Save(sa, repo, event), nil
		},
	}
}

// generateSigningSecret returns 32 random bytes URL-safe-base64 encoded
// without padding.
// NewGeneratedWebhookCredentials returns bearer credentials with a fresh
// auth token + HMAC signing secret. Used at SA create AND at application
// service-account provisioning, so every sync account can sign the scheduled
// job firings (and future webhooks) of its application.
func NewGeneratedWebhookCredentials() serviceaccount.WebhookCredentials {
	token := generateAuthToken()
	secret := generateSigningSecret()
	return serviceaccount.WebhookCredentials{
		AuthType:      serviceaccount.AuthBearer,
		Token:         &token,
		SigningSecret: &secret,
	}
}

func generateSigningSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
