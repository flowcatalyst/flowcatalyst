//go:build integration

package runtimeapi_test

import (
	"context"
	"encoding/json"
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
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	ingestionapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/runtimeapi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// TestMain sets FLOWCATALYST_APP_KEY before the shared fixture boots: the
// ingestion resolver's service-account repository needs encryption.
func TestMain(m *testing.M) {
	key, err := encryption.GenerateKey()
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("FLOWCATALYST_APP_KEY", key)
	testpg.RunMain(m)
}

const (
	permRuntime = "platform:ingestion:runtime:operate"
	permTest    = "platform:ingestion:ingestion:test"
	permView    = "platform:ingestion:ingestion:view"
	base        = "/api/ingestion-runtime/v1"
)

// goodTarget is a target of the application app: its job code names it, as
// a target's must.
func goodTarget(app string) string {
	return `{"code":"` + app + `:orders:order:received","targetUrl":"https://hooks.example.com/orders","pool":"orders","mode":"IMMEDIATE","batchSize":100}`
}

func (e *env) definitionJSON(source string) string {
	return `{"name":"Orders","source":` + source + `,"split":{"kind":"none"},"target":` + goodTarget(e.app.Code) + `}`
}

func unique() string { return strings.ToLower(tsid.GenerateUntyped()) }

type env struct {
	t      *testing.T
	pool   *pgxpool.Pool
	repo   *ingestion.Repository
	api    humatest.TestAPI
	who    *auth.AuthContext
	now    time.Time
	app    *application.Application
	client *client.Client
	config *platformconfig.Service
}

// newEnv is one integration application and one client behind the real
// routes on a real database. The principal is a client-tier runtime service
// account, so its polls take only this test's ingestions.
func newEnv(t *testing.T) *env {
	t.Helper()
	httpcompat.Init()
	e := &env{t: t, pool: testpg.Pool(t), now: time.Now().UTC()}
	e.repo = ingestion.NewRepository(e.pool)
	e.app = application.NewIntegration("ingr-"+unique(), "Ingest runtime")
	e.client = client.New("Acme", "acme-"+unique())
	e.who = e.principal(permRuntime, permTest, permView)
	uow := testpg.NewUoW(t)
	apps, clients := application.NewRepository(e.pool), client.NewRepository(e.pool)
	e.persist(uow, func(ctx context.Context, tx *usecasepgx.DbTx) error {
		if err := apps.Persist(ctx, e.app, tx); err != nil {
			return err
		}
		return clients.Persist(ctx, e.client, tx)
	})

	_, api := humatest.New(t, apiroute.PlatformAPIConfig("test", "dev"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, auth.WithContext(ctx.Context(), e.who)))
	})
	resolver := ingestion.NewResolver(apps, clients, dispatchpool.NewRepository(e.pool),
		connection.NewRepository(e.pool), serviceaccount.NewRepository(e.pool), nil)
	enc, err := encryption.New(os.Getenv("FLOWCATALYST_APP_KEY"))
	require.NoError(t, err)
	e.config = &platformconfig.Service{
		Repo: platformconfig.NewRepository(e.pool).WithEncryption(enc), UoW: uow, Apps: apps, Clients: clients,
		Audit: audit.NewRepository(e.pool),
	}
	runtimeapi.Register(api, &runtimeapi.State{
		Repo: e.repo, Jobs: dispatchjob.NewRepository(e.pool), Resolver: resolver,
		Connections: connection.NewRepository(e.pool), Config: e.config, Audit: audit.NewRepository(e.pool), Now: func() time.Time { return e.now },
	})
	ingestionapi.Register(api, &ingestionapi.State{
		Repo:     e.repo,
		Resolver: resolver,
		Apps:     apps, Clients: clients, Audit: audit.NewRepository(e.pool), UoW: uow,
		Configs: e.config.Repo,
	})
	// As the server does, after every route has registered.
	httpcompat.RelaxRequestBodies(api)
	httpcompat.RelaxRequestBodiesDeep(api, runtimeapi.PollOperationID, runtimeapi.ReportOperationID, runtimeapi.TracesOperationID)
	e.api = api
	return e
}

// principal is a runtime's service account as the runtime API requires one
// (anchor tier, bound to no application), also holding whatever operator
// permissions a test needs for the operator routes it calls.
func (e *env) principal(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{
		PrincipalID: "prn_ingrtester001", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: perms,
		PrincipalType: auth.PrincipalTypeService,
	}
}

