package guard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
)

var ok = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })

func csrf(t *testing.T, trusted ...string) http.Handler {
	t.Helper()
	mw, err := CSRF(trusted, PlainDenier)
	if err != nil {
		t.Fatal(err)
	}
	return mw(ok)
}

func TestCSRF(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		headers map[string]string
		trusted []string
		allowed bool
	}{
		{"GET is never checked", http.MethodGet, map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, nil, true},
		{"HEAD is never checked", http.MethodHead, map[string]string{"Sec-Fetch-Site": "cross-site"}, nil, true},
		{"same-origin form post", http.MethodPost, map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "https://fc.example"}, nil, true},
		{"same-origin Datastar post", http.MethodPost, map[string]string{"Sec-Fetch-Site": "same-origin", ds.RequestHeader: "true"}, nil, true},
		{"user-initiated (none)", http.MethodPost, map[string]string{"Sec-Fetch-Site": "none"}, nil, true},
		{"cross-site post", http.MethodPost, map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, nil, false},
		{"same-site sibling subdomain", http.MethodPost, map[string]string{"Sec-Fetch-Site": "same-site", "Origin": "https://evil.fc.example"}, nil, false},
		{"cross-site delete", http.MethodDelete, map[string]string{"Sec-Fetch-Site": "cross-site"}, nil, false},
		{"cross-site put", http.MethodPut, map[string]string{"Sec-Fetch-Site": "cross-site"}, nil, false},
		{"cross-site patch", http.MethodPatch, map[string]string{"Sec-Fetch-Site": "cross-site"}, nil, false},
		{"old browser, Origin matches Host", http.MethodPost, map[string]string{"Origin": "https://fc.example"}, nil, true},
		{"old browser, Origin from elsewhere", http.MethodPost, map[string]string{"Origin": "https://evil.example"}, nil, false},
		{"opaque Origin null", http.MethodPost, map[string]string{"Origin": "null"}, nil, false},
		{"no provenance headers at all", http.MethodPost, nil, nil, false},
		{"Datastar header alone is not enough", http.MethodPost, map[string]string{ds.RequestHeader: "true"}, nil, false},
		{"trusted public origin behind a proxy", http.MethodPost, map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://platform.example"}, []string{"https://platform.example/"}, true},
		{"trusted list does not open other origins", http.MethodPost, map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, []string{"https://platform.example"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "https://fc.example/ui/logout", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			csrf(t, tc.trusted...).ServeHTTP(rec, req)
			if tc.allowed && rec.Code != http.StatusTeapot {
				t.Fatalf("refused (status %d), want allowed", rec.Code)
			}
			if !tc.allowed && rec.Code != http.StatusForbidden {
				t.Fatalf("status %d, want 403", rec.Code)
			}
		})
	}
}

func TestCSRFRejectsBadTrustedOrigin(t *testing.T) {
	for _, o := range []string{"fc.example", "ftp://fc.example", "https://"} {
		if _, err := CSRF([]string{o}, PlainDenier); err == nil {
			t.Errorf("trusted origin %q accepted", o)
		}
	}
}

func withUser(r *http.Request, roles ...string) *http.Request {
	ac := &auth.AuthContext{PrincipalID: "p1", Scope: auth.ScopeAnchor, Roles: roles, PrincipalType: auth.PrincipalTypeUser}
	if len(roles) > 0 {
		ac.Permissions = []string{"platform:messaging:dispatch-pool:view"}
	}
	return r.WithContext(auth.WithContext(r.Context(), ac))
}

func TestRequireSession(t *testing.T) {
	cases := []struct {
		name     string
		method   string
		target   string
		headers  map[string]string
		authed   bool
		status   int
		location string
	}{
		{"signed in", http.MethodGet, "/ui/dispatch-pools", map[string]string{"Accept": "text/html"}, true, http.StatusTeapot, ""},
		{
			"browser navigation redirects to login with a return path", http.MethodGet, "/ui/dispatch-pools?q=a%20b&status=ACTIVE",
			map[string]string{"Accept": "text/html,application/xhtml+xml"},
			false, http.StatusFound,
			"/auth/login?redirect=%2Fui%2Fdispatch-pools%3Fq%3Da%2520b%26status%3DACTIVE",
		},
		{"HEAD navigation redirects too", http.MethodHead, "/ui/", map[string]string{"Accept": "text/html"}, false, http.StatusFound, "/auth/login?redirect=%2Fui%2F"},
		{
			"Datastar action gets 401, not a redirect it would follow", http.MethodGet, "/ui/dispatch-pools",
			map[string]string{"Accept": "text/event-stream, text/html, application/json", ds.RequestHeader: "true"},
			false, http.StatusUnauthorized, "",
		},
		{"fetch for JSON gets 401", http.MethodGet, "/ui/dispatch-pools", map[string]string{"Accept": "application/json"}, false, http.StatusUnauthorized, ""},
		{"form post gets 401", http.MethodPost, "/ui/logout", map[string]string{"Accept": "text/html"}, false, http.StatusUnauthorized, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.target, nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			if tc.authed {
				req = withUser(req, "platform:admin")
			}
			rec := httptest.NewRecorder()
			RequireSession(ok).ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d", rec.Code, tc.status)
			}
			if got := rec.Header().Get("Location"); got != tc.location {
				t.Fatalf("Location %q, want %q", got, tc.location)
			}
		})
	}
}

func TestProfileOnly(t *testing.T) {
	h := ProfileOnly(PlainDenier)(ok)
	for _, tc := range []struct {
		name   string
		req    *http.Request
		status int
	}{
		{"user with a role", withUser(httptest.NewRequest(http.MethodGet, "/ui/", nil), "platform:admin"), http.StatusTeapot},
		{"role-less user", withUser(httptest.NewRequest(http.MethodGet, "/ui/", nil)), http.StatusForbidden},
		{"no session (RequireSession's job)", httptest.NewRequest(http.MethodGet, "/ui/", nil), http.StatusTeapot},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, tc.req)
			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d", rec.Code, tc.status)
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	SecurityHeaders(ok).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/", nil))
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"script-src 'self' 'unsafe-eval'", "frame-ancestors 'none'", "connect-src 'self'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP %q lacks %q", csp, want)
		}
	}
	if strings.Contains(csp, "script-src 'self' 'unsafe-eval' 'unsafe-inline'") {
		t.Error("CSP allows inline scripts")
	}
	if rec.Header().Get("X-Frame-Options") != "DENY" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("headers: %v", rec.Header())
	}
}
