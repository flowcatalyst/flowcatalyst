package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
	routerapi "github.com/flowcatalyst/flowcatalyst-go/internal/router/api"
)

// fakePlatformAuth stands in for the platform authenticator: X-Test-Scope
// names the caller's scope; no header, no AuthContext.
func fakePlatformAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if scope := r.Header.Get("X-Test-Scope"); scope != "" {
			r = r.WithContext(auth.WithContext(r.Context(), &auth.AuthContext{
				PrincipalID: "prn_debugtest", Scope: auth.Scope(scope),
			}))
		}
		next.ServeHTTP(w, r)
	})
}

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

func serveGate(gate func(http.Handler) http.Handler, prep func(*http.Request)) int {
	req := httptest.NewRequest(http.MethodGet, "/router/debug/dump", nil)
	if prep != nil {
		prep(req)
	}
	rec := httptest.NewRecorder()
	gate(okHandler).ServeHTTP(rec, req)
	return rec.Code
}

// With the router's Basic auth configured, it is the debug credential.
func TestDebugGate_RouterBasicAuth(t *testing.T) {
	gate := newDebugGate(routerapi.BasicAuthConfig{Username: "ops", Password: "s3cret"}, fakePlatformAuth)
	require.NotNil(t, gate)
	assert.Equal(t, http.StatusUnauthorized, serveGate(gate, nil))
	assert.Equal(t, http.StatusUnauthorized, serveGate(gate, func(r *http.Request) { r.SetBasicAuth("ops", "wrong") }))
	assert.Equal(t, http.StatusOK, serveGate(gate, func(r *http.Request) { r.SetBasicAuth("ops", "s3cret") }))
}

// Without router Basic auth (AUTH_MODE=NONE, as production's router task
// runs today), the platform bearer is required, and only an anchor
// principal passes.
func TestDebugGate_PlatformAnchorOnly(t *testing.T) {
	gate := newDebugGate(routerapi.BasicAuthConfig{}, fakePlatformAuth)
	require.NotNil(t, gate)
	assert.Equal(t, http.StatusUnauthorized, serveGate(gate, nil), "anonymous")
	assert.Equal(t, http.StatusForbidden, serveGate(gate, func(r *http.Request) { r.Header.Set("X-Test-Scope", "CLIENT") }))
	assert.Equal(t, http.StatusOK, serveGate(gate, func(r *http.Request) { r.Header.Set("X-Test-Scope", "ANCHOR") }))
}

// Nothing to authenticate with: no gate, so the surface is not mounted.
func TestDebugGate_NothingToAuthenticateWithMeansNoGate(t *testing.T) {
	assert.Nil(t, newDebugGate(routerapi.BasicAuthConfig{}, nil))
}

// Through the real router mount: with the gate the endpoints answer only an
// authenticated caller; with no gate they do not exist.
func TestMountRouterHTTP_DebugSurface(t *testing.T) {
	t.Setenv("AUTH_MODE", "NONE") // the router prefix itself is open, as in production today
	srv, err := router.NewServer(router.ServerConfig{})
	require.NoError(t, err)

	gated := chi.NewRouter()
	MountRouterHTTP(gated, "/router", srv, nil, EnvCfg{}, newDebugGate(resolveRouterAuth(), fakePlatformAuth))
	for _, p := range []string{"/router/debug/pprof/goroutine?debug=2", "/router/debug/dump", "/router/debug/vars"} {
		rec := httptest.NewRecorder()
		gated.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "anonymous %s", p)

		req := httptest.NewRequest(http.MethodGet, p, nil)
		req.Header.Set("X-Test-Scope", "ANCHOR")
		rec = httptest.NewRecorder()
		gated.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "anchor %s", p)
	}

	ungated := chi.NewRouter()
	MountRouterHTTP(ungated, "/router", srv, nil, EnvCfg{}, nil)
	rec := httptest.NewRecorder()
	ungated.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/router/debug/pprof/goroutine?debug=2", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code, "not mounted without a gate")
}

// The Go runtime and process collectors are on the metrics listener.
func TestMetricsListenerServesRuntimeMetrics(t *testing.T) {
	rec := httptest.NewRecorder()
	metricsRouter(EnvCfg{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "go_goroutines ")
}
