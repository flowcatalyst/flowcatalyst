//go:build integration

package settled_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/settled"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

const testSecret = "settled-test-secret"

// seedJob inserts a job in the given status directly into the write table —
// mirrors processing_pg_test.go's seedJob helper.
func seedJob(t *testing.T, pool *pgxpool.Pool, id, status string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_dispatch_jobs (id, code, target_url, status, data_only, payload, max_retries)
		 VALUES ($1, 'settled:test:evt', 'http://example.invalid/hook', $2, FALSE, '{}', 3)`,
		id, status)
	require.NoError(t, err)
}

func statusAndLastError(t *testing.T, pool *pgxpool.Pool, id string) (string, *string) {
	t.Helper()
	var status string
	var lastError *string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status, last_error FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&status, &lastError))
	return status, lastError
}

// harness wires the handler behind httptest and returns the base URL + the
// auth service the router would sign tokens with.
func harness(t *testing.T, pool *pgxpool.Pool) (string, *scheduler.DispatchAuthService) {
	t.Helper()
	authSvc := scheduler.NewDispatchAuthService(testSecret)
	h := settled.New(dispatchjob.NewRepository(pool), authSvc)
	r := chi.NewRouter()
	h.Mount(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts.URL, authSvc
}

type settledJobWire struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

func postSettled(t *testing.T, baseURL, reason string, jobs []settledJobWire) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"reason": reason, "jobs": jobs})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/dispatch/settled", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// TestServe_HappyPath is the hook's core contract: a QUEUED job with a
// validly-signed token is reset to PENDING and the reason is recorded.
func TestServe_HappyPath(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	baseURL, authSvc := harness(t, pool)

	id := tsid.GenerateUntyped()
	seedJob(t, pool, id, "QUEUED")

	code, body := postSettled(t, baseURL, "head failed under BLOCK_ON_ERROR",
		[]settledJobWire{{ID: id, Token: authSvc.Sign(id)}})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, float64(1), body["settled"])

	status, lastError := statusAndLastError(t, pool, id)
	assert.Equal(t, "PENDING", status)
	require.NotNil(t, lastError)
	assert.Equal(t, "head failed under BLOCK_ON_ERROR", *lastError)
}

// TestServe_BadToken proves a forged/garbage token cannot settle a job — the
// hook self-verifies exactly like the processing endpoint, and a router
// impersonator must not be able to un-block a group's ordering by request.
func TestServe_BadToken(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	baseURL, _ := harness(t, pool)

	id := tsid.GenerateUntyped()
	seedJob(t, pool, id, "QUEUED")

	code, body := postSettled(t, baseURL, "forged", []settledJobWire{{ID: id, Token: "not-a-real-token"}})
	assert.Equal(t, http.StatusUnauthorized, code)
	assert.Equal(t, float64(0), body["settled"])

	status, _ := statusAndLastError(t, pool, id)
	assert.Equal(t, "QUEUED", status, "an unverified job must not be touched")
}

// TestServe_PartialBatch proves one bad token in a batch doesn't sink the
// rest — the router built the batch from one group's buffered messages, and
// a single corrupt entry must not block settling the others.
func TestServe_PartialBatch(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	baseURL, authSvc := harness(t, pool)

	good := tsid.GenerateUntyped()
	bad := tsid.GenerateUntyped()
	seedJob(t, pool, good, "QUEUED")
	seedJob(t, pool, bad, "QUEUED")

	code, body := postSettled(t, baseURL, "partial", []settledJobWire{
		{ID: good, Token: authSvc.Sign(good)},
		{ID: bad, Token: "garbage"},
	})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, float64(1), body["settled"])

	goodStatus, _ := statusAndLastError(t, pool, good)
	assert.Equal(t, "PENDING", goodStatus)
	badStatus, _ := statusAndLastError(t, pool, bad)
	assert.Equal(t, "QUEUED", badStatus, "the badly-tokened job in the same batch must be left untouched")
}

// TestServe_TerminalStatusIgnored proves the hook is idempotent /
// non-destructive against a job a concurrent path already advanced past
// QUEUED/PROCESSING (e.g. the reaper got there first, or the job somehow
// already completed) — it must not resurrect a terminal job back to
// PENDING.
func TestServe_TerminalStatusIgnored(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	baseURL, authSvc := harness(t, pool)

	id := tsid.GenerateUntyped()
	seedJob(t, pool, id, "COMPLETED")

	code, body := postSettled(t, baseURL, "late", []settledJobWire{{ID: id, Token: authSvc.Sign(id)}})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, float64(0), body["settled"])

	status, _ := statusAndLastError(t, pool, id)
	assert.Equal(t, "COMPLETED", status)
}

// TestServe_EmptyBatch proves an empty jobs array is accepted as a no-op
// (the router may call this with nothing to settle) rather than treated as
// malformed input.
func TestServe_EmptyBatch(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	baseURL, _ := harness(t, pool)

	code, body := postSettled(t, baseURL, "", []settledJobWire{})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, float64(0), body["settled"])
}
