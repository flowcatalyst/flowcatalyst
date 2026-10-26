//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	ingestionapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// TestMain sets FLOWCATALYST_APP_KEY before the shared fixture boots: the
// resolver's service-account repository needs encryption.
func TestMain(m *testing.M) {
	key, err := encryption.GenerateKey()
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("FLOWCATALYST_APP_KEY", key)
	testpg.RunMain(m)
}

const (
	permView    = "platform:ingestion:ingestion:view"
	permCreate  = "platform:ingestion:ingestion:create"
	permUpdate  = "platform:ingestion:ingestion:update"
	permDelete  = "platform:ingestion:ingestion:delete"
	permPromote = "platform:ingestion:ingestion:promote"
	permRoute   = "platform:ingestion:ingestion:route"
	// permIdentity: an SQS or S3 source without a credential (every body
	// here is one); permSecrets: a definition that names secrets.
	permIdentity = "platform:ingestion:ingestion:runtime-identity"
	permSecrets  = "platform:ingestion:ingestion:bind-secrets"
)

// ingestionPerms are the grants the ingestion routes check, route aside: the
// ingestion admin's.
var ingestionPerms = []string{permView, permCreate, permUpdate, permDelete, permPromote, permIdentity, permSecrets}

func without(perms []string, drop string) []string {
	var out []string
	for _, p := range perms {
		if p != drop {
			out = append(out, p)
		}
	}
	return out
}

// Principals. Scope is reach and a permission is authority: each is built
// from the permissions it holds.
func anchor(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_ingtester0001", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: perms}
}

func clientTier(clients []string, perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_ingtester0002", Scope: auth.ScopeClient, Clients: clients, AllApplications: true, Permissions: perms}
}

// appScoped is an application's service account: anchor tier, bound to
// specific applications.
func appScoped(apps []string, perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_ingtester0003", Scope: auth.ScopeAnchor, Applications: apps, Permissions: perms}
}

// env is two integration applications, two clients and a dispatch pool,
// unique to a test, behind the real routes on a real database.
type env struct {
	t        *testing.T
	pool     *pgxpool.Pool
	uow      *usecasepgx.UnitOfWork
	api      humatest.TestAPI
	who      *auth.AuthContext
	appA     *application.Application
	appB     *application.Application
	clientA  *client.Client
	clientB  *client.Client
	poolCode string
	// state is what the routes run on; a test swaps SetArchive to fail it.
	state *ingestionapi.State
}

func unique() string { return strings.ToLower(tsid.GenerateUntyped()) }

func persist(t *testing.T, uow *usecasepgx.UnitOfWork, fn func(context.Context, *usecasepgx.DbTx) error) {
	t.Helper()
	ctx := context.Background()
	tx, err := uow.Pool().Begin(ctx)
	require.NoError(t, err)
	if err := fn(ctx, usecasepgx.WrapTxForBootstrap(tx)); err != nil {
		_ = tx.Rollback(ctx)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit(ctx))
}

func newEnv(t *testing.T) *env {
	t.Helper()
	httpcompat.Init()
	e := &env{t: t, pool: testpg.Pool(t), uow: testpg.NewUoW(t), who: anchor(ingestionPerms...)}
	e.appA = application.NewIntegration("inga-"+unique(), "Ingest A")
	e.appB = application.NewIntegration("ingb-"+unique(), "Ingest B")
	e.clientA = client.New("Acme", "acme-"+unique())
	e.clientB = client.New("Beta", "beta-"+unique())
	e.poolCode = "pool-" + unique()
	apps, clients := application.NewRepository(e.pool), client.NewRepository(e.pool)
	persist(t, e.uow, func(ctx context.Context, tx *usecasepgx.DbTx) error {
		for _, a := range []*application.Application{e.appA, e.appB} {
			if err := apps.Persist(ctx, a, tx); err != nil {
				return err
			}
		}
		for _, c := range []*client.Client{e.clientA, e.clientB} {
			if err := clients.Persist(ctx, c, tx); err != nil {
				return err
			}
		}
		return dispatchpool.NewRepository(e.pool).Persist(ctx, dispatchpool.New(e.poolCode, e.poolCode), tx)
	})

	_, api := humatest.New(t, apiroute.PlatformAPIConfig("test", "dev"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, auth.WithContext(ctx.Context(), e.who)))
	})
	archive, err := artifact.NewFileStore(t.TempDir())
	require.NoError(t, err)
	e.state = &ingestionapi.State{
		SetArchive: archive,
		Repo:       ingestion.NewRepository(e.pool),
		Resolver: ingestion.NewResolver(apps, clients, dispatchpool.NewRepository(e.pool),
			connection.NewRepository(e.pool), serviceaccount.NewRepository(e.pool), nil),
		Apps: apps, Clients: clients, Audit: audit.NewRepository(e.pool), UoW: e.uow,
	}
	ingestionapi.Register(api, e.state)
	e.api = api
	return e
}

