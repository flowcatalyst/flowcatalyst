//go:build integration

package processing_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// authCount reads one fc_dispatch_job_auth_* counter for the process endpoint.
func authCount(t *testing.T, name string) float64 {
	t.Helper()
	mfs, err := dispatchjob.MetricsRegistry.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "endpoint" && l.GetValue() == dispatchjob.AuthEndpointProcess {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	t.Fatalf("metric %s{endpoint=process} not registered", name)
	return 0
}

// A job token signed before FLOWCATALYST_APP_KEY was rotated (still on the
// queue, or signed by a scheduler not yet redeployed) is accepted while the
// old key is listed as previous, and counted; without the old key it is
// rejected, and the rejection is counted instead of only looping silently.
func TestProcess_TokenSignedBeforeAKeyRotation(t *testing.T) {
	pool := testpg.Pool(t)
	serve := func(v *scheduler.DispatchAuthService) string {
		r := chi.NewRouter()
		processing.New(dispatchjob.NewRepository(pool), v).Mount(r)
		ts := httptest.NewServer(r)
		t.Cleanup(ts.Close)
		return ts.URL
	}
	oldToken := scheduler.NewDispatchAuthService("old-derived").Sign("djproc_rot_missing")

	rotated := serve(scheduler.NewDispatchAuthService("new-derived", "old-derived"))
	prevBefore := authCount(t, "fc_dispatch_job_auth_previous_key_total")
	code, out := callProcess(t, rotated, "djproc_rot_missing", oldToken)
	assert.Equal(t, http.StatusOK, code, "authenticated: the (absent) job is acked, not refused")
	assert.Equal(t, "job not found", out["message"])
	assert.Equal(t, prevBefore+1, authCount(t, "fc_dispatch_job_auth_previous_key_total"))

	newOnly := serve(scheduler.NewDispatchAuthService("new-derived"))
	rejBefore := authCount(t, "fc_dispatch_job_auth_rejected_total")
	code, _ = callProcess(t, newOnly, "djproc_rot_missing", oldToken)
	assert.Equal(t, http.StatusUnauthorized, code)
	assert.Equal(t, rejBefore+1, authCount(t, "fc_dispatch_job_auth_rejected_total"))
}
