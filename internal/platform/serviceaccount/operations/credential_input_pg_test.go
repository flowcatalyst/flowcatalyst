//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

func storedCreds(t *testing.T, id string) (token, secret string) {
	t.Helper()
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT COALESCE(wh_auth_token_ref, ''), COALESCE(wh_signing_secret_ref, '') FROM iam_service_accounts WHERE id = $1`, id).Scan(&token, &secret))
	return token, secret
}

func bearer(token, secret string) *serviceaccount.WebhookCredentials {
	c := &serviceaccount.WebhookCredentials{AuthType: serviceaccount.AuthBearer}
	if token != "" {
		c.Token = &token
	}
	if secret != "" {
		c.SigningSecret = &secret
	}
	return c
}

// TB1: a webhook credential a caller supplies is its value, never a stored
// ciphertext: the repository would keep an "encrypted:" value as it is and
// decrypt it when the platform signs a delivery.
func TestWebhookCredentialInput_RefusesAStoredCiphertext(t *testing.T) {
	t.Parallel()
	repo := serviceaccount.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	pool := testpg.Pool(t)
	update := operations.UpdateServiceAccount(repo, principal.NewRepository(pool), client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool))

	// A ciphertext copied out of the database: a real one, from another account.
	donor := mustCreate(t, repo, uow, "sacipher-donor", "Donor")
	_, err := runOp(uow, operations.CreateServiceAccount(repo), operations.CreateCommand{
		Code: "sacipher-donor2", Name: "Donor", WebhookCredentials: bearer("donor-token-plaintext", "donor-signing-plaintext"),
	})
	require.NoError(t, err)
	d2, err := repo.FindByCode(context.Background(), "sacipher-donor2")
	require.NoError(t, err)
	copied, copiedSecret := storedCreds(t, d2.ID)
	require.Contains(t, copied, "encrypted:")
	_ = donor

	variants := map[string]*serviceaccount.WebhookCredentials{
		"token":                  bearer(copied, ""),
		"signing secret":         bearer("", copiedSecret),
		"token, leading space":   bearer(" "+copied, ""),
		"token, mixed case":      bearer("Encrypted:"+copied[len("encrypted:"):], ""),
		"secret, upper case":     bearer("", "ENCRYPTED:AAAA"),
		"both, tabs and padding": bearer("\t encrypted:AAAAAAAA ", "  Encrypted:BBBBBBBB"),
	}

	for name, creds := range variants {
		code := "sacipher-" + string(rune('a'+len(name)%26)) + "-" + name[:3]
		_, err := runOp(uow, operations.CreateServiceAccount(repo), operations.CreateCommand{Code: code, Name: "X", WebhookCredentials: creds})
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SECRET_VALUE_INVALID")
		assert.Equal(t, "A secret must be supplied as its value, not as a stored ciphertext", usecase.AsError(err).Message, name)
		found, err := repo.FindByCode(context.Background(), code)
		require.NoError(t, err)
		assert.Nil(t, found, "create refused, nothing stored: %s", name)
	}

	// Update: refused, and the account's own credentials are untouched.
	own, err := runOp(uow, operations.CreateServiceAccount(repo), operations.CreateCommand{
		Code: "sacipher-own", Name: "Own", WebhookCredentials: bearer("own-token-plaintext", "own-signing-plaintext"),
	})
	require.NoError(t, err)
	beforeTok, beforeSec := storedCreds(t, own.ServiceAccountID)
	for name, creds := range variants {
		_, err := runTxOp(uow, update, operations.UpdateCommand{ID: own.ServiceAccountID, WebhookCredentials: creds})
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SECRET_VALUE_INVALID")
		_ = name
	}
	tok, sec := storedCreds(t, own.ServiceAccountID)
	assert.Equal(t, beforeTok, tok, "a refused update changes nothing")
	assert.Equal(t, beforeSec, sec)

	// Plaintext: stored encrypted, read back as the plaintext.
	_, err = runTxOp(uow, update, operations.UpdateCommand{ID: own.ServiceAccountID, WebhookCredentials: bearer("new-plain-token", "new-plain-secret")})
	require.NoError(t, err)
	tok, sec = storedCreds(t, own.ServiceAccountID)
	assert.Contains(t, tok, "encrypted:")
	assert.Contains(t, sec, "encrypted:")
	assert.NotContains(t, tok, "new-plain-token")
	got, err := repo.FindByID(context.Background(), own.ServiceAccountID)
	require.NoError(t, err)
	assert.Equal(t, "new-plain-token", *got.WebhookCredentials.Token)
	assert.Equal(t, "new-plain-secret", *got.WebhookCredentials.SigningSecret)

	// An unrelated change on the loaded account neither changes the
	// credentials nor seals them twice: one decrypt gives the plaintext back.
	newName := "Renamed"
	_, err = runTxOp(uow, update, operations.UpdateCommand{ID: own.ServiceAccountID, Name: &newName})
	require.NoError(t, err)
	got, err = repo.FindByID(context.Background(), own.ServiceAccountID)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", got.Name)
	assert.Equal(t, "new-plain-token", *got.WebhookCredentials.Token)
	assert.Equal(t, "new-plain-secret", *got.WebhookCredentials.SigningSecret)
	tok, _ = storedCreds(t, own.ServiceAccountID)
	assert.NotContains(t, tok, "encrypted:encrypted:")
}

// The repository keeps an "encrypted:" value it is given as it is (a row loaded
// whose decryption failed and saved again must not be sealed twice): the
// boundary refusal above is the only place a caller's value is judged.
func TestRepositoryPersist_KeepsAStoredCiphertextAsItIs(t *testing.T) {
	t.Parallel()
	repo := serviceaccount.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	ev, err := runOp(uow, operations.CreateServiceAccount(repo), operations.CreateCommand{
		Code: "sapersist-keep", Name: "Keep", WebhookCredentials: bearer("keep-token", "keep-secret"),
	})
	require.NoError(t, err)
	tok, sec := storedCreds(t, ev.ServiceAccountID)
	require.Contains(t, tok, "encrypted:")

	sa, err := repo.FindByID(context.Background(), ev.ServiceAccountID)
	require.NoError(t, err)
	sa.WebhookCredentials.Token, sa.WebhookCredentials.SigningSecret = &tok, &sec // as loaded when it cannot be decrypted
	dbtx, err := testpg.Pool(t).Begin(context.Background())
	require.NoError(t, err)
	require.NoError(t, repo.Persist(context.Background(), sa, usecasepgx.WrapTxForBootstrap(dbtx)))
	require.NoError(t, dbtx.Commit(context.Background()))
	tok2, sec2 := storedCreds(t, ev.ServiceAccountID)
	assert.Equal(t, tok, tok2)
	assert.Equal(t, sec, sec2)
}
