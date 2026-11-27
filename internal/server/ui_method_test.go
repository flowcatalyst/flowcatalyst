package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/consoleredirect"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui"
)

type noUIPools struct{}

func (noUIPools) FindWithFilters(context.Context, *string, *string) ([]dispatchpool.DispatchPool, error) {
	return nil, nil
}

func (noUIPools) FindByID(context.Context, string) (*dispatchpool.DispatchPool, error) {
	return nil, nil
}

// uiWiring assembles the router the way Run does for the console: the /ui
// sub-router, then the console redirect fallback.
func uiWiring(t *testing.T) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	if err := ui.Mount(r, ui.Config{
		Authenticate:   func(next http.Handler) http.Handler { return next },
		DisplayName:    func(context.Context, string) string { return "x" },
		DispatchPools:  noUIPools{},
		SignInUpstream: http.NotFoundHandler(),
	}); err != nil {
		t.Fatal(err)
	}
	mountFallback(r, consoleredirect.Handler(), consoleredirect.IsAPIPath)
	return r
}

// A wrong-method request under /ui never falls through to a redirect: a GET to
// the POST-only sign-out route is a 405 that wears the console's security
// headers. Unknown paths and ordinary routes keep answering as before.
func TestUIWrongMethodDoesNotFallThrough(t *testing.T) {
	h := uiWiring(t)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ui/logout", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /ui/logout: status %d, want 405 (body %q)", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "" {
		t.Errorf("GET /ui/logout redirected to %q", loc)
	}
	if w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("405 lacks the console security headers: %v", w.Header())
	}

	// An unknown /ui path answers 404 (never redirects back to /ui/).
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ui/no-such-page", nil))
	if w.Code != http.StatusNotFound || w.Header().Get("Location") != "" {
		t.Errorf("GET /ui/no-such-page: status %d loc %q, want 404 without redirect", w.Code, w.Header().Get("Location"))
	}

	// A legacy root/SPA path redirects to its /ui page.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/ui/dashboard" {
		t.Errorf("GET /dashboard: status %d loc %q, want 302 /ui/dashboard", w.Code, w.Header().Get("Location"))
	}

	// A normal GET page still routes (the sign-in page answers 404 from the
	// stub upstream, not 405), and the POST action still reaches its handler.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/ui/logout", nil))
	if w.Code == http.StatusMethodNotAllowed || w.Code == http.StatusNotFound {
		t.Errorf("POST /ui/logout: status %d, want it routed", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ui/", nil))
	if w.Code == http.StatusMethodNotAllowed {
		t.Errorf("GET /ui/: status %d, want it routed", w.Code)
	}
}
