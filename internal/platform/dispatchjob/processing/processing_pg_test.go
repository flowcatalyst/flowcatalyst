//go:build integration

package processing_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

const testSecret = "process-test-secret"

// seedJob inserts a QUEUED write-side job. data_only=false so the delivery
// carries the CloudEvents envelope.
func seedJob(t *testing.T, pool *pgxpool.Pool, id, targetURL string, maxRetries, attemptCount int) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_dispatch_jobs
		     (id, code, target_url, status, data_only, payload, max_retries, attempt_count)
		 VALUES ($1, 'proc:test:evt', $2, 'QUEUED', FALSE, '{"hello":"world"}', $3, $4)`,
		id, targetURL, maxRetries, attemptCount)
	require.NoError(t, err)
}

func jobRow(t *testing.T, pool *pgxpool.Pool, id string) (status string, attempts int32, scheduledFor *time.Time) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT status, attempt_count, scheduled_for FROM msg_dispatch_jobs WHERE id = $1`, id).
		Scan(&status, &attempts, &scheduledFor)
	require.NoError(t, err)
	return
}

func attemptCount(t *testing.T, pool *pgxpool.Pool, id string) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM msg_dispatch_job_attempts WHERE dispatch_job_id = $1`, id).Scan(&n))
	return n
}

// callProcess posts {messageId} to the mounted handler with a signed bearer
// (unless token is overridden) and returns the HTTP status + decoded body.
func callProcess(t *testing.T, baseURL, jobID, token string) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"messageId": jobID})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/dispatch/process", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// harness wires the handler behind httptest and returns the base URL + auth.
func harness(t *testing.T, pool *pgxpool.Pool) (string, *scheduler.DispatchAuthService) {
	t.Helper()
	auth := scheduler.NewDispatchAuthService(testSecret)
	h := processing.New(dispatchjob.NewRepository(pool), auth)
	r := chi.NewRouter()
	h.Mount(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts.URL, auth
}

func TestProcess_Success(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harness(t, pool)

	var gotBody atomic.Value
	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody.Store(string(b))
		assert.Equal(t, "djproc_ok01", r.Header.Get("X-Dispatch-Job-Id"))
		assert.Equal(t, "proc:test:evt", r.Header.Get("X-Event-Type"))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(sub.Close)

	seedJob(t, pool, "djproc_ok01", sub.URL, 3, 0)
	code, out := callProcess(t, base, "djproc_ok01", auth.Sign("djproc_ok01"))

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, out["ack"])

	status, _, _ := jobRow(t, pool, "djproc_ok01")
	assert.Equal(t, "COMPLETED", status)
	assert.Equal(t, 1, attemptCount(t, pool, "djproc_ok01"))

	// The delivered body is the CloudEvents envelope, not the {messageId} blob.
	var env map[string]any
	require.NoError(t, json.Unmarshal([]byte(gotBody.Load().(string)), &env))
	assert.Equal(t, "djproc_ok01", env["id"])
	assert.Equal(t, "proc:test:evt", env["type"])
}

func TestProcess_RetryableFailureSchedulesBackoff(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harness(t, pool)

	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(sub.Close)

	// attempt 1 of 3 → retry, not terminal.
	seedJob(t, pool, "djproc_retry1", sub.URL, 3, 0)
	code, out := callProcess(t, base, "djproc_retry1", auth.Sign("djproc_retry1"))
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, out["ack"], "poller owns retries, so the endpoint always acks")

	status, attempts, scheduled := jobRow(t, pool, "djproc_retry1")
	assert.Equal(t, "PENDING", status)
	assert.EqualValues(t, 1, attempts, "attempt budget consumed")
	require.NotNil(t, scheduled)
	assert.True(t, scheduled.After(time.Now()), "backoff scheduled in the future")
	assert.Equal(t, 1, attemptCount(t, pool, "djproc_retry1"))
}

func TestProcess_ExhaustedRetriesFails(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harness(t, pool)

	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(sub.Close)

	// max_retries=2, already 1 attempt → this attempt is #2 == max → terminal.
	seedJob(t, pool, "djproc_fail1", sub.URL, 2, 1)
	code, _ := callProcess(t, base, "djproc_fail1", auth.Sign("djproc_fail1"))
	assert.Equal(t, http.StatusOK, code)

	status, _, _ := jobRow(t, pool, "djproc_fail1")
	assert.Equal(t, "FAILED", status)
}

func TestProcess_Deferral429DoesNotSpendBudget(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harness(t, pool)

	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(sub.Close)

	seedJob(t, pool, "djproc_429", sub.URL, 3, 0)
	callProcess(t, base, "djproc_429", auth.Sign("djproc_429"))

	status, attempts, scheduled := jobRow(t, pool, "djproc_429")
	assert.Equal(t, "PENDING", status)
	assert.EqualValues(t, 0, attempts, "429 is back-pressure, not a failed attempt")
	require.NotNil(t, scheduled)
	assert.True(t, scheduled.After(time.Now()))
}

func TestProcess_BadTokenUnauthorized(t *testing.T) {
	pool := testpg.Pool(t)
	base, _ := harness(t, pool)

	seedJob(t, pool, "djproc_auth", "http://example.invalid/hook", 3, 0)
	code, out := callProcess(t, base, "djproc_auth", "not-a-valid-token")
	assert.Equal(t, http.StatusUnauthorized, code)
	assert.Equal(t, false, out["ack"], "forged callback is not acked")

	// Job untouched — never marked PROCESSING.
	status, _, _ := jobRow(t, pool, "djproc_auth")
	assert.Equal(t, "QUEUED", status)
}

func TestProcess_AlreadyTerminalAcksWithoutRedelivery(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harness(t, pool)

	var hits int32
	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(sub.Close)

	seedJob(t, pool, "djproc_term", sub.URL, 3, 0)
	_, err := pool.Exec(context.Background(),
		`UPDATE msg_dispatch_jobs SET status = 'COMPLETED' WHERE id = 'djproc_term'`)
	require.NoError(t, err)

	code, out := callProcess(t, base, "djproc_term", auth.Sign("djproc_term"))
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, out["ack"])
	assert.EqualValues(t, 0, atomic.LoadInt32(&hits), "terminal job is not re-delivered")
}
