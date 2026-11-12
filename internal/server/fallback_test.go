package server

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMountFallback(t *testing.T) {
	r := chi.NewRouter()
	r.Post("/auth/login", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	r.Post("/api/clients/{id}/activate", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	spa := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot) // stands for "the fallback answered"
	})
	mountFallback(r, spa, func(p string) bool { return strings.HasPrefix(p, "/api/") })

	cases := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/dashboard", http.StatusTeapot},                        // unknown path: fallback
		{http.MethodGet, "/auth/login", http.StatusTeapot},                       // client route colliding with POST /auth/login
		{http.MethodPost, "/auth/login", http.StatusNoContent},                   // the API route itself
		{http.MethodDelete, "/auth/login", http.StatusMethodNotAllowed},          // non-GET mismatch
		{http.MethodGet, "/api/clients/x/activate", http.StatusMethodNotAllowed}, // GET to a POST-only API route
		{http.MethodGet, "/api/unknown", http.StatusTeapot},                      // unknown API path: the fallback's own 404
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.want {
			t.Errorf("%s %s: status %d, want %d", tc.method, tc.path, w.Code, tc.want)
		}
	}

	// Without an API-path rule every GET mismatch reaches the fallback.
	r2 := chi.NewRouter()
	r2.Post("/api/x", func(w http.ResponseWriter, _ *http.Request) {})
	mountFallback(r2, spa, nil)
	w := httptest.NewRecorder()
	r2.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/x", nil))
	if w.Code != http.StatusTeapot {
		t.Errorf("nil isAPIPath: status %d, want the fallback", w.Code)
	}
}

// /swagger-ui loads third-party script: the assets are pinned to an exact
// version with Subresource Integrity, and the response carries the
// framing/sniffing headers.
func TestSwaggerUIPinnedAndHardened(t *testing.T) {
	w := httptest.NewRecorder()
	swaggerUIHandler(w, httptest.NewRequest(http.MethodGet, "/swagger-ui", nil))
	body := w.Body.String()

	for _, asset := range []string{"swagger-ui.css", "swagger-ui-bundle.js"} {
		re := regexp.MustCompile(`(?:href|src)="https://unpkg\.com/swagger-ui-dist@\d+\.\d+\.\d+/` +
			regexp.QuoteMeta(asset) + `"[^>]*integrity="sha384-[A-Za-z0-9+/]{64}"[^>]*crossorigin="anonymous"`)
		if !re.MatchString(body) {
			t.Errorf("%s is not pinned with integrity+crossorigin in:\n%s", asset, body)
		}
	}
	if strings.Contains(body, "swagger-ui-dist/") {
		t.Error("an unpinned swagger-ui-dist/ URL remains")
	}
	if got := strings.Count(body, `integrity="sha384-`); got != 2 {
		t.Errorf("integrity attributes = %d, want 2", got)
	}
	if !strings.Contains(body, "url:'/q/openapi'") {
		t.Error("spec URL changed")
	}

	h := w.Header()
	for k, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "same-origin",
		"Content-Security-Policy": "frame-ancestors 'none'; object-src 'none'; base-uri 'self'",
	} {
		if got := h.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}
