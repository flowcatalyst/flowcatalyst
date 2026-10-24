package ui

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/pages"
)

func twoFactorRouter(t *testing.T, on bool) http.Handler {
	t.Helper()
	cfg := Config{Authenticate: fakeAuth, CookieSecure: true, DispatchPools: noPools{}}
	if on {
		cfg.TwoFactor = &pages.AuthMFA{}
	}
	r := chi.NewRouter()
	if err := Mount(r, cfg); err != nil {
		t.Fatal(err)
	}
	return r
}

// The two-factor and passkey pages exist only when wired, the sign-in ones need
// no session, the security page does, and an unsafe request is held to the
// same cross-site check as the rest of /ui.
func TestTwoFactorPagesMount(t *testing.T) {
	off := twoFactorRouter(t, false)
	for _, target := range []string{"/ui/auth/passkey", "/ui/auth/2fa", "/ui/auth/2fa/enroll"} {
		if rec := do(t, off, req{method: http.MethodGet, target: target}); rec.Code != http.StatusNotFound {
			t.Errorf("not wired, %s: status %d", target, rec.Code)
		}
	}

	on := twoFactorRouter(t, true)
	rec := do(t, on, req{method: http.MethodGet, target: "/ui/auth/passkey"})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Sign in with a passkey") {
		t.Fatalf("passkey page without a session: status %d", rec.Code)
	}
	if rec.Header().Get("Cache-Control") != "no-store" || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "script-src 'self'") {
		t.Errorf("headers: %v", rec.Header())
	}
	// No challenge: back to the default sign-in.
	rec = do(t, on, req{method: http.MethodGet, target: "/ui/auth/2fa"})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/auth/login" {
		t.Errorf("challenge without a cookie: status %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	// The security page is behind the session guard.
	rec = do(t, on, req{method: http.MethodGet, target: "/ui/auth/security"})
	if rec.Code != http.StatusFound || !strings.HasPrefix(rec.Header().Get("Location"), "/auth/login?redirect=") {
		t.Errorf("security page without a session: status %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	rec = do(t, on, req{method: http.MethodPost, target: "/ui/auth/security/2fa/TOTP/remove", headers: map[string]string{"Sec-Fetch-Site": "same-origin"}})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("security post without a session: status %d", rec.Code)
	}
	// A form on another site cannot post a code or start an enrolment.
	for _, target := range []string{"/ui/auth/2fa/verify", "/ui/auth/2fa/start", "/ui/auth/2fa/enroll/start", "/ui/auth/2fa/enroll/done", "/ui/auth/security/2fa/totp"} {
		rec = do(t, on, req{method: http.MethodPost, target: target, session: "admin", headers: map[string]string{"Sec-Fetch-Site": "cross-site"}})
		if rec.Code != http.StatusForbidden {
			t.Errorf("cross-site %s: status %d", target, rec.Code)
		}
	}
}