// body is a create request for an SQS ingestion of app and client (an id or an
// identifier).
func (e *env) body(app *application.Application, clientRef, code string) map[string]any {
	return map[string]any{
		"application": app.Code, "client": clientRef, "code": code, "name": "Orders " + code,
		"source": map[string]any{"kind": "sqs", "region": "eu-west-1", "queue": "q-" + code},
		"split":  map[string]any{"kind": "none"},
		"target": map[string]any{
			"code": app.Code + ":orders:order:received", "targetUrl": "https://hooks.example.com/orders",
			"pool": e.poolCode, "mode": "IMMEDIATE", "batchSize": 100,
		},
	}
}

// definitionOf is the body of an update or a saved version: a create body
// without its identity.
func (e *env) definitionOf(code, queue string) map[string]any {
	b := e.body(e.appA, "", code)
	delete(b, "application")
	delete(b, "client")
	delete(b, "code")
	b["source"] = map[string]any{"kind": "sqs", "region": "eu-west-1", "queue": queue}
	return b
}

func (e *env) as(who *auth.AuthContext) { e.who = who }

type response struct {
	code int
	raw  string
	json map[string]any
}

func (e *env) do(method, path string, body any) response {
	e.t.Helper()
	var args []any
	if body != nil {
		args = append(args, body)
	}
	rec := e.api.Do(method, path, args...)
	r := response{code: rec.Code, raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &r.json)
	return r
}

func (r response) errCode() string { s, _ := r.json["error"].(string); return s }

func (r response) str(key string) string { s, _ := r.json[key].(string); return s }

func (r response) num(key string) int { f, _ := r.json[key].(float64); return int(f) }

// create makes an ingestion as an administrator and returns its id.
func (e *env) create(code string) string {
	e.t.Helper()
	was := e.who
	e.as(anchor(ingestionPerms...))
	r := e.do(http.MethodPost, "/api/ingestions", e.body(e.appA, e.clientA.Identifier, code))
	e.who = was
	require.Equal(e.t, http.StatusCreated, r.code, r.raw)
	return r.str("id")
}

func (e *env) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	require.NoError(e.t, e.pool.QueryRow(context.Background(), query, args...).Scan(&n))
	return n
}

func (e *env) auditRows(id string) int {
	return e.count(`SELECT count(*) FROM aud_logs WHERE entity_type = 'Ingestion' AND entity_id = $1`, id)
}

func (e *env) eventRows(id string) int {
	return e.count(`SELECT count(*) FROM msg_events WHERE subject = $1`, "platform.ingestion."+id)
}

