package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
	routerapi "github.com/flowcatalyst/flowcatalyst-go/internal/router/api"
)

// fakePlatformAuth stands in for the platform authenticator: X-Test-Scope
// names the caller's scope; no header, no AuthContext.
func fakePlatformAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if scope := r.Header.Get("X-Test-Scope"); scope != "" {
			ctx := auth.WithContext(r.Context(), &auth.AuthContext{
				PrincipalID: "prn_debugtest", Scope: auth.Scope(scope),
			})
			if r.Header.Get("X-Test-Cookie") != "" {
				// What the Authenticator records when the fc_session cookie
				// authenticated the request.
				ctx = platformmw.WithSessionCookieAuth(ctx)
			}
			r = r.WithContext(ctx)
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

// F16 follow-up: the session cookie and browser-cached Basic credentials are
// ambient, so a page on another site could start a CPU profile (GET
// .../debug/pprof/profile) with a link. The gate refuses a request the browser
// marks as cross-site or same-site, and a cookie-authenticated request whose
// provenance it cannot see (no Sec-Fetch-Site: a GET navigation carries no
// Origin). Tools (curl, go tool pprof) with a bearer or Basic credentials send
// no Sec-Fetch-Site and are not affected.
func TestDebugGate_CrossSiteNavigationRefused(t *testing.T) {
	platform := newDebugGate(routerapi.BasicAuthConfig{}, fakePlatformAuth)
	basic := newDebugGate(routerapi.BasicAuthConfig{Username: "ops", Password: "s3cret"}, fakePlatformAuth)
	cookie := func(sfs string) func(*http.Request) {
		return func(r *http.Request) {
			r.Header.Set("X-Test-Scope", "ANCHOR")
			r.Header.Set("X-Test-Cookie", "1")
			if sfs != "" {
				r.Header.Set("Sec-Fetch-Site", sfs)
			}
		}
	}
	bearer := func(sfs string) func(*http.Request) {
		return func(r *http.Request) {
			r.Header.Set("X-Test-Scope", "ANCHOR")
			if sfs != "" {
				r.Header.Set("Sec-Fetch-Site", sfs)
			}
		}
	}
	withBasic := func(sfs string) func(*http.Request) {
		return func(r *http.Request) {
			r.SetBasicAuth("ops", "s3cret")
			if sfs != "" {
				r.Header.Set("Sec-Fetch-Site", sfs)
			}
		}
	}
	for _, tc := range []struct {
		name string
		gate func(http.Handler) http.Handler
		prep func(*http.Request)
		want int
	}{
		{"cookie, cross-site link", platform, cookie("cross-site"), http.StatusForbidden},
		{"cookie, same-site sibling link", platform, cookie("same-site"), http.StatusForbidden},
		{"cookie, provenance unknown", platform, cookie(""), http.StatusForbidden},
		{"cookie, typed in the address bar", platform, cookie("none"), http.StatusOK},
		{"cookie, link on the platform's own page", platform, cookie("same-origin"), http.StatusOK},
		{"bearer, a tool", platform, bearer(""), http.StatusOK},
		{"bearer, cross-site", platform, bearer("cross-site"), http.StatusForbidden},
		{"basic, a tool", basic, withBasic(""), http.StatusOK},
		{"basic cached by the browser, cross-site link", basic, withBasic("cross-site"), http.StatusForbidden},
		{"basic, same-origin", basic, withBasic("same-origin"), http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, serveGate(tc.gate, tc.prep))
		})
	}
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