// clientTier narrows a principal to one client, as a person or an operator
// account below anchor tier is.
func clientTier(a *auth.AuthContext, client string) *auth.AuthContext {
	a.Scope, a.Clients = auth.ScopeClient, []string{client}
	return a
}

func (e *env) persist(uow *usecasepgx.UnitOfWork, fn func(context.Context, *usecasepgx.DbTx) error) {
	e.t.Helper()
	ctx := context.Background()
	tx, err := uow.Pool().Begin(ctx)
	require.NoError(e.t, err)
	if err := fn(ctx, usecasepgx.WrapTxForBootstrap(tx)); err != nil {
		_ = tx.Rollback(ctx)
		require.NoError(e.t, err)
	}
	require.NoError(e.t, tx.Commit(ctx))
}

// save stores an ingestion of the source JSON and returns it.
func (e *env) save(code, source string, live bool) *ingestion.Ingestion {
	e.t.Helper()
	def, err := ingestion.ParseDefinition([]byte(e.definitionJSON(source)))
	require.NoError(e.t, err)
	i := ingestion.New(e.app.ID, e.client.ID, code, def, live, nil, nil)
	e.persist(testpg.NewUoW(e.t), func(ctx context.Context, tx *usecasepgx.DbTx) error { return e.repo.Persist(ctx, i, tx) })
	return i
}

const (
	sftpSource = `{"kind":"sftp","host":"sftp.example.com","port":22,"directory":"/in","username":"feed","credential":{"name":"sftp-password"},"every":"PT5M"}`
	sqsSource  = `{"kind":"sqs","region":"eu-west-1","queue":"orders"}`
)

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

// expiresIn is the seconds from createdAt to expiresAt of a test response.
func (r response) expiresIn(t *testing.T) float64 {
	t.Helper()
	created, err := time.Parse(time.RFC3339Nano, r.str("createdAt"))
	require.NoError(t, err)
	expires, err := time.Parse(time.RFC3339Nano, r.str("expiresAt"))
	require.NoError(t, err)
	return expires.Sub(created).Seconds()
}

func (r response) errCode() string { s, _ := r.json["error"].(string); return s }

func (r response) str(key string) string { s, _ := r.json[key].(string); return s }

// list reads a JSON array field as objects.
func (r response) list(key string) []map[string]any {
	var out []map[string]any
	arr, _ := r.json[key].([]any)
	for _, x := range arr {
		m, _ := x.(map[string]any)
		out = append(out, m)
	}
	return out
}

func (e *env) poll(runtime string, version int, kinds ...map[string]any) response {
	e.t.Helper()
	body := map[string]any{"runtimeId": runtime, "contractVersion": version}
	if len(kinds) > 0 {
		body["kinds"] = kinds
	}
	r := e.do(http.MethodPost, base+"/poll", body)
	// The runtime role reaches every ingestion on the platform, and the tests share
	// one database: a test sees only the ingestions of its own application.
	if arr, ok := r.json["ingestions"].([]any); ok {
		own := []any{}
		for _, x := range arr {
			if m, _ := x.(map[string]any); m != nil && m["applicationCode"] == e.app.Code {
				own = append(own, x)
			}
		}
		r.json["ingestions"] = own
	}
	return r
}

func (e *env) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	require.NoError(e.t, e.pool.QueryRow(context.Background(), query, args...).Scan(&n))
	return n
}

