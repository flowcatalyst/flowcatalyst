//go:build integration

package pages

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/branding"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/publicapi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// The theme, client branding and names pages end to end against Postgres,
// through the platformconfig.Service the API runs: what the pages save is what
// the public login-theme endpoint serves and branding.PlatformName reads, the
// override layers over the platform theme, and forged posts change nothing.
func TestThemeAndNamesPagesAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	// The pages save a global login theme and a platform name; the database is shared
	// with the sign-in page tests, which expect the default branding.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM app_platform_configs WHERE application_code = 'platform' AND ((section = 'login' AND property = 'theme') OR (section = 'branding' AND property = 'platform-name'))`)
	})
	repo := platformconfig.NewRepository(pool)
	clients := client.NewRepository(pool)
	uow := testpg.NewUoW(t)
	_, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients), clientops.CreateCommand{Name: "UI Theme Co", Identifier: "ui-themeco"}, testpg.TestEC())
	require.NoError(t, err)
	clientID := *clientIDOf(t, clients, "ui-themeco")

	svc := &platformconfig.Service{Repo: repo, UoW: uow, Apps: application.NewRepository(pool), Clients: clients, Audit: audit.NewRepository(pool)}
	ops := ConfigOps{Repo: repo, UoW: uow}
	sh := shellForTest()
	pagesUnderTest := []mounter{
		&ThemeSettings{Config: svc, Ops: ops, Shell: sh},
		&ClientTheme{Clients: clients, Config: svc, Ops: ops, Shell: sh},
		&NamesSettings{Config: svc, Ops: ops, Shell: sh},
	}
	as := func(ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
		t.Helper()
		return serveMounted(t, ac, q, pagesUnderTest...)
	}
	admin := cfgAdmin()
	admin.Permissions = []string{"platform:*:*:*"}
	post := func(path string, form url.Values) req {
		return req{method: http.MethodPost, target: path, datastar: true, multipart: true, form: form}
	}
	find := func(section, property string, cid *string) *platformconfig.Config {
		t.Helper()
		scope := platformconfig.ScopeGlobal
		if cid != nil {
			scope = platformconfig.ScopeClient
		}
		c, err := repo.FindByCoordinate(ctx, "platform", section, property, scope, cid)
		require.NoError(t, err)
		return c
	}
	// served is what the sign-in pages receive (GET /api/public/login-theme).
	served := func(query string) map[string]any {
		t.Helper()
		r := chi.NewRouter()
		publicapi.New(repo).RegisterRoutes(r)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/public/login-theme"+query, nil))
		require.Equal(t, http.StatusOK, rec.Code)
		var out map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		return out
	}

	// No theme stored yet: the page shows the built-in look.
	assert.Contains(t, as(admin, req{target: "/ui/platform/settings/theme"}).Body.String(), `name="brandName" value="FlowCatalyst"`)

	// The platform-wide theme.
	rec := as(admin, post("/ui/platform/settings/theme", themeForm("brandName", "Pg Portal", "footerText", "Pg footer")))
	require.Contains(t, rec.Body.String(), "Theme saved successfully", rec.Body.String())
	global := find("login", "theme", nil)
	require.NotNil(t, global)
	assert.Equal(t, platformconfig.ValuePlain, global.ValueType)
	assert.Equal(t, "Pg Portal", served("")["brandName"], "the login page serves what the page saved")
	assert.Contains(t, as(admin, req{target: "/ui/platform/settings/theme"}).Body.String(), `name="brandName" value="Pg Portal"`)

	// A client's override starts from the platform theme and layers over it.
	themePath := "/ui/clients/" + clientID + "/theme"
	assert.Contains(t, as(admin, req{target: themePath}).Body.String(), `name="footerText" value="Pg footer"`)
	body := as(admin, post(themePath, themeForm("enabled", "on", "brandName", "Pg Client", "footerText", "Pg footer"))).Body.String()
	require.Contains(t, body, "Login branding saved", body)
	override := find("login", "theme", &clientID)
	require.NotNil(t, override, "stored under the client's id")
	assert.Equal(t, "Pg Client", served("?clientId=" + clientID)["brandName"])
	assert.Equal(t, "Pg Portal", served("")["brandName"], "the platform theme is untouched")
	assert.Contains(t, as(admin, req{target: themePath}).Body.String(), `{thmEnabled: true}`)

	// Switched off, the override is deleted, not copied.
	body = as(admin, post(themePath, themeForm())).Body.String()
	require.Contains(t, body, "reset to the platform theme", body)
	assert.Nil(t, find("login", "theme", &clientID))
	assert.Equal(t, "Pg Portal", served("?clientId=" + clientID)["brandName"])
	assert.Contains(t, as(admin, req{target: themePath}).Body.String(), `{thmEnabled: false}`)

	// An unknown client is refused, nothing is stored for it.
	assert.Equal(t, http.StatusNotFound, as(admin, req{method: http.MethodPost, target: "/ui/clients/clt_nobody0/theme", form: themeForm("enabled", "on")}).Code)

	// An invalid theme is refused before the use case.
	body = as(admin, post("/ui/platform/settings/theme", themeForm("accentColor", "red;x"))).Body.String()
	assert.Contains(t, body, "Accent color must be a hex color")
	assert.Equal(t, global.Value, find("login", "theme", nil).Value)

	// The platform name.
	body = as(admin, post("/ui/platform/settings/names", url.Values{"platformName": {"  Pg Platform "}})).Body.String()
	require.Contains(t, body, "Platform name updated.", body)
	assert.Equal(t, "Pg Platform", branding.PlatformName(ctx, repo), "emails, 2FA and passkeys read it")
	assert.Contains(t, as(admin, req{target: "/ui/platform/settings/names"}).Body.String(), `value="Pg Platform"`)
	body = as(admin, post("/ui/platform/settings/names", url.Values{"platformName": {"  "}})).Body.String()
	assert.Contains(t, body, "Platform name cannot be empty.")
	assert.Equal(t, "Pg Platform", branding.PlatformName(ctx, repo))

	// The permission matrix, against the real rules: forged posts change nothing.
	viewer := cfgUser(permCfgView)
	nobody := cfgUser()
	// A client user without write access to the platform's config: the page's
	// coarse gate admits it (anchor tier is what the permission decides), the
	// use case's own check refuses.
	clientUser := &auth.AuthContext{PrincipalID: "prn_cu", Scope: auth.ScopeClient, Roles: []string{"r"}, Clients: []string{clientID}, Permissions: []string{permCfgView, permCfgUpdate}, PrincipalType: auth.PrincipalTypeUser}
	before := map[string]time.Time{"theme": find("login", "theme", nil).UpdatedAt, "name": find("branding", "platform-name", nil).UpdatedAt}
	for name, ac := range map[string]*auth.AuthContext{"viewer": viewer, "nobody": nobody, "client user": clientUser} {
		for _, q := range []req{
			post("/ui/platform/settings/theme", themeForm("brandName", "Hacked")),
			post(themePath, themeForm("enabled", "on", "brandName", "Hacked")),
			post(themePath, themeForm()),
			post("/ui/platform/settings/names", url.Values{"platformName": {"Hacked"}}),
		} {
			b := as(ac, q).Body.String()
			assert.NotContains(t, b, "saved successfully", "%s %s", name, q.target)
			assert.NotContains(t, b, "Platform name updated.", "%s %s", name, q.target)
		}
	}
	assert.Nil(t, find("login", "theme", &clientID), "a forged post stored an override")
	assert.True(t, before["theme"].Equal(find("login", "theme", nil).UpdatedAt))
	assert.True(t, before["name"].Equal(find("branding", "platform-name", nil).UpdatedAt))
	assert.Equal(t, "Pg Platform", branding.PlatformName(ctx, repo))
	assert.Equal(t, "Pg Portal", served("")["brandName"])

	// A viewer sees the values, without the controls.
	vbody := as(viewer, req{target: "/ui/platform/settings/theme"}).Body.String()
	assert.Contains(t, vbody, `name="brandName" value="Pg Portal"`)
	assert.NotContains(t, vbody, "Save Changes")
	assert.Equal(t, http.StatusForbidden, as(nobody, req{target: "/ui/platform/settings/names"}).Code)
}