// Each route needs its permission: refused without it, whatever the scope,
// and not refused with it. Anchor scope alone grants nothing.
func TestPermissionMatrix(t *testing.T) {
	e := newEnv(t)
	id := e.create("matrix-" + unique())

	// also is what the route needs on top of its own permission for these
	// bodies: a create is live by default (promote), an update makes its
	// version live (promote), and every body is an SQS source without a
	// credential that changes the queue (runtime identity).
	routes := []struct {
		name, method, path string
		body               any
		perm               string
		also               []string
	}{
		{"list", http.MethodGet, "/api/ingestions", nil, permView, nil},
		{"get", http.MethodGet, "/api/ingestions/" + id, nil, permView, nil},
		{"create", http.MethodPost, "/api/ingestions", e.body(e.appA, e.clientA.Identifier, "m-"+unique()), permCreate, []string{permPromote, permIdentity}},
		{"update", http.MethodPut, "/api/ingestions/" + id, nil, permUpdate, []string{permPromote, permIdentity}},
		{"pause", http.MethodPost, "/api/ingestions/" + id + "/pause", nil, permUpdate, nil},
		{"resume", http.MethodPost, "/api/ingestions/" + id + "/resume", nil, permUpdate, nil},
		{"save version", http.MethodPost, "/api/ingestions/" + id + "/versions", e.definitionOf("c", "q-save"), permUpdate, []string{permIdentity}},
		{"list versions", http.MethodGet, "/api/ingestions/" + id + "/versions", nil, permView, nil},
		{"make live", http.MethodPost, "/api/ingestions/" + id + "/versions/1/live", nil, permPromote, []string{permIdentity}},
		{"definition", http.MethodGet, "/api/ingestions/" + id + "/definition", nil, permView, nil},
		{"history", http.MethodGet, "/api/ingestions/" + id + "/history", nil, permView, nil},
		{"failures", http.MethodGet, "/api/ingestions/" + id + "/failures", nil, permView, nil},
		{"source kinds", http.MethodGet, "/api/ingestion-source-kinds", nil, permView, nil},
		{"delete", http.MethodDelete, "/api/ingestions/" + id, nil, permDelete, nil},
	}
	for _, rt := range routes {
		body := rt.body
		if rt.name == "update" {
			def := e.definitionOf("c", "q-matrix")
			def["expectedVersion"] = 1
			body = def
		}
		e.as(anchor(without(ingestionPerms, rt.perm)...))
		r := e.do(rt.method, rt.path, body)
		assert.Equal(t, http.StatusForbidden, r.code, "%s without %s: %s", rt.name, rt.perm, r.raw)
		assert.Equal(t, "PERMISSION_REQUIRED", r.errCode(), rt.name)

		e.as(anchor())
		assert.Equal(t, http.StatusForbidden, e.do(rt.method, rt.path, body).code, "%s: an anchor holding no permission is refused", rt.name)

		e.as(anchor(append([]string{rt.perm}, rt.also...)...))
		r = e.do(rt.method, rt.path, body)
		assert.NotEqual(t, http.StatusForbidden, r.code, "%s with %s: %s", rt.name, rt.perm, r.raw)
		assert.Less(t, r.code, 500, "%s: %s", rt.name, r.raw)
	}

	// Saving a version live is also a promotion. (The delete route above
	// removed the first ingestion.)
	id = e.create("matrix-live-" + unique())
	live := e.definitionOf("c", "q-live")
	live["live"] = true
	e.as(anchor(permUpdate, permIdentity))
	r := e.do(http.MethodPost, "/api/ingestions/"+id+"/versions", live)
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	e.as(anchor(permUpdate, permPromote, permIdentity))
	assert.Equal(t, http.StatusCreated, e.do(http.MethodPost, "/api/ingestions/"+id+"/versions", live).code)
}

