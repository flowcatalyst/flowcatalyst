//go:build integration

package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/client"
	"github.com/flowcatalyst/go-sdk/outbox"
	"github.com/flowcatalyst/go-sdk/outboxpgx"
	"github.com/flowcatalyst/go-sdk/sync"
	"github.com/flowcatalyst/go-sdk/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// goSDKPlatform serves the whole platform router (WirePlatform on embedded
// Postgres) over HTTP and mints a bearer for an anchor user holding every
// platform permission: what an application's service account looks like to
// the Go SDK, end to end through routing, auth and huma's body validation.
func goSDKPlatform(t *testing.T) (*httptest.Server, *pgxpool.Pool, string) {
	t.Helper()
	ctx := context.Background()
	pool := testpg.Pool(t)
	keyPath, err := EnsureSigningKeyFile(filepath.Join(t.TempDir(), "jwt.pem"))
	require.NoError(t, err)
	cfg := EnvCfg{
		JWTIssuer: "https://platform.example.com", JWTSigningKeyPath: keyPath,
		SessionTTLSecs: 3600, AccessTokenTTLSecs: 3600, RefreshTokenTTLSecs: 3600,
		FunctionsArtifactStore: "file://" + t.TempDir(),
	}
	r := chi.NewRouter()
	_, err = WirePlatform(r, pool, cfg, dispatch.Settings{})
	require.NoError(t, err)
	repos := buildRepos(pool)
	svcs, err := buildServices(cfg, pool, repos)
	require.NoError(t, err)

	perms := []string{"platform:*:*:*"}
	pid := ids.NewPrincipalID()
	roleName := "gosdke2e:all-" + strings.ToLower(string(pid))
	_, err = pool.Exec(ctx, `INSERT INTO iam_principals (id, type, scope, name, active, email) VALUES ($1, 'USER', 'ANCHOR', 'Go SDK e2e', TRUE, $2)`,
		pid, strings.ToLower(string(pid))+"@gosdk.example.com")
	require.NoError(t, err)
	roleID := tsid.Generate(tsid.Role)
	_, err = pool.Exec(ctx, `INSERT INTO iam_roles (id, name, display_name) VALUES ($1, $2, 'Go SDK e2e')`, roleID, roleName)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO iam_role_permissions (role_id, permission) VALUES ($1, $2)`, roleID, perms[0])
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source) VALUES ($1, $2, 'TEST')`, pid, roleName)
	require.NoError(t, err)
	p, err := repos.principalRepo.FindByID(ctx, pid)
	require.NoError(t, err)
	bearer, err := svcs.authSvc.GenerateAccessTokenWithScope(p, perms)
	require.NoError(t, err)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, pool, bearer
}

// goSDKApp creates an application with its own service account (connections
// are delivered by it) and a client, directly in the database.
func goSDKApp(t *testing.T, pool *pgxpool.Pool, appCode, clientIdentifier string) (clientID string) {
	t.Helper()
	ctx := context.Background()
	saID := "sva_" + strings.ToLower(tsid.GenerateUntyped())
	// A real service account behind the principal: the application holds the
	// principal id, a connection must end up holding the account's own id.
	realSA := "sac_" + saID[4:]
	_, err := pool.Exec(ctx, `INSERT INTO iam_service_accounts (id, code, name) VALUES ($1, $1, $2)`, realSA, appCode+" SA")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO iam_principals (id, type, name, active, service_account_id) VALUES ($1, 'SERVICE', $2, TRUE, $3)`, saID, appCode+" SA", realSA)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO app_applications (id, code, name, service_account_id) VALUES ($1, $2, $2, $3)`,
		tsid.Generate(tsid.Application), appCode, saID)
	require.NoError(t, err)
	clientID = string(ids.NewClientID())
	_, err = pool.Exec(ctx, `INSERT INTO tnt_clients (id, name, identifier) VALUES ($1, $2, $2)`, clientID, clientIdentifier)
	require.NoError(t, err)
	return clientID
}

