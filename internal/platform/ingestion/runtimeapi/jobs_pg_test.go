//go:build integration

package runtimeapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/runtimeapi"
	platformconfigops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// targets are the records a target resolves: a pool, a signing service
// account, and a connection fronting another one.
type targets struct {
	app           string // the code of the ingestions' application
	pool          *dispatchpool.DispatchPool
	account       *serviceaccount.ServiceAccount
	connection    *connection.Connection
	connectionAcc *serviceaccount.ServiceAccount
}

func (e *env) makeTargets() targets {
	e.t.Helper()
	tg := targets{app: e.app.Code}
	tg.pool = dispatchpool.New("pool-"+unique(), "Pool")
	// The signing account is the application's own, as a target's must be.
	tg.account = serviceaccount.New("sa-"+unique(), "Signer")
	tg.account.ApplicationID = &e.app.ID
	tg.connectionAcc = serviceaccount.New("csa-"+unique(), "Connection signer")
	tg.connection = connection.New("conn-"+unique(), "Connection", tg.connectionAcc.ID)
	e.persist(testpg.NewUoW(e.t), func(ctx context.Context, tx *usecasepgx.DbTx) error {
		sas := serviceaccount.NewRepository(e.pool)
		if err := sas.Persist(ctx, tg.account, tx); err != nil {
			return err
		}
		if err := sas.Persist(ctx, tg.connectionAcc, tx); err != nil {
			return err
		}
		if err := connection.NewRepository(e.pool).Persist(ctx, tg.connection, tx); err != nil {
			return err
		}
		return dispatchpool.NewRepository(e.pool).Persist(ctx, tg.pool, tx)
	})
	return tg
}

// target renders the definition's target; extra is spliced in verbatim.
func (tg targets) target(extra string) string {
	return fmt.Sprintf(`{"code":"%s:orders:order:received","targetUrl":"https://hooks.example.com/orders","pool":%q,"mode":"NEXT_ON_ERROR","batchSize":100%s}`,
		tg.app, tg.pool.Code, extra)
}

func (tg targets) viaConnection() string {
	return tg.target(fmt.Sprintf(`,"connection":%q`, tg.connection.Code))
}

func (tg targets) viaAccount() string {
	return tg.target(fmt.Sprintf(`,"serviceAccount":%q`, tg.account.Code))
}

// saveTarget stores an ingestion of the source with the target JSON.
func (e *env) saveTarget(code, source, target string, live bool) *ingestion.Ingestion {
	e.t.Helper()
	def, err := ingestion.ParseDefinition([]byte(`{"name":"Orders","source":` + source + `,"split":{"kind":"none"},"target":` + target + `}`))
	require.NoError(e.t, err)
	i := ingestion.New(e.app.ID, e.client.ID, code, def, live, nil, nil)
	e.persist(testpg.NewUoW(e.t), func(ctx context.Context, tx *usecasepgx.DbTx) error { return e.repo.Persist(ctx, i, tx) })
	return i
}

// pause pauses the stored ingestion.
func (e *env) pause(id string) {
	e.t.Helper()
	i, err := e.repo.FindByID(context.Background(), id)
	require.NoError(e.t, err)
	i.Pause()
	e.persist(testpg.NewUoW(e.t), func(ctx context.Context, tx *usecasepgx.DbTx) error { return e.repo.Persist(ctx, i, tx) })
}

func jobsPath(id, runtime string) string {
	return base + "/ingestions/" + id + "/jobs?runtimeId=" + runtime
}

func records(n int, prefix string) []map[string]any {
	out := make([]map[string]any, n)
	for k := range out {
		out[k] = map[string]any{"payload": fmt.Sprintf(`{"n":%d}`, k), "externalId": fmt.Sprintf("%s-%d", prefix, k)}
	}
	return out
}

type jobRow struct {
	source, code, targetURL, mode, payload, contentType string
	pool, connection, account, client                   *string
	group, correlation, subject, external               *string
	metadata                                            string
}

