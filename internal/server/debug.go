package server

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	routerapi "github.com/flowcatalyst/flowcatalyst-go/internal/router/api"
)

// newDebugGate builds the middleware that guards the debug surface
// (routerapi.MountDebug). A profile holds memory contents and the dump names
// messages, so the surface is never reachable anonymously:
//
//   - When the router's Basic auth is configured (FC_ROUTER_AUTH_USER/PASS),
//     it is the credential — the same one /monitoring/* takes. The router
//     prefix's own middleware already checks it; this checks again, so the
//     gate holds wherever it is mounted.
//   - Otherwise the platform's bearer or session: the caller must be an
//     anchor principal. (Go's router has no router:operate permission;
//     anchor is the platform's operator tier.)
//   - With neither available (no router auth, no platform in this process)
//     it returns nil and the caller does not mount the surface at all.
//
// The platform authenticator given here must not honour the dev-only
// X-FC-Test-Principal headers; WirePlatform's PlatformHandles.Authenticate
// does not.
func newDebugGate(basic routerapi.BasicAuthConfig, platformAuth func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	switch {
	case basic.Username != "":
		user, pass := []byte(basic.Username), []byte(basic.Password)
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				u, p, ok := r.BasicAuth()
				if !ok || subtle.ConstantTimeCompare([]byte(u), user) != 1 ||
					subtle.ConstantTimeCompare([]byte(p), pass) != 1 {
					w.Header().Set("WWW-Authenticate", `Basic realm="FlowCatalyst Router", charset="UTF-8"`)
					debugDenied(w, http.StatusUnauthorized, "UNAUTHORIZED", "router credentials required")
					return
				}
				next.ServeHTTP(w, r)
			})
		}
	case platformAuth != nil:
		return func(next http.Handler) http.Handler {
			return platformAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ac := auth.FromContext(r.Context())
				if ac == nil {
					w.Header().Set("WWW-Authenticate", `Bearer realm="FlowCatalyst"`)
					debugDenied(w, http.StatusUnauthorized, "UNAUTHENTICATED", "a platform bearer token or session is required")
					return
				}
				if !ac.IsAnchor() {
					debugDenied(w, http.StatusForbidden, "FORBIDDEN", "the debug endpoints need an anchor principal")
					return
				}
				next.ServeHTTP(w, r)
			}))
		}
	default:
		return nil
	}
}

func debugDenied(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": msg})
}
