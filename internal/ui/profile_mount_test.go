package ui

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/pages"
)

func profileRouter(t *testing.T) http.Handler {
	t.Helper()
	up := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"principalId": "prn_1", "name": "Ada Lovelace", "email": "ada@example.com", "roles": []string{}})
	})
	r := chi.NewRouter()
	if err := Mount(r, Config{
		Authenticate: fakeAuth, CookieSecure: true, DispatchPools: noPools{}, SignInUpstream: up,
		TwoFactor: &pages.AuthMFA{},
	}); err != nil {
		t.Fatal(err)
	}
	return r
}

// The profile is the one signed-in page a user without a platform role may
// reach (the SPA's rule, and the API's /auth/* allow-list); every other page
// stays behind the role-less gate. A signed-out visit goes to sign-in, and a
// profile post is held to the same cross-site check as the rest of /ui.
func TestProfileMount(t *testing.T) {
	h := profileRouter(t)

	rec := do(t, h, req{method: http.MethodGet, target: "/ui/profile", session: "roleless"})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Ada Lovelace") || strings.Contains(rec.Body.String(), "fc-sidebar-link") {
		t.Fatalf("role-less profile: status %d", rec.Code)
	}
	if rec := do(t, h, req{method: http.MethodGet, target: "/ui/dispatch-pools", session: "roleless"}); rec.Code != http.StatusForbidden {
		t.Errorf("role-less user reached a platform page: status %d", rec.Code)
	}
	if rec := do(t, h, req{method: http.MethodGet, target: "/ui/profile", session: "admin"}); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `href="/ui/profile"`) {
		t.Errorf("admin profile: status %d (the shell's profile link must be /ui/profile)", rec.Code)
	}

	rec = do(t, h, req{method: http.MethodGet, target: "/ui/profile"})
	if rec.Code != http.StatusFound || !strings.HasPrefix(rec.Header().Get("Location"), "/ui/auth/login?redirect=") {
		t.Errorf("signed-out profile: status %d to %q", rec.Code, rec.Header().Get("Location"))
	}

	for _, target := range []string{
		"/ui/profile/password", "/ui/profile/password/email-code",
		"/ui/profile/developer-credential", "/ui/profile/developer-credential/revoke",
	} {
		cross := map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}
		if rec := do(t, h, req{method: http.MethodPost, target: target, session: "admin", headers: cross}); rec.Code != http.StatusForbidden {
			t.Errorf("cross-site POST %s: status %d", target, rec.Code)
		}
		if rec := do(t, h, req{method: http.MethodPost, target: target, headers: map[string]string{"Sec-Fetch-Site": "same-origin"}}); rec.Code != http.StatusUnauthorized {
			t.Errorf("signed-out POST %s: status %d", target, rec.Code)
		}
	}
}