// The Go SDK's Synchronizer against the real platform: every category is
// accepted through the whole HTTP stack (routing, bearer auth, huma body
// validation), lands where it should, and reports F13/F15 fields.
func TestGoSDKSynchronizerAgainstThePlatform(t *testing.T) {
	srv, pool, bearer := goSDKPlatform(t)
	ctx := context.Background()
	const app = "gosdke2e"
	clientIdentifier := "gosdke2e-acme"
	clientID := goSDKApp(t, pool, app, clientIdentifier)

	set := sync.ForApplication(app).ForClient(clientIdentifier).
		AddRole(sync.MakeRole("admin").WithDisplayName("Admin").WithPermissions("gosdke2e:sales:order:view")).
		AddEventType(sync.MakeEventType("gosdke2e:sales:order:placed", "Order placed")).
		AddDispatchPool(sync.MakeDispatchPool("gosdke2e-pool", "Orders").WithConcurrency(3).WithRateLimit(120)).
		AddConnection(sync.MakeConnection("gosdke2e-hooks", "Hooks").WithExternalID("ext-1")).
		AddSubscription(sync.MakeSubscription("gosdke2e-ship", "Ship", "https://orders.example.com/hooks/ship").
			WithConnectionCode("gosdke2e-hooks").WithDispatchPool("gosdke2e-pool").Bind("gosdke2e:sales:order:placed", "")).
		AddProcess(sync.MakeProcess("gosdke2e:sales:checkout", "Checkout").WithBody("graph TD; A-->B").WithTags("sales")).
		AddScheduledJob(sync.MakeScheduledJob("gosdke2e:nightly", "Nightly", "0 0 2 * * *").
			WithTargetURL("https://orders.example.com/jobs").WithTimezone("Europe/London")).
		AddPrincipal(sync.MakePrincipal("someone@gosdke2e.example.com").WithName("Someone").
			WithRoles("gosdke2e:admin", "gosdke2e:typo")).
		WithOpenAPISpec(json.RawMessage(`{"openapi":"3.1.0","info":{"title":"Orders","version":"1.0.0"},"paths":{}}`)).
		AddDoc(sync.MakeDoc("getting-started", "# Getting started\n\nHello."))

	s := sync.NewSynchronizer(client.New(srv.URL, client.WithToken(bearer), client.WithRetry(1, 0)))
	out := s.Sync(ctx, set, sync.DefaultOptions())
	require.False(t, out.HasErrors(), "%v", out.Errors())
	for name, c := range map[string]*sync.CategoryResult{
		"roles": out.Roles, "event types": out.EventTypes, "pools": out.DispatchPools, "connections": out.Connections,
		"subscriptions": out.Subscriptions, "processes": out.Processes, "scheduled jobs": out.ScheduledJobs,
		"principals": out.Principals, "openapi": out.OpenAPI, "docs": out.Docs,
	} {
		require.NotNil(t, c, name)
		assert.Positive(t, c.Created, name)
	}
	assert.Equal(t, []string{"gosdke2e:typo"}, out.Principals.SkippedRoles, "F15: an unknown role is skipped and reported")

	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM msg_connections WHERE code = 'gosdke2e-hooks' AND client_id = $1`, clientID).Scan(&n))
	assert.Equal(t, 1, n, "the connection is scoped to the client named by its identifier")
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM msg_subscriptions s JOIN msg_connections c ON c.id = s.connection_id
		  WHERE s.code = 'gosdke2e-ship' AND c.code = 'gosdke2e-hooks'`).Scan(&n))
	assert.Equal(t, 1, n, "the subscription's connectionCode resolved")
	var rateLimit *int
	require.NoError(t, pool.QueryRow(ctx, `SELECT rate_limit FROM msg_dispatch_pools WHERE code = 'gosdke2e-pool'`).Scan(&rateLimit))
	require.NotNil(t, rateLimit)
	assert.Equal(t, 120, *rateLimit)

	// A second identical run changes nothing it should not: the OpenAPI
	// document is unchanged.
	again := s.Sync(ctx, set, sync.OpenAPIOnly())
	require.False(t, again.HasErrors(), "%v", again.Errors())
	assert.Equal(t, uint32(0), again.OpenAPI.Touched())
}