// The platform serves the current contract version and the one before it; a
// runtime of either gets the same answer, in the contract's form.
func TestPoll_ServesCurrentAndPreviousContract(t *testing.T) {
	e := newEnv(t)
	i := e.save("orders", sqsSource, true)

	for _, v := range []int{ingestion.ContractVersion, ingestion.MinContractVersion} {
		r := e.poll("rt-"+unique(), v)
		require.Equal(t, http.StatusOK, r.code, "version %d: %s", v, r.raw)
		assert.Equal(t, "PT3M", r.str("leaseFor"))
		assert.NotEmpty(t, r.str("asOf"))
		got := r.list("ingestions")
		require.Len(t, got, 1)
		assert.Equal(t, i.ID, got[0]["id"])
		assert.EqualValues(t, 1, got[0]["version"])
		assert.Equal(t, string(e.client.ID), got[0]["clientId"])
		assert.Equal(t, e.client.Identifier, got[0]["clientIdentifier"])
		assert.Equal(t, e.app.Code, got[0]["applicationCode"])
		assert.Equal(t, "orders", got[0]["code"])
		assert.Equal(t, map[string]any{"kind": "sqs", "region": "eu-west-1", "queue": "orders"}, got[0]["source"])
		target, _ := got[0]["target"].(map[string]any)
		assert.Equal(t, "https://hooks.example.com/orders", target["targetUrl"])
		assert.Equal(t, "orders", target["pool"])
		assert.Equal(t, "IMMEDIATE", target["mode"])
		assert.EqualValues(t, 100, target["batchSize"])
		assert.Empty(t, r.list("tests"))
	}
	for _, v := range []int{ingestion.ContractVersion + 1, ingestion.MinContractVersion - 1} {
		r := e.poll("rt-"+unique(), v)
		assert.Equal(t, http.StatusUpgradeRequired, r.code, "version %d: %s", v, r.raw)
	}
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_runtimes WHERE runtime_id = 'rt-never'`))
}

// A runtime receives the live version only: a newer saved version waits until
// it is made live.
func TestPoll_LiveVersionOnly(t *testing.T) {
	e := newEnv(t)
	i := e.save("versions", sqsSource, true)
	next, err := ingestion.ParseDefinition([]byte(e.definitionJSON(`{"kind":"sqs","region":"eu-west-1","queue":"orders-v2"}`)))
	require.NoError(t, err)
	i, err = e.repo.FindByID(context.Background(), i.ID)
	require.NoError(t, err)
	i.SaveVersion(next, false, nil, nil)
	e.persist(testpg.NewUoW(t), func(ctx context.Context, tx *usecasepgx.DbTx) error { return e.repo.Persist(ctx, i, tx) })

	got := e.poll("rt-"+unique(), ingestion.ContractVersion).list("ingestions")
	require.Len(t, got, 1)
	assert.EqualValues(t, 1, got[0]["version"])
	assert.Equal(t, "orders", got[0]["source"].(map[string]any)["queue"])
}

// State belongs to the lease holder: a write from any other runtime is a 409,
// a value is limited to 4000 characters, and a missing key is a 404.
func TestState_Routes(t *testing.T) {
	e := newEnv(t)
	i := e.save("polled", sftpSource, true)
	holder, other := "rt-h-"+unique(), "rt-o-"+unique()
	require.Len(t, e.poll(holder, ingestion.ContractVersion).list("ingestions"), 1)

	path := func(key, runtime string) string {
		return base + "/state/" + i.ID + "/" + key + "?runtimeId=" + runtime
	}
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, path("watermark", holder), nil).code)
	assert.Equal(t, http.StatusNoContent, e.do(http.MethodPut, path("watermark", holder), map[string]any{"value": "2030-01-01"}).code)
	r := e.do(http.MethodGet, path("watermark", holder), nil)
	assert.Equal(t, http.StatusOK, r.code)
	assert.Equal(t, "2030-01-01", r.str("value"))

	r = e.do(http.MethodPut, path("watermark", other), map[string]any{"value": "stolen"})
	assert.Equal(t, http.StatusConflict, r.code)
	assert.Equal(t, "LEASE_NOT_HELD", r.errCode())
	assert.Equal(t, "LEASE_NOT_HELD", e.do(http.MethodGet, path("watermark", other), nil).errCode())
	assert.Equal(t, "2030-01-01", e.do(http.MethodGet, path("watermark", holder), nil).str("value"), "the refused write changed nothing")

	assert.Equal(t, http.StatusNoContent, e.do(http.MethodPut, path("long", holder), map[string]any{"value": strings.Repeat("é", 4000)}).code,
		"4000 characters, however many bytes")
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPut, path("long", holder), map[string]any{"value": strings.Repeat("a", 4001)}).code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPut, path("bad key!", holder), map[string]any{"value": "x"}).code)
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPut, base+"/state/ing_nothing0000/k?runtimeId="+holder, map[string]any{"value": "x"}).code)

	e.now = e.now.Add(4 * time.Minute) // past the 3-minute lease the holder never renewed
	assert.Equal(t, "LEASE_NOT_HELD", e.do(http.MethodPut, path("watermark", holder), map[string]any{"value": "late"}).errCode())
}

// The report route records for a holder and ignores a runtime that lost its lease.
func TestReport_Route(t *testing.T) {
	e := newEnv(t)
	i := e.save("polled", sftpSource, true)
	holder, other := "rt-h-"+unique(), "rt-o-"+unique()
	require.Len(t, e.poll(holder, ingestion.ContractVersion).list("ingestions"), 1)

	report := func(runtime string) map[string]any {
		return map[string]any{
			"runtimeId": runtime,
			"routes":    []map[string]any{{"ingestionId": i.ID, "version": 1, "status": "STARTED"}},
			"metrics": []map[string]any{{
				"ingestionId": i.ID, "from": "2030-01-01T11:59:00Z", "to": "2030-01-01T12:00:00Z",
				"completed": 3, "failed": 0, "records": 30, "dispatchJobs": 30,
			}},
			"failures": []map[string]any{{"ingestionId": i.ID, "version": 1, "at": "2030-01-01T12:00:00Z", "kind": "SOURCE", "message": "timeout"}},
		}
	}
	assert.Equal(t, http.StatusNoContent, e.do(http.MethodPost, base+"/report", report(other)).code, "ignored, not refused")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_route_states WHERE ingestion_id = $1`, i.ID))
	assert.Equal(t, http.StatusNoContent, e.do(http.MethodPost, base+"/report", report(holder)).code)
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_route_states WHERE ingestion_id = $1 AND runtime_id = $2`, i.ID, holder))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_exchange_metrics WHERE ingestion_id = $1 AND runtime_id = $2 AND completed = 3`, i.ID, holder))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_failures WHERE ingestion_id = $1`, i.ID))

	bad := report(holder)
	bad["metrics"] = []map[string]any{{
		"ingestionId": i.ID, "from": "2030-01-01T12:00:00Z", "to": "2030-01-01T11:00:00Z",
		"completed": 1, "failed": 0, "records": 1, "dispatchJobs": 1,
	}}
	assert.Equal(t, "REPORT_WINDOW_INVALID", e.do(http.MethodPost, base+"/report", bad).errCode())
	bad = report(holder)
	bad["routes"] = []map[string]any{{"ingestionId": i.ID, "version": 1, "status": "RUNNING"}}
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, base+"/report", bad).code, "a status the contract does not name")
	bad = report(holder)
	bad["failures"] = []map[string]any{{"ingestionId": i.ID, "version": 1, "at": "2030-01-01T12:00:00Z", "kind": "SOURCE", "message": strings.Repeat("x", 2001)}}
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, base+"/report", bad).code, "a message over 2000 characters")
}

// Test connection end to end: queued by the operator, taken by a runtime that
// carries the kind at its next poll, answered through its report, read back.
func TestIngestionTest_Lifecycle(t *testing.T) {
	e := newEnv(t)
	kind := "mix" + unique()
	i := e.save("custom", `{"kind":"custom","type":"`+kind+`","settings":{}}`, true)

	r := e.do(http.MethodPost, "/api/ingestion-tests", map[string]any{"ingestionId": i.ID})
	require.Equal(t, http.StatusAccepted, r.code, r.raw)
	id := r.str("id")
	assert.Equal(t, "PENDING", r.str("status"))
	assert.Equal(t, kind, r.str("kind"))
	assert.Equal(t, "no runtime carries this kind", r.str("notice"), "waiting for a runtime is not a failure")

	rt := "rt-" + unique()
	polled := e.poll(rt, ingestion.ContractVersion, map[string]any{"kind": kind})
	require.Equal(t, http.StatusOK, polled.code, polled.raw)
	tests := polled.list("tests")
	require.Len(t, tests, 1)
	assert.Equal(t, id, tests[0]["id"])
	assert.Equal(t, i.ID, tests[0]["ingestion"].(map[string]any)["id"])
	assert.Equal(t, "custom", tests[0]["ingestion"].(map[string]any)["code"])
	assert.Equal(t, "RUNNING", e.do(http.MethodGet, "/api/ingestion-tests/"+id, nil).str("status"))
	assert.Empty(t, e.poll("rt-"+unique(), ingestion.ContractVersion, map[string]any{"kind": kind}).list("tests"), "taken once")

	answer := map[string]any{"runtimeId": rt, "tests": []map[string]any{{
		"id": id, "outcome": "NO_RESULTS", "steps": []string{"logged in", "asked once"}, "records": 0, "excerpt": "{}",
	}}}
	assert.Equal(t, http.StatusNoContent, e.do(http.MethodPost, base+"/report", answer).code)
	r = e.do(http.MethodGet, "/api/ingestion-tests/"+id, nil)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, "DONE", r.str("status"))
	assert.Equal(t, "NO_RESULTS", r.str("outcome"))
	assert.Equal(t, []any{"logged in", "asked once"}, r.json["steps"])
	assert.NotContains(t, r.json, "excerpt", "the ingestion does not keep payloads")
	assert.Equal(t, rt, r.str("runtimeId"))

	// A definition that is not saved yet is tried the same way, as ing_test.
	unsaved := e.do(http.MethodPost, "/api/ingestion-tests", map[string]any{
		"application": e.app.Code, "client": e.client.Identifier,
		"definition": json.RawMessage(e.definitionJSON(`{"kind":"custom","type":"` + kind + `","settings":{}}`)),
	})
	require.Equal(t, http.StatusAccepted, unsaved.code, unsaved.raw)
	tests = e.poll(rt, ingestion.ContractVersion, map[string]any{"kind": kind}).list("tests")
	require.Len(t, tests, 1)
	assert.Equal(t, "ing_test", tests[0]["ingestion"].(map[string]any)["id"])
	assert.Equal(t, e.app.Code, tests[0]["ingestion"].(map[string]any)["applicationCode"])

	// Unanswered, a request expires after two minutes (the clock of the
	// operator's routes is the real one, so the expiry is moved back).
	assert.InDelta(t, ingestion.TestRequestTTL.Seconds(), unsaved.expiresIn(t), 5)
	_, err := e.pool.Exec(context.Background(), `UPDATE ing_test_requests SET expires_at = now() - interval '1 second' WHERE id = $1`, unsaved.str("id"))
	require.NoError(t, err)
	assert.Equal(t, "EXPIRED", e.do(http.MethodGet, "/api/ingestion-tests/"+unsaved.str("id"), nil).str("status"))
}

// The test routes need the test grant, and reach is by client and application.
func TestIngestionTest_PermissionAndReach(t *testing.T) {
	e := newEnv(t)
	i := e.save("sqs", sqsSource, true)
	r := e.do(http.MethodPost, "/api/ingestion-tests", map[string]any{"ingestionId": i.ID})
	require.Equal(t, http.StatusAccepted, r.code, r.raw)
	id := r.str("id")

	e.who = e.principal(permView, permRuntime)
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodPost, "/api/ingestion-tests", map[string]any{"ingestionId": i.ID}).code)
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodGet, "/api/ingestion-tests/"+id, nil).code)

	e.who = clientTier(e.principal(permTest), "clt_someoneelse0")
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodPost, "/api/ingestion-tests", map[string]any{"ingestionId": i.ID}).code, "another client's ingestion")
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodGet, "/api/ingestion-tests/"+id, nil).code)

	e.who = e.principal(permTest)
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, "/api/ingestion-tests/ingt_nothing000", nil).code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/api/ingestion-tests", map[string]any{}).code, "names nothing to test")
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/api/ingestion-tests",
		map[string]any{"ingestionId": i.ID, "application": e.app.Code}).code, "names both")
}

// The runtime role is platform-wide: a runtime's account (anchor tier, bound to
// no application) runs every client's and every application's ingestions. An
// account that is narrower (client tier, or an application's) is not a
// runtime's and is refused (owner ruling R1), never confined to its reach.
func TestPoll_ReachIsPlatformWide(t *testing.T) {
	e := newEnv(t)
	i := e.save("mine", sqsSource, true)
	e.who = e.principal(permRuntime)
	got := e.poll("rt-"+unique(), ingestion.ContractVersion).list("ingestions")
	require.Len(t, got, 1)
	assert.Equal(t, i.ID, got[0]["id"])

	e.who = clientTier(e.principal(permRuntime), string(e.client.ID))
	r := e.poll("rt-"+unique(), ingestion.ContractVersion)
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_RUNTIME_ANCHOR_REQUIRED", r.errCode())

	e.who = e.principal(permRuntime)
	e.who.AllApplications, e.who.Applications = false, []string{string(e.app.ID)}
	r = e.poll("rt-"+unique(), ingestion.ContractVersion)
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_RUNTIME_APPLICATION_BOUND", r.errCode(), "even bound to the very application whose ingestions it would run")
}