// Below anchor tier a caller sees and changes only its own clients' ingestions.
func TestClientTierReach(t *testing.T) {
	e := newEnv(t)
	mine := e.create("mine-" + unique())
	// A second client's ingestion.
	e.as(anchor(ingestionPerms...))
	r := e.do(http.MethodPost, "/api/ingestions", e.body(e.appA, string(e.clientB.ID), "theirs-"+unique()))
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	theirs := r.str("id")

	e.as(clientTier([]string{string(e.clientA.ID)}, ingestionPerms...))
	list := e.do(http.MethodGet, "/api/ingestions?applicationCode="+e.appA.Code, nil)
	require.Equal(t, http.StatusOK, list.code, list.raw)
	var ids []string
	for _, it := range list.json["ingestions"].([]any) {
		ids = append(ids, it.(map[string]any)["id"].(string))
	}
	assert.Equal(t, []string{mine}, ids, "only the caller's client")

	assert.Equal(t, http.StatusOK, e.do(http.MethodGet, "/api/ingestions/"+mine, nil).code)
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/ingestions/" + theirs},
		{http.MethodPost, "/api/ingestions/" + theirs + "/pause"},
		{http.MethodDelete, "/api/ingestions/" + theirs},
		{http.MethodGet, "/api/ingestions/" + theirs + "/versions"},
		{http.MethodGet, "/api/ingestions/" + theirs + "/definition"},
	} {
		r := e.do(c.method, c.path, nil)
		assert.Equal(t, http.StatusForbidden, r.code, "%s %s: %s", c.method, c.path, r.raw)
	}
	r = e.do(http.MethodPost, "/api/ingestions", e.body(e.appA, string(e.clientB.ID), "sneak-"+unique()))
	assert.Equal(t, http.StatusForbidden, r.code, "create under a client the caller does not reach: %s", r.raw)
	assert.Equal(t, "SCOPE_FORBIDDEN", r.errCode())
	assert.Equal(t, http.StatusCreated, e.do(http.MethodPost, "/api/ingestions", e.body(e.appA, e.clientA.Identifier, "ok-"+unique())).code)

	// Its own client's list filter is fine; another client's is simply empty.
	other := e.do(http.MethodGet, "/api/ingestions?clientId="+string(e.clientB.ID), nil)
	assert.Equal(t, http.StatusOK, other.code)
	assert.Empty(t, other.json["ingestions"])
}

// An application's service account reaches only its own application's
// ingestions, even at anchor tier.
func TestApplicationScopedReach(t *testing.T) {
	e := newEnv(t)
	inA := e.create("in-a-" + unique())
	e.as(anchor(ingestionPerms...))
	r := e.do(http.MethodPost, "/api/ingestions", e.body(e.appB, e.clientA.Identifier, "in-b-"+unique()))
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	inB := r.str("id")

	e.as(appScoped([]string{string(e.appA.ID)}, ingestionPerms...))
	list := e.do(http.MethodGet, "/api/ingestions?clientId="+e.clientA.Identifier, nil)
	require.Equal(t, http.StatusOK, list.code, list.raw)
	items := list.json["ingestions"].([]any)
	require.Len(t, items, 1)
	assert.Equal(t, inA, items[0].(map[string]any)["id"])

	assert.Equal(t, http.StatusOK, e.do(http.MethodGet, "/api/ingestions/"+inA, nil).code)
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodGet, "/api/ingestions/"+inB, nil).code)
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodPost, "/api/ingestions/"+inB+"/pause", nil).code)
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodDelete, "/api/ingestions/"+inB, nil).code)
	r = e.do(http.MethodPost, "/api/ingestions", e.body(e.appB, e.clientA.Identifier, "sneak-"+unique()))
	assert.Equal(t, http.StatusForbidden, r.code, "create under another application: %s", r.raw)
	assert.Equal(t, http.StatusCreated, e.do(http.MethodPost, "/api/ingestions", e.body(e.appA, e.clientA.Identifier, "ok-"+unique())).code)
}

// The client may be named by its id or its identifier, either way resolving
// to the same client; an unknown client is a 404 after the permission check.
func TestClientReference(t *testing.T) {
	e := newEnv(t)
	byID := e.do(http.MethodPost, "/api/ingestions", e.body(e.appA, string(e.clientA.ID), "by-id-"+unique()))
	require.Equal(t, http.StatusCreated, byID.code, byID.raw)
	byName := e.do(http.MethodPost, "/api/ingestions", e.body(e.appA, strings.ToUpper(e.clientA.Identifier), "by-name-"+unique()))
	require.Equal(t, http.StatusCreated, byName.code, byName.raw)
	assert.Equal(t, string(e.clientA.ID), byID.str("clientId"))
	assert.Equal(t, string(e.clientA.ID), byName.str("clientId"))
	assert.Equal(t, e.clientA.Identifier, byName.str("clientIdentifier"))

	for _, ref := range []string{string(e.clientA.ID), e.clientA.Identifier} {
		r := e.do(http.MethodGet, "/api/ingestions?clientId="+ref, nil)
		require.Equal(t, http.StatusOK, r.code, r.raw)
		assert.Len(t, r.json["ingestions"], 2, "filter by %s", ref)
	}

	unknown := e.do(http.MethodPost, "/api/ingestions", e.body(e.appA, "no-such-client", "x-"+unique()))
	assert.Equal(t, http.StatusNotFound, unknown.code, unknown.raw)
	assert.Equal(t, "CLIENT_NOT_FOUND", unknown.errCode())
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, "/api/ingestions?clientId=no-such-client", nil).code)

	// Resolved only after the permission check: without it an unknown and a
	// real client look the same.
	e.as(anchor())
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodPost, "/api/ingestions", e.body(e.appA, "no-such-client", "y")).code)
}