// The Go SDK's outbox rows, sent exactly as the outbox processor sends them
// ({"items":[<payload>...]}), through the real batch routes with a bearer.
func TestGoSDKOutboxRowsAgainstThePlatform(t *testing.T) {
	srv, pool, bearer := goSDKPlatform(t)
	ctx := context.Background()
	clientIdentifier := "gosdke2e-outbox"
	clientID := goSDKApp(t, pool, "gosdke2eob", clientIdentifier)
	require.NoError(t, outboxpgx.InitSchema(ctx, pool))
	_, err := pool.Exec(ctx, `DELETE FROM outbox_messages`)
	require.NoError(t, err)

	ev := e2eEvent{id: tsid.GenerateUntyped()}
	sink := outboxpgx.NewSink(outboxpgx.Config{ClientCode: clientIdentifier, AuditEnabled: true, ApplicationCode: "gosdke2eob"})
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	dbtx := usecasepgx.WrapTxForBootstrap(tx)
	require.NoError(t, sink.WriteEvent(ctx, dbtx, ev))
	require.NoError(t, sink.WriteAudit(ctx, dbtx, ev, struct {
		OrderID string `json:"orderId"`
		Secret  string `json:"secret"`
	}{"ord_1", "s3cret"}))
	require.NoError(t, sink.WriteDispatchJob(ctx, tx, outbox.NewDispatchJob("gosdke2eob:billing:export", "https://billing.example.com/x").
		WithClientCode(clientIdentifier).WithCorrelationID("corr-e2e")))
	require.NoError(t, tx.Commit(ctx))

	endpoint := map[string]string{
		"EVENT": "/api/events/batch", "AUDIT_LOG": "/api/audit-logs/batch", "DISPATCH_JOB": "/api/dispatch-jobs/batch",
	}
	rows, err := pool.Query(ctx, `SELECT type, payload FROM outbox_messages`)
	require.NoError(t, err)
	type pending struct{ typ, payload string }
	var all []pending
	for rows.Next() {
		var p pending
		require.NoError(t, rows.Scan(&p.typ, &p.payload))
		all = append(all, p)
	}
	rows.Close()
	require.Len(t, all, 3)
	for _, p := range all {
		req, err := http.NewRequest(http.MethodPost, srv.URL+endpoint[p.typ], strings.NewReader(`{"items":[`+p.payload+`]}`))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+bearer)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Less(t, resp.StatusCode, 300, "%s: %d %s", p.typ, resp.StatusCode, body)
		if p.typ != "DISPATCH_JOB" {
			var res struct {
				Results []struct{ Status, Error string } `json:"results"`
			}
			require.NoError(t, json.Unmarshal(body, &res), string(body))
			require.Len(t, res.Results, 1)
			require.Equal(t, "SUCCESS", res.Results[0].Status, "%s: %s", p.typ, res.Results[0].Error)
		}
	}

	var data, evClient string
	require.NoError(t, pool.QueryRow(ctx, `SELECT data::text, client_id FROM msg_events WHERE deduplication_id = $1`, ev.id).Scan(&data, &evClient))
	assert.JSONEq(t, `{"orderId":"ord_1"}`, data)
	assert.Equal(t, clientID, evClient)
	var opJSON string
	require.NoError(t, pool.QueryRow(ctx, `SELECT operation_json::text FROM aud_logs WHERE entity_id = 'ord_1' ORDER BY performed_at DESC LIMIT 1`).Scan(&opJSON))
	assert.JSONEq(t, `{"orderId":"ord_1","secret":"***"}`, opJSON)
	var jobClient string
	require.NoError(t, pool.QueryRow(ctx, `SELECT client_id FROM msg_dispatch_jobs WHERE correlation_id = 'corr-e2e'`).Scan(&jobClient))
	assert.Equal(t, clientID, jobClient)
}

