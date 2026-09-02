package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// BasicAuthConfig configures the optional HTTP BasicAuth middleware.
// Empty Username disables auth entirely.
type BasicAuthConfig struct {
	Username string
	Password string
	// Realm is the WWW-Authenticate realm shown by browsers in the
	// password prompt. Defaults to "FlowCatalyst Router" when empty.
	Realm string
}

// publicPaths are the URLs that bypass authentication:
// probes + Prometheus + the OpenAPI surface must be
// reachable by orchestration tooling without credentials.
var publicPaths = map[string]struct{}{
	"/health":           {},
	"/q/health":         {},
	"/health/live":      {},
	"/health/ready":     {},
	"/health/startup":   {},
	"/q/health/live":    {},
	"/q/health/ready":   {},
	"/metrics":          {},
	"/q/metrics":        {},
	"/ready":            {},
	"/openapi.json":     {},
	"/openapi.yaml":     {},
	"/openapi-3.0.json": {},
	"/openapi-3.0.yaml": {},
	"/openapi-3.1.json": {},
	"/openapi-3.1.yaml": {},
}

// publicPrefixes covers paths where any subroute is public — the docs
// renderer serves arbitrary asset paths under /docs.
var publicPrefixes = []string{"/docs"}

// mountRelativePath returns the request path to test against
// IsPublicPath, relative to whatever prefix this middleware is mounted
// under (e.g. run.go nests the router API in r.Route("/router", ...)
// by default). chi.Mux.Mount — which is what r.Route uses under the
// hood — shifts RouteContext.RoutePath to the mount-relative remainder
// *before* a subrouter's own middleware runs, so a request for
// "/router/health/live" has RoutePath == "/health/live" by the time we
// get here. When this middleware is registered directly on a
// top-level, unmounted router, RoutePath is still "" at this point
// (chi only fills it in during the final route match) — in that case
// fall back to r.URL.Path, exactly like chi's own router does
// internally (see Mux.routeHTTP). This keeps public-path matching
// correct whether the router API is mounted under a prefix or not,
// without hard-coding the prefix here.
func mountRelativePath(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil && rctx.RoutePath != "" {
		return rctx.RoutePath
	}
	return r.URL.Path
}

// IsPublicPath reports whether path is exempt from auth.
func IsPublicPath(path string) bool {
	if _, ok := publicPaths[path]; ok {
		return true
	}
	for _, p := range publicPrefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// BasicAuthMiddleware returns a chi-compatible middleware that enforces
// HTTP BasicAuth on every non-public route. A zero Config disables auth
// (returns the identity middleware) so callers can wire it
// unconditionally and let env config decide.
func BasicAuthMiddleware(cfg BasicAuthConfig) func(http.Handler) http.Handler {
	if cfg.Username == "" {
		// No-op when not configured.
		return func(next http.Handler) http.Handler { return next }
	}
	realm := cfg.Realm
	if realm == "" {
		realm = "FlowCatalyst Router"
	}
	expectedUser := []byte(cfg.Username)
	expectedPass := []byte(cfg.Password)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if IsPublicPath(mountRelativePath(r)) {
				next.ServeHTTP(w, r)
				return
			}
			user, pass, ok := r.BasicAuth()
			if !ok ||
				subtle.ConstantTimeCompare([]byte(user), expectedUser) != 1 ||
				subtle.ConstantTimeCompare([]byte(pass), expectedPass) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`", charset="UTF-8"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