func TestCreateValidationAndConflicts(t *testing.T) {
	e := newEnv(t)
	code := "dup-" + unique()
	require.Equal(t, http.StatusCreated, e.do(http.MethodPost, "/api/ingestions", e.body(e.appA, e.clientA.Identifier, code)).code)

	dup := e.do(http.MethodPost, "/api/ingestions", e.body(e.appA, e.clientA.Identifier, code))
	assert.Equal(t, http.StatusConflict, dup.code, dup.raw)
	assert.Equal(t, "INGESTION_CODE_EXISTS", dup.errCode())
	// The same code under another client is another ingestion.
	assert.Equal(t, http.StatusCreated, e.do(http.MethodPost, "/api/ingestions", e.body(e.appA, e.clientB.Identifier, code)).code)

	bad := e.body(e.appA, e.clientA.Identifier, "Not Valid")
	r := e.do(http.MethodPost, "/api/ingestions", bad)
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
	assert.Equal(t, "INGESTION_INVALID", r.errCode())

	badTarget := e.body(e.appA, e.clientA.Identifier, "bad-target-"+unique())
	badTarget["target"].(map[string]any)["pool"] = "no-such-pool"
	r = e.do(http.MethodPost, "/api/ingestions", badTarget)
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
	assert.Equal(t, "INGESTION_TARGET_INVALID", r.errCode())

	notIntegration := application.New("plain-"+unique(), "Plain")
	persist(t, e.uow, func(ctx context.Context, tx *usecasepgx.DbTx) error {
		return application.NewRepository(e.pool).Persist(ctx, notIntegration, tx)
	})
	r = e.do(http.MethodPost, "/api/ingestions", e.body(notIntegration, e.clientA.Identifier, "p-"+unique()))
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
	assert.Equal(t, "INGESTION_APPLICATION_INVALID", r.errCode())

	notLive := e.body(e.appA, e.clientA.Identifier, "draft-"+unique())
	notLive["live"] = false
	r = e.do(http.MethodPost, "/api/ingestions", notLive)
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	assert.Equal(t, 0, r.num("version"), "created with live false: nothing runs yet")
	assert.Equal(t, 1, r.num("latestVersion"))

	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, "/api/ingestions/ing_nosuchid00000", nil).code)
}