func (e *env) job(id string) jobRow {
	e.t.Helper()
	var j jobRow
	require.NoError(e.t, e.pool.QueryRow(context.Background(), `SELECT source, code, target_url, mode, payload, payload_content_type,
		dispatch_pool_id, connection_id, service_account_id, client_id, message_group, correlation_id, subject, external_id, metadata::text
		FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&j.source, &j.code, &j.targetURL, &j.mode, &j.payload, &j.contentType,
		&j.pool, &j.connection, &j.account, &j.client, &j.group, &j.correlation, &j.subject, &j.external, &j.metadata))
	return j
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

// A job is built from the ingestion's own target: its code and URL, the pool
// resolved, the connection (so pausing it holds the job) with the connection's
// account signing, the ingestion's client and mode, a source naming the
// ingestion, and metadata naming it and the runtime.
func TestJobs_BuiltFromTheIngestionsTarget(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	i := e.saveTarget("via-connection", sqsSource, tg.viaConnection(), true)
	rt := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(rt, ingestion.ContractVersion).code)

	r := e.do(http.MethodPost, jobsPath(i.ID, rt), map[string]any{"records": []map[string]any{{
		"payload": `{"order":1}`, "payloadContentType": "application/x-ndjson", "messageGroup": "order-1",
		"externalId": "ext-1", "correlationId": "corr-1", "subject": "orders/1",
	}, {"payload": "plain"}}})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.EqualValues(t, 2, r.json["created"])
	results := r.list("results")
	require.Len(t, results, 2)

	j := e.job(results[0]["id"].(string))
	assert.Equal(t, "ingestion:"+e.app.Code+":via-connection", j.source)
	assert.Equal(t, e.app.Code+":orders:order:received", j.code)
	assert.Equal(t, "https://hooks.example.com/orders", j.targetURL)
	assert.Equal(t, "NEXT_ON_ERROR", j.mode)
	assert.Equal(t, tg.pool.ID, deref(j.pool))
	assert.Equal(t, tg.connection.ID, deref(j.connection), "the job carries the connection, so its pause holds it")
	assert.Equal(t, tg.connectionAcc.ID, deref(j.account), "the connection's account signs")
	assert.Equal(t, string(e.client.ID), deref(j.client))
	assert.Equal(t, `{"order":1}`, j.payload)
	assert.Equal(t, "application/x-ndjson", j.contentType)
	assert.Equal(t, "order-1", deref(j.group))
	assert.Equal(t, "ext-1", deref(j.external))
	assert.Equal(t, "corr-1", deref(j.correlation))
	assert.Equal(t, "orders/1", deref(j.subject))
	var meta []map[string]string
	require.NoError(t, json.Unmarshal([]byte(j.metadata), &meta))
	assert.ElementsMatch(t, []map[string]string{{"key": "ingestionId", "value": i.ID}, {"key": "runtimeId", "value": rt}}, meta)

	assert.Equal(t, "application/json", e.job(results[1]["id"].(string)).contentType, "the default content type")

	// Without a connection, the explicit service account signs.
	direct := e.saveTarget("via-account", sqsSource, tg.viaAccount(), true)
	r = e.do(http.MethodPost, jobsPath(direct.ID, rt), map[string]any{"records": records(1, "a")})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	j = e.job(r.list("results")[0]["id"].(string))
	assert.Nil(t, j.connection)
	assert.Equal(t, tg.account.ID, deref(j.account))

	// And with neither, the delivery is unsigned.
	bare := e.saveTarget("bare", sqsSource, tg.target(""), true)
	r = e.do(http.MethodPost, jobsPath(bare.ID, rt), map[string]any{"records": records(1, "b")})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	j = e.job(r.list("results")[0]["id"].(string))
	assert.Nil(t, j.connection)
	assert.Nil(t, j.account)
}

// Where a job goes, what signs it and for whom are the ingestion's: a field
// for any of them in a record is refused, and one beside the records is
// ignored, so the job is the ingestion's either way.
func TestJobs_RuntimeCannotChangeTheTarget(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	i := e.saveTarget("fixed", sqsSource, tg.viaConnection(), true)
	rt := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(rt, ingestion.ContractVersion).code)
	before := e.count(`SELECT count(*) FROM msg_dispatch_jobs WHERE dispatch_pool_id = $1`, tg.pool.ID)

	for _, field := range []string{"targetUrl", "code", "dispatchPoolId", "serviceAccountId", "clientId", "clientCode", "connectionId", "mode", "source"} {
		rec := map[string]any{"payload": "x", field: "https://evil.example.com"}
		r := e.do(http.MethodPost, jobsPath(i.ID, rt), map[string]any{"records": []map[string]any{rec}})
		assert.GreaterOrEqual(t, r.code, 400, "%s in a record: %s", field, r.raw)
		assert.Less(t, r.code, 500, "%s in a record: %s", field, r.raw)
	}
	assert.Equal(t, before, e.count(`SELECT count(*) FROM msg_dispatch_jobs WHERE dispatch_pool_id = $1`, tg.pool.ID), "nothing was made")

	r := e.do(http.MethodPost, jobsPath(i.ID, rt), map[string]any{
		"targetUrl": "https://evil.example.com", "clientId": "clt_other0000000", "serviceAccountId": "sac_other0000000",
		"records": []map[string]any{{"payload": "x"}},
	})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	j := e.job(r.list("results")[0]["id"].(string))
	assert.Equal(t, "https://hooks.example.com/orders", j.targetURL)
	assert.Equal(t, string(e.client.ID), deref(j.client))
	assert.Equal(t, tg.connectionAcc.ID, deref(j.account))
}

// An externalId makes one job per ingestion however often it is sent, across
// requests and within one; another ingestion's same key is its own.
func TestJobs_ExternalIDIsIdempotent(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	i := e.saveTarget("idem", sqsSource, tg.viaAccount(), true)
	other := e.saveTarget("idem-other", sqsSource, tg.viaAccount(), true)
	rt := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(rt, ingestion.ContractVersion).code)

	first := e.do(http.MethodPost, jobsPath(i.ID, rt), map[string]any{"records": []map[string]any{{"payload": "1", "externalId": "k1"}, {"payload": "2", "externalId": "k2"}}})
	require.Equal(t, http.StatusOK, first.code, first.raw)
	assert.EqualValues(t, 2, first.json["created"])
	ids := []any{first.list("results")[0]["id"], first.list("results")[1]["id"]}

	again := e.do(http.MethodPost, jobsPath(i.ID, rt), map[string]any{"records": []map[string]any{
		{"payload": "2", "externalId": "k2"}, {"payload": "3", "externalId": "k3"}, {"payload": "4"}, {"payload": "3 again", "externalId": "k3"},
	}})
	require.Equal(t, http.StatusOK, again.code, again.raw)
	assert.EqualValues(t, 2, again.json["created"], "k3 once and the record with no key")
	results := again.list("results")
	require.Len(t, results, 4)
	assert.Equal(t, ids[1], results[0]["id"], "the redelivery answers the job it made")
	assert.Equal(t, true, results[0]["existing"])
	assert.Equal(t, results[1]["id"], results[3]["id"], "the same key within one request is one job")
	assert.Equal(t, true, results[3]["existing"])
	assert.NotContains(t, results[1], "existing")
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM msg_dispatch_jobs WHERE external_id = 'k2' AND source = $1`, "ingestion:"+e.app.Code+":idem"))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM msg_dispatch_jobs WHERE external_id = 'k3' AND source = $1`, "ingestion:"+e.app.Code+":idem"))

	r := e.do(http.MethodPost, jobsPath(other.ID, rt), map[string]any{"records": []map[string]any{{"payload": "1", "externalId": "k1"}}})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.EqualValues(t, 1, r.json["created"], "keys are per ingestion")
	assert.NotEqual(t, ids[0], r.list("results")[0]["id"])
}

// The runtime must have reported the ingestion's kind, and for a polled
// source hold its lease.
func TestJobs_KindAndLease(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	shared := e.saveTarget("shared", sqsSource, tg.viaAccount(), true)
	polled := e.saveTarget("polled", sftpSource, tg.viaAccount(), true)
	body := map[string]any{"records": records(1, "x")}

	never := "rt-never-" + unique()
	r := e.do(http.MethodPost, jobsPath(shared.ID, never), body)
	assert.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Equal(t, "RUNTIME_KIND_NOT_CARRIED", r.errCode(), "a runtime that has not polled carries nothing")

	holder, other := "rt-h-"+unique(), "rt-o-"+unique()
	require.Equal(t, http.StatusOK, e.poll(holder, ingestion.ContractVersion).code)
	assert.Equal(t, http.StatusOK, e.do(http.MethodPost, jobsPath(shared.ID, holder), body).code)
	assert.Equal(t, http.StatusOK, e.do(http.MethodPost, jobsPath(polled.ID, holder), body).code, "the holder of the lease")

	require.Equal(t, http.StatusOK, e.poll(other, ingestion.ContractVersion).code)
	assert.Equal(t, http.StatusOK, e.do(http.MethodPost, jobsPath(shared.ID, other), body).code, "a shared source is any carrier's")
	r = e.do(http.MethodPost, jobsPath(polled.ID, other), body)
	assert.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Equal(t, "LEASE_NOT_HELD", r.errCode())

	limited := "rt-l-" + unique()
	require.Equal(t, http.StatusOK, e.poll(limited, ingestion.ContractVersion, map[string]any{"kind": "http"}).code)
	r = e.do(http.MethodPost, jobsPath(shared.ID, limited), body)
	assert.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Equal(t, "RUNTIME_KIND_NOT_CARRIED", r.errCode(), "it reported http only, not sqs")

	e.now = e.now.Add(4 * time.Minute) // the holder's lease runs out unrenewed
	assert.Equal(t, "LEASE_NOT_HELD", e.do(http.MethodPost, jobsPath(polled.ID, holder), body).errCode())
}

// Only a live, active ingestion takes jobs.
func TestJobs_UnknownPausedAndNotLive(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	rt := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(rt, ingestion.ContractVersion).code)
	body := map[string]any{"records": records(1, "x")}

	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPost, jobsPath("ing_nothing0000", rt), body).code)

	paused := e.saveTarget("paused", sqsSource, tg.viaAccount(), true)
	e.pause(paused.ID)
	r := e.do(http.MethodPost, jobsPath(paused.ID, rt), body)
	assert.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Equal(t, "INGESTION_NOT_LIVE", r.errCode())

	draft := e.saveTarget("draft", sqsSource, tg.viaAccount(), false)
	assert.Equal(t, "INGESTION_NOT_LIVE", e.do(http.MethodPost, jobsPath(draft.ID, rt), body).errCode())
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM msg_dispatch_jobs WHERE source IN ($1, $2)`,
		"ingestion:"+e.app.Code+":paused", "ingestion:"+e.app.Code+":draft"))

	// A target that no longer resolves is a conflict on the ingestion.
	broken := e.saveTarget("broken", sqsSource, tg.viaAccount(), true)
	tg.pool.Suspend()
	e.persist(testpg.NewUoW(t), func(ctx context.Context, tx *usecasepgx.DbTx) error {
		return dispatchpool.NewRepository(e.pool).Persist(ctx, tg.pool, tx)
	})
	r = e.do(http.MethodPost, jobsPath(broken.ID, rt), body)
	assert.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Equal(t, "INGESTION_TARGET_INVALID", r.errCode())
}