// A Go SDK use case whose plan carries dispatch jobs
// (usecaseop.Emit(event).Dispatch(...)): its outbox rows, claimed in the
// processor's order (message_group, created_at) and sent per type as one
// {"items":[...]} batch, are accepted by the real routes. The jobs keep their
// SDK ids, take the plan's correlation id, and a job naming its client by
// identifier (clientCode, F14) lands on that client while one naming none is
// unscoped.
func TestGoSDKPlanDispatchJobsAgainstThePlatform(t *testing.T) {
	srv, pool, bearer := goSDKPlatform(t)
	ctx := context.Background()
	const app = "gosdke2epl"
	clientIdentifier := "gosdke2e-plan"
	clientID := goSDKApp(t, pool, app, clientIdentifier)
	require.NoError(t, outboxpgx.InitSchema(ctx, pool))
	_, err := pool.Exec(ctx, `DELETE FROM outbox_messages`)
	require.NoError(t, err)

	corr := "corr-plan-" + strings.ToLower(tsid.GenerateUntyped())
	ec := usecase.WithCorrelation("prn_e2e", corr)
	ev := e2ePlanEvent{id: tsid.GenerateUntyped(), corr: corr}
	scopedID := tsid.GenerateUntyped()
	scoped := outbox.NewDispatchJob(app+":billing:export", "https://billing.example.com/export").
		WithID(scopedID).WithClientCode(clientIdentifier).WithEventID(ev.id).WithMessageGroup("billing:inv_1")
	unscoped, err := outbox.NewDispatchJob(app+":notify", "https://hooks.example.com/n").
		WithJSONPayload(map[string]string{"orderId": "ord_1"})
	require.NoError(t, err)

	op := usecaseop.Operation[struct{}, e2ePlanEvent]{
		Name:      "e2e-plan",
		Authorize: usecaseop.Public[struct{}],
		Execute: func(context.Context, struct{}, usecase.ExecutionContext) (usecaseop.Plan[e2ePlanEvent], error) {
			return usecaseop.Emit(ev).Dispatch(scoped, unscoped), nil
		},
	}
	sink := outboxpgx.NewSink(outboxpgx.Config{ClientCode: clientIdentifier, AuditEnabled: true, ApplicationCode: app})
	_, err = usecaseop.Run(ctx, usecasepgx.New(pool, sink), op, struct{}{}, ec)
	require.NoError(t, err)

	rows, err := pool.Query(ctx, `SELECT id, type, payload FROM outbox_messages WHERE status = 0 ORDER BY message_group, created_at`)
	require.NoError(t, err)
	byType := map[string][]string{}
	var jobIDs []string
	for rows.Next() {
		var id, typ, payload string
		require.NoError(t, rows.Scan(&id, &typ, &payload))
		byType[typ] = append(byType[typ], payload)
		if typ == "DISPATCH_JOB" {
			jobIDs = append(jobIDs, id)
		}
	}
	rows.Close()
	require.Len(t, byType["EVENT"], 1)
	require.Len(t, byType["DISPATCH_JOB"], 2)
	require.Len(t, byType["AUDIT_LOG"], 1)

	endpoint := map[string]string{
		"EVENT": "/api/events/batch", "AUDIT_LOG": "/api/audit-logs/batch", "DISPATCH_JOB": "/api/dispatch-jobs/batch",
	}
	for typ, payloads := range byType {
		req, err := http.NewRequest(http.MethodPost, srv.URL+endpoint[typ], strings.NewReader(`{"items":[`+strings.Join(payloads, ",")+`]}`))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+bearer)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Less(t, resp.StatusCode, 300, "%s: %d %s", typ, resp.StatusCode, body)
		var res struct {
			Results []struct{ ID, Status, Error string } `json:"results"`
		}
		require.NoError(t, json.Unmarshal(body, &res), string(body))
		require.Len(t, res.Results, len(payloads), typ)
		for i, r := range res.Results {
			require.Equal(t, "SUCCESS", r.Status, "%s item %d: %s", typ, i, r.Error)
			if typ == "DISPATCH_JOB" {
				assert.Equal(t, jobIDs[i], r.ID, "the job keeps the id the SDK gave it")
			}
		}
	}

	type job struct {
		id, kind, code    string
		clientID, eventID *string
	}
	jr, err := pool.Query(ctx, `SELECT id, kind, code, client_id, event_id FROM msg_dispatch_jobs WHERE correlation_id = $1`, corr)
	require.NoError(t, err)
	got := map[string]job{}
	for jr.Next() {
		var j job
		require.NoError(t, jr.Scan(&j.id, &j.kind, &j.code, &j.clientID, &j.eventID))
		got[j.code] = j
	}
	jr.Close()
	require.Len(t, got, 2, "both jobs carry the plan's correlation id")
	s := got[app+":billing:export"]
	assert.Equal(t, scopedID, s.id)
	assert.Equal(t, "TASK", s.kind)
	require.NotNil(t, s.clientID, "clientCode resolved (F14)")
	assert.Equal(t, clientID, *s.clientID)
	require.NotNil(t, s.eventID)
	assert.Equal(t, ev.id, *s.eventID)
	u := got[app+":notify"]
	assert.Contains(t, jobIDs, u.id)
	assert.Nil(t, u.clientID, "a job naming no client is unscoped; the sink's clientCode is not inherited")

	var evClient string
	require.NoError(t, pool.QueryRow(ctx, `SELECT client_id FROM msg_events WHERE deduplication_id = $1`, ev.id).Scan(&evClient))
	assert.Equal(t, clientID, evClient)
}

