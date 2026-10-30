//go:build integration

package pages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	fnops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// fnsPG is the functions page on Postgres: the handlers run the real use
// cases (FunctionOps over the unit of work), so the rows, the encryption at
// rest, the domain events and the audit records are the API's.
type fnsPG struct {
	t         *testing.T
	page      *Functions
	repo      *function.Repository
	uow       *usecasepgx.UnitOfWork
	artifacts artifact.Store
	loader    *runtimes.Loader
	// bodies are every answer the page gave, to check none carries a secret.
	bodies []string
}

func newFnsPG(t *testing.T) *fnsPG {
	t.Helper()
	// SECRET and DB values are encrypted at rest; the repository reads the key
	// when it is built.
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("FLOWCATALYST_APP_KEY", key)

	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	apps := application.NewRepository(pool)
	domains := functiondomain.NewRepository(pool)
	clients := client.NewRepository(pool)
	uow := testpg.NewUoW(t)
	store, err := artifact.NewFileStore(t.TempDir())
	require.NoError(t, err)
	b, err := budget.New(256<<20, 0)
	require.NoError(t, err)
	e, err := engine.New(context.Background(), engine.Config{Budget: b})
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Close(context.Background()) })

	wiring := fnops.WiringDeps{
		Subscriptions: subscription.NewRepository(pool), ScheduledJobs: scheduledjob.NewRepository(pool),
		DispatchPools: dispatchpool.NewRepository(pool), ServiceAccounts: serviceaccount.NewRepository(pool),
		RunnerURLTemplate: "http://127.0.0.1:8095",
	}
	return &fnsPG{
		t: t, repo: repo, uow: uow, artifacts: store, loader: runtimes.NewLoader(e),
		page: &Functions{
			Functions: repo,
			Ops:       FunctionOps{Repo: repo, Apps: apps, Wiring: wiring, Domains: domains, Artifacts: store, UoW: uow},
			Zones:     domains, Clients: clients, ClientNames: clients, Apps: apps, ServiceAccounts: serviceaccount.NewRepository(pool),
			Shell: shellForTest(),
		},
	}
}

// do serves one request as ac and records the answer.
func (p *fnsPG) do(ac *auth.AuthContext, q req) *httpResult {
	p.t.Helper()
	rec := serveMounted(p.t, ac, q, p.page, &ClientOptions{Clients: p.page.Clients, Shell: p.page.Shell})
	body := rec.Body.String()
	p.bodies = append(p.bodies, body)
	return &httpResult{code: rec.Code, body: body, location: rec.Header().Get("Location")}
}

type httpResult struct {
	code           int
	body, location string
}

func (p *fnsPG) post(ac *auth.AuthContext, target string, form url.Values) *httpResult {
	p.t.Helper()
	return p.do(ac, req{method: http.MethodPost, target: target, form: form, datastar: true, multipart: true})
}

// settingRaw is a setting's stored value.
func (p *fnsPG) settingRaw(id string, kind function.SettingKind, key string) (string, bool) {
	p.t.Helper()
	var v string
	err := testpg.Pool(p.t).QueryRow(context.Background(), `SELECT value FROM fng_settings WHERE function_id = $1 AND kind = $2 AND key = $3`, id, string(kind), key).Scan(&v)
	if err != nil {
		return "", false
	}
	return v, true
}

func fnsJSDescribe(eventType string) string {
	return fmt.Sprintf(`{"abi":1,"endpoints":[{"path":"/on-order","auth":"webhook"}],"subscriptions":[{"eventType":%q,"path":"/on-order"}],`+
		`"config":["GREETING"],"secrets":["API_KEY"],"db":["main"]}`, eventType)
}

