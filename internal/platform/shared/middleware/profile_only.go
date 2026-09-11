package middleware

import (
	"net/http"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// ProfileOnlyWithoutRole confines a USER who holds no platform role (and so
// no permission) to their own profile: everything but the self-service
// surface answers 403 NO_PLATFORM_ROLE. Mounted right after Authenticator so
// it covers every huma and chi route in the authenticated group — the
// per-handler checks alone left gaps (ungated /bff lookups; the anchor
// tier short-circuiting permission checks for a zero-role anchor user).
//
// Unauthenticated requests pass untouched (the group also hosts the public
// login flows), as do service accounts — app service accounts authorize via
// their applications claim, not roles.
func ProfileOnlyWithoutRole(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ac := auth.FromContext(r.Context()); ac.IsRoleless() && !profileAllowed(r) {
			// The platform error envelope ({"error": code, "message"}), like
			// every other /api and /bff rejection.
			httperror.Write(w, usecase.Authorization("NO_PLATFORM_ROLE",
				"Your account has no platform access. Only your profile is available."))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// profileAllowed is the self-service allowlist for a role-less user: the
// session/profile/credential routes under /auth (me, change-password,
// login history, 2FA, passkeys, logout, client selection, the login flows
// themselves), the portal plane's own login routes, and GET /api/me.
func profileAllowed(r *http.Request) bool {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/auth/"), strings.HasPrefix(p, "/portal/"):
		return true
	case p == "/api/me" && r.Method == http.MethodGet:
		return true
	}
	return false
}
