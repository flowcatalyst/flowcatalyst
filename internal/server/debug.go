package server

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
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
//
// Provenance (F16 follow-up): the session cookie and Basic credentials a
// browser has cached are ambient, so a link on another site could start a CPU
// profile. A request the browser marks as not from this origin
// (Sec-Fetch-Site other than same-origin or none) is refused, 403
// CROSS_ORIGIN_REQUEST; so is a cookie-authenticated request without
// Sec-Fetch-Site (a GET navigation carries no Origin, so nothing else tells).
// Tools (curl, go tool pprof) send no Sec-Fetch-Site and use a bearer token or
// the Basic credentials, which are not affected.
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
				if fetchFromElsewhere(r) {
					debugCrossOrigin(w)
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
				if fetchFromElsewhere(r) ||
					(platformmw.AuthenticatedBySessionCookie(r.Context()) && r.Header.Get("Sec-Fetch-Site") == "") {
					debugCrossOrigin(w)
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

// fetchFromElsewhere reports a browser request that did not come from this
// origin or from the user (Sec-Fetch-Site present and neither same-origin nor
// none).
func fetchFromElsewhere(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
		return false
	default:
		return true
	}
}

func debugCrossOrigin(w http.ResponseWriter) {
	debugDenied(w, http.StatusForbidden, platformmw.CodeCrossOriginRequest,
		"the debug endpoints answer only requests from this site or from tools; "+
			"open them from the address bar, or use a bearer token")
}

func debugDenied(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": msg})
}
