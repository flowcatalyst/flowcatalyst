package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The router-config document briefly lived on the internal listener,
// unauthenticated, reached through a network alias. It is now an ordinary
// authenticated route on the API listener, so this listener must NOT answer
// for it — otherwise the credential is decorative and the document is still
// readable by anything that can reach the metrics port.
func TestMetricsListenerDoesNotServeTheRouterConfig(t *testing.T) {
	h := metricsRouter(EnvCfg{PlatformEnabled: true})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/dispatch/router-config", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/dispatch/router-config on the internal listener = %d, want 404", rec.Code)
	}
}

// What it does serve is unchanged.
func TestMetricsListenerStillServesHealthAndReady(t *testing.T) {
	h := metricsRouter(EnvCfg{PlatformEnabled: true})

	for _, path := range []string{"/health", "/ready", "/metrics"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, rec.Code)
		}
	}
}
