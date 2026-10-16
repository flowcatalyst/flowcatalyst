//go:build integration

package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

func TestMain(m *testing.M) {
	// TestSecretCommandRunsWithoutShell re-executes this binary as a "secret
	// program". That child needs no database; booting embedded Postgres for it
	// outlasts the secret command's 30s timeout and the test gets killed.
	if os.Getenv("FC_TEST_HELPER_SECRET") == "1" {
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
