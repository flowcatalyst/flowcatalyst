package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/pages"
)

// emptyTabs, emptySets and noTests are stores with nothing in them.
type emptyTabs struct{ noIngestions }

func (emptyTabs) ListRouteStates(context.Context, string) ([]ingestion.RouteState, error) {
	return nil, nil
}

func (emptyTabs) ActivityOf(context.Context, string, time.Time) (*ingestion.Activity, error) {
	return &ingestion.Activity{}, nil
}

func (emptyTabs) ListFailures(context.Context, string, string, int) (*ingestion.FailurePage, error) {
	return &ingestion.FailurePage{}, nil
}

func (emptyTabs) GetTraceWindow(context.Context, string) (*ingestion.TraceWindow, error) {
	return nil, nil
}

func (emptyTabs) QueryTraceLog(context.Context, ingestion.TraceLogFilter) ([]ingestion.TraceLogRecord, error) {
	return nil, nil
}

func (emptyTabs) FindWithFilters(context.Context, audit.FilterParams) ([]audit.Log, error) {
	return nil, nil
}

type emptySets struct{}

func (emptySets) ListSets(context.Context) ([]ingestion.Set, error) { return nil, nil }
func (emptySets) FindSetByName(context.Context, string) (*ingestion.Set, error) {
	return nil, nil
}

func (emptySets) ListSetApplies(context.Context, string) ([]ingestion.SetApply, error) {
	return nil, nil
}

func (emptySets) FindSetApply(context.Context, string, string) (*ingestion.SetApply, error) {
	return nil, nil
}

func (emptySets) ApplyFiles(context.Context, ingestion.SetApply) ([]ingestion.SetFile, error) {
	return nil, nil
}

type noTests struct{ noIngestions }

func (noTests) QueueTest(context.Context, *string, ids.ApplicationID, ids.ClientID, ingestion.Definition, *string, time.Time) (*ingestion.TestRequest, error) {
	return nil, nil
}

func (noTests) FindTest(context.Context, string) (*ingestion.TestRequest, error) { return nil, nil }

func (noTests) ApplicationCode(context.Context, ids.ApplicationID) (string, error) { return "", nil }

func (noTests) CheckAuthor(context.Context, *auth.AuthContext, ingestion.Change) error { return nil }

// platformUser reaches every client and application, as a platform user does.
func platformUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ac := &auth.AuthContext{
			PrincipalID: "prn_1", Email: "ada@example.com", Scope: auth.ScopeAnchor, PrincipalType: auth.PrincipalTypeUser,
			Roles: []string{"platform:super-admin"}, Permissions: []string{"platform:*:*:*"}, AllApplications: true,
		}
		next.ServeHTTP(w, r.WithContext(auth.WithContext(r.Context(), ac)))
	})
}

func ingestionAreasRouter(t *testing.T, enabled bool) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	if err := Mount(r, Config{
		Authenticate: platformUser, CookieSecure: true, DispatchPools: noPools{},
		IngestionEnabled: enabled, Ingestions: noIngestions{}, IngestionOps: pages.IngestionOps{},
		IngestionTabs: emptyTabs{}, IngestionAudit: emptyTabs{}, IngestionSets: emptySets{}, IngestionTests: noTests{},
	}); err != nil {
		t.Fatal(err)
	}
	return r
}

func serveIngestion(h http.Handler, method, target string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://fc.example"+target, nil)
	r.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// With the feature flag off the sets, the drawer's panels and test connection
// do not exist (404); on, they are there (the unknown records are 404 too, so
// the check is on the pages that answer 200).
func TestIngestionTabsSetsAndTestsFollowTheFeatureFlag(t *testing.T) {
	off := ingestionAreasRouter(t, false)
	for _, target := range []string{
		"/ui/ingestion-sets", "/ui/ingestion-sets/billing", "/ui/ingestion-sets/billing/applies/isa_1/files",
		"/ui/ingestions/ing_1/activity", "/ui/ingestions/ing_1/failures", "/ui/ingestions/ing_1/history",
		"/ui/ingestion-tests/itr_1/stream",
	} {
		if rec := serveIngestion(off, http.MethodGet, target); rec.Code != http.StatusNotFound {
			t.Errorf("flag off, GET %s: status %d", target, rec.Code)
		}
	}
	for _, target := range []string{"/ui/ingestions/ing_1/test", "/ui/ingestion-sets/billing/release", "/ui/ingestion-sets/billing/applies/isa_1/reapply"} {
		if rec := serveIngestion(off, http.MethodPost, target); rec.Code != http.StatusNotFound {
			t.Errorf("flag off, POST %s: status %d", target, rec.Code)
		}
	}

	on := ingestionAreasRouter(t, true)
	rec := serveIngestion(on, http.MethodGet, "/ui/ingestion-sets")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `<h1 class="fc-page-title">Ingestion sets</h1>`) {
		t.Fatalf("flag on, sets: status %d", rec.Code)
	}
	for _, target := range []string{"/ui/ingestions/ing_1/activity", "/ui/ingestion-tests/itr_1/stream", "/ui/ingestion-sets/billing"} {
		if rec := serveIngestion(on, http.MethodGet, target); rec.Code != http.StatusNotFound {
			t.Errorf("flag on, unknown record %s: status %d", target, rec.Code)
		}
	}
	// The posts are mounted: a post without the page's origin is refused by the
	// guard, which only a matched route runs.
	for _, target := range []string{"/ui/ingestions/ing_1/test", "/ui/ingestion-sets/billing/release"} {
		if rec := serveIngestion(on, http.MethodPost, target); rec.Code == http.StatusNotFound {
			t.Errorf("flag on, POST %s: not mounted", target)
		}
	}
}
