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

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

type scopeFixture struct {
	srv                  *httptest.Server
	pool                 *pgxpool.Pool
	ownCode, otherCode   string
	ownID, otherID       string
	ownSub, otherSub     string
	audits               *audit.Repository
	scoped, allAppsAdmin *auth.AuthContext
}

func insertApp(t *testing.T, pool *pgxpool.Pool, code string) string {
	t.Helper()
	id := tsid.Generate(tsid.Application)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO app_applications (id, code, name) VALUES ($1, $2, $2)`, id, code)
	require.NoError(t, err)
	return id
}

func insertSub(t *testing.T, pool *pgxpool.Pool, code, appCode string) string {
	t.Helper()
	id := tsid.Generate(tsid.Subscription)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_subscriptions (id, code, application_code, name, target) VALUES ($1, $2, $3, $2, 'https://t.test/h')`,
		id, code, appCode)
	require.NoError(t, err)
	return id
}

// newScopeFixture mounts the dispatch-job and audit routes behind a caller
// chosen per request via the X-Test-Caller header ("scoped" or "admin"), with
// two applications and one subscription each.
func newScopeFixture(t *testing.T) *scopeFixture {
	t.Helper()
	pool := testpg.Pool(t)
	sfx := strings.ToLower(tsid.GenerateUntyped())[5:]
	f := &scopeFixture{pool: pool, audits: audit.NewRepository(pool)}
	f.ownCode, f.otherCode = "own"+sfx, "oth"+sfx
	f.ownID = insertApp(t, pool, f.ownCode)
	f.otherID = insertApp(t, pool, f.otherCode)
	f.ownSub = insertSub(t, pool, "s-own-"+sfx, f.ownCode)
	f.otherSub = insertSub(t, pool, "s-oth-"+sfx, f.otherCode)

	perms := []string{
		"platform:messaging:batch:dispatch-jobs-write",
		"platform:messaging:batch:events-write",
		"platform:admin:batch:audit-logs-write",
	}
	f.scoped = &auth.AuthContext{
		PrincipalID: "p_scoped", Scope: auth.ScopeAnchor,
		Permissions: perms, Applications: []string{f.ownID},
	}
	f.allAppsAdmin = &auth.AuthContext{
		PrincipalID: "p_admin", Scope: auth.ScopeAnchor,
		Permissions: perms, AllApplications: true,
	}

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ac := f.allAppsAdmin
			if req.Header.Get("X-Test-Caller") == "scoped" {
				ac = f.scoped
			}
			next.ServeHTTP(w, req.WithContext(auth.WithContext(req.Context(), ac)))
		})
	})
	RegisterRoutes(r, &DispatchJobsBatchState{
		Repo: dispatchjob.NewRepository(pool),
		Apps: application.NewRepository(pool),
		Subs: subscription.NewRepository(pool),
	})
	RegisterAuditRoutes(r, &AuditBatchState{Repo: f.audits, Apps: application.NewRepository(pool), Clients: client.NewRepository(pool)})
	f.srv = httptest.NewServer(r)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *scopeFixture) post(t *testing.T, caller, path, body string) (int, string) {
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

func djItem(code, extra string) string {
	return `{"code":"` + code + `","targetUrl":"https://target.test/hook","payload":"{}","serviceAccountId":"sa_x"` + extra + `}`
}

func TestAppScope_DispatchJobsBatch(t *testing.T) {
	f := newScopeFixture(t)
	const path = "/api/dispatch-jobs/batch"

	// Own-app job (and own-app subscription) accepted.
	code, body := f.post(t, "scoped", path, `{"items":[`+djItem(f.ownCode+":a:b:c", "")+`,`+djItem(f.ownCode+":a:b:d", `,"subscriptionId":"`+f.ownSub+`"`)+`]}`)
	require.Equal(t, http.StatusCreated, code, body)

	// Mixed batch: the other-app item rejects the WHOLE batch, naming its index.
	code, body = f.post(t, "scoped", path, `{"items":[`+djItem(f.ownCode+":a:b:c", "")+`,`+djItem(f.otherCode+":a:b:c", "")+`]}`)
	assert.Equal(t, http.StatusForbidden, code, body)
	assert.Contains(t, body, "items[1]")

	// Own code but another application's subscription: refused.
	code, body = f.post(t, "scoped", path, `{"items":[`+djItem(f.ownCode+":a:b:c", `,"subscriptionId":"`+f.otherSub+`"`)+`]}`)
	assert.Equal(t, http.StatusForbidden, code, body)
	assert.Contains(t, body, "items[0]")

	// Unknown application code and unknown subscription: refused.
	code, _ = f.post(t, "scoped", path, `{"items":[`+djItem("nosuchapp:a:b:c", "")+`]}`)
	assert.Equal(t, http.StatusForbidden, code)
	code, _ = f.post(t, "scoped", path, `{"items":[`+djItem(f.ownCode+":a:b:c", `,"subscriptionId":"sub_missing"`)+`]}`)
	assert.Equal(t, http.StatusForbidden, code)

	// Non-scoped caller unaffected: other app, unknown app, foreign subscription.
	code, body = f.post(t, "admin", path, `{"items":[`+djItem(f.otherCode+":a:b:c", `,"subscriptionId":"`+f.ownSub+`"`)+`,`+djItem("nosuchapp:a:b:c", "")+`]}`)
	assert.Equal(t, http.StatusCreated, code, body)
}

func TestAppScope_DispatchJobSingular(t *testing.T) {
	f := newScopeFixture(t)
	code, body := f.post(t, "scoped", "/api/dispatch-jobs", djItem(f.ownCode+":a:b:c", ""))
	assert.Equal(t, http.StatusCreated, code, body)
	code, _ = f.post(t, "scoped", "/api/dispatch-jobs", djItem(f.otherCode+":a:b:c", ""))
	assert.Equal(t, http.StatusForbidden, code)
	code, _ = f.post(t, "admin", "/api/dispatch-jobs", djItem(f.otherCode+":a:b:c", ""))
	assert.Equal(t, http.StatusCreated, code)
}

func TestAppScope_AuditBatch(t *testing.T) {
	f := newScopeFixture(t)
	const path = "/api/audit-logs/batch"
	item := func(app, eid string) string {
		a := ""
		if app != "" {
			a = `,"applicationCode":"` + app + `"`
		}
		return `{"entityType":"widget","entityId":"` + eid + `","operation":"CREATE","principalId":"prn_x"` + a + `}`
	}
	sfx := strings.ToLower(tsid.GenerateUntyped())

	// Mixed batch: own ok, other refused, no applicationCode refused — positional.
	code, body := f.post(t, "scoped", path, `{"items":[`+
		item(f.ownCode, "a1"+sfx)+`,`+item(f.otherCode, "a2"+sfx)+`,`+item("", "a3"+sfx)+`,`+item("nosuchapp", "a4"+sfx)+`]}`)
	require.Equal(t, http.StatusOK, code, body)
	var resp BatchResponse
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	require.Len(t, resp.Results, 4)
	assert.Equal(t, "SUCCESS", resp.Results[0].Status)
	assert.NotEmpty(t, resp.Results[0].ID)
	for i := 1; i < 4; i++ {
		assert.Equal(t, "BAD_REQUEST", resp.Results[i].Status, "item %d", i)
		assert.NotEmpty(t, resp.Results[i].Error)
	}
	logs, err := f.audits.FindWithFilters(context.Background(), audit.FilterParams{EntityID: new("a2" + sfx), Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, logs)

	// Non-scoped caller unaffected: other app OK, no code OK, unknown code stays SKIPPED.
	code, body = f.post(t, "admin", path, `{"items":[`+
		item(f.otherCode, "b1"+sfx)+`,`+item("", "b2"+sfx)+`,`+item("nosuchapp", "b3"+sfx)+`]}`)
	require.Equal(t, http.StatusOK, code, body)
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	require.Len(t, resp.Results, 3)
	assert.Equal(t, "SUCCESS", resp.Results[0].Status)
	assert.Equal(t, "SUCCESS", resp.Results[1].Status)
	assert.Equal(t, "SKIPPED", resp.Results[2].Status)
}
