//go:build integration

package settled_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/settled"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

func settledAuthCount(t *testing.T, name string) float64 {
	t.Helper()
	mfs, err := dispatchjob.MetricsRegistry.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "endpoint" && l.GetValue() == dispatchjob.AuthEndpointSettled {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	t.Fatalf("metric %s{endpoint=settled} not registered", name)
	return 0
}

// Not parallel: it reads process-wide counters, so no other test of this
// package may move them meanwhile (parallel tests wait for the serial ones).
func TestServe_TokenSignedBeforeAKeyRotation(t *testing.T) {
	pool := testpg.Pool(t)
	serve := func(v *scheduler.DispatchAuthService) string {
		r := chi.NewRouter()
		settled.New(dispatchjob.NewRepository(pool), v).Mount(r)
		ts := httptest.NewServer(r)
		t.Cleanup(ts.Close)
		return ts.URL
	}
	id := tsid.GenerateUntyped()
	seedJob(t, pool, id, "QUEUED")
	oldToken := scheduler.NewDispatchAuthService("old-derived").Sign(id)

	rejBefore := settledAuthCount(t, "fc_dispatch_job_auth_rejected_total")
	code, _ := postSettled(t, serve(scheduler.NewDispatchAuthService("new-derived")), "r", []settledJobWire{{ID: id, Token: oldToken}})
	assert.Equal(t, http.StatusUnauthorized, code)
	assert.Equal(t, rejBefore+1, settledAuthCount(t, "fc_dispatch_job_auth_rejected_total"))

	prevBefore := settledAuthCount(t, "fc_dispatch_job_auth_previous_key_total")
	code, body := postSettled(t, serve(scheduler.NewDispatchAuthService("new-derived", "old-derived")), "r",
		[]settledJobWire{{ID: id, Token: oldToken}})
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, float64(1), body["settled"])
	assert.Equal(t, prevBefore+1, settledAuthCount(t, "fc_dispatch_job_auth_previous_key_total"))
}