// A request carries at most 1000 records, and 1000 is fine.
func TestJobs_RecordCap(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	i := e.saveTarget("cap", sqsSource, tg.viaAccount(), true)
	rt := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(rt, ingestion.ContractVersion).code)

	r := e.do(http.MethodPost, jobsPath(i.ID, rt), map[string]any{"records": records(runtimeMax+1, "over")})
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw[:min(len(r.raw), 300)])
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_job_keys WHERE ingestion_id = $1`, i.ID))

	r = e.do(http.MethodPost, jobsPath(i.ID, rt), map[string]any{"records": records(runtimeMax, "full")})
	require.Equal(t, http.StatusOK, r.code, r.raw[:min(len(r.raw), 300)])
	assert.EqualValues(t, runtimeMax, r.json["created"])
	assert.Len(t, r.list("results"), runtimeMax)
	assert.Equal(t, runtimeMax, e.count(`SELECT count(*) FROM msg_dispatch_jobs WHERE source = $1`, "ingestion:"+e.app.Code+":cap"))
}

const runtimeMax = 1000

// A record that cannot become a job is answered with its error; the rest of
// the request is made.
func TestJobs_RecordErrorsAreIsolated(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	i := e.saveTarget("isolated", sqsSource, tg.viaAccount(), true)
	rt := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(rt, ingestion.ContractVersion).code)

	r := e.do(http.MethodPost, jobsPath(i.ID, rt), map[string]any{"records": []map[string]any{
		{"payload": "ok-0", "externalId": "i0"},
		{"payload": ""},
		{"payload": "long group", "messageGroup": strings.Repeat("g", 201)},
		{"payload": "blank key", "externalId": "  "},
		{"payload": "ok-4", "externalId": "i4"},
	}})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.EqualValues(t, 2, r.json["created"])
	results := r.list("results")
	require.Len(t, results, 5)
	for n, want := range map[int]string{1: "payload is required", 2: "messageGroup", 3: "externalId"} {
		assert.EqualValues(t, n, results[n]["index"])
		assert.Contains(t, results[n]["error"], want)
		assert.NotContains(t, results[n], "id")
	}
	for _, n := range []int{0, 4} {
		assert.NotEmpty(t, results[n]["id"])
		assert.NotContains(t, results[n], "error")
	}
	assert.Equal(t, 2, e.count(`SELECT count(*) FROM msg_dispatch_jobs WHERE source = $1`, "ingestion:"+e.app.Code+":isolated"))
	assert.Equal(t, 2, e.count(`SELECT count(*) FROM ing_job_keys WHERE ingestion_id = $1`, i.ID), "a refused record claims no key")
}

// The runtime permission is needed, and reach still applies.
func TestJobs_PermissionAndReach(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	i := e.saveTarget("reach", sqsSource, tg.viaAccount(), true)
	rt := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(rt, ingestion.ContractVersion).code)
	body := map[string]any{"records": records(1, "x")}

	e.who = e.principal(permView, permTest)
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodPost, jobsPath(i.ID, rt), body).code)
	// A forged or mis-provisioned runtime account (below anchor tier, or an
	// application's) is refused, even for its own client's ingestion.
	e.who = clientTier(e.principal(permRuntime), string(e.client.ID))
	r := e.do(http.MethodPost, jobsPath(i.ID, rt), body)
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_RUNTIME_ANCHOR_REQUIRED", r.errCode())
	e.who = e.principal(permRuntime)
	e.who.AllApplications, e.who.Applications = false, []string{string(e.app.ID)}
	r = e.do(http.MethodPost, jobsPath(i.ID, rt), body)
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_RUNTIME_APPLICATION_BOUND", r.errCode())
	e.who = e.principal(permRuntime)
	assert.Equal(t, http.StatusOK, e.do(http.MethodPost, jobsPath(i.ID, rt), body).code, "the runtime role is platform-wide")
}

const secretsSection = "secrets"

// setSecret stores a SECRET at the ingestion's application, the "secrets"
// section and the client (or the global scope when clientID is nil).
func (e *env) setSecret(name, value string, clientID *string) {
	e.t.Helper()
	secret := "SECRET"
	// Setting platform config confines the caller to its applications, so the
	// administrator here reaches every application.
	admin := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_optestrunner1", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"},
	})
	_, err := usecaseop.Run(admin, testpg.NewUoW(e.t), platformconfigops.SetProperty(e.config.Repo), platformconfigops.SetPropertyCommand{
		ApplicationCode: e.app.Code, Section: secretsSection, Property: name, Value: value, ValueType: &secret, ClientID: clientID,
	}, testpg.TestEC())
	require.NoError(e.t, err)
}

func secretPath(id, name, runtime string) string {
	return base + "/ingestions/" + id + "/secrets/" + name + "?runtimeId=" + runtime
}

// captureLogs sends slog's default logger to a JSON buffer for the test and
// returns the records written, one map per line.
func captureLogs(t *testing.T) func() []map[string]any {
	t.Helper()
	var mu sync.Mutex
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&lockedWriter{mu: &mu, w: &buf}, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		var out []map[string]any
		for line := range bytes.SplitSeq(buf.Bytes(), []byte("\n")) {
			var rec map[string]any
			if json.Unmarshal(line, &rec) == nil {
				out = append(out, rec)
			}
		}
		return out
	}
}

type lockedWriter struct {
	mu *sync.Mutex
	w  *bytes.Buffer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// securityLines are the records of the secret fetch security event.
func securityLines(records []map[string]any) []map[string]any {
	var out []map[string]any
	for _, r := range records {
		if r["msg"] == runtimeapi.SecretFetchEvent {
			out = append(out, r)
		}
	}
	return out
}

// fetchCounter is fc_ingestion_secret_fetch_total{outcome}.
func fetchCounter(t *testing.T, outcome string) float64 {
	t.Helper()
	families, err := ingestion.MetricsRegistry.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() != "fc_ingestion_secret_fetch_total" {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "outcome" && l.GetValue() == outcome {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	t.Fatalf("no fc_ingestion_secret_fetch_total{outcome=%q}", outcome)
	return 0
}

// A runtime reads the secrets its ingestion's live definition binds, at the
// ingestion's client; the answer is not cached. The fetch is security
// telemetry, not a config reveal (owner ruling 2026-10-25): no aud_logs row,
// one INFO line "security.ingestion_secret_fetch" naming the ingestion, the
// application, the client, the secret, the runtime, the principal and the
// version (never the value), the counter's success series, and the
// (ingestion, secret, runtime) last-fetched record, which the ingestion's API
// shows.
func TestSecrets_FetchIsSecurityTelemetryNotAnAuditRow(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	i := e.saveTarget("secrets", sftpSource, tg.viaAccount(), true)
	cid := string(e.client.ID)
	e.setSecret("sftp-password", "s3cr3t-pw", &cid)
	holder := "rt-" + unique()
	require.Len(t, e.poll(holder, ingestion.ContractVersion).list("ingestions"), 1)
	audits := e.count(`SELECT count(*) FROM aud_logs WHERE principal_id = $1`, e.who.PrincipalID)
	successes := fetchCounter(t, ingestion.SecretFetchSuccess)
	logs := captureLogs(t)

	rec := e.api.Get(secretPath(i.ID, "sftp-password", holder))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	var got struct{ Value, UpdatedAt string }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "s3cr3t-pw", got.Value)
	assert.NotEmpty(t, got.UpdatedAt)
	require.Equal(t, http.StatusOK, e.api.Get(secretPath(i.ID, "sftp-password", holder)).Code)

	assert.Equal(t, audits, e.count(`SELECT count(*) FROM aud_logs WHERE principal_id = $1`, e.who.PrincipalID), "no audit row per fetch")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM aud_logs WHERE operation = 'SECRET_REVEALED' AND operation_json->>'ingestionId' = $1`, i.ID))
	assert.Equal(t, successes+2, fetchCounter(t, ingestion.SecretFetchSuccess))

	lines := securityLines(logs())
	require.Len(t, lines, 2)
	line := lines[0]
	assert.Equal(t, "INFO", line["level"])
	assert.Equal(t, runtimeapi.SecretFetchEvent, line["event"])
	assert.Equal(t, "success", line["outcome"])
	assert.Equal(t, i.ID, line["ingestionId"])
	assert.Equal(t, e.app.Code, line["applicationCode"])
	assert.Equal(t, cid, line["clientId"])
	assert.Equal(t, "sftp-password", line["secretName"])
	assert.Equal(t, holder, line["runtimeId"])
	assert.Equal(t, e.who.PrincipalID, line["principalId"])
	assert.Equal(t, float64(1), line["version"])
	raw, _ := json.Marshal(logs())
	assert.NotContains(t, string(raw), "s3cr3t-pw", "the value is never logged")

	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_secret_fetches WHERE ingestion_id = $1 AND secret_name = 'sftp-password'
		AND runtime_id = $2 AND fetch_count = 2 AND version = 1 AND principal_id = $3`, i.ID, holder, e.who.PrincipalID), "upserted, counted")

	listed := e.do(http.MethodGet, "/api/ingestions/"+i.ID+"/secrets", nil)
	require.Equal(t, http.StatusOK, listed.code, listed.raw)
	assert.NotContains(t, listed.raw, "s3cr3t-pw")
	var body struct {
		Secrets []struct {
			Name    string
			Bound   bool
			Fetches []struct {
				RuntimeID  string
				FetchCount int
			}
		}
	}
	require.NoError(t, json.Unmarshal([]byte(listed.raw), &body))
	require.Len(t, body.Secrets, 1)
	assert.Equal(t, "sftp-password", body.Secrets[0].Name)
	assert.True(t, body.Secrets[0].Bound)
	require.Len(t, body.Secrets[0].Fetches, 1)
	assert.Equal(t, holder, body.Secrets[0].Fetches[0].RuntimeID)
	assert.Equal(t, 2, body.Secrets[0].Fetches[0].FetchCount)

	// The record goes with the runtime: pruned once it is unseen for 7 days.
	_, err := e.pool.Exec(context.Background(), `UPDATE ing_runtimes SET last_seen_at = now() - interval '8 days' WHERE runtime_id = $1`, holder)
	require.NoError(t, err)
	p, err := e.repo.Prune(context.Background(), time.Now().UTC())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, p.SecretFetches, int64(1))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_secret_fetches WHERE runtime_id = $1`, holder))
}