// An update that changes nothing answers 200 and writes nothing: no version,
// no event, no audit row. A real change saves and promotes a new version.
func TestUpdate(t *testing.T) {
	e := newEnv(t)
	code := "upd-" + unique()
	id := e.create(code)
	baseline := e.do(http.MethodGet, "/api/ingestions/"+id, nil)
	require.Equal(t, 1, baseline.num("version"))
	auditBefore, eventsBefore := e.auditRows(id), e.eventRows(id)
	require.Equal(t, 1, auditBefore)
	require.Equal(t, 1, eventsBefore)

	same := e.definitionOf(code, "q-"+code)
	same["name"] = "Orders " + code
	same["expectedVersion"] = 1
	r := e.do(http.MethodPut, "/api/ingestions/"+id, same)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, 1, r.num("version"))
	assert.Equal(t, 1, r.num("latestVersion"))
	assert.Equal(t, auditBefore, e.auditRows(id), "a no-op update writes no audit row")
	assert.Equal(t, eventsBefore, e.eventRows(id), "a no-op update emits no event")
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_versions WHERE ingestion_id = $1`, id))

	changed := e.definitionOf(code, "q-changed")
	changed["expectedVersion"] = 1
	r = e.do(http.MethodPut, "/api/ingestions/"+id, changed)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, 2, r.num("version"))
	assert.Equal(t, 2, r.num("latestVersion"))
	assert.Equal(t, auditBefore+1, e.auditRows(id))
	assert.Equal(t, eventsBefore+1, e.eventRows(id))
	assert.Equal(t, 2, e.count(`SELECT count(*) FROM ing_versions WHERE ingestion_id = $1`, id))

	// The caller still holding version 1 loses, and nothing is written.
	stale := e.definitionOf(code, "q-stale")
	stale["expectedVersion"] = 1
	r = e.do(http.MethodPut, "/api/ingestions/"+id, stale)
	assert.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Equal(t, "INGESTION_VERSION_CONFLICT", r.errCode())
	assert.Equal(t, auditBefore+1, e.auditRows(id))

	// A definition that does not parse is refused whole.
	broken := e.definitionOf(code, "q-x")
	broken["expectedVersion"] = 2
	broken["source"] = map[string]any{"kind": "sqs", "region": "eu-west-1", "queue": "q", "bogus": 1}
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPut, "/api/ingestions/"+id, broken).code)
}

func TestVersionsPauseAndDelete(t *testing.T) {
	e := newEnv(t)
	code := "ver-" + unique()
	id := e.create(code)

	saved := e.definitionOf(code, "q-two")
	saved["note"] = "try the second queue"
	r := e.do(http.MethodPost, "/api/ingestions/"+id+"/versions", saved)
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	assert.Equal(t, 2, r.num("version"))
	assert.Equal(t, false, r.json["live"])
	assert.Equal(t, 1, e.do(http.MethodGet, "/api/ingestions/"+id, nil).num("version"), "a saved version is not run until it is live")

	list := e.do(http.MethodGet, "/api/ingestions/"+id+"/versions", nil)
	require.Equal(t, http.StatusOK, list.code, list.raw)
	versions := list.json["versions"].([]any)
	require.Len(t, versions, 2)
	assert.Equal(t, float64(2), versions[0].(map[string]any)["version"], "newest first")
	assert.Equal(t, false, versions[0].(map[string]any)["live"])
	assert.Equal(t, true, versions[1].(map[string]any)["live"])

	r = e.do(http.MethodPost, "/api/ingestions/"+id+"/versions/2/live", nil)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, 2, r.num("version"))
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPost, "/api/ingestions/"+id+"/versions/9/live", nil).code)
	assert.Equal(t, 1, e.do(http.MethodPost, "/api/ingestions/"+id+"/versions/1/live", nil).num("version"), "the pointer moves back")

	r = e.do(http.MethodPost, "/api/ingestions/"+id+"/pause", nil)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, "PAUSED", r.str("status"))
	assert.Equal(t, 1, r.num("version"), "pausing changes no version")
	assert.Equal(t, "ACTIVE", e.do(http.MethodPost, "/api/ingestions/"+id+"/resume", nil).str("status"))

	assert.Equal(t, http.StatusNoContent, e.do(http.MethodDelete, "/api/ingestions/"+id, nil).code)
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, "/api/ingestions/"+id, nil).code)
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_versions WHERE ingestion_id = $1`, id))
}

