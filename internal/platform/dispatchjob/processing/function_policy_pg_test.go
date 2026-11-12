//go:build integration

package processing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// A job that is not a function's target is delivered under the customer
// policy, which refuses loopback and carries no exemption, even when the
// destination is exactly what the internal policy exempts as the runner. A
// function-linked job to the same server is delivered. The job rows are
// inserted directly, as if they predated the policy split.
func TestProcess_CustomerTargetOnTheRunnerAddressIsRefused_FunctionTargetIsDelivered(t *testing.T) {
	pool := testpg.Pool(t)

	var hits atomic.Int32
	runner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(runner.Close)
	u, err := url.Parse(runner.URL)
	require.NoError(t, err)

	customer := &netguard.Policy{} // production strictness, no exemptions
	internal := &netguard.Policy{}
	internal.AllowURL(runner.URL) // what ApplyDeliveryPolicy does for the runner

	auth := scheduler.NewDispatchAuthService(testSecret)
	h := processing.New(dispatchjob.NewRepository(pool), auth).
		WithDeliveryPolicies(customer, internal).
		WithFunctionTargetResolver(func(_ context.Context, job *dispatchjob.DispatchJob) bool {
			return job.Code == "fnpolicy:function"
		})
	r := chi.NewRouter()
	h.Mount(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	seed := func(id, code string) {
		_, err := pool.Exec(context.Background(),
			`INSERT INTO msg_dispatch_jobs (id, code, target_url, status, data_only, payload, max_retries, attempt_count)
			 VALUES ($1, $2, $3, 'QUEUED', FALSE, '{}', 3, 0)`, id, code, "http://"+u.Host+"/fn/x")
		require.NoError(t, err)
	}

	seed("djfnp_cust01", "fnpolicy:customer")
	code, _ := callProcess(t, ts.URL, "djfnp_cust01", auth.Sign("djfnp_cust01"))
	assert.Equal(t, http.StatusOK, code)
	assert.EqualValues(t, 0, hits.Load(), "a customer target must not reach the runner address")
	var msg string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT COALESCE(error_message,'') FROM msg_dispatch_job_attempts WHERE dispatch_job_id = $1`, "djfnp_cust01").Scan(&msg))
	assert.Contains(t, msg, "destination not allowed", "the attempt records the guard's refusal")
	status, _, _ := jobRow(t, pool, "djfnp_cust01")
	assert.NotEqual(t, "COMPLETED", status)

	seed("djfnp_func01", "fnpolicy:function")
	code, _ = callProcess(t, ts.URL, "djfnp_func01", auth.Sign("djfnp_func01"))
	assert.Equal(t, http.StatusOK, code)
	assert.EqualValues(t, 1, hits.Load(), "a function target is delivered through the internal policy")
	status, _, _ = jobRow(t, pool, "djfnp_func01")
	assert.Equal(t, "COMPLETED", status)
}
