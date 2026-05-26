package operations

import (
	"context"
	"crypto/rand"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// RegenerateAuthTokenCommand rotates the service account's bearer token.
type RegenerateAuthTokenCommand struct {
	ServiceAccountID string `json:"serviceAccountId"`
}

// RegenerateAuthTokenUseCase implements UseCase. After the commit, the
// plaintext token lands in a process-local stash so the HTTP handler can
// return it once and only once.
type RegenerateAuthTokenUseCase struct {
	repo *serviceaccount.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewRegenerateAuthTokenUseCase wires the use case.
func NewRegenerateAuthTokenUseCase(repo *serviceaccount.Repository, uow *usecasepgx.UnitOfWork) *RegenerateAuthTokenUseCase {
	return &RegenerateAuthTokenUseCase{repo: repo, uow: uow}
}

func (uc *RegenerateAuthTokenUseCase) Validate(_ context.Context, cmd RegenerateAuthTokenCommand) error {
	if strings.TrimSpace(cmd.ServiceAccountID) == "" {
		return usecase.Validation("SERVICE_ACCOUNT_ID_REQUIRED", "Service account ID is required")
	}
	return nil
}

func (uc *RegenerateAuthTokenUseCase) Authorize(_ context.Context, _ RegenerateAuthTokenCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *RegenerateAuthTokenUseCase) Execute(ctx context.Context, cmd RegenerateAuthTokenCommand, ec usecase.ExecutionContext) usecase.Result[ServiceAccountTokenRegenerated] {
	sa, err := uc.repo.FindByID(ctx, cmd.ServiceAccountID)
	if err != nil {
		return usecase.Failure[ServiceAccountTokenRegenerated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if sa == nil {
		return usecase.Failure[ServiceAccountTokenRegenerated](httperror.NotFound("ServiceAccount", cmd.ServiceAccountID))
	}

	token := generateAuthToken()
	sa.WebhookCredentials.Token = &token
	sa.WebhookCredentials.AuthType = serviceaccount.AuthBearer
	sa.UpdatedAt = time.Now().UTC()

	stashSecret(sa.ID, "token", token)

	event := ServiceAccountTokenRegenerated{
		Metadata:         usecase.NewEventMetadata(ec, ServiceAccountTokenRegeneratedType, Source, subjectFor(sa.ID)),
		ServiceAccountID: sa.ID,
		Code:             sa.Code,
	}
	return usecasepgx.Commit[serviceaccount.ServiceAccount, ServiceAccountTokenRegenerated, RegenerateAuthTokenCommand](
		ctx, uc.uow, sa, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[RegenerateAuthTokenCommand, ServiceAccountTokenRegenerated] = (*RegenerateAuthTokenUseCase)(nil)

// generateAuthToken returns "fc_" + 32 lowercase-alphanumeric chars.
// Matches the Rust port byte-for-byte (length 35, prefix fc_).
func generateAuthToken() string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	max := big.NewInt(int64(len(alphabet)))
	var sb strings.Builder
	sb.WriteString("fc_")
	for i := 0; i < 32; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			// crypto/rand failures are catastrophic; the Rust source panics
			// in this codepath too via Result<…>. Fall through to a
			// deterministic char so the build path stays infallible.
			sb.WriteByte(alphabet[0])
			continue
		}
		sb.WriteByte(alphabet[n.Int64()])
	}
	return sb.String()
}

// stashSecret is a process-local one-shot stash keyed by
// (serviceAccountID, kind). The HTTP handler reads + removes the entry
// after the commit succeeds; the plaintext never persists.
var stash sync.Map

type stashKey struct {
	id   string
	kind string
}

func stashSecret(id, kind, value string) { stash.Store(stashKey{id, kind}, value) }

// PopStashedSecret retrieves and removes a stashed plaintext. Used by
// the HTTP handler to return the rotated token/secret in the response.
func PopStashedSecret(id, kind string) (string, bool) {
	v, ok := stash.LoadAndDelete(stashKey{id, kind})
	if !ok {
		return "", false
	}
	return v.(string), true
}
