package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/assets"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
)

type noPools struct{}

func (noPools) FindWithFilters(context.Context, *string, *string) ([]dispatchpool.DispatchPool, error) {
	return nil, nil
}

// fakeAuth stands in for the platform Authenticator: a "session" cookie
// value names the principal kind.
func fakeAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("fc_session")
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		ac := &auth.AuthContext{PrincipalID: "prn_1", Email: "ada@example.com", Scope: auth.ScopeAnchor, PrincipalType: auth.PrincipalTypeUser}
		if c.Value == "admin" {
			ac.Roles = []string{"platform:super-admin"}
			ac.Permissions = []string{"platform:*:*:*"}
		}
		next.ServeHTTP(w, r.WithContext(auth.WithContext(r.Context(), ac)))
	})
}

func router(t *testing.T) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	if err := Mount(r, Config{
		Authenticate:  fakeAuth,
		CookieSecure:  true,
		Version:       "1.2.3",
		DisplayName:   func(context.Context, string) string { return "Ada Lovelace" },
		DispatchPools: noPools{},
	}); err != nil {
		t.Fatal(err)
	}
	return r
}

type req struct {
	method, target, session string
	headers                 map[string]string
}

func do(t *testing.T, h http.Handler, rq req) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(rq.method, "https://fc.example"+rq.target, nil)
	r.Header.Set("Accept", "text/html")
	for k, v := range rq.headers {
		r.Header.Set(k, v)
	}
	if rq.session != "" {
		r.AddCookie(&http.Cookie{Name: "fc_session", Value: rq.session})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestUIRouteGroup(t *testing.T) {
	h := router(t)

	t.Run("signed-out navigation goes to the login page", func(t *testing.T) {
		rec := do(t, h, req{method: http.MethodGet, target: "/ui/dispatch-pools"})
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/auth/login?redirect=%2Fui%2Fdispatch-pools" {
			t.Fatalf("status %d Location %q", rec.Code, rec.Header().Get("Location"))
		}
	})
	t.Run("signed-out Datastar action gets 401", func(t *testing.T) {
		rec := do(t, h, req{method: http.MethodGet, target: "/ui/dispatch-pools", headers: map[string]string{ds.RequestHeader: "true"}})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status %d", rec.Code)
		}
	})
	t.Run("signed-in page has the shell, the user and the security headers", func(t *testing.T) {
		rec := do(t, h, req{method: http.MethodGet, target: "/ui/dispatch-pools", session: "admin"})
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		body := rec.Body.String()
		for _, want := range []string{
			`<span class="fc-user-name">Ada Lovelace</span>`, `<span class="fc-avatar">AL</span>`, `<span>1.2.3</span>`,
			`action="/ui/logout"`, assets.URL("kit.css"), assets.URL("datastar.js"),
			`href="` + assets.URL("lucide-sprite.svg") + `#i-`, // icons use the served sprite
			`data-on:datastar-fetch=`, `data-signals:sidebar-collapsed="false"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("page lacks %s", want)
			}
		}
		if rec.Header().Get("Content-Security-Policy") == "" || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("security headers missing: %v", rec.Header())
		}
	})
	t.Run("the sidebar cookie renders the page collapsed", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/ui/dispatch-pools", nil)
		r.Header.Set("Accept", "text/html")
		r.AddCookie(&http.Cookie{Name: "fc_session", Value: "admin"})
		r.AddCookie(&http.Cookie{Name: "fc_sidebar", Value: "collapsed"})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if !strings.Contains(rec.Body.String(), `data-signals:sidebar-collapsed="true"`) {
			t.Error("collapsed state not rendered")
		}
	})
	t.Run("role-less user is refused", func(t *testing.T) {
		rec := do(t, h, req{method: http.MethodGet, target: "/ui/dispatch-pools", session: "roleless"})
		if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "fc-sidebar") {
			t.Fatalf("status %d", rec.Code)
		}
	})
	t.Run("/ui/ lands on the first ported page", func(t *testing.T) {
		rec := do(t, h, req{method: http.MethodGet, target: "/ui/", session: "admin"})
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/ui/dispatch-pools" {
			t.Fatalf("status %d Location %q", rec.Code, rec.Header().Get("Location"))
		}
	})
	t.Run("assets need no session", func(t *testing.T) {
		rec := do(t, h, req{method: http.MethodGet, target: assets.URL("kit.css")})
		if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
			t.Fatalf("status %d", rec.Code)
		}
	})
	t.Run("unknown page is a 404", func(t *testing.T) {
		rec := do(t, h, req{method: http.MethodGet, target: "/ui/nope", session: "admin"})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status %d", rec.Code)
		}
	})
	t.Run("cross-site sign-out is blocked and keeps the session", func(t *testing.T) {
		rec := do(t, h, req{
			method: http.MethodPost, target: "/ui/logout", session: "admin",
			headers: map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"},
		})
		if rec.Code != http.StatusForbidden || rec.Header().Get("Set-Cookie") != "" {
			t.Fatalf("status %d Set-Cookie %q", rec.Code, rec.Header().Get("Set-Cookie"))
		}
	})
	t.Run("same-origin sign-out clears the cookie and goes to login", func(t *testing.T) {
		rec := do(t, h, req{
			method: http.MethodPost, target: "/ui/logout", session: "admin",
			headers: map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "https://fc.example"},
		})
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/auth/login" {
			t.Fatalf("status %d Location %q", rec.Code, rec.Header().Get("Location"))
		}
		c := rec.Header().Get("Set-Cookie")
		for _, want := range []string{"fc_session=", "Max-Age=0", "HttpOnly", "Secure", "SameSite=Lax", "Path=/"} {
			if !strings.Contains(c, want) {
				t.Errorf("Set-Cookie %q lacks %s", c, want)
			}
		}
	})
	t.Run("GET cannot sign out", func(t *testing.T) {
		rec := do(t, h, req{method: http.MethodGet, target: "/ui/logout", session: "admin"})
		if rec.Header().Get("Set-Cookie") != "" || rec.Code == http.StatusSeeOther {
			t.Fatalf("GET changed state: status %d", rec.Code)
		}
	})
}
