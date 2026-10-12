//go:build integration

package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// clientCodeFixture mounts the dispatch-job routes behind a caller chosen per
// request (X-Test-Caller: "anchor" or "client"), with two clients: the
// client-scoped caller reaches A only.
type clientCodeFixture struct {
	srv              *httptest.Server
	pool             *pgxpool.Pool
	aID, aCode       string
	bID, bCode       string
	anchor, clientAC *auth.AuthContext
}

func insertClient(t *testing.T, pool *pgxpool.Pool, identifier string) string {
	t.Helper()
	id := tsid.Generate(tsid.Client)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tnt_clients (id, name, identifier) VALUES ($1, $2, $2)`, id, identifier)
	require.NoError(t, err)
	return id
}

func newClientCodeFixture(t *testing.T) *clientCodeFixture {
	t.Helper()
	pool := testpg.Pool(t)
	sfx := strings.ToLower(tsid.GenerateUntyped())
	f := &clientCodeFixture{pool: pool, aCode: "acme-" + sfx, bCode: "beta-" + sfx}
	f.aID = insertClient(t, pool, f.aCode)
	f.bID = insertClient(t, pool, f.bCode)
	perms := []string{"platform:messaging:batch:dispatch-jobs-write"}
	f.anchor = &auth.AuthContext{PrincipalID: "p_cc_anchor", Scope: auth.ScopeAnchor, Permissions: perms, AllApplications: true}
	f.clientAC = &auth.AuthContext{
		PrincipalID: "p_cc_client", Scope: auth.ScopeClient, Clients: []string{f.aID},
		Permissions: perms, AllApplications: true,
	}

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ac := f.anchor
			if req.Header.Get("X-Test-Caller") == "client" {
				ac = f.clientAC
			}
			next.ServeHTTP(w, req.WithContext(auth.WithContext(req.Context(), ac)))
		})
	})
	RegisterRoutes(r, &DispatchJobsBatchState{Repo: dispatchjob.NewRepository(pool), Clients: client.NewRepository(pool)})
	f.srv = httptest.NewServer(r)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *clientCodeFixture) post(t *testing.T, caller, path, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, f.srv.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Caller", caller)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if rerr != nil {
			break
		}
	}
	return resp.StatusCode, sb.String()
}

// storedClient answers whether job id was written, and its client_id.
func (f *clientCodeFixture) storedClient(t *testing.T, id string) (bool, *string) {
	t.Helper()
	var n int
	require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT count(*) FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&n))
	if n == 0 {
		return false, nil
	}
	var cid *string
	require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT client_id FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&cid))
	return true, cid
}

func envelope(t *testing.T, body string) (string, string) {
	t.Helper()
	var env struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &env), body)
	return env.Error, env.Message
}

// ccItem is a batch item with a fixed id (so the test can look the row up)
// and the given client members.
func ccItem(id, clientMembers string) string {
	return `{"id":"` + id + `","code":"cc:a:b:c","targetUrl":"https://target.test/hook","payload":"{}"` + clientMembers + `}`
}

// F14 (owner ruling 2026-10-12): POST /api/dispatch-jobs/batch and POST
// /api/dispatch-jobs accept clientCode. An explicit clientId wins; a known
// code resolves to its client and passes the client guard; an unknown code is
// refused for every caller (400 CLIENT_NOT_FOUND for an anchor, the same 403
// as an inaccessible client for anyone else), never stored unscoped.
func TestDispatchJobs_ClientCode(t *testing.T) {
	f := newClientCodeFixture(t)
	const batch = "/api/dispatch-jobs/batch"
	code := func(c string) string { return `,"clientCode":"` + c + `"` }

	// Anchor: a known code resolves.
	id := tsid.GenerateUntyped()
	status, body := f.post(t, "anchor", batch, `{"items":[`+ccItem(id, code(f.aCode))+`]}`)
	require.Equal(t, http.StatusCreated, status, body)
	ok, cid := f.storedClient(t, id)
	require.True(t, ok)
	require.NotNil(t, cid)
	assert.Equal(t, f.aID, *cid)

	// An explicit clientId wins over the code.
	id = tsid.GenerateUntyped()
	status, body = f.post(t, "anchor", batch, `{"items":[`+ccItem(id, `,"clientId":"`+f.bID+`"`+code(f.aCode))+`]}`)
	require.Equal(t, http.StatusCreated, status, body)
	_, cid = f.storedClient(t, id)
	require.NotNil(t, cid)
	assert.Equal(t, f.bID, *cid)

	// A blank code is no code.
	id = tsid.GenerateUntyped()
	status, body = f.post(t, "anchor", batch, `{"items":[`+ccItem(id, code(""))+`]}`)
	require.Equal(t, http.StatusCreated, status, body)
	_, cid = f.storedClient(t, id)
	assert.Nil(t, cid)

	// Anchor, unknown code: refused, the whole batch, nothing written.
	good, bad := tsid.GenerateUntyped(), tsid.GenerateUntyped()
	status, body = f.post(t, "anchor", batch, `{"items":[`+ccItem(good, code(f.aCode))+`,`+ccItem(bad, code("nosuch-client"))+`]}`)
	require.Equal(t, http.StatusBadRequest, status, body)
	errCode, msg := envelope(t, body)
	assert.Equal(t, "CLIENT_NOT_FOUND", errCode)
	assert.Equal(t, "Client not found: nosuch-client", msg)
	ok, _ = f.storedClient(t, good)
	assert.False(t, ok, "a refused batch writes nothing")

	// Client-scoped caller: its own client's code resolves.
	id = tsid.GenerateUntyped()
	status, body = f.post(t, "client", batch, `{"items":[`+ccItem(id, code(f.aCode))+`]}`)
	require.Equal(t, http.StatusCreated, status, body)
	_, cid = f.storedClient(t, id)
	require.NotNil(t, cid)
	assert.Equal(t, f.aID, *cid)

	// Another client's code and an unknown code: the same 403, naming the
	// code (never the other client's id).
	for _, c := range []string{f.bCode, "nosuch-client"} {
		id = tsid.GenerateUntyped()
		status, body = f.post(t, "client", batch, `{"items":[`+ccItem(id, code(c))+`]}`)
		require.Equal(t, http.StatusForbidden, status, body)
		errCode, msg = envelope(t, body)
		assert.Equal(t, "FORBIDDEN", errCode)
		assert.Equal(t, "No access to client: "+c, msg)
		ok, _ = f.storedClient(t, id)
		assert.False(t, ok)
	}

	// Singular create: same rules.
	single := func(clientMembers string) string {
		return `{"code":"cc:a:b:c","targetUrl":"https://target.test/hook","payload":"{}","serviceAccountId":"sa_x"` + clientMembers + `}`
	}
	status, body = f.post(t, "anchor", "/api/dispatch-jobs", single(code(f.aCode)))
	require.Equal(t, http.StatusCreated, status, body)
	var created CreatedResponse
	require.NoError(t, json.Unmarshal([]byte(body), &created))
	_, cid = f.storedClient(t, created.ID)
	require.NotNil(t, cid)
	assert.Equal(t, f.aID, *cid)

	status, body = f.post(t, "anchor", "/api/dispatch-jobs", single(code("nosuch-client")))
	require.Equal(t, http.StatusBadRequest, status, body)
	errCode, msg = envelope(t, body)
	assert.Equal(t, "CLIENT_NOT_FOUND", errCode)
	assert.Equal(t, "Client not found: nosuch-client", msg)

	status, body = f.post(t, "client", "/api/dispatch-jobs", single(code(f.bCode)))
	require.Equal(t, http.StatusForbidden, status, body)
	_, msg = envelope(t, body)
	assert.Equal(t, "No access to client: "+f.bCode, msg)

	status, body = f.post(t, "client", "/api/dispatch-jobs", single(`,"clientId":"`+f.aID+`"`+code(f.bCode)))
	require.Equal(t, http.StatusCreated, status, body, "an explicit clientId wins over the code")
}