// Only a name the live definition references is read; the platform config is
// otherwise out of the runtime's reach, and there is no global fallback.
func TestSecrets_UnreferencedAndUnsetNamesAreNotFound(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	i := e.saveTarget("secrets", sftpSource, tg.viaAccount(), true)
	cid := string(e.client.ID)
	e.setSecret("other-secret", "not-yours", &cid)
	e.setSecret("sftp-password", "global-only", nil)
	holder := "rt-" + unique()
	require.Len(t, e.poll(holder, ingestion.ContractVersion).list("ingestions"), 1)

	r := e.do(http.MethodGet, secretPath(i.ID, "other-secret", holder), nil)
	assert.Equal(t, http.StatusNotFound, r.code, r.raw)
	assert.NotContains(t, r.raw, "not-yours")
	r = e.do(http.MethodGet, secretPath(i.ID, "sftp-password", holder), nil)
	assert.Equal(t, http.StatusNotFound, r.code, "named, but set only globally: %s", r.raw)
	assert.NotContains(t, r.raw, "global-only")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM aud_logs WHERE operation = 'SECRET_REVEALED' AND operation_json->>'ingestionId' = $1`, i.ID))

	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, secretPath("ing_nothing0000", "sftp-password", holder), nil).code)
}

// The runtime must carry the kind and, for a polled source, hold the lease;
// the ingestion must be live.
func TestSecrets_RuntimeMustCarryTheKindAndHoldTheLease(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	polled := e.saveTarget("polled", sftpSource, tg.viaAccount(), true)
	shared := e.saveTarget("shared", `{"kind":"rabbitmq","uri":"amqps://mq.example.com","queue":"orders","username":"feed","password":{"name":"mq-password"}}`, tg.viaAccount(), true)
	cid := string(e.client.ID)
	e.setSecret("sftp-password", "s3cr3t-pw", &cid)
	e.setSecret("mq-password", "mq-pw", &cid)
	holder, other, limited := "rt-h-"+unique(), "rt-o-"+unique(), "rt-l-"+unique()
	require.Len(t, e.poll(holder, ingestion.ContractVersion).list("ingestions"), 2)
	require.Equal(t, http.StatusOK, e.poll(other, ingestion.ContractVersion).code)
	require.Equal(t, http.StatusOK, e.poll(limited, ingestion.ContractVersion, map[string]any{"kind": "http"}).code)

	assert.Equal(t, http.StatusOK, e.do(http.MethodGet, secretPath(polled.ID, "sftp-password", holder), nil).code)
	r := e.do(http.MethodGet, secretPath(polled.ID, "sftp-password", other), nil)
	assert.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Equal(t, "LEASE_NOT_HELD", r.errCode())
	r = e.do(http.MethodGet, secretPath(shared.ID, "mq-password", limited), nil)
	assert.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Equal(t, "RUNTIME_KIND_NOT_CARRIED", r.errCode())
	assert.Equal(t, "RUNTIME_KIND_NOT_CARRIED", e.do(http.MethodGet, secretPath(shared.ID, "mq-password", "rt-never-"+unique()), nil).errCode())
	assert.Equal(t, http.StatusOK, e.do(http.MethodGet, secretPath(shared.ID, "mq-password", other), nil).code, "a shared source is any carrier's")

	e.pause(shared.ID)
	assert.Equal(t, "INGESTION_NOT_LIVE", e.do(http.MethodGet, secretPath(shared.ID, "mq-password", other), nil).errCode())
}

// A refused fetch is audited (SECRET_FETCH_REFUSED, on the ingestion, so it
// reads in its history), logged at WARN with the reason and counted as
// refused: a name the live definition does not bind, no lease where one is
// needed, a client that is not active. No value is in any of them, and the
// fetch record is not touched.
func TestSecrets_RefusalIsAuditedAndLogged(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	i := e.saveTarget("secrets", sftpSource, tg.viaAccount(), true)
	cid := string(e.client.ID)
	e.setSecret("sftp-password", "s3cr3t-pw", &cid)
	e.setSecret("other-secret", "not-yours", &cid)
	holder, other := "rt-h-"+unique(), "rt-o-"+unique()
	require.Len(t, e.poll(holder, ingestion.ContractVersion).list("ingestions"), 1)
	require.Equal(t, http.StatusOK, e.poll(other, ingestion.ContractVersion).code)
	refusals := func(reason string) int {
		return e.count(`SELECT count(*) FROM aud_logs WHERE entity_type = 'Ingestion' AND entity_id = $1
			AND operation = 'SECRET_FETCH_REFUSED' AND principal_id = $2 AND client_id = $3
			AND operation_json->>'reason' = $4 AND operation_json->>'runtimeId' IS NOT NULL`, i.ID, e.who.PrincipalID, cid, reason)
	}
	refused := fetchCounter(t, ingestion.SecretFetchRefused)
	logs := captureLogs(t)

	r := e.do(http.MethodGet, secretPath(i.ID, "other-secret", holder), nil)
	assert.Equal(t, http.StatusNotFound, r.code, r.raw)
	assert.Equal(t, "SECRET_NOT_FOUND", r.errCode())
	assert.Equal(t, 1, refusals("SECRET_NOT_FOUND"), "a name the live definition does not bind")

	r = e.do(http.MethodGet, secretPath(i.ID, "sftp-password", other), nil)
	assert.Equal(t, "LEASE_NOT_HELD", r.errCode())
	assert.Equal(t, 1, refusals("LEASE_NOT_HELD"), "no lease where one is needed")

	e.client.SetStatus(client.StatusSuspended, nil)
	e.persist(testpg.NewUoW(t), func(ctx context.Context, tx *usecasepgx.DbTx) error {
		return client.NewRepository(e.pool).Persist(ctx, e.client, tx)
	})
	r = e.do(http.MethodGet, secretPath(i.ID, "sftp-password", holder), nil)
	assert.Equal(t, "CLIENT_NOT_ACTIVE", r.errCode())
	assert.Equal(t, 1, refusals("CLIENT_NOT_ACTIVE"), "an inactive client")

	assert.Equal(t, refused+3, fetchCounter(t, ingestion.SecretFetchRefused))
	lines := securityLines(logs())
	require.Len(t, lines, 3)
	for _, l := range lines {
		assert.Equal(t, "WARN", l["level"])
		assert.Equal(t, "refused", l["outcome"])
		assert.Equal(t, i.ID, l["ingestionId"])
		assert.NotEmpty(t, l["reason"])
	}
	raw, _ := json.Marshal(logs())
	assert.NotContains(t, string(raw), "s3cr3t-pw")
	assert.NotContains(t, string(raw), "not-yours")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_secret_fetches WHERE ingestion_id = $1`, i.ID), "a refusal is no fetch")

	// The refusal reads in the ingestion's history.
	h := e.do(http.MethodGet, "/api/ingestions/"+i.ID+"/history", nil)
	require.Equal(t, http.StatusOK, h.code, h.raw)
	assert.Contains(t, h.raw, "SECRET_FETCH_REFUSED")
}

