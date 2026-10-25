//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// ingPG is the ingestions page on Postgres: the handlers run the real use
// cases (IngestionOps over the unit of work), so the rows, the reach rules,
// the kind permission and the set ownership are the API's.
type ingPG struct {
	t       *testing.T
	page    *Ingestions
	repo    *ingestion.Repository
	app     *application.Application
	client  *client.Client
	other   *client.Client
	poolRef string
}

func newIngPG(t *testing.T) *ingPG {
	t.Helper()
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("FLOWCATALYST_APP_KEY", key)

	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	apps, clients := application.NewRepository(pool), client.NewRepository(pool)
	repo := ingestion.NewRepository(pool)
	resolver := ingestion.NewResolver(apps, clients, dispatchpool.NewRepository(pool), connection.NewRepository(pool), serviceaccount.NewRepository(pool), nil)

	p := &ingPG{t: t, repo: repo, poolRef: "pool-" + ingUnique()}
	p.app = application.NewIntegration("ingui-"+ingUnique(), "Ingest UI")
	p.client = client.New("Acme", "acme-"+ingUnique())
	p.other = client.New("Beta", "beta-"+ingUnique())
	tx, err := uow.Pool().Begin(context.Background())
	require.NoError(t, err)
	db := usecasepgx.WrapTxForBootstrap(tx)
	ctx := context.Background()
	require.NoError(t, apps.Persist(ctx, p.app, db))
	require.NoError(t, clients.Persist(ctx, p.client, db))
	require.NoError(t, clients.Persist(ctx, p.other, db))
	require.NoError(t, dispatchpool.NewRepository(pool).Persist(ctx, dispatchpool.New(p.poolRef, p.poolRef), db))
	require.NoError(t, tx.Commit(ctx))

	p.page = &Ingestions{
		Store: repo, Ops: IngestionOps{Repo: repo, Resolver: resolver, UoW: uow},
		Apps: apps, Clients: clients, ClientNames: clients, Shell: shellForTest(),
	}
	return p
}

func ingUnique() string { return strings.ToLower(tsid.GenerateUntyped()) }

func (p *ingPG) do(ac *auth.AuthContext, q req) *ingResult {
	p.t.Helper()
	rec := serveMounted(p.t, ac, q, p.page, &ClientOptions{Clients: p.page.Clients, Shell: p.page.Shell})
	return &ingResult{code: rec.Code, body: rec.Body.String(), location: rec.Header().Get("Location")}
}

type ingResult struct {
	code           int
	body, location string
}

func (p *ingPG) post(ac *auth.AuthContext, target string, form url.Values) *ingResult {
	p.t.Helper()
	return p.do(ac, req{method: http.MethodPost, target: target, form: form, datastar: true, multipart: true})
}

func (p *ingPG) form(code string) url.Values {
	f := ingCreateForm()
	f.Set("application", p.app.Code)
	f.Set("client", p.client.Identifier)
	f.Set("code", code)
	f.Set("target.pool", p.poolRef)
	// A target's job code names the ingestion's own application.
	f.Set("target.code", p.app.Code+":sync:order:received")
	return f
}

// find reads the stored ingestion.
func (p *ingPG) find(code string) *ingestion.Ingestion {
	p.t.Helper()
	i, err := p.repo.FindByCode(context.Background(), p.app.ID, p.client.ID, code)
	require.NoError(p.t, err)
	require.NotNil(p.t, i, code)
	return i
}

func ingAnchor(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_ingtester0001", Email: "ing@flowcatalyst.local", Scope: auth.ScopeAnchor, AllApplications: true, Roles: []string{"r"}, Permissions: perms, PrincipalType: auth.PrincipalTypeUser}
}

