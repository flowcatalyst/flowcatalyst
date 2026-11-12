//go:build integration

package serviceaccount

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// A credential stored under a different key (a rotated or wrong
// FLOWCATALYST_APP_KEY) is ABSENT for delivery: the resolver reports no usable
// credential, with a reason that says why, and the stored ciphertext is never
// handed out. The admin read still reports the account HAS a credential.
func TestResolver_UndecryptableCredentialIsAbsentNotCiphertext(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := NewRepository(pool)
	require.NotNil(t, repo.enc)

	otherKey, err := encryption.GenerateKey()
	require.NoError(t, err)
	other, err := encryption.New(otherKey)
	require.NoError(t, err)
	foreign, err := other.Encrypt("the-real-signing-secret")
	require.NoError(t, err)
	foreign = "encrypted:" + foreign

	const id = "sa_undecrypt_0001"
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_service_accounts (id, code, name, active, wh_auth_type, wh_signing_secret_ref)
		 VALUES ($1, 'undecrypt-code', 'Undecryptable', true, 'BEARER_TOKEN', $2)`, id, foreign)
	require.NoError(t, err)

	var logs syncBuf
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	resolve := NewCachedOutboundCredsByIDResolver(repo, time.Minute)
	creds, err := resolve(ctx, id)
	require.NoError(t, err)
	assert.True(t, creds.Empty(), "an undecryptable credential is not a usable one")
	assert.Empty(t, creds.SigningSecret)
	assert.Empty(t, creds.BearerToken)
	assert.Contains(t, creds.Reason, "could not be decrypted")
	assert.Contains(t, creds.Reason, "undecrypt-code")
	assert.NotContains(t, creds.Reason, foreign)

	_, err = resolve(ctx, id) // cached: no second lookup, no second log line
	require.NoError(t, err)
	out := logs.String()
	assert.Equal(t, 1, strings.Count(out, `"level":"ERROR"`), "one error, at the point of failure: %s", out)
	assert.Contains(t, out, id)
	assert.Contains(t, out, "undecrypt-code")
	assert.NotContains(t, out, foreign, "the ciphertext is never logged")

	// The admin read still says a credential exists (and a re-persist of the
	// loaded row would write the same ciphertext back).
	sa, err := repo.FindByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, sa)
	require.NotNil(t, sa.WebhookCredentials.SigningSecret, "the account still HAS a stored credential")
	assert.Equal(t, foreign, *sa.WebhookCredentials.SigningSecret)
	assert.True(t, IsUndecryptable(sa.WebhookCredentials.SigningSecret))
	assert.EqualValues(t, "BEARER_TOKEN", sa.WebhookCredentials.AuthType)
}

// A credential that decrypts under the configured key is handed out exactly
// as before.
func TestResolver_DecryptableCredentialStillResolves(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := NewRepository(pool)
	require.NotNil(t, repo.enc)
	tok, err := repo.enc.Encrypt("tok-plain")
	require.NoError(t, err)
	tok = "encrypted:" + tok
	sec, err := repo.enc.Encrypt("sec-plain")
	require.NoError(t, err)
	sec = "encrypted:" + sec

	const id = "sa_decrypt_00001"
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_service_accounts (id, code, name, active, wh_auth_type, wh_auth_token_ref, wh_signing_secret_ref)
		 VALUES ($1, 'decrypt-code', 'Decryptable', true, 'BEARER_TOKEN', $2, $3)`, id, tok, sec)
	require.NoError(t, err)

	creds, err := NewCachedOutboundCredsByIDResolver(repo, time.Minute)(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "tok-plain", creds.BearerToken)
	assert.Equal(t, "sec-plain", creds.SigningSecret)
	assert.Empty(t, creds.Reason)
	assert.Equal(t, "decrypt-code", creds.SignedBy)
}
