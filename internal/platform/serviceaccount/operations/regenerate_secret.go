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
}

// RegenerateSigningSecret rotates the signing secret. Plaintext lands in
// the process-local stash for the HTTP handler to read once.
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
			stashSecret(sa.ID, "signing_secret", secret)

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
// without padding (matches the Rust port).
func generateSigningSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
