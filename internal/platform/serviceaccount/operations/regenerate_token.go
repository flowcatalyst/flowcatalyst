package operations

import (
	"context"
	"crypto/rand"
	"math/big"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// RegenerateAuthTokenCommand rotates the service account's bearer token.
type RegenerateAuthTokenCommand struct {
	ServiceAccountID string `json:"serviceAccountId"`
	// Disclose receives the freshly minted plaintext, once, during Execute.
	// The caller owns the sink (a local in the handler), so the secret's
	// lifetime is the request's and a failed commit discloses nothing.
	Disclose func(plaintext string) `json:"-"`
}

// RegenerateAuthToken rotates the service account's bearer token, disclosing
// the plaintext through cmd.Disclose — a sink the CALLER owns.
//
// It used to land in a process-wide stash with a TTL, which meant a rotation
// whose commit failed still left a live plaintext readable for two minutes.
// A caller-owned sink makes three properties structural rather than
// remembered: the plaintext cannot outlive the request (the sink is a local
// in the handler); an unauthorised or rejected request never reaches the
// minting path; and a rolled-back commit discloses nothing, because the
// handler only reads its local on the success path.
func RegenerateAuthToken(repo *serviceaccount.Repository) usecaseop.Operation[RegenerateAuthTokenCommand, ServiceAccountTokenRegenerated] {
	return usecaseop.Operation[RegenerateAuthTokenCommand, ServiceAccountTokenRegenerated]{
		Name: "RegenerateAuthToken",
		Validate: func(_ context.Context, cmd RegenerateAuthTokenCommand) error {
			if strings.TrimSpace(cmd.ServiceAccountID) == "" {
				return usecase.Validation("SERVICE_ACCOUNT_ID_REQUIRED", "Service account ID is required")
			}
			return nil
		},
		// The coarse anchor-only permission is enforced at the controller; this
		// admin-managed rotation has no per-client resource check, so the
		// operation is intentionally open.
		Authorize: usecaseop.Public[RegenerateAuthTokenCommand],
		Execute: func(ctx context.Context, cmd RegenerateAuthTokenCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ServiceAccountTokenRegenerated], error) {
			sa, err := repo.FindByID(ctx, cmd.ServiceAccountID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if sa == nil {
				return nil, httperror.NotFound("ServiceAccount", cmd.ServiceAccountID)
			}

			token := generateAuthToken()
			sa.WebhookCredentials.Token = &token
			sa.WebhookCredentials.AuthType = serviceaccount.AuthBearer
			sa.UpdatedAt = time.Now().UTC()

			if cmd.Disclose != nil {
				cmd.Disclose(token)
			}

			event := ServiceAccountTokenRegenerated{
				Metadata:         usecase.NewEventMetadata(ec, ServiceAccountTokenRegeneratedType, Source, subjectFor(sa.ID)),
				ServiceAccountID: sa.ID,
				Code:             sa.Code,
			}
			return usecaseop.Save(sa, repo, event), nil
		},
	}
}

// generateAuthToken returns "fc_" + 32 lowercase-alphanumeric chars
// (length 35, prefix fc_).
func generateAuthToken() string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	max := big.NewInt(int64(len(alphabet)))
	var sb strings.Builder
	sb.WriteString("fc_")
	for i := 0; i < 32; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			// crypto/rand failures are catastrophic. Fall through to a
			// deterministic char so the build path stays infallible.
			sb.WriteByte(alphabet[0])
			continue
		}
		sb.WriteByte(alphabet[n.Int64()])
	}
	return sb.String()
}
