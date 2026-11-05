//go:build integration

package sdk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// jobFixture mounts the dispatch-job routes with every repository wired, an
// application, two clients (A and B) and connections in each scope, behind a
// caller chosen per request (X-Test-Caller: "anchor", or "client" reaching
// client A only).
type jobFixture struct {
	srv           *httptest.Server
	pool          *pgxpool.Pool
	repo          *dispatchjob.Repository
	app, otherApp string
	aID, bID      string
	// Connections: own = app + client A; shared = no app, no client;
	// clientB = app + client B; otherApp = the other application, no client.
	own, shared, clientB, otherAppConn string
	// sub is a subscription of app whose connection is own.
	sub string
}

func insertConnection(t *testing.T, pool *pgxpool.Pool, code string, appCode, clientID *string) string {
	t.Helper()
	id := tsid.Generate(tsid.Connection)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_connections (id, code, name, status, service_account_id, application_code, client_id)
		 VALUES ($1, $2, $2, 'ACTIVE', 'sac_jobfixture001', $3, $4)`, id, code, appCode, clientID)
	require.NoError(t, err)
	return id
}

func newJobFixture(t *testing.T) *jobFixture {
	t.Helper()
	pool := testpg.Pool(t)
	sfx := strings.ToLower(tsid.GenerateUntyped())
	f := &jobFixture{pool: pool, repo: dispatchjob.NewRepository(pool), app: "djc" + sfx, otherApp: "djo" + sfx}
	insertApp(t, pool, f.app)
	insertApp(t, pool, f.otherApp)
	f.aID = insertClient(t, pool, "djca-"+sfx)
	f.bID = insertClient(t, pool, "djcb-"+sfx)
	f.own = insertConnection(t, pool, "own-"+sfx, &f.app, &f.aID)
	f.shared = insertConnection(t, pool, "shared-"+sfx, nil, nil)
	f.clientB = insertConnection(t, pool, "b-"+sfx, &f.app, &f.bID)
	f.otherAppConn = insertConnection(t, pool, "other-"+sfx, &f.otherApp, nil)
	f.sub = insertSub(t, pool, "djsub-"+sfx, f.app)
	_, err := pool.Exec(context.Background(), `UPDATE msg_subscriptions SET connection_id = $2 WHERE id = $1`, f.sub, f.own)
	require.NoError(t, err)

	perms := []string{"platform:messaging:batch:dispatch-jobs-write"}
	anchor := &auth.AuthContext{PrincipalID: "p_djc_anchor", Scope: auth.ScopeAnchor, Permissions: perms, AllApplications: true}
	clientAC := &auth.AuthContext{PrincipalID: "p_djc_client", Scope: auth.ScopeClient, Clients: []string{f.aID}, Permissions: perms, AllApplications: true}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ac := anchor
			if req.Header.Get("X-Test-Caller") == "client" {
				ac = clientAC
			}
			next.ServeHTTP(w, req.WithContext(auth.WithContext(req.Context(), ac)))
		})
	})
	RegisterRoutes(r, &DispatchJobsBatchState{
		Repo: f.repo, Clients: client.NewRepository(pool), Connections: connection.NewRepository(pool),
		Subs: subscription.NewRepository(pool),
	})
	f.srv = httptest.NewServer(r)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *jobFixture) post(t *testing.T, caller, path, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, f.srv.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Caller", caller)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(raw)
}

// batchIDs posts items as one batch and returns the per-item result ids,
// requiring a 201 with every item SUCCESS.
func (f *jobFixture) batchIDs(t *testing.T, caller string, items ...string) []string {
	t.Helper()
	code, body := f.post(t, caller, "/api/dispatch-jobs/batch", `{"items":[`+strings.Join(items, ",")+`]}`)
	require.Equal(t, http.StatusCreated, code, body)
	var br BatchResponse
	require.NoError(t, json.Unmarshal([]byte(body), &br))
	require.Len(t, br.Results, len(items))
	out := make([]string, len(items))
	for i, r := range br.Results {
		require.Equal(t, "SUCCESS", r.Status, body)
		out[i] = r.ID
	}
	return out
}

// item builds a batch item of f.app with the extra members appended.
func (f *jobFixture) item(extra string) string {
	s := `{"code":"` + f.app + `:billing:invoice:sync","targetUrl":"https://target.test/hook","payload":"{}"`
	if extra != "" {
		s += "," + extra
	}
	return s + "}"
}

func (f *jobFixture) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, f.pool.QueryRow(context.Background(), sql, args...).Scan(&n))
	return n
}

func (f *jobFixture) storedConnection(t *testing.T, id string) *string {
	t.Helper()
	j, err := f.repo.FindByID(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, j, id)
	return j.ConnectionID
}

// A batch item's connectionId is stored on the job (connection_id), checked
// as a subscription's connectionId is: it must exist, and be global or the
// job's client's, shared or the job's application's. Any refusal rejects the
// whole batch. The singular route takes it the same way.
func TestDispatchJobs_ConnectionID(t *testing.T) {
	f := newJobFixture(t)
	clientA := `"clientId":"` + f.aID + `"`

	ids := f.batchIDs(t, "client",
		f.item(clientA+`,"connectionId":"`+f.own+`"`),
		f.item(clientA+`,"connectionId":"`+f.shared+`"`),
		f.item(clientA+`,"connectionId":"  "`),
		f.item(clientA),
	)
	if c := f.storedConnection(t, ids[0]); assert.NotNil(t, c) {
		assert.Equal(t, f.own, *c, "the client's own connection of the job's application")
	}
	if c := f.storedConnection(t, ids[1]); assert.NotNil(t, c) {
		assert.Equal(t, f.shared, *c, "a shared, global connection is usable by any job")
	}
	assert.Nil(t, f.storedConnection(t, ids[2]), "a blank connectionId is none")
	assert.Nil(t, f.storedConnection(t, ids[3]))

	// Singular.
	code, body := f.post(t, "client", "/api/dispatch-jobs",
		`{"code":"`+f.app+`:billing:invoice:sync","targetUrl":"https://target.test/hook","payload":"{}","serviceAccountId":"sac_x","clientId":"`+f.aID+`","connectionId":"`+f.own+`"}`)
	require.Equal(t, http.StatusCreated, code, body)
	var created CreatedResponse
	require.NoError(t, json.Unmarshal([]byte(body), &created))
	if c := f.storedConnection(t, created.ID); assert.NotNil(t, c) {
		assert.Equal(t, f.own, *c)
	}

	before := f.count(t, `SELECT count(*) FROM msg_dispatch_jobs WHERE code = $1`, f.app+":billing:invoice:sync")
	for name, tc := range map[string]struct {
		caller, item, want string
	}{
		"unknown connection":                {"anchor", f.item(`"connectionId":"con_doesnotexist0"`), "CONNECTION_NOT_FOUND"},
		"another client's connection":       {"client", f.item(clientA + `,"connectionId":"` + f.clientB + `"`), "CONNECTION_SCOPE_MISMATCH"},
		"a client's connection, no client":  {"anchor", f.item(`"connectionId":"` + f.own + `"`), "CONNECTION_SCOPE_MISMATCH"},
		"another application's connection":  {"anchor", f.item(`"connectionId":"` + f.otherAppConn + `"`), "CONNECTION_SCOPE_MISMATCH"},
		"not the subscription's connection": {"client", f.item(clientA + `,"subscriptionId":"` + f.sub + `","connectionId":"` + f.shared + `"`), "CONNECTION_MISMATCH"},
	} {
		// The good item first: a refusal anywhere rejects the whole batch.
		code, body := f.post(t, tc.caller, "/api/dispatch-jobs/batch", `{"items":[`+f.item(clientA)+`,`+tc.item+`]}`)
		assert.Equal(t, http.StatusBadRequest, code, name+": "+body)
		assert.Contains(t, body, tc.want, name)
	}
	assert.Equal(t, before, f.count(t, `SELECT count(*) FROM msg_dispatch_jobs WHERE code = $1`, f.app+":billing:invoice:sync"),
		"a refused batch writes nothing")

	// The subscription's own connection may be named alongside it.
	ids = f.batchIDs(t, "client", f.item(clientA+`,"subscriptionId":"`+f.sub+`","connectionId":"`+f.own+`"`))
	if c := f.storedConnection(t, ids[0]); assert.NotNil(t, c) {
		assert.Equal(t, f.own, *c)
	}
}

// An idempotencyKey makes the create idempotent within the job's application
// and client: a repeat, in a later request or later in the same batch,
// answers the first job's id and writes nothing; another client or another
// application with the same key gets its own job.
func TestDispatchJobs_IdempotencyKey(t *testing.T) {
	f := newJobFixture(t)
	key := "order-" + strings.ToLower(tsid.GenerateUntyped())
	keyed := func(extra string) string {
		return f.item(`"idempotencyKey":"` + key + `"` + extra)
	}
	stored := func() int {
		return f.count(t, `SELECT count(*) FROM msg_dispatch_jobs WHERE idempotency_key = $1`, key)
	}

	first := f.batchIDs(t, "anchor", keyed(`,"clientId":"`+f.aID+`"`))
	require.Equal(t, 1, stored())

	// A repeat in a later batch, next to a new keyless item: the repeat
	// answers the first id, the new item is created.
	again := f.batchIDs(t, "anchor", f.item(""), keyed(`,"clientId":"`+f.aID+`"`))
	assert.Equal(t, first[0], again[1], "the repeat answers the first job's id")
	assert.NotEqual(t, first[0], again[0])
	assert.Equal(t, 1, stored(), "the repeat wrote no second job")

	// The singular route honours the same key.
	code, body := f.post(t, "anchor", "/api/dispatch-jobs",
		`{"code":"`+f.app+`:billing:invoice:sync","targetUrl":"https://target.test/hook","payload":"{}","serviceAccountId":"sac_x","clientId":"`+f.aID+`","idempotencyKey":"`+key+`"}`)
	require.Equal(t, http.StatusCreated, code, body)
	var created CreatedResponse
	require.NoError(t, json.Unmarshal([]byte(body), &created))
	assert.Equal(t, first[0], created.ID)
	assert.Equal(t, 1, stored())

	// Another client, no client, and another application: their own jobs. Two
	// items of one batch with one key: the first wins, the second answers it.
	other := f.batchIDs(t, "anchor",
		keyed(`,"clientId":"`+f.bID+`"`),
		keyed(""),
		keyed(""),
		`{"code":"`+f.otherApp+`:x:y:z","targetUrl":"https://target.test/hook","payload":"{}","idempotencyKey":"`+key+`"}`,
	)
	assert.NotEqual(t, first[0], other[0], "another client's key is its own")
	assert.NotEqual(t, first[0], other[1], "a client-less job's key is its own")
	assert.Equal(t, other[1], other[2], "a repeat within one batch answers the earlier item")
	assert.NotEqual(t, other[1], other[3], "another application's key is its own")
	assert.Equal(t, 4, stored(), "A, B, no client, other application")
	for _, id := range append(first, other...) {
		j, err := f.repo.FindByID(context.Background(), id)
		require.NoError(t, err)
		if assert.NotNil(t, j, id) && assert.NotNil(t, j.IdempotencyKey) {
			assert.Equal(t, key, *j.IdempotencyKey)
		}
	}

	// A key over 100 characters is refused, not truncated.
	code, body = f.post(t, "anchor", "/api/dispatch-jobs/batch", `{"items":[`+f.item(`"idempotencyKey":"`+strings.Repeat("k", 101)+`"`)+`]}`)
	assert.Equal(t, http.StatusBadRequest, code, body)
	assert.Contains(t, body, "INVALID_IDEMPOTENCY_KEY")
}

// Batch metadata is a list of {key, value}, stored as sent. An object (the
// shape the Laravel SDK sent before 2026-11-05) does not decode, and the
// batch is refused whole.
func TestDispatchJobs_MetadataShape(t *testing.T) {
	f := newJobFixture(t)
	ids := f.batchIDs(t, "anchor", f.item(`"metadata":[{"key":"orderId","value":"o-1"},{"key":"tenant","value":"acme"}]`))
	j, err := f.repo.FindByID(context.Background(), ids[0])
	require.NoError(t, err)
	require.NotNil(t, j)
	assert.Equal(t, []dispatchjob.Metadata{{Key: "orderId", Value: "o-1"}, {Key: "tenant", Value: "acme"}}, j.Metadata)

	code, body := f.post(t, "anchor", "/api/dispatch-jobs/batch", `{"items":[`+f.item(`"metadata":{"orderId":"o-1"}`)+`]}`)
	assert.Equal(t, http.StatusBadRequest, code, body)
	assert.Contains(t, body, "INVALID_JSON")
}
