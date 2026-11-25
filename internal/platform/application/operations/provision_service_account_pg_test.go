//go:build integration

package operations_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/webhook"
)

// Provisioning returns the account's webhook credentials once, as the service
// account's own create does: minted at provisioning, stored encrypted on the
// account, and the stored signing secret is the one the platform signs the
// application's deliveries with, so a signature it makes verifies with the
// secret the operator was shown.
func TestProvisionServiceAccount_ReturnsWebhookCredentialsOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	appRepo := application.NewRepository(pool)
	saRepo := serviceaccount.NewRepository(pool)
	uow := testpg.NewUoW(t)
	app := mustCreateApp(t, appRepo, uow, "appprov-webhook", "Prov Webhook App")

	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.ProvisionServiceAccount(appRepo, saRepo, principal.NewRepository(pool), platformauth.NewRepository(pool).OAuthClients, credentialpublish.Config{}),
		operations.ProvisionServiceAccountCommand{ApplicationID: app.ApplicationID}, testpg.TestEC())
	require.NoError(t, err)
	assert.Regexp(t, `^fc_[0-9a-z]{32}$`, res.AuthToken, "bearer token format (wire contract)")
	assert.Regexp(t, `^[A-Za-z0-9_-]{43}$`, res.SigningSecret, "32 random bytes, URL-safe base64 without padding")

	// Stored as the create flow stores them: BEARER, both credentials, and
	// encrypted at rest (the plaintext is not in the row).
	sa, err := saRepo.FindByID(ctx, res.ServiceAccountID)
	require.NoError(t, err)
	require.NotNil(t, sa)
	assert.Equal(t, serviceaccount.AuthBearer, sa.WebhookCredentials.AuthType)
	require.NotNil(t, sa.WebhookCredentials.Token)
	require.NotNil(t, sa.WebhookCredentials.SigningSecret)
	assert.Equal(t, res.AuthToken, *sa.WebhookCredentials.Token)
	assert.Equal(t, res.SigningSecret, *sa.WebhookCredentials.SigningSecret)
	var tokenRef, secretRef string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT wh_auth_token_ref, wh_signing_secret_ref FROM iam_service_accounts WHERE id = $1`, sa.ID).Scan(&tokenRef, &secretRef))
	assert.True(t, strings.HasPrefix(tokenRef, "encrypted:"), "auth token stored encrypted")
	assert.True(t, strings.HasPrefix(secretRef, "encrypted:"), "signing secret stored encrypted")
	assert.NotContains(t, tokenRef, res.AuthToken)
	assert.NotContains(t, secretRef, res.SigningSecret)

	// The platform's own lookup for the application's deliveries hands out
	// the stored credentials; a signature made with them verifies with the
	// returned secret, the way the application's SDK verifies it.
	creds, err := serviceaccount.NewCachedOutboundCredsResolver(saRepo, time.Minute)(ctx, app.ApplicationID)
	require.NoError(t, err)
	assert.Equal(t, res.AuthToken, creds.BearerToken)
	body := []byte(`{"jobId":"sjb_provision_test"}`)
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	signature := webhook.NewValidator(creds.SigningSecret).ComputeSignature(ts, body)
	assert.NoError(t, webhook.NewValidator(res.SigningSecret).Validate(signature, ts, body),
		"a delivery signed with the stored secret verifies with the returned one")
	assert.Error(t, webhook.NewValidator(res.SigningSecret).Validate(signature, ts, []byte(`{"jobId":"other"}`)))
}

// Provisioning an application that already has a service account is refused
// (409 ALREADY_PROVISIONED) and changes nothing: the running application's
// signing secret and token stay as they are, and no new secret is returned.
func TestProvisionServiceAccount_AlreadyProvisioned_KeepsCredentials(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	appRepo := application.NewRepository(pool)
	saRepo := serviceaccount.NewRepository(pool)
	uow := testpg.NewUoW(t)
	app := mustCreateApp(t, appRepo, uow, "appprov-twice", "Prov Twice App")
	op := operations.ProvisionServiceAccount(appRepo, saRepo, principal.NewRepository(pool), platformauth.NewRepository(pool).OAuthClients, credentialpublish.Config{})
	cmd := operations.ProvisionServiceAccountCommand{ApplicationID: app.ApplicationID}

	first, err := usecaseop.RunTx(testpg.AnchorCtx(), uow, op, cmd, testpg.TestEC())
	require.NoError(t, err)

	again, err := usecaseop.RunTx(testpg.AnchorCtx(), uow, op, cmd, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "ALREADY_PROVISIONED")
	assert.Empty(t, again.AuthToken)
	assert.Empty(t, again.SigningSecret)

	sa, err := saRepo.FindByID(ctx, first.ServiceAccountID)
	require.NoError(t, err)
	require.NotNil(t, sa)
	assert.Equal(t, first.SigningSecret, *sa.WebhookCredentials.SigningSecret, "the signing secret is not replaced")
	assert.Equal(t, first.AuthToken, *sa.WebhookCredentials.Token, "the auth token is not replaced")
	var accounts int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM iam_service_accounts WHERE application_id = $1`, app.ApplicationID).Scan(&accounts))
	assert.Equal(t, 1, accounts, "no second account")
}
