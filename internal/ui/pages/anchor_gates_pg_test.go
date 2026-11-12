//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// gateCaller is an anchor-tier console user holding exactly perms, with
// all-applications access.
func gateCaller(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{
		PrincipalID: "prn_uigate0001", Email: "gate@flowcatalyst.local", Scope: auth.ScopeAnchor,
		AllApplications: true, Permissions: perms, PrincipalType: auth.PrincipalTypeUser,
	}
}

// The console twins of the API routes: saving a client's applications (enable
// and disable) and provisioning a login client were anchor-only. They apply
// the API's gates.
func TestConsoleAnchorGates(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("FLOWCATALYST_APP_KEY", key)
	pool := testpg.Pool(t)
	clients := client.NewRepository(pool)
	apps := application.NewRepository(pool)
	configs := application.NewClientConfigRepo(pool)
	oauth := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)

	appEv, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(apps),
		appops.CreateCommand{Code: "ui-gate-app", Name: "UI gate app"}, testpg.TestEC())
	require.NoError(t, err)
	cliEv, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "UI gate client", Identifier: "ui-gate-client"}, testpg.TestEC())
	require.NoError(t, err)

	cliPage := &ClientsPage{
		Clients: clients, Ops: ClientOps{Repo: clients, Apps: apps, Configs: configs, UoW: uow},
		Apps: apps, Configs: configs, Shell: shellForTest(),
	}
	appPage := &Applications{
		Apps: apps,
		Ops: ApplicationOps{
			Repo: apps, ServiceAccounts: serviceaccount.NewRepository(pool), Principals: principal.NewRepository(pool),
			OAuthClients: oauth, Roles: role.NewRepository(pool), UoW: uow,
		},
		Logins: oauth, Shell: shellForTest(),
	}

	t.Run("save applications", func(t *testing.T) {
		q := req{
			method: http.MethodPost, target: "/ui/clients/" + cliEv.ClientID + "/applications",
			form: url.Values{"applicationIds": {appEv.ApplicationID}},
		}
		enabled := func() bool {
			c, err := configs.FindByApplicationAndClient(context.Background(), appEv.ApplicationID, cliEv.ClientID)
			require.NoError(t, err)
			return c != nil && c.Enabled
		}
		for _, perms := range [][]string{nil, {"platform:admin:application:enable-client"}, {"platform:admin:application:disable-client"}} {
			rec := serveMounted(t, gateCaller(perms...), q, cliPage)
			assert.Equal(t, http.StatusForbidden, rec.Code, "perms %v: %s", perms, rec.Body.String())
			assert.False(t, enabled(), "state unchanged with %v", perms)
		}
		scoped := gateCaller("platform:admin:application:enable-client", "platform:admin:application:disable-client")
		scoped.AllApplications, scoped.Applications = false, []string{appEv.ApplicationID}
		rec := serveMounted(t, scoped, q, cliPage)
		assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		assert.False(t, enabled())

		rec = serveMounted(t, gateCaller("platform:admin:application:enable-client", "platform:admin:application:disable-client"), q, cliPage)
		assert.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
		assert.True(t, enabled())
	})

	t.Run("provision login client", func(t *testing.T) {
		q := req{
			method: http.MethodPost, target: "/ui/applications/" + appEv.ApplicationID + "/provision-login-client",
			form: url.Values{"clientType": {"PUBLIC"}, "redirectUris": {"https://ui-gate.test/cb"}},
		}
		has := func() bool {
			ok, err := oauth.HasLoginClientForApplication(context.Background(), appEv.ApplicationID)
			require.NoError(t, err)
			return ok
		}
		rec := serveMounted(t, gateCaller("platform:admin:application:view"), q, appPage)
		assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "permission required: platform:admin:application:update")
		assert.False(t, has(), "nothing created")

		rec = serveMounted(t, gateCaller("platform:admin:application:update", "platform:admin:application:view"), q, appPage)
		assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.True(t, has())
	})
}
