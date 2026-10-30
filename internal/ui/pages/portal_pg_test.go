//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// The portal pages end to end against Postgres: the handlers run the real
// use cases (PortalOps over the unit of work), so the rows, the OAuth client
// provisioned with an app, the domain events and the audit records are the
// API's. Permissions are the real auth checks on real client ids.
func TestPortalPagesAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	// A CONFIDENTIAL OAuth client's secret is hashed with the platform's key,
	// read at call time.
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("FLOWCATALYST_APP_KEY", key)
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	clients := client.NewRepository(pool)
	identities := portalidentity.NewRepository(pool)
	apps := portalidentity.NewAppRepository(pool)
	oauth := platformauth.NewRepository(pool).OAuthClients

	mkClient := func(name, identifier string) string {
		ev, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients), clientops.CreateCommand{Name: name, Identifier: identifier}, testpg.TestEC())
		require.NoError(t, err)
		return ev.ClientID
	}
	acme, globex := mkClient("Portal UI Acme", "portal-ui-acme"), mkClient("Portal UI Globex", "portal-ui-globex")

	sh := shellForTest()
	store := PortalReader{Identities: identities, PortalApps: apps}
	ops := PortalOps{Identities: identities, Apps: apps, Clients: clients, OAuthClients: oauth, UoW: uow}
	do := func(t *testing.T, caller func() *httptest.ResponseRecorder) string {
		rec := caller()
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		return rec.Body.String()
	}
	get := func(ac *auth.AuthContext, q req) func() *httptest.ResponseRecorder {
		return func() *httptest.ResponseRecorder {
			return serveMounted(t, ac, q, &PortalApps{Store: store, Ops: ops, Clients: clients, Shell: sh}, &PortalUsers{Store: store, Ops: ops, Clients: clients, Shell: sh})
		}
	}
	admin := anchorAdmin()

	// Create an app through the dialog: the app and its OAuth client are
	// written in one transaction and audited; the secret is shown once.
	body := do(t, get(admin, postForm("/ui/portal-apps/new",
		"client", acme, "name", "UI Customers", "code", "UI-Customers", "description", "For customers",
		"redirectUris", "https://ui-customers.test/cb", "clientType", "CONFIDENTIAL")))
	app, err := apps.FindByClientAndCode(ctx, acme, "ui-customers")
	require.NoError(t, err)
	require.NotNil(t, app)
	assert.Equal(t, "UI Customers", app.Name)
	assert.Equal(t, "For customers", deref(app.Description))
	assert.True(t, app.Active)
	linked, err := apps.LinkedOAuthClients(ctx, []string{app.ID})
	require.NoError(t, err)
	require.Len(t, linked[app.ID], 1)
	oc, err := oauth.FindByID(ctx, linked[app.ID][0].ID)
	require.NoError(t, err)
	require.NotNil(t, oc)
	assert.Equal(t, []string{"https://ui-customers.test/cb"}, oc.RedirectURIs)
	assert.Equal(t, []string{"authorization_code"}, oc.GrantTypes)
	assert.True(t, oc.PKCERequired)
	require.NotNil(t, oc.PortalAppID)
	assert.Equal(t, app.ID, *oc.PortalAppID)
	assert.Equal(t, 1, auditCount(t, app.ID), "create is audited like the API")

	secret := regexp.MustCompile(`id="portal-cred-secret" class="fc-mono-chip break-all">([^<]+)<`).FindStringSubmatch(body)
	require.Len(t, secret, 2, "the credentials dialog shows the secret")
	assert.Contains(t, body, `id="portal-cred-client-id" class="fc-mono-chip break-all">`+oc.ClientID+"<")
	// The secret is stored hashed: the plaintext is in no column of the row.
	var row string
	require.NoError(t, pool.QueryRow(ctx, `SELECT row_to_json(o)::text FROM oauth_clients o WHERE id = $1`, oc.ID).Scan(&row))
	assert.NotContains(t, row, secret[1], "the client secret is not stored in the clear")
	// And it is in no later answer.
	for _, target := range []string{"/ui/portal-apps?client=" + acme, "/ui/portal-users?client=" + acme} {
		assert.NotContains(t, do(t, get(admin, req{target: target})), secret[1], target)
	}

	// The same code again: the use case's conflict on the code field, and no
	// second OAuth client.
	dup := do(t, get(admin, postForm("/ui/portal-apps/new", "client", acme, "name", "Again", "code", "ui-customers")))
	assert.Contains(t, dup, `id="portal-app-form-code-err"`)
	assert.Contains(t, dup, "already exists")
	again, err := oauth.FindByPortalClient(ctx, acme)
	require.NoError(t, err)
	assert.Len(t, again, 1)

	// A second, public app for the second client.
	do(t, get(admin, postForm("/ui/portal-apps/new", "client", globex, "name", "UI Globex SPA", "code", "ui-globex", "clientType", "PUBLIC")))
	gapp, err := apps.FindByClientAndCode(ctx, globex, "ui-globex")
	require.NoError(t, err)
	require.NotNil(t, gapp)

	// Identities (the portal backend's ensure, which has no admin page).
	ensure := func(clientID, email, appID string) string {
		ev, err := usecaseop.Run(ctx, uow, portalidentity.Ensure(identities, clients, apps),
			portalidentity.EnsureCommand{ClientID: clientID, Email: email, Source: "INVITE", PortalAppID: appID}, testpg.TestEC())
		require.NoError(t, err)
		return ev.IdentityID
	}
	ann := ensure(acme, "ann@ui-portal.test", app.ID)
	bob := ensure(acme, "bob@ui-portal.test", "")
	gus := ensure(globex, "gus@ui-portal.test", gapp.ID)

	list := do(t, get(admin, req{target: "/ui/portal-users?client=" + acme}))
	assert.Contains(t, list, "ann@ui-portal.test")
	assert.Contains(t, list, "bob@ui-portal.test")
	assert.NotContains(t, list, "gus@ui-portal.test", "another client's identity")
	assert.Contains(t, list, "No portal app")
	apl := do(t, get(admin, req{target: "/ui/portal-apps?client=" + acme}))
	assert.Contains(t, apl, "1 portal user(s) have no portal app")

	// The search and the app filters are the repository's.
	assert.Contains(t, do(t, get(admin, req{target: "/ui/portal-users?client=" + acme + "&q=BOB"})), "bob@ui-portal.test")
	filtered := do(t, get(admin, req{target: "/ui/portal-users?client=" + acme + "&app=ui-customers"}))
	assert.Contains(t, filtered, "ann@ui-portal.test")
	assert.NotContains(t, filtered, "bob@ui-portal.test")
	unassigned := do(t, get(admin, req{target: "/ui/portal-users?client=" + acme + "&app=__unassigned__"}))
	assert.Contains(t, unassigned, "bob@ui-portal.test")
	assert.NotContains(t, unassigned, "ann@ui-portal.test")

	// Suspend, reactivate, grant and revoke.
	status := func(id string) portalidentity.Status {
		i, err := identities.FindByID(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, i)
		return i.Status
	}
	do(t, get(admin, postForm("/ui/portal-users/"+ann+"/suspend", "client", acme)))
	assert.Equal(t, portalidentity.StatusDisabled, status(ann))
	do(t, get(admin, postForm("/ui/portal-users/"+ann+"/reactivate", "client", acme)))
	assert.Equal(t, portalidentity.StatusActive, status(ann))
	do(t, get(admin, postForm("/ui/portal-users/"+bob+"/grant", "client", acme, "app", "ui-customers")))
	granted, err := identities.FindByID(ctx, bob)
	require.NoError(t, err)
	require.Len(t, granted.Apps, 1)
	assert.Equal(t, app.ID, granted.Apps[0].AppID)
	assert.Equal(t, portalidentity.SourceAdmin, granted.Apps[0].Source)
	do(t, get(admin, postForm("/ui/portal-users/"+bob+"/apps/ui-customers/revoke", "client", acme)))
	revoked, err := identities.FindByID(ctx, bob)
	require.NoError(t, err)
	assert.Empty(t, revoked.Apps)

	// Assign the user with no app: bob gets the app (a suspended user too).
	assigned := do(t, get(admin, postForm("/ui/portal-apps/"+app.ID+"/assign-unassigned", "client", acme)))
	assert.Contains(t, assigned, "1 user(s) can now sign in to UI Customers")
	n, err := identities.CountUnassigned(ctx, acme)
	require.NoError(t, err)
	assert.Zero(t, n)

	// Permissions are the real checks on real client ids.
	acmeViewer := portalCaller([]string{acme}, permPortalView)
	globexManager := portalCaller([]string{globex}, permPortalView, permPortalManage)
	acmeManager := portalCaller([]string{acme}, permPortalView, permPortalManage)
	assert.Contains(t, do(t, get(acmeViewer, req{target: "/ui/portal-users?client=" + acme})), "ann@ui-portal.test")
	assert.Equal(t, http.StatusForbidden, get(globexManager, req{target: "/ui/portal-users?client=" + acme})().Code)
	assert.Equal(t, http.StatusForbidden, get(globexManager, req{target: "/ui/portal-apps?client=" + acme})().Code)
	before := audits(t)
	for name, c := range map[string]struct {
		ac *auth.AuthContext
		q  req
	}{
		"viewer deletes a user":                 {acmeViewer, postForm("/ui/portal-users/"+ann+"/delete", "client", acme)},
		"viewer suspends a user":                {acmeViewer, postForm("/ui/portal-users/"+ann+"/suspend", "client", acme)},
		"other manager deletes a user":          {globexManager, postForm("/ui/portal-users/"+ann+"/delete", "client", acme)},
		"viewer deletes an app":                 {acmeViewer, postForm("/ui/portal-apps/"+app.ID+"/delete", "client", acme)},
		"other manager deletes an app":          {globexManager, postForm("/ui/portal-apps/"+app.ID+"/delete", "client", acme)},
		"viewer creates an app":                 {acmeViewer, postForm("/ui/portal-apps/new", "client", acme, "name", "Forged", "code", "forged")},
		"manager of acme deletes globex's user": {acmeManager, postForm("/ui/portal-users/"+gus+"/delete", "client", acme)},
		"manager of acme deletes globex's app":  {acmeManager, postForm("/ui/portal-apps/"+gapp.ID+"/delete", "client", acme)},
		"manager of acme names globex":          {acmeManager, postForm("/ui/portal-users/"+gus+"/delete", "client", globex)},
	} {
		rec := get(c.ac, c.q)()
		assert.Equal(t, http.StatusOK, rec.Code, name) // a Datastar refusal is a flash on a 200
		assert.True(t, strings.Contains(strings.ToLower(rec.Body.String()), "permission required") ||
			strings.Contains(strings.ToLower(rec.Body.String()), "no access to this client") ||
			strings.Contains(rec.Body.String(), "not found"), "%s: the refusal is shown:\n%s", name, rec.Body.String())
	}
	assert.Equal(t, before, audits(t), "refused posts wrote nothing")
	still, err := identities.FindByID(ctx, ann)
	require.NoError(t, err)
	assert.NotNil(t, still, "the viewer could not delete the user")
	stillG, err := identities.FindByID(ctx, gus)
	require.NoError(t, err)
	assert.NotNil(t, stillG, "a manager of another client could not delete the user")
	stillApp, err := apps.FindByID(ctx, app.ID)
	require.NoError(t, err)
	assert.NotNil(t, stillApp)

	// The manager deletes a user, then the app with its OAuth client.
	do(t, get(acmeManager, postForm("/ui/portal-users/"+bob+"/delete", "client", acme)))
	gone, err := identities.FindByID(ctx, bob)
	require.NoError(t, err)
	assert.Nil(t, gone)
	del := do(t, get(acmeManager, postForm("/ui/portal-apps/"+app.ID+"/delete", "client", acme)))
	assert.Contains(t, del, "UI Customers")
	goneApp, err := apps.FindByID(ctx, app.ID)
	require.NoError(t, err)
	assert.Nil(t, goneApp)
	goneOC, err := oauth.FindByID(ctx, oc.ID)
	require.NoError(t, err)
	assert.Nil(t, goneOC, "the app's OAuth client is deleted with it")
}

// audits is the number of audit rows (a refused post writes none).
func audits(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(), `SELECT count(*) FROM aud_logs`).Scan(&n))
	return n
}