// A set owns the ingestions it applied: their definitions are read-only until
// it is released. Pausing is operational and still allowed.
func TestSetOwnedIsReadOnly(t *testing.T) {
	e := newEnv(t)
	code := "owned-" + unique()
	id := e.create(code)
	setID := tsid.Generate(tsid.IngestionSet)
	now := time.Now().UTC()
	_, err := e.pool.Exec(context.Background(), `INSERT INTO ing_sets (id, name, created_at, updated_at) VALUES ($1, $2, $3, $3)`, setID, "set-"+unique(), now)
	require.NoError(t, err)
	_, err = e.pool.Exec(context.Background(), `UPDATE ing_ingestions SET managed_by = $2 WHERE id = $1`, id, setID)
	require.NoError(t, err)

	upd := e.definitionOf(code, "q-new")
	upd["expectedVersion"] = 1
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPut, "/api/ingestions/" + id, upd},
		{http.MethodPost, "/api/ingestions/" + id + "/versions", e.definitionOf(code, "q-v")},
		{http.MethodPost, "/api/ingestions/" + id + "/versions/1/live", nil},
		{http.MethodDelete, "/api/ingestions/" + id, nil},
	} {
		r := e.do(c.method, c.path, c.body)
		assert.Equal(t, http.StatusConflict, r.code, "%s %s: %s", c.method, c.path, r.raw)
		assert.Equal(t, "INGESTION_SET_OWNED", r.errCode())
	}
	assert.Equal(t, "PAUSED", e.do(http.MethodPost, "/api/ingestions/"+id+"/pause", nil).str("status"))
}

// A source kind may declare a permission that creating or changing an
// ingestion of it needs: camel-route always (platform:ingestion:ingestion:route),
// and whatever a live runtime reports for its own kinds.
func TestKindRequiredPermission(t *testing.T) {
	e := newEnv(t)
	custom := func(code, kind string) map[string]any {
		b := e.body(e.appA, e.clientA.Identifier, code)
		b["source"] = map[string]any{"kind": "custom", "type": kind, "settings": map[string]any{"yaml": "- from: direct:x"}}
		return b
	}

	// camel-route needs route even when no runtime reports it.
	code := "camel-" + unique()
	e.as(anchor(ingestionPerms...))
	r := e.do(http.MethodPost, "/api/ingestions", custom(code, "camel-route"))
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_KIND_PERMISSION_REQUIRED", r.errCode())
	e.as(anchor(append([]string{permRoute}, ingestionPerms...)...))
	r = e.do(http.MethodPost, "/api/ingestions", custom(code, "camel-route"))
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	routeID := r.str("id")

	// Changing it needs route too; so does turning an ingestion into one.
	e.as(anchor(ingestionPerms...))
	upd := custom(code, "camel-route")
	delete(upd, "application")
	delete(upd, "client")
	delete(upd, "code")
	upd["expectedVersion"] = 1
	upd["source"] = map[string]any{"kind": "custom", "type": "camel-route", "settings": map[string]any{"yaml": "- from: direct:y"}}
	r = e.do(http.MethodPut, "/api/ingestions/"+routeID, upd)
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	plain := e.create("plain-" + unique())
	toRoute := e.definitionOf("c", "q")
	toRoute["source"] = upd["source"]
	toRoute["expectedVersion"] = 1
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodPut, "/api/ingestions/"+plain, toRoute).code)
	// Removing one needs only delete.
	assert.Equal(t, http.StatusNoContent, e.do(http.MethodDelete, "/api/ingestions/"+routeID, nil).code)

	// A kind a live runtime reports with its own requirement.
	kind := "mix-" + unique()
	const need = "platform:ingestion:ingestion:mix"
	runtime := func(seen time.Time) {
		reported, _ := json.Marshal([]map[string]any{{"kind": kind, "description": "A telematics feed", "requiredPermission": need}})
		_, err := e.pool.Exec(context.Background(),
			`INSERT INTO ing_runtimes (runtime_id, kinds, contract_version, first_seen_at, last_seen_at)
			 VALUES ($1, $2, 1, $3, $3) ON CONFLICT (runtime_id) DO UPDATE SET kinds = $2, last_seen_at = $3`,
			"rt-"+kind, reported, seen)
		require.NoError(t, err)
	}
	runtime(time.Now().UTC())
	e.as(anchor(ingestionPerms...))
	r = e.do(http.MethodPost, "/api/ingestions", custom("mix-a-"+unique(), kind))
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	e.as(anchor(append([]string{need}, ingestionPerms...)...))
	assert.Equal(t, http.StatusCreated, e.do(http.MethodPost, "/api/ingestions", custom("mix-b-"+unique(), kind)).code)

	listed := e.do(http.MethodGet, "/api/ingestion-source-kinds", nil)
	require.Equal(t, http.StatusOK, listed.code, listed.raw)
	var found map[string]any
	builtIn := 0
	for _, k := range listed.json["kinds"].([]any) {
		km := k.(map[string]any)
		if km["kind"] == kind {
			found = km
		}
		if km["builtIn"] == true {
			builtIn++
		}
	}
	require.NotNil(t, found, "a kind a live runtime reports is listed")
	assert.Equal(t, need, found["requiredPermission"])
	assert.Equal(t, "A telematics feed", found["description"])
	assert.Equal(t, 7, builtIn)

	// A runtime that has not polled for a while no longer reports it.
	runtime(time.Now().UTC().Add(-time.Hour))
	e.as(anchor(ingestionPerms...))
	listed = e.do(http.MethodGet, "/api/ingestion-source-kinds", nil)
	for _, k := range listed.json["kinds"].([]any) {
		assert.NotEqual(t, kind, k.(map[string]any)["kind"])
	}
}