type e2ePlanEvent struct{ id, corr string }

func (e e2ePlanEvent) EventID() string           { return e.id }
func (e2ePlanEvent) EventType() string           { return "gosdke2epl:sales:order:placed" }
func (e2ePlanEvent) SpecVersion() string         { return "1.0" }
func (e2ePlanEvent) Source() string              { return "gosdke2epl:sales" }
func (e2ePlanEvent) Subject() string             { return "sales.order.ord_1" }
func (e2ePlanEvent) Time() time.Time             { return time.Now() }
func (e2ePlanEvent) PrincipalID() string         { return "prn_e2e" }
func (e e2ePlanEvent) CorrelationID() string     { return e.corr }
func (e2ePlanEvent) CausationID() string         { return "" }
func (e2ePlanEvent) ExecutionID() string         { return "" }
func (e2ePlanEvent) MessageGroup() string        { return "sales:order:ord_1" }
func (e2ePlanEvent) ToDataJSON() ([]byte, error) { return []byte(`{"orderId":"ord_1"}`), nil }

type e2eEvent struct{ id string }

func (e e2eEvent) EventID() string           { return e.id }
func (e2eEvent) EventType() string           { return "gosdke2eob:sales:order:placed" }
func (e2eEvent) SpecVersion() string         { return "1.0" }
func (e2eEvent) Source() string              { return "" } // the platform defaults it (F14)
func (e2eEvent) Subject() string             { return "sales.order.ord_1" }
func (e2eEvent) Time() time.Time             { return time.Now() }
func (e2eEvent) PrincipalID() string         { return "prn_e2e" }
func (e2eEvent) CorrelationID() string       { return "corr-e2e" }
func (e2eEvent) CausationID() string         { return "" }
func (e2eEvent) ExecutionID() string         { return "" }
func (e2eEvent) MessageGroup() string        { return "sales:order:ord_1" }
func (e2eEvent) ToDataJSON() ([]byte, error) { return []byte(`{"orderId":"ord_1"}`), nil }
