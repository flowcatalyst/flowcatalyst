//go:build integration

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

func TestMain(m *testing.M) {
	// TestSecretCommandRunsWithoutShell re-executes this binary as a "secret
	// program". That child needs no database; booting embedded Postgres for it
	// outlasts the secret command's 30s timeout and the test gets killed.
	if os.Getenv("FC_TEST_HELPER_SECRET") == "1" {
		os.Exit(m.Run())
	}
	// TestOutboxPausedLeader... re-execute this binary as an `fcdev outbox` it can stop and kill;
	// the poller uses the parent's database.
	if os.Getenv("FC_TEST_HELPER_OUTBOX") == "1" {
		os.Exit(m.Run())
	}
	testpg.RunMain(m)
}

// TestInitServiceAccount_IsAppConfined locks in that `fcdev init` provisions
// the same shape as the normal ProvisionServiceAccount path: exactly the
// platform:application-service role, access to exactly the one application,
// and no all-applications flag.
func TestInitServiceAccount_IsAppConfined(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("FLOWCATALYST_APP_KEY", key)
	ctx := context.Background()
	pool := testpg.Pool(t)

	require.NoError(t, seed.NewSeeder(pool).Run(ctx))
	require.NoError(t, createAdmin(ctx, pool, "init-admin@example.test", "correct-horse-battery-staple-9"))

	appRepo := application.NewRepository(pool)
	app := application.New("initsa", "Init SA App")
	require.NoError(t, infraPersist(ctx, pool, func(tx *usecasepgx.DbTx) error {
		return appRepo.Persist(ctx, app, tx)
	}))

	res, err := provisionAppCredentials(ctx, pool, app.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, res.OAuthClientID)
	assert.NotEmpty(t, res.OAuthClientSecret)

	p, err := principal.NewRepository(pool).FindByID(ctx, ids.PrincipalID(res.ServicePrincipalID))
	require.NoError(t, err)
	require.NotNil(t, p)

	assert.False(t, p.AllApplications, "init SA must not be all-applications")
	assert.Equal(t, []string{string(app.ID)}, ids.Strings(p.AccessibleApplicationIDs), "access to exactly the one application")
	require.Len(t, p.Roles, 1, "exactly one role")
	assert.Equal(t, "platform:application-service", p.Roles[0].Role.String())
	require.NotNil(t, p.Roles[0].AssignmentSource)
	assert.Equal(t, "PROVISIONED", *p.Roles[0].AssignmentSource)

	got, err := appRepo.FindByID(ctx, string(app.ID))
	require.NoError(t, err)
	require.NotNil(t, got.ServiceAccountID)
	assert.Equal(t, res.ServicePrincipalID, *got.ServiceAccountID, "application is linked to the SA principal")
}

// readEnvFileForTest reads KEY=VALUE lines (comments skipped, quotes removed).
func readEnvFileForTest(t *testing.T, path string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	env := map[string]string{}
	for line := range strings.SplitSeq(string(b), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			env[k] = unquoteEnvValue(v)
		}
	}
	return env
}

func runInitForTest(t *testing.T, dbURL, root string, extra ...string) (string, error) {
	t.Helper()
	cmd := newInitCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{
		"--yes", "--database-url", dbURL, "--root", root,
		"--embedded-db-path", t.TempDir(),
		"--admin-email", "init-env-admin@example.test", "--admin-password", "correct-horse-battery-staple-9",
		"--code", "initenv", "--name", "Init Env App",
		"--api-base-url", "http://localhost:1",
	}, extra...))
	err := cmd.Execute()
	return out.String(), err
}

// `fcdev init` writes the account's webhook credentials, the ones the
// platform stores, beside the client credentials; a second run on the same
// application writes nothing and says how to get a signing secret; with
// --regenerate-signing-secret it replaces only the signing secret.
func TestInitWritesWebhookCredentials(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("FLOWCATALYST_APP_KEY", key)
	ctx := context.Background()
	pool := testpg.Pool(t)
	dbURL := pool.Config().ConnString()

	root := t.TempDir()
	envPath := filepath.Join(root, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("APP_NAME=shop\nFLOWCATALYST_SIGNING_SECRET=set-by-hand\n"), 0o600))

	out, err := runInitForTest(t, dbURL, root)
	require.NoError(t, err, out)

	env := readEnvFileForTest(t, envPath)
	sa, err := serviceaccount.NewRepository(pool).FindByCode(ctx, "app:initenv")
	require.NoError(t, err)
	require.NotNil(t, sa)
	require.NotNil(t, sa.WebhookCredentials.SigningSecret)
	require.NotNil(t, sa.WebhookCredentials.Token)
	assert.Equal(t, *sa.WebhookCredentials.SigningSecret, env["FLOWCATALYST_SIGNING_SECRET"], "the secret the platform signs with")
	assert.Equal(t, *sa.WebhookCredentials.Token, env["FLOWCATALYST_WEBHOOK_AUTH_TOKEN"], "the token the platform sends")
	assert.NotEmpty(t, env["FLOWCATALYST_CLIENT_ID"])
	assert.NotEmpty(t, env["FLOWCATALYST_CLIENT_SECRET"])
	assert.Equal(t, "initenv", env["FLOWCATALYST_APP_CODE"])
	assert.Equal(t, "shop", env["APP_NAME"])
	first := readFileForTest(t, envPath)
	assert.Contains(t, first, "# replaced by fcdev init: FLOWCATALYST_SIGNING_SECRET=set-by-hand\n",
		"the hand-set secret is kept, not lost")
	assert.NotContains(t, out, env["FLOWCATALYST_SIGNING_SECRET"], "secrets are written to .env, not printed")

	// Again, same code: refused, nothing written, the way to a secret named.
	out, err = runInitForTest(t, dbURL, root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FLOWCATALYST_SIGNING_SECRET was not written")
	assert.Contains(t, err.Error(), "fcdev init --code initenv --regenerate-signing-secret")
	assert.Contains(t, err.Error(), "refuses every delivery")
	assert.Equal(t, first, readFileForTest(t, envPath), "nothing written: "+out)

	// Explicit regeneration: a new signing secret, stored and written; the
	// client credentials and the token are left as they are.
	out, err = runInitForTest(t, dbURL, root, "--regenerate-signing-secret")
	require.NoError(t, err, out)
	after := readEnvFileForTest(t, envPath)
	sa, err = serviceaccount.NewRepository(pool).FindByCode(ctx, "app:initenv")
	require.NoError(t, err)
	assert.NotEqual(t, env["FLOWCATALYST_SIGNING_SECRET"], after["FLOWCATALYST_SIGNING_SECRET"])
	assert.Equal(t, *sa.WebhookCredentials.SigningSecret, after["FLOWCATALYST_SIGNING_SECRET"])
	assert.Equal(t, env["FLOWCATALYST_WEBHOOK_AUTH_TOKEN"], after["FLOWCATALYST_WEBHOOK_AUTH_TOKEN"])
	assert.Equal(t, env["FLOWCATALYST_CLIENT_ID"], after["FLOWCATALYST_CLIENT_ID"])
	assert.Equal(t, env["FLOWCATALYST_CLIENT_SECRET"], after["FLOWCATALYST_CLIENT_SECRET"])
	assert.Contains(t, out, "Restart any running copy")
	assert.Equal(t, 1, strings.Count(readFileForTest(t, envPath), "# replaced by fcdev init: FLOWCATALYST_SIGNING_SECRET="))
}
