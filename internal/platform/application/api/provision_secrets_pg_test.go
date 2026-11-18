//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// POST /api/applications/{id}/provision-service-account answers the account's
// webhook credentials beside its OAuth client, in the shape of the service
// account create response's `webhook`, and they are the stored ones. None of
// the three plaintext secrets reaches a domain event, an audit row or a log
// line.
//
// Not parallel: it replaces the default slog logger to read what was logged
// (parallel tests wait until the sequential ones are done).
func TestProvisionServiceAccount_ResponseCarriesWebhookSecretsAndNothingLeaks(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := application.NewRepository(pool)
	roles := role.NewRepository(pool)
	saRepo := serviceaccount.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)
	ensureApplicationServiceRole(t, roles, uow)
	s := &State{
		Repo: repo, Roles: roles, Principals: principal.NewRepository(pool),
		ServiceAccounts: saRepo, OAuthClients: oauthRepo, UoW: uow,
	}
	created, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateApplication(repo),
		operations.CreateCommand{Code: "provsecrets-app", Name: "Prov Secrets"}, testpg.TestEC())
	require.NoError(t, err)

	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	caller := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_provsecrets01", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"},
	})
	out, err := s.provisionServiceAccount(caller, &apicommon.IDInput{ID: created.ApplicationID})
	require.NoError(t, err)

	// The wire shape: serviceAccount.webhook.{authToken,signingSecret}.
	raw, err := json.Marshal(out.Body)
	require.NoError(t, err)
	var wire struct {
		ServiceAccount struct {
			OAuthClient struct {
				ClientSecret string `json:"clientSecret"`
			} `json:"oauthClient"`
			Webhook *struct {
				AuthToken     string `json:"authToken"`
				SigningSecret string `json:"signingSecret"`
			} `json:"webhook"`
		} `json:"serviceAccount"`
	}
	require.NoError(t, json.Unmarshal(raw, &wire), string(raw))
	require.NotNil(t, wire.ServiceAccount.Webhook, "the response carries the webhook credentials: %s", raw)
	token, secret := wire.ServiceAccount.Webhook.AuthToken, wire.ServiceAccount.Webhook.SigningSecret
	clientSecret := wire.ServiceAccount.OAuthClient.ClientSecret
	assert.Regexp(t, `^fc_[0-9a-z]{32}$`, token)
	require.NotEmpty(t, secret)
	require.NotEmpty(t, clientSecret)

	sa, err := saRepo.FindFirstByApplicationID(ctx, created.ApplicationID)
	require.NoError(t, err)
	require.NotNil(t, sa)
	assert.Equal(t, token, *sa.WebhookCredentials.Token, "the answered token is the stored one")
	assert.Equal(t, secret, *sa.WebhookCredentials.SigningSecret, "the answered signing secret is the stored one")

	// The provisioning wrote its events and audit rows (so the search below is
	// not of nothing): the account, the application and the OAuth client.
	entities := []string{sa.ID, created.ApplicationID, out.Body.ServiceAccount.OAuthClient.ID}
	var events, audits int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM msg_events WHERE split_part(subject, '.', 3) = ANY($1)`, entities).Scan(&events))
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM aud_logs WHERE entity_id = ANY($1)`, entities).Scan(&audits))
	assert.GreaterOrEqual(t, events, 3, "service account created, application provisioned, OAuth client created")
	assert.GreaterOrEqual(t, audits, 3)

	for name, plaintext := range map[string]string{"auth token": token, "signing secret": secret, "client secret": clientSecret} {
		var inEvents, inAudit int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM msg_events WHERE position($1 in data::text) > 0 OR position($1 in coalesce(context_data::text, '')) > 0`,
			plaintext).Scan(&inEvents))
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM aud_logs WHERE position($1 in coalesce(operation_json::text, '')) > 0`, plaintext).Scan(&inAudit))
		assert.Zero(t, inEvents, "the %s is in a domain event", name)
		assert.Zero(t, inAudit, "the %s is in an audit row", name)
		assert.NotContains(t, logs.String(), plaintext, "the %s is in a log line", name)
	}
}