func TestFunctionsPageAgainstPostgres(t *testing.T) {
	const (
		secretValue = "sentinel-secret-5d1f"
		dsnValue    = "postgres://u:sentinel-dsn-9c2e@db.internal/app"
	)
	ctx := context.Background()
	pool := testpg.Pool(t)
	p := newFnsPG(t)
	appID := testpg.EnsureApplication(t, "uifnapp")
	admin := anchorAdmin()

	// Create through the dialog.
	res := p.post(admin, "/ui/functions/new", url.Values{
		"application": {appID}, "name": {"ui-pg-fn"}, "client": {""}, "pool": {""}, "serviceAccountId": {""},
		"memoryMb": {"128"}, "maxConcurrency": {""}, "timeoutMs": {""}, "maxBodyBytes": {""},
	})
	require.Contains(t, res.body, "Function created", res.body)
	fn, err := p.repo.FindByAddress(ctx, "uifnapp.ui-pg-fn")
	require.NoError(t, err)
	require.NotNil(t, fn, "the function was created")
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM fng_functions WHERE id = $1`, fn.ID) })
	assert.Equal(t, int32(128), fn.Limits.MemoryMB)
	assert.Equal(t, int32(16), fn.Limits.MaxConcurrency, "a blank limit keeps the default")
	assert.Contains(t, res.body, `"selFn":"`+fn.ID+`"`, "the new function opens in the drawer")
	assert.Equal(t, 1, auditCount(t, fn.ID), "create is audited like the API")

	// The same address again is the name field's conflict.
	dup := p.post(admin, "/ui/functions/new", url.Values{"application": {appID}, "name": {"ui-pg-fn"}})
	assert.Contains(t, dup.body, `id="fns-form-name-err"`)
	assert.Contains(t, dup.body, "already exists")

	// It is listed, searchable by address prefix, and its drawer opens.
	list := p.do(admin, req{target: "/ui/functions?q=uifnapp.ui-pg"})
	assert.Contains(t, list.body, "uifnapp.ui-pg-fn")
	assert.Contains(t, p.do(admin, req{target: "/ui/functions/" + fn.ID, datastar: true}).body, "No versions published yet.")

	// Edit: description, pool, warm; a service account is set, then cleared.
	sa := serviceaccount.New("uifnapp-sa", "UI fn SA")
	sa.ApplicationID = ids.PtrOf[ids.ApplicationID](&appID)
	token, signing := "tok_uifn", "sig_uifn"
	sa.WebhookCredentials = serviceaccount.WebhookCredentials{AuthType: serviceaccount.AuthBearer, Token: &token, SigningSecret: &signing}
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, serviceaccount.NewRepository(pool).Persist(ctx, sa, usecasepgx.WrapTxForBootstrap(tx)))
	require.NoError(t, tx.Commit(ctx))
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM iam_service_accounts WHERE id = $1`, sa.ID) })

	edit := func(kv ...string) *httpResult {
		form := url.Values{
			"description": {"Thumbnails"}, "pool": {"px"}, "warm": {"true"}, "serviceAccountId": {sa.ID},
			"memoryMb": {"128"}, "maxConcurrency": {"16"}, "timeoutMs": {"30000"}, "maxBodyBytes": {"1048576"}, "drawer": {fn.ID}, "tab": {"overview"},
		}
		for i := 0; i+1 < len(kv); i += 2 {
			form.Set(kv[i], kv[i+1])
		}
		return p.post(admin, "/ui/functions/"+fn.ID+"/edit", form)
	}
	require.Contains(t, edit().body, "Function updated")
	got, err := p.repo.FindByID(ctx, fn.ID)
	require.NoError(t, err)
	assert.Equal(t, "Thumbnails", deref(got.Description))
	assert.Equal(t, "px", deref(got.Pool))
	assert.True(t, got.Warm)
	assert.Equal(t, sa.ID, deref(got.ServiceAccountID))
	edit("serviceAccountId", "", "pool", "")
	got, _ = p.repo.FindByID(ctx, fn.ID)
	assert.Nil(t, got.ServiceAccountID, "a blank account clears it")
	assert.Nil(t, got.Pool, "a blank pool returns to the default")
	// A limit above the ceiling is the field's error.
	assert.Contains(t, edit("memoryMb", "999999").body, `id="fns-form-memory-mb-err"`)
	edit("serviceAccountId", sa.ID)
	got, _ = p.repo.FindByID(ctx, fn.ID)
	assert.Equal(t, sa.ID, deref(got.ServiceAccountID), "the account is bound for the promote below")
	assert.GreaterOrEqual(t, auditCount(t, fn.ID), 4, "the create and the updates are audited")

	// Publish a version (the CLI's job) and make it ready.
	src := []byte("globalThis.__fc = { describe() { return `" + fnsJSDescribe("uifnapp:orders:order:created") + "`; }, handle() { return { meta: '{}', body: new Uint8Array(0) }; } };")
	digest := fnsDigest(src)
	require.NoError(t, p.artifacts.Put(ctx, digest, strings.NewReader(string(src))))
	pub, err := usecaseop.RunTx(testpg.AnchorCtx(), p.uow, fnops.Publish(p.repo, p.artifacts, p.loader),
		fnops.PublishCommand{FunctionID: fn.ID, Digest: digest, Runtime: runtimes.JS}, testpg.TestEC())
	require.NoError(t, err)
	require.NoError(t, p.repo.SetVersionReady(ctx, pub.Version.ID, time.Now().UTC()))
	versions := p.do(admin, req{target: "/ui/functions/" + fn.ID + "?tab=versions", datastar: true})
	assert.Contains(t, versions.body, `/ui/functions/`+fn.ID+`/versions/1/promote`, "a ready version offers Promote")
	assert.Contains(t, versions.body, "From version 1 — no version promoted yet")
	assert.Contains(t, versions.body, "One or more declared keys have no value set.")

	// Promote is refused while declared settings are missing.
	refused := p.post(admin, "/ui/functions/"+fn.ID+"/versions/1/promote", url.Values{"drawer": {fn.ID}})
	assert.NotContains(t, refused.body, "Promoted")
	aliases, err := p.repo.ListAliases(ctx, fn.ID)
	require.NoError(t, err)
	assert.Empty(t, aliases, "a refused promote moved nothing")

	// Settings: a config value, a secret and a DSN. The secret and DSN are
	// stored encrypted, and appear in no answer and no audit record.
	assert.Contains(t, p.post(admin, "/ui/functions/"+fn.ID+"/settings/config", url.Values{"key": {"GREETING"}, "value": {"hi there"}, "drawer": {fn.ID}, "tab": {"settings"}}).body, "Set config value")
	sec := p.post(admin, "/ui/functions/"+fn.ID+"/settings/secrets", url.Values{"key": {"API_KEY"}, "value": {secretValue}, "drawer": {fn.ID}, "tab": {"settings"}})
	assert.Contains(t, sec.body, "Set secret")
	p.post(admin, "/ui/functions/"+fn.ID+"/settings/db", url.Values{"key": {"main"}, "value": {dsnValue}, "drawer": {fn.ID}, "tab": {"settings"}})
	raw, ok := p.settingRaw(fn.ID, function.SettingSecret, "API_KEY")
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(raw, "encrypted:"), "a secret is stored encrypted: %q", raw)
	assert.NotContains(t, raw, secretValue)
	rawDB, _ := p.settingRaw(fn.ID, function.SettingDB, "main")
	assert.True(t, strings.HasPrefix(rawDB, "encrypted:"))
	drawer := p.do(admin, req{target: "/ui/functions/" + fn.ID + "?tab=settings", datastar: true})
	assert.Contains(t, drawer.body, "hi there")
	assert.NotContains(t, drawer.body, "One or more declared keys have no value set.")
	for _, table := range []string{"aud_logs", "msg_events"} {
		var leaked int
		require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM `+table+` t WHERE to_jsonb(t)::text LIKE '%sentinel-%'`).Scan(&leaked))
		assert.Zero(t, leaked, "%s carries a secret value", table)
	}

	// Promote now wires the function; the flash says what was created.
	promoted := p.post(admin, "/ui/functions/"+fn.ID+"/versions/1/promote", url.Values{"drawer": {fn.ID}, "tab": {"versions"}})
	require.Contains(t, promoted.body, "Version 1 is now live", promoted.body)
	assert.Contains(t, promoted.body, "pool fn-uifnapp-ui-pg-fn · 1 subscription created")
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM msg_dispatch_pools WHERE code = 'fn-uifnapp-ui-pg-fn'`) })
	subs, err := subscription.NewRepository(pool).FindByFunctionID(ctx, fn.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Contains(t, promoted.body, `id="fns-alias-live"`, "the drawer shows the alias")

	// Retire is refused while the alias points at the version.
	inUse := p.post(admin, "/ui/functions/"+fn.ID+"/versions/1/retire", url.Values{"drawer": {fn.ID}})
	assert.Contains(t, inUse.body, "fc-flash")
	v1, _ := p.repo.GetVersionByNumber(ctx, fn.ID, 1)
	assert.Equal(t, function.VersionReady, v1.Status)

	// Demote removes the live alias and the wiring; then retire works.
	demoted := p.post(admin, "/ui/functions/"+fn.ID+"/aliases/live/delete", url.Values{"drawer": {fn.ID}, "tab": {"versions"}})
	assert.Contains(t, demoted.body, "Alias removed — 1 subscription deleted")
	subs, _ = subscription.NewRepository(pool).FindByFunctionID(ctx, fn.ID)
	assert.Empty(t, subs)
	p.post(admin, "/ui/functions/"+fn.ID+"/versions/1/retire", url.Values{"drawer": {fn.ID}})
	v1, _ = p.repo.GetVersionByNumber(ctx, fn.ID, 1)
	assert.Equal(t, function.VersionRetired, v1.Status)

	// Routes: a hostname needs a claimed zone.
	notClaimed := p.post(admin, "/ui/functions/"+fn.ID+"/routes", url.Values{"hostname": {"api.uifn-unclaimed.test"}, "pathPrefix": {"/"}})
	assert.Contains(t, notClaimed.body, `id="fns-route-form-hostname-err"`)
	zone := functiondomain.New("uifn-claimed.test", nil)
	ztx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, functiondomain.NewRepository(pool).Persist(ctx, zone, usecasepgx.WrapTxForBootstrap(ztx)))
	require.NoError(t, ztx.Commit(ctx))
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM fng_domains WHERE id = $1`, zone.ID) })
	saved := p.post(admin, "/ui/functions/"+fn.ID+"/routes", url.Values{"hostname": {"API.uifn-claimed.test"}, "pathPrefix": {"hooks"}, "aliasPrefixes": {"qa, dev"}, "drawer": {fn.ID}, "tab": {"routes"}})
	require.Contains(t, saved.body, "Route saved", saved.body)
	routes, err := p.repo.ListRoutesByFunction(ctx, fn.ID)
	require.NoError(t, err)
	require.Len(t, routes, 1)
	assert.Equal(t, "api.uifn-claimed.test", routes[0].Hostname)
	assert.Equal(t, "/hooks", routes[0].PathPrefix)
	assert.Equal(t, []string{"dev", "qa"}, routes[0].AliasPrefixes)
	assert.Contains(t, p.do(admin, req{target: "/ui/functions/" + fn.ID + "?tab=routes", datastar: true}).body, "uifn-claimed.test")
	p.post(admin, "/ui/functions/"+fn.ID+"/routes/"+routes[0].ID+"/delete", url.Values{"drawer": {fn.ID}})
	routes, _ = p.repo.ListRoutesByFunction(ctx, fn.ID)
	assert.Empty(t, routes)

	// A setting is removed.
	p.post(admin, "/ui/functions/"+fn.ID+"/settings/secrets/delete?key=API_KEY", url.Values{"drawer": {fn.ID}})
	_, ok = p.settingRaw(fn.ID, function.SettingSecret, "API_KEY")
	assert.False(t, ok)

	// Delete needs the typed address: a wrong one changes nothing.
	wrong := p.post(admin, "/ui/functions/"+fn.ID+"/delete", url.Values{"confirm": {"uifnapp.ui-pg"}, "drawer": {fn.ID}})
	assert.Contains(t, wrong.body, `id="fns-delete-form-confirm-err"`)
	still, err := p.repo.FindByID(ctx, fn.ID)
	require.NoError(t, err)
	require.NotNil(t, still, "a wrong address deleted nothing")
	right := p.post(admin, "/ui/functions/"+fn.ID+"/delete", url.Values{"confirm": {" uifnapp.ui-pg-fn "}, "drawer": {fn.ID}})
	assert.Contains(t, right.body, `"selFn":""`)
	gone, err := p.repo.FindByID(ctx, fn.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)

	// No answer of the whole run carried a secret or a DSN.
	for i, b := range p.bodies {
		assert.NotContains(t, b, secretValue, "answer %d", i)
		assert.NotContains(t, b, "sentinel-dsn", "answer %d", i)
		assert.NotContains(t, b, "hunter2", "answer %d", i)
	}
}

// A caller scoped to one client cannot read or change another client's
// function through the page: the use cases' scope check is the guard, the
// form is not.
func TestFunctionsPageClientScopeAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	p := newFnsPG(t)
	appID := testpg.EnsureApplication(t, "uifnscope")
	clients := client.NewRepository(pool)
	for _, c := range []struct{ name, ident string }{{"Fn Scope A", "uifn-scope-a"}, {"Fn Scope B", "uifn-scope-b"}} {
		_, err := usecaseop.Run(testpg.AnchorCtx(), testpg.NewUoW(t), clientops.CreateClient(clients), clientops.CreateCommand{Name: c.name, Identifier: c.ident}, testpg.TestEC())
		require.NoError(t, err)
	}
	a, err := clients.FindByIdentifier(ctx, "uifn-scope-a")
	require.NoError(t, err)
	b, err := clients.FindByIdentifier(ctx, "uifn-scope-b")
	require.NoError(t, err)

	// Anchor creates a function for client A.
	res := p.post(anchorAdmin(), "/ui/functions/new", url.Values{"application": {appID}, "name": {"scoped-fn"}, "client": {"uifn-scope-a"}})
	require.Contains(t, res.body, "Function created", res.body)
	fn, err := p.repo.FindByAddress(ctx, "uifnscope.scoped-fn")
	require.NoError(t, err)
	require.NotNil(t, fn)
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM fng_functions WHERE id = $1`, fn.ID) })
	require.Equal(t, string(a.ID), deref(fn.ClientID))

	clientUser := func(c *client.Client) *auth.AuthContext {
		return &auth.AuthContext{
			PrincipalID: "prn_cu", Email: "cu@example.com", Scope: auth.ScopeClient, Clients: []string{string(c.ID)}, Roles: []string{"r"},
			Permissions: []string{fnPermView, fnPermManage, fnPermPromote, fnPermSecrets, fnPermRoutes}, PrincipalType: auth.PrincipalTypeUser,
		}
	}
	other := clientUser(b)
	own := clientUser(a)

	// B's user does not see it, cannot open it, and every write is refused.
	assert.NotContains(t, p.do(other, req{target: "/ui/functions?q=uifnscope."}).body, "uifnscope.scoped-fn")
	assert.Equal(t, http.StatusForbidden, p.do(other, req{target: "/ui/functions/" + fn.ID}).code)
	for _, w := range []struct {
		target string
		form   url.Values
	}{
		{"/ui/functions/" + fn.ID + "/edit", url.Values{"description": {"hijacked"}, "warm": {"true"}}},
		{"/ui/functions/" + fn.ID + "/settings/config", url.Values{"key": {"K"}, "value": {"v"}}},
		{"/ui/functions/" + fn.ID + "/settings/secrets", url.Values{"key": {"K"}, "value": {"sentinel-forged"}}},
		{"/ui/functions/" + fn.ID + "/aliases", url.Values{"name": {"qa"}, "version": {"1"}}},
		{"/ui/functions/" + fn.ID + "/routes", url.Values{"hostname": {"x.example.com"}}},
		{"/ui/functions/" + fn.ID + "/delete", url.Values{"confirm": {"uifnscope.scoped-fn"}}},
	} {
		p.post(other, w.target, w.form)
	}
	still, err := p.repo.FindByID(ctx, fn.ID)
	require.NoError(t, err)
	require.NotNil(t, still, "another client's user deleted the function")
	assert.Nil(t, still.Description, "another client's user edited the function")
	settings, _ := p.repo.ListSettings(ctx, fn.ID)
	assert.Empty(t, settings, "another client's user wrote settings")
	routes, _ := p.repo.ListRoutesByFunction(ctx, fn.ID)
	assert.Empty(t, routes)

	// A's own user can.
	assert.Contains(t, p.do(own, req{target: "/ui/functions?q=uifnscope."}).body, "uifnscope.scoped-fn")
	p.post(own, "/ui/functions/"+fn.ID+"/settings/config", url.Values{"key": {"K"}, "value": {"v"}})
	settings, _ = p.repo.ListSettings(ctx, fn.ID)
	assert.Len(t, settings, 1)
	// A client user cannot create a platform-owned function.
	plat := p.post(own, "/ui/functions/new", url.Values{"application": {appID}, "name": {"platform-fn"}})
	missing, _ := p.repo.FindByAddress(ctx, "uifnscope.platform-fn")
	assert.Nil(t, missing, "a client user created a platform-owned function: %s", plat.body)
}

func fnsDigest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
