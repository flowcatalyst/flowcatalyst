//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

const (
	permEnable  = "platform:admin:application:enable-client"
	permDisable = "platform:admin:application:disable-client"
	permUpdate  = "platform:admin:application:update"
	permSACreat = "platform:iam:service-account:create"
)

// anchorCaller is an anchor-tier caller holding exactly perms. apps nil means
// all-applications access; otherwise it is confined to those applications.
func anchorCaller(apps []string, perms ...string) context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_anchorgate001", Scope: auth.ScopeAnchor,
		AllApplications: apps == nil, Applications: apps, Permissions: perms,
	})
}

func wantRefused(t *testing.T, err error, code string) {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "want %s, got %v", code, err)
	assert.Equal(t, usecase.KindAuthorization, ue.Kind)
	assert.Equal(t, code, ue.Code)
}

// Enabling an application for a client, attaching a service account and
// provisioning a login client were anchor-only: any anchor principal, an
// application's own service account included, passed. They now need their
// permission, and an application-scoped caller is confined to its own
// application.
func TestApplicationAnchorGates(t *testing.T) {
	pool := testpg.Pool(t)
	repo := application.NewRepository(pool)
	configs := application.NewClientConfigRepo(pool)
	clients := client.NewRepository(pool)
	roles := role.NewRepository(pool)
	uow := testpg.NewUoW(t)
	principals := principal.NewRepository(pool)
	sas := serviceaccount.NewRepository(pool)
	oauth := platformauth.NewRepository(pool).OAuthClients
	s := &State{
		Repo: repo, ClientConfigRepo: configs, ClientRepo: clients, Roles: roles,
		Principals: principals, ServiceAccounts: sas, OAuthClients: oauth, UoW: uow,
	}
	mkApp := func(code string) string {
		t.Helper()
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateApplication(repo),
			operations.CreateCommand{Code: code, Name: code}, testpg.TestEC())
		require.NoError(t, err)
		return ev.ApplicationID
	}
	mkClient := func(identifier string) string {
		t.Helper()
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients),
			clientops.CreateCommand{Name: identifier, Identifier: identifier}, testpg.TestEC())
		require.NoError(t, err)
		return ev.ClientID
	}
	enabled := func(appID, clientID string) bool {
		t.Helper()
		c, err := configs.FindByApplicationAndClient(context.Background(), appID, clientID)
		require.NoError(t, err)
		return c != nil && c.Enabled
	}

	appA := mkApp("ag-app-a")
	appB := mkApp("ag-app-b")
	cli := mkClient("ag-client-1")

	t.Run("enable", func(t *testing.T) {
		in := &clientToggleInput{ID: ids.ApplicationID(appA), ClientID: cli}
		_, err := s.enableForClient(anchorCaller(nil, permDisable, "platform:admin:application:view"), in)
		wantRefused(t, err, "PERMISSION_REQUIRED")
		assert.False(t, enabled(appA, cli))

		// Holds the permission but is confined to another application.
		_, err = s.enableForClient(anchorCaller([]string{appB}, permEnable), in)
		wantRefused(t, err, "APPLICATION_ACCESS_REQUIRED")
		assert.False(t, enabled(appA, cli))

		_, err = s.enableForClient(anchorCaller([]string{appA}, permEnable), in)
		require.NoError(t, err)
		assert.True(t, enabled(appA, cli))
	})

	t.Run("disable", func(t *testing.T) {
		in := &clientToggleInput{ID: ids.ApplicationID(appA), ClientID: cli}
		require.True(t, enabled(appA, cli))
		_, err := s.disableForClient(anchorCaller(nil, permEnable), in)
		wantRefused(t, err, "PERMISSION_REQUIRED")
		assert.True(t, enabled(appA, cli), "still enabled")

		_, err = s.disableForClient(anchorCaller([]string{appB}, permDisable), in)
		wantRefused(t, err, "APPLICATION_ACCESS_REQUIRED")
		assert.True(t, enabled(appA, cli))

		_, err = s.disableForClient(anchorCaller(nil, permDisable), in)
		require.NoError(t, err)
		assert.False(t, enabled(appA, cli))
	})

	t.Run("provision login client", func(t *testing.T) {
		in := &provisionLoginClientInput{
			ID:   ids.ApplicationID(appA),
			Body: ProvisionLoginClientRequest{ClientType: "PUBLIC", RedirectURIs: []string{"https://ag.test/cb"}},
		}
		hasLogin := func() bool {
			ok, err := oauth.HasLoginClientForApplication(context.Background(), appA)
			require.NoError(t, err)
			return ok
		}
		_, err := s.provisionLoginClient(anchorCaller(nil, permEnable, permDisable), in)
		wantRefused(t, err, "PERMISSION_REQUIRED")
		assert.False(t, hasLogin(), "nothing created")

		_, err = s.provisionLoginClient(anchorCaller([]string{appB}, permUpdate), in)
		wantRefused(t, err, "APPLICATION_ACCESS_REQUIRED")
		assert.False(t, hasLogin())

		_, err = s.provisionLoginClient(anchorCaller(nil, permUpdate), in)
		require.NoError(t, err)
		assert.True(t, hasLogin())
	})

	t.Run("attach service account", func(t *testing.T) {
		// A service account to attach: provision one on another application,
		// as a super-admin, and take its id.
		ensureApplicationServiceRole(t, roles, uow)
		donor := mkApp("ag-donor")
		_, err := s.provisionServiceAccount(anchorCaller(nil, "platform:*:*:*"), &apicommon.IDInput{ID: donor})
		require.NoError(t, err)
		d, err := repo.FindByID(context.Background(), donor)
		require.NoError(t, err)
		require.NotNil(t, d.ServiceAccountID)
		pr, err := principals.FindByID(context.Background(), ids.PrincipalID(*d.ServiceAccountID))
		require.NoError(t, err)
		require.NotNil(t, pr.ServiceAccountID)
		target := mkApp("ag-attach-target")
		in := &attachSAInput{ID: ids.ApplicationID(target), Body: AttachServiceAccountRequest{ServiceAccountID: *pr.ServiceAccountID}}
		attached := func() bool {
			a, err := repo.FindByID(context.Background(), target)
			require.NoError(t, err)
			return a.ServiceAccountID != nil
		}

		_, err = s.attachServiceAccount(anchorCaller(nil, permEnable, permDisable), in)
		wantRefused(t, err, "PERMISSION_REQUIRED")
		assert.False(t, attached())

		_, err = s.attachServiceAccount(anchorCaller([]string{appB}, permSACreat, permUpdate), in)
		wantRefused(t, err, "APPLICATION_ACCESS_REQUIRED")
		assert.False(t, attached())

		_, err = s.attachServiceAccount(anchorCaller(nil, permSACreat, permUpdate), in)
		require.NoError(t, err)
		assert.True(t, attached())
	})
}
