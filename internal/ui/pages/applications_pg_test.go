//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appdocs"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The applications page end to end against Postgres: create, edit,
// deactivate/activate, provision the service account and a login client
// (credentials shown once), and the delete refused while the service
// account references the application.
func TestApplicationsPageAgainstPostgres(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("FLOWCATALYST_APP_KEY", key) // client secrets are hashed with it
	pool := testpg.Pool(t)
	apps := application.NewRepository(pool)
	oauth := platformauth.NewRepository(pool).OAuthClients
	page := &Applications{
		Apps: apps,
		Ops: ApplicationOps{
			Repo: apps, ServiceAccounts: serviceaccount.NewRepository(pool), Principals: principal.NewRepository(pool),
			OAuthClients: oauth, UoW: testpg.NewUoW(t),
		},
		Logins: oauth, Docs: appdocs.NewRepository(pool), Shell: shellForTest(),
	}
	do := func(q req) string {
		t.Helper()
		rec := serveMounted(t, anchorAdmin(), q, page)
		if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
			t.Fatalf("%s %s: status %d: %s", q.method, q.target, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	body := do(req{method: http.MethodPost, target: "/ui/applications/new", datastar: true, form: url.Values{
		"type": {"APPLICATION"}, "code": {"ui-pg-app"}, "name": {"UI PG app"}, "description": {"From the UI"}, "website": {"https://ui.example.com"},
	}})
	require.Contains(t, body, "Application created", body)
	a, err := apps.FindByCode(context.Background(), "ui-pg-app")
	require.NoError(t, err)
	require.NotNil(t, a)
	assert.Equal(t, "https://ui.example.com", deref(a.Website))
	base := "/ui/applications/" + string(a.ID)

	do(req{method: http.MethodPost, target: base + "/edit", datastar: true, form: url.Values{"name": {"UI PG app 2"}, "description": {""}}})
	do(req{method: http.MethodPost, target: base + "/deactivate", datastar: true, form: url.Values{}})
	got, _ := apps.FindByID(context.Background(), string(a.ID))
	assert.Equal(t, "UI PG app 2", got.Name)
	assert.False(t, got.Active)
	do(req{method: http.MethodPost, target: base + "/activate", datastar: true, form: url.Values{}})

	sa := do(req{method: http.MethodPost, target: base + "/provision-service-account", datastar: true, form: url.Values{"drawer": {string(a.ID)}}})
	assert.Contains(t, sa, "Service Account Provisioned")
	assert.Contains(t, sa, "will not be shown again")
	got, _ = apps.FindByID(context.Background(), string(a.ID))
	require.NotNil(t, got.ServiceAccountID, "the service account is attached")

	login := do(req{method: http.MethodPost, target: base + "/provision-login-client", datastar: true, form: url.Values{"clientType": {"CONFIDENTIAL"}, "redirectUris": {"https://ui.example.com/cb"}}})
	assert.Contains(t, login, "Login Client Provisioned")
	assert.Contains(t, login, "Client Secret", "a CONFIDENTIAL client's secret is shown once")
	has, err := oauth.HasLoginClientForApplication(context.Background(), string(a.ID))
	require.NoError(t, err)
	assert.True(t, has)
	assert.Contains(t, do(req{target: base, datastar: true}), "Login client settings")

	do(req{method: http.MethodPost, target: base + "/deactivate", datastar: true, form: url.Values{}})
	refused := do(req{method: http.MethodPost, target: base + "/delete", datastar: true, form: url.Values{}})
	assert.Contains(t, refused, "fc-flash-error")
	assert.Contains(t, refused, "still reference it")
}