func TestDefinitionHistoryAndFailures(t *testing.T) {
	e := newEnv(t)
	code := "read-" + unique()
	id := e.create(code)
	upd := e.definitionOf(code, "q-two")
	upd["expectedVersion"] = 1
	require.Equal(t, http.StatusOK, e.do(http.MethodPut, "/api/ingestions/"+id, upd).code)

	rec := e.api.Get("/api/ingestions/" + id + "/definition")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "application/yaml", rec.Header().Get("Content-Type"))
	files, err := ingestion.ParseFiles(rec.Body.Bytes())
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, e.appA.Code, files[0].Application)
	assert.Equal(t, e.clientA.Identifier, files[0].Client, "a file names its client by identifier")
	assert.Equal(t, code, files[0].Code)
	assert.Equal(t, ingestion.Sqs{Region: "eu-west-1", Queue: "q-two"}, files[0].Source)

	h := e.do(http.MethodGet, "/api/ingestions/"+id+"/history", nil)
	require.Equal(t, http.StatusOK, h.code, h.raw)
	entries := h.json["entries"].([]any)
	require.Len(t, entries, 2)
	assert.Equal(t, "UpdateCommand", entries[0].(map[string]any)["operation"], "newest first")
	assert.Equal(t, "CreateCommand", entries[1].(map[string]any)["operation"])
	paged := e.do(http.MethodGet, "/api/ingestions/"+id+"/history?limit=1", nil)
	assert.Len(t, paged.json["entries"], 1)
	assert.Equal(t, true, paged.json["hasMore"])

	for n := 1; n <= 3; n++ {
		at := time.Now().UTC().Add(time.Duration(n) * time.Second)
		_, err := e.pool.Exec(context.Background(),
			`INSERT INTO ing_failures (id, ingestion_id, kind, message, after_records, occurred_at, created_at) VALUES ($1, $2, 'SOURCE', $3, $4, $5, $5)`,
			tsid.Generate(tsid.IngestionFailure), id, fmt.Sprintf("failure %d", n), n, at)
		require.NoError(t, err)
		time.Sleep(2 * time.Millisecond) // TSIDs order failures; keep the ids apart
	}
	first := e.do(http.MethodGet, "/api/ingestions/"+id+"/failures?limit=2", nil)
	require.Equal(t, http.StatusOK, first.code, first.raw)
	failures := first.json["failures"].([]any)
	require.Len(t, failures, 2)
	assert.Equal(t, "failure 3", failures[0].(map[string]any)["message"], "newest first")
	assert.Equal(t, true, first.json["hasMore"])
	next := first.json["nextCursor"].(string)
	last := e.do(http.MethodGet, "/api/ingestions/"+id+"/failures?limit=2&after="+next, nil)
	require.Len(t, last.json["failures"], 1)
	assert.Equal(t, "failure 1", last.json["failures"].([]any)[0].(map[string]any)["message"])
	assert.Equal(t, false, last.json["hasMore"])
}