// Reach and the runtime permission apply to secrets as to everything else.
func TestSecrets_PermissionAndReach(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	i := e.saveTarget("secrets", sftpSource, tg.viaAccount(), true)
	cid := string(e.client.ID)
	e.setSecret("sftp-password", "s3cr3t-pw", &cid)
	holder := "rt-" + unique()
	require.Len(t, e.poll(holder, ingestion.ContractVersion).list("ingestions"), 1)

	e.who = e.principal(permView, "platform:admin:config:reveal", "platform:application-service:config:reveal")
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodGet, secretPath(i.ID, "sftp-password", holder), nil).code,
		"the reveal grants of platform config do not open this route")
	e.who = clientTier(e.principal(permRuntime), string(e.client.ID))
	assert.Equal(t, "INGESTION_RUNTIME_ANCHOR_REQUIRED", e.do(http.MethodGet, secretPath(i.ID, "sftp-password", holder), nil).errCode())
	e.who = e.principal(permRuntime)
	e.who.AllApplications, e.who.Applications = false, []string{string(e.app.ID)}
	assert.Equal(t, "INGESTION_RUNTIME_APPLICATION_BOUND", e.do(http.MethodGet, secretPath(i.ID, "sftp-password", holder), nil).errCode())
	e.who = e.principal(permRuntime)
	assert.Equal(t, http.StatusOK, e.do(http.MethodGet, secretPath(i.ID, "sftp-password", holder), nil).code, "the runtime role is platform-wide")
}
