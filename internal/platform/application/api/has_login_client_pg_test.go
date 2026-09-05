//go:build integration

package api

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestMain seeds FLOWCATALYST_APP_KEY before the embedded-PG boot, mirroring
// application/operations' ops_pg_test.go: a CONFIDENTIAL OAuth client (not
// exercised by this file today, but shared package-wide setup) encrypts its
// secret via encryption.FromEnv, which reads the env at call time.
func TestMain(m *testing.M) {
	key, err := encryption.GenerateKey()
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("FLOWCATALYST_APP_KEY", key)
	testpg.RunMain(m)
}

// TestGetApplication_HasLoginClient pins HasLoginClient on the detail
// endpoint: false before a login client is provisioned, true after —
// distinguishing a login (authorization_code) OAuth client from any other
// client that might be linked to the application (e.g. a service-account
// client, which uses client_credentials).
func TestGetApplication_HasLoginClient(t *testing.T) {
	pool := testpg.Pool(t)
	repo := application.NewRepository(pool)
	oauthClients := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)

	ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateApplication(repo),
		operations.CreateCommand{Code: "hlc-app", Name: "HasLoginClient App"}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{Repo: repo, OAuthClients: oauthClients, UoW: uow}
	authCtx := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_hlc_admin", Scope: auth.ScopeAnchor,
	})

	// Before provisioning: no login client yet.
	out, err := s.getByID(authCtx, &apicommon.IDInput{ID: ev.ApplicationID})
	require.NoError(t, err)
	assert.False(t, out.Body.HasLoginClient)

	// Provision a login client (authorization_code grant, linked to the app).
	_, err = s.provisionLoginClient(authCtx, &provisionLoginClientInput{
		ID: ev.ApplicationID,
		Body: ProvisionLoginClientRequest{
			RedirectURIs: []string{"https://hlc-app.example.com/callback"},
		},
	})
	require.NoError(t, err)

	// After provisioning: the detail endpoint now reports it.
	out, err = s.getByID(authCtx, &apicommon.IDInput{ID: ev.ApplicationID})
	require.NoError(t, err)
	assert.True(t, out.Body.HasLoginClient)

	// List responses deliberately leave the flag false (documented on
	// ApplicationResponse.HasLoginClient) — cheap to keep unset there.
	list, err := s.list(authCtx, &listInput{})
	require.NoError(t, err)
	for _, a := range list.Body.Applications {
		if a.ID == ev.ApplicationID {
			assert.False(t, a.HasLoginClient, "list responses must not populate HasLoginClient")
		}
	}
}