func TestIngestionsPageAgainstPostgres(t *testing.T) {
	p := newIngPG(t)
	admin := ingAnchor(ingPermView, ingPermCreate, ingPermUpdate, ingPermDelete, ingPermPromote, ingPermSecrets, ingPermIdentity)
	code := "feed-" + ingUnique()

	// Create through the form: the stored definition is what the form said.
	res := p.post(admin, "/ui/ingestions/new", p.form(code))
	require.Equal(t, http.StatusOK, res.code, res.body)
	assert.Contains(t, res.body, "Ingestion created")
	i := p.find(code)
	assert.Equal(t, 1, i.Version)
	assert.True(t, i.Active)
	assert.Equal(t, "New feed", i.Definition.Name)
	assert.Equal(t, ingestion.BearerAuth{Token: ingestion.SecretRef{Name: "push-token"}}, i.Definition.Source.(ingestion.HTTP).Auth)
	assert.Equal(t, p.poolRef, i.Definition.Target.Pool)

	// The code is taken, the pool unknown: each lands on its field.
	dup := p.post(admin, "/ui/ingestions/new", p.form(code))
	assert.Contains(t, dup.body, `id="ings-form-code-err"`)
	f := p.form(code + "-2")
	f.Set("target.pool", "no-such-pool")
	assert.Contains(t, p.post(admin, "/ui/ingestions/new", f).body, `id="ings-form-target-pool-err"`)
	f = p.form(code + "-3")
	f.Set("application", "no-such-app")
	assert.Contains(t, p.post(admin, "/ui/ingestions/new", f).body, `id="ings-form-application-err"`)
	f = p.form(code + "-4")
	f.Set("client", "no-such-client")
	assert.Contains(t, p.post(admin, "/ui/ingestions/new", f).body, `id="ings-form-client-err"`)

	// The list and the drawer read it back.
	list := p.do(admin, req{target: "/ui/ingestions?q=" + code})
	assert.Contains(t, list.body, `id="ings-`+i.ID+`"`)
	drawer := p.do(admin, req{target: "/ui/ingestions/" + i.ID, datastar: true})
	assert.Contains(t, drawer.body, "application: "+p.app.Code)

	// A change is a saved version that is not live until it is made live.
	change := p.form(code)
	for _, k := range []string{"application", "client", "code", "live"} {
		change.Del(k)
	}
	change.Set("name", "Changed")
	change.Set("note", "second")
	require.Equal(t, http.StatusOK, p.post(admin, "/ui/ingestions/"+i.ID+"/edit", change).code)
	i = p.find(code)
	assert.Equal(t, 1, i.Version)
	assert.Equal(t, 2, i.LatestVersion)
	assert.Equal(t, "New feed", i.Definition.Name)
	versions := p.do(admin, req{target: "/ui/ingestions/" + i.ID + "?tab=versions&a=1&b=2"})
	assert.Contains(t, versions.body, "second")
	assert.Contains(t, versions.body, "Changed")

	live := p.post(admin, "/ui/ingestions/"+i.ID+"/versions/2/live", url.Values{"drawer": {i.ID}})
	assert.Contains(t, live.body, "Version 2 is live")
	i = p.find(code)
	assert.Equal(t, 2, i.Version)
	assert.Equal(t, "Changed", i.Definition.Name)

	// Pause and resume.
	p.post(admin, "/ui/ingestions/"+i.ID+"/pause", url.Values{"drawer": {i.ID}})
	assert.False(t, p.find(code).Active)
	p.post(admin, "/ui/ingestions/"+i.ID+"/resume", url.Values{"drawer": {i.ID}})
	assert.True(t, p.find(code).Active)

	// Remove.
	gone := p.post(admin, "/ui/ingestions/"+i.ID+"/delete", url.Values{"drawer": {i.ID}})
	assert.Contains(t, gone.body, "Ingestion removed")
	got, err := p.repo.FindByID(context.Background(), i.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestIngestionsPageEnforcesThePermissionsAndReach(t *testing.T) {
	p := newIngPG(t)
	admin := ingAnchor(ingPermView, ingPermCreate, ingPermUpdate, ingPermDelete, ingPermPromote, ingPermSecrets, ingPermIdentity)
	code := "reach-" + ingUnique()
	require.Equal(t, http.StatusOK, p.post(admin, "/ui/ingestions/new", p.form(code)).code)
	i := p.find(code)

	t.Run("each action needs its permission", func(t *testing.T) {
		viewer := ingAnchor(ingPermView)
		for _, target := range []string{"/pause", "/resume", "/delete", "/versions/1/live"} {
			assert.Contains(t, p.post(viewer, "/ui/ingestions/"+i.ID+target, url.Values{"drawer": {i.ID}}).body, "ermission required", target)
		}
		assert.Contains(t, p.post(viewer, "/ui/ingestions/new", p.form(code+"-x")).body, "ermission required")
		assert.Equal(t, 1, p.find(code).Version)
		assert.True(t, p.find(code).Active)
	})
	t.Run("a client user reaches its own clients only", func(t *testing.T) {
		other := &auth.AuthContext{
			PrincipalID: "prn_ingtester0002", Scope: auth.ScopeClient, Clients: []string{string(p.other.ID)}, AllApplications: true, Roles: []string{"r"},
			Permissions: []string{ingPermView, ingPermCreate, ingPermUpdate, ingPermDelete, ingPermPromote}, PrincipalType: auth.PrincipalTypeUser,
		}
		assert.NotContains(t, p.do(other, req{target: "/ui/ingestions?q=" + code}).body, i.ID)
		assert.Contains(t, p.do(other, req{target: "/ui/ingestions/" + i.ID, datastar: true}).body, "no access")
		assert.Contains(t, p.post(other, "/ui/ingestions/"+i.ID+"/pause", url.Values{"drawer": {i.ID}}).body, "no access")
		assert.True(t, p.find(code).Active)
		// Nor can it create under a client it does not reach.
		assert.Contains(t, p.post(other, "/ui/ingestions/new", p.form(code+"-y")).body, `id="ings-form-`)
		none, err := p.repo.FindByCode(context.Background(), p.app.ID, p.client.ID, code+"-y")
		require.NoError(t, err)
		assert.Nil(t, none)
	})
	t.Run("an application's account reaches its application only", func(t *testing.T) {
		ac := ingAnchor(ingPermView, ingPermUpdate)
		ac.AllApplications, ac.Applications = false, []string{"app_somebody_else"}
		assert.NotContains(t, p.do(ac, req{target: "/ui/ingestions?q=" + code}).body, i.ID)
		assert.Contains(t, p.post(ac, "/ui/ingestions/"+i.ID+"/pause", url.Values{"drawer": {i.ID}}).body, "no access")
		assert.True(t, p.find(code).Active)
	})
}

func TestIngestionsOfASetAreReadOnlyAndAKindNeedsItsPermission(t *testing.T) {
	p := newIngPG(t)
	ctx := context.Background()
	admin := ingAnchor(ingPermView, ingPermCreate, ingPermUpdate, ingPermDelete, ingPermPromote, ingPermSecrets, ingPermIdentity)
	code := "owned-" + ingUnique()
	require.Equal(t, http.StatusOK, p.post(admin, "/ui/ingestions/new", p.form(code)).code)
	i := p.find(code)

	// A set owns it.
	setID := tsid.Generate(tsid.IngestionSet)
	pool := testpg.Pool(t)
	now := time.Now().UTC()
	_, err := pool.Exec(ctx, `INSERT INTO ing_sets (id, name, created_at, updated_at) VALUES ($1, $2, $3, $3)`, setID, "set-"+ingUnique(), now)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE ing_ingestions SET managed_by = $2 WHERE id = $1`, i.ID, setID)
	require.NoError(t, err)

	drawer := p.do(admin, req{target: "/ui/ingestions/" + i.ID, datastar: true})
	assert.Contains(t, drawer.body, "Managed by a set")
	assert.NotContains(t, drawer.body, "/ui/ingestions/"+i.ID+"/edit")
	// The forged writes are the use case's refusal; pause still works.
	change := p.form(code)
	for _, k := range []string{"application", "client", "code"} {
		change.Del(k)
	}
	assert.Contains(t, p.post(admin, "/ui/ingestions/"+i.ID+"/edit", change).body, "belongs to set")
	assert.Contains(t, p.post(admin, "/ui/ingestions/"+i.ID+"/delete", url.Values{"drawer": {i.ID}}).body, "belongs to set")
	assert.Contains(t, p.post(admin, "/ui/ingestions/"+i.ID+"/versions/1/live", url.Values{"drawer": {i.ID}}).body, "belongs to set")
	assert.Equal(t, 1, p.find(code).LatestVersion)
	p.post(admin, "/ui/ingestions/"+i.ID+"/pause", url.Values{"drawer": {i.ID}})
	assert.False(t, p.find(code).Active)

	// A runtime reports camel-route; creating one needs the route permission.
	_, err = p.repo.Poll(ctx, ingestion.PollRequest{
		RuntimeID: "rt-" + ingUnique(), ContractVersion: ingestion.ContractVersion, Now: now, Lease: 3 * time.Minute,
		Kinds: []ingestion.RuntimeKind{{Kind: "camel-route", RequiredPermission: auth.PermIngestionRoute}},
	})
	require.NoError(t, err)
	route := p.form("route-" + ingUnique())
	route.Set("kind", "camel-route")
	route.Set("camel-route.s.yaml", "- from:\n    uri: timer:x\n")
	withoutRoute := p.do(admin, req{target: "/ui/ingestions/new", datastar: true})
	assert.NotContains(t, withoutRoute.body, `<option value="camel-route"`)
	refused := p.post(admin, "/ui/ingestions/new", route)
	assert.Contains(t, refused.body, `id="ings-form-kind-err"`)
	assert.Contains(t, refused.body, "ermission required for source kind camel-route")
	none, err := p.repo.FindByCode(ctx, p.app.ID, p.client.ID, route.Get("code"))
	require.NoError(t, err)
	assert.Nil(t, none)

	router := ingAnchor(ingPermView, ingPermCreate, ingPermUpdate, ingPermDelete, ingPermPromote, ingPermRoute, ingPermSecrets, ingPermIdentity)
	assert.Contains(t, p.do(router, req{target: "/ui/ingestions/new", datastar: true}).body, `<option value="camel-route"`)
	created := p.post(router, "/ui/ingestions/new", route)
	assert.Contains(t, created.body, "Ingestion created")
	made, err := p.repo.FindByCode(ctx, p.app.ID, p.client.ID, route.Get("code"))
	require.NoError(t, err)
	require.NotNil(t, made)
	assert.Equal(t, "camel-route", ingestion.RunKind(made.Definition.Source))
	// Changing it needs the permission too; removing needs only delete.
	assert.Contains(t, p.do(admin, req{target: "/ui/ingestions/" + made.ID + "/edit", datastar: true}).body, "platform:ingestion:ingestion:route")
	assert.Contains(t, p.post(admin, "/ui/ingestions/"+made.ID+"/delete", url.Values{"drawer": {made.ID}}).body, "Ingestion removed")
}
