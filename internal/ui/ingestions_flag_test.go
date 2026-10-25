package ui

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/pages"
)

type noIngestions struct{}

func (noIngestions) List(context.Context, ingestion.ListFilter) (*ingestion.Page, error) {
	return &ingestion.Page{}, nil
}

func (noIngestions) FindByID(context.Context, string) (*ingestion.Ingestion, error) { return nil, nil }

func (noIngestions) ListVersions(context.Context, string) ([]ingestion.Version, error) {
	return nil, nil
}

func (noIngestions) LiveKinds(context.Context) ([]ingestion.RuntimeKind, error) { return nil, nil }

func (noIngestions) NoticesOf(context.Context, []string) (map[string]ingestion.Notices, error) {
	return map[string]ingestion.Notices{}, nil
}

func (noIngestions) ListSecretFetches(context.Context, string) ([]ingestion.SecretFetch, error) {
	return nil, nil
}

func (noIngestions) AnyLiveRuntimeCarries(context.Context) (func(string) bool, error) {
	return func(string) bool { return true }, nil
}

func ingestionRouter(t *testing.T, enabled bool) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	if err := Mount(r, Config{
		Authenticate: fakeAuth, CookieSecure: true, DispatchPools: noPools{},
		IngestionEnabled: enabled, Ingestions: noIngestions{}, IngestionOps: pages.IngestionOps{},
	}); err != nil {
		t.Fatal(err)
	}
	return r
}

// With the feature flag off the ingestion pages do not exist (404) and the
// sidebar does not link them; on, both are there.
func TestIngestionPagesFollowTheFeatureFlag(t *testing.T) {
	off := ingestionRouter(t, false)
	for _, target := range []string{"/ui/ingestions", "/ui/ingestions/new", "/ui/ingestions/ing_1", "/ui/ingestions/ing_1/tabs/activity"} {
		if rec := do(t, off, req{method: http.MethodGet, target: target, session: "admin"}); rec.Code != http.StatusNotFound {
			t.Errorf("flag off, %s: status %d", target, rec.Code)
		}
	}
	// The mutating routes are not there either (CSRF aside: same origin here).
	if rec := do(t, off, req{method: http.MethodPost, target: "/ui/ingestions/ing_1/pause", session: "admin"}); rec.Code == http.StatusOK {
		t.Errorf("flag off, pause: status %d", rec.Code)
	}
	page := do(t, off, req{method: http.MethodGet, target: "/ui/dispatch-pools", session: "admin"})
	if strings.Contains(page.Body.String(), "/ui/ingestions") {
		t.Error("the sidebar links ingestions with the flag off")
	}

	on := ingestionRouter(t, true)
	rec := do(t, on, req{method: http.MethodGet, target: "/ui/ingestions", session: "admin"})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `<h1 class="fc-page-title">Ingestions</h1>`) {
		t.Fatalf("flag on: status %d", rec.Code)
	}
	for _, want := range []string{`href="/ui/ingestions" class="fc-nav-item" title="Ingestions" aria-current="page"`, `href="/ui/ingestion-sets"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("flag on: page lacks %s", want)
		}
	}
}
