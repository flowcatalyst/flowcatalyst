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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// newIngestServer mounts RegisterRoutes behind a middleware that injects
// the given AuthContext — the same way the real chi auth middleware does.
func newIngestServer(t *testing.T, ac *auth.AuthContext) (*httptest.Server, *dispatchjob.Repository) {
	t.Helper()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(auth.WithContext(req.Context(), ac)))
		})
	})
	RegisterRoutes(r, &DispatchJobsBatchState{Repo: repo})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, repo
}

func anchorAC() *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "p_dj_test", Scope: auth.ScopeAnchor}
}

func postJSON(t *testing.T, url, body string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
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
	return resp, sb.String()
}

// TestCreateDispatchJob_PersistsAndMatchesBatchOfOne pins that POST
// /api/dispatch-jobs persists a job retrievable via the repository, with
// the identical field shape a batch-of-1 with the same inputs produces
// (both run through jobFromItem + InsertBatch).
func TestCreateDispatchJob_PersistsAndMatchesBatchOfOne(t *testing.T) {
	srv, repo := newIngestServer(t, anchorAC())
	ctx := context.Background()

	const itemFields = `
		"kind": "EVENT",
		"code": "it:singular:dispatch:created",
		"source": "test://dj-singular",
		"subject": "dj-subj",
		"eventId": "evtdjsingle01",
		"correlationId": "corr-dj-1",
		"targetUrl": "https://target.test/hook",
		"payload": "{\"k\":\"v\"}",
		"serviceAccountId": "sadjsingle001",
		"subscriptionId": "subdjsingle01",
		"messageGroup": "mg-dj-1"`

	// Singular create — carries the singular-only fields too.
	resp, body := postJSON(t, srv.URL+"/api/dispatch-jobs", `{`+itemFields+`,
		"retryStrategy": "FIXED_DELAY",
		"idempotencyKey": "idem-dj-1",
		"metadata": {"b": "2", "a": "1"}
	}`)
	require.Equal(t, http.StatusCreated, resp.StatusCode, body)
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &created))
	require.NotEmpty(t, created.ID)

	single, err := repo.FindByID(ctx, created.ID)
	require.NoError(t, err)
	require.NotNil(t, single, "singular POST must persist a retrievable job")

	// Batch-of-1 with the same item fields.
	resp, body = postJSON(t, srv.URL+"/api/dispatch-jobs/batch", `{"items":[{`+itemFields+`}]}`)
	require.Equal(t, http.StatusCreated, resp.StatusCode, body)
	var bres struct {
		Results []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &bres))
	require.Len(t, bres.Results, 1)
	require.Equal(t, "SUCCESS", bres.Results[0].Status)

	batch, err := repo.FindByID(ctx, bres.Results[0].ID)
	require.NoError(t, err)
	require.NotNil(t, batch)

	// Identical shape, modulo identity/timestamps and the singular-only
	// fields the batch contract cannot carry.
	single.ID, batch.ID = "", ""
	single.CreatedAt = batch.CreatedAt
	single.UpdatedAt = batch.UpdatedAt
	single.IdempotencyKey = nil // singular-only
	single.Metadata = batch.Metadata
	single.RetryStrategy = batch.RetryStrategy // singular-only knob
	assert.Equal(t, batch, single,
		"singular create must persist the identical job shape as a batch-of-1")

	// Shared ingest defaults — same for both paths.
	assert.Equal(t, common.DispatchPending, batch.Status)
	assert.Equal(t, int32(99), batch.Sequence)
	assert.Equal(t, uint32(30), batch.TimeoutSeconds)
	assert.Equal(t, uint32(3), batch.MaxRetries)
	assert.Equal(t, "application/json", batch.PayloadContentType)
	assert.Equal(t, dispatchjob.ProtocolHTTPWebhook, batch.Protocol)
}

// TestCreateDispatchJob_SingularOnlyFields pins the fields only the
// singular contract carries: retryStrategy, idempotencyKey, metadata map.
func TestCreateDispatchJob_SingularOnlyFields(t *testing.T) {
	srv, repo := newIngestServer(t, anchorAC())

	resp, body := postJSON(t, srv.URL+"/api/dispatch-jobs", `{
		"code": "it:singular:dispatch:extras",
		"targetUrl": "https://target.test/hook",
		"payload": "{}",
		"serviceAccountId": "sa_dj_extras",
		"retryStrategy": "FIXED_DELAY",
		"idempotencyKey": "idem-dj-2",
		"metadata": {"b": "2", "a": "1"},
		"sequence": 7
	}`)
	require.Equal(t, http.StatusCreated, resp.StatusCode, body)
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &created))

	job, err := repo.FindByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, dispatchjob.RetryFixed, job.RetryStrategy)
	require.NotNil(t, job.IdempotencyKey)
	assert.Equal(t, "idem-dj-2", *job.IdempotencyKey)
	assert.Equal(t, []dispatchjob.Metadata{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}}, job.Metadata)
	assert.Equal(t, int32(7), job.Sequence)
	assert.Equal(t, dispatchjob.KindEvent, job.Kind, "kind defaults to EVENT")
}

// TestCreateDispatchJob_BadPayloadEnvelopeMatchesBatch pins that the
// singular endpoint rejects bad payloads with the same {error, message}
// envelope (and 400 status) as the batch endpoint.
func TestCreateDispatchJob_BadPayloadEnvelopeMatchesBatch(t *testing.T) {
	srv, _ := newIngestServer(t, anchorAC())

	type envelope struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}

	// Malformed JSON: identical envelope code on both endpoints.
	resp, body := postJSON(t, srv.URL+"/api/dispatch-jobs", `{not-json`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, body)
	var senv envelope
	require.NoError(t, json.Unmarshal([]byte(body), &senv))
	assert.Equal(t, "INVALID_JSON", senv.Error)

	resp, body = postJSON(t, srv.URL+"/api/dispatch-jobs/batch", `{not-json`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, body)
	var benv envelope
	require.NoError(t, json.Unmarshal([]byte(body), &benv))
	assert.Equal(t, senv.Error, benv.Error, "singular and batch must share the error envelope code")

	// Missing required field (Rust serde-required) → 400 VALIDATION.
	resp, body = postJSON(t, srv.URL+"/api/dispatch-jobs", `{
		"code": "it:singular:dispatch:bad",
		"payload": "{}",
		"serviceAccountId": "sa_dj_bad"
	}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, body)
	require.NoError(t, json.Unmarshal([]byte(body), &senv))
	assert.Equal(t, "VALIDATION", senv.Error)
	assert.Contains(t, senv.Message, "targetUrl")
}

// TestCreateDispatchJob_TenantGuard pins the 403 for a clientId outside
// the caller's tenants — the same guard the batch loop applies.
func TestCreateDispatchJob_TenantGuard(t *testing.T) {
	srv, _ := newIngestServer(t, &auth.AuthContext{
		PrincipalID: "p_dj_client",
		Scope:       auth.ScopeClient,
		Clients:     []string{"clt_djmine"},
		Permissions: []string{"WRITE_DISPATCH_JOBS"},
	})

	resp, body := postJSON(t, srv.URL+"/api/dispatch-jobs", `{
		"code": "it:singular:dispatch:tenant",
		"targetUrl": "https://target.test/hook",
		"payload": "{}",
		"serviceAccountId": "sa_dj_tenant",
		"clientId": "clt_djtheirs"
	}`)
	require.Equal(t, http.StatusForbidden, resp.StatusCode, body)
	assert.Contains(t, body, `"error":"FORBIDDEN"`)
	assert.Contains(t, body, "No access to client")
}
