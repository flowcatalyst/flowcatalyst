package pages

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

type fakePools struct {
	pools      []dispatchpool.DispatchPool
	err        error
	lastStatus *string
	calls      int
}

func (f *fakePools) FindWithFilters(_ context.Context, status, _ *string) ([]dispatchpool.DispatchPool, error) {
	f.calls++
	f.lastStatus = status
	if f.err != nil {
		return nil, f.err
	}
	var out []dispatchpool.DispatchPool
	for _, p := range f.pools {
		if status == nil || string(p.Status) == *status {
			out = append(out, p)
		}
	}
	return out, nil
}

func pool(id, code, name string, status dispatchpool.Status, client string, created time.Time) dispatchpool.DispatchPool {
	p := dispatchpool.DispatchPool{ID: id, Code: code, Name: name, Status: status, Concurrency: 10, CreatedAt: created, UpdatedAt: created}
	if client != "" {
		cid := ids.ClientID("clt_" + client)
		ident := client
		p.ClientID, p.ClientIdentifier = &cid, &ident
	}
	return p
}

func fixturePools() *fakePools {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	rate := int32(600)
	billing := pool("dpl_1", "billing", "Billing", dispatchpool.StatusActive, "", day)
	billing.RateLimit = &rate
	return &fakePools{pools: []dispatchpool.DispatchPool{
		billing,
		pool("dpl_2", "acme-orders", "Acme orders", dispatchpool.StatusSuspended, "acme", day.Add(48*time.Hour)),
		pool("dpl_3", "archive", "Old <b>pool</b>", dispatchpool.StatusArchived, "", day.Add(-48*time.Hour)),
		pool("dpl_4", "globex-feed", "Globex feed", dispatchpool.StatusActive, "globex", day.Add(24*time.Hour)),
	}}
}

func anchorAdmin() *auth.AuthContext {
	return &auth.AuthContext{
		PrincipalID: "prn_1", Email: "admin@flowcatalyst.local", Scope: auth.ScopeAnchor,
		Roles: []string{"platform:super-admin"}, Permissions: []string{"platform:*:*:*"}, PrincipalType: auth.PrincipalTypeUser,
	}
}

func serve(t *testing.T, finder PoolFinder, ac *auth.AuthContext, target string, datastar bool) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Route("/ui", func(r chi.Router) {
		(&DispatchPools{Pools: finder, Shell: &shell.Shell{Version: "test"}}).Mount(r)
	})
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Accept", "text/html")
	if datastar {
		req.Header.Set(ds.RequestHeader, "true")
		req.Header.Set("Accept", "text/event-stream, text/html, application/json")
	}
	if ac != nil {
		req = req.WithContext(auth.WithContext(req.Context(), ac))
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

var rowIDs = regexp.MustCompile(`id="pool-(dpl_\d)"`)

func rowOrder(body string) []string {
	var out []string
	for _, m := range rowIDs.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}

func TestPoolsPageRendersInsideTheShell(t *testing.T) {
	f := fixturePools()
	rec := serve(t, f, anchorAdmin(), "/ui/dispatch-pools", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<title>Dispatch Pools · FlowCatalyst</title>`,
		`class="fc-sidebar"`,
		`href="/ui/dispatch-pools" class="fc-nav-item" title="Dispatch Pools" aria-current="page"`, // ported entry, current
		`href="/connections" class="fc-nav-item"`,                                                  // unported entry → SPA
		`<h1 class="fc-page-title">Dispatch Pools</h1>`,
		`href="/dispatch-pools/new"`, // Create opens the SPA
		`id="pools-list"`, `id="pools-table"`, `id="pools-rows"`, `id="pools-summary"`,
		`600/min`, `Anchor-level`, `acme`,
		`Old &lt;b&gt;pool&lt;/b&gt;`, // escaped
		`aria-sort="ascending"`,
		`4 dispatch pools`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	if strings.Contains(body, "<b>pool</b>") {
		t.Error("pool name rendered unescaped")
	}
	if got := rowOrder(body); strings.Join(got, ",") != "dpl_2,dpl_3,dpl_1,dpl_4" {
		t.Errorf("default order (code asc) = %v", got)
	}
	if f.lastStatus != nil {
		t.Errorf("no status filter expected, got %q", *f.lastStatus)
	}
}

func TestPoolsFilterSearchSort(t *testing.T) {
	t.Run("status goes to the repository", func(t *testing.T) {
		f := fixturePools()
		body := serve(t, f, anchorAdmin(), "/ui/dispatch-pools?status=ACTIVE", false).Body.String()
		if f.lastStatus == nil || *f.lastStatus != "ACTIVE" {
			t.Fatalf("status passed = %v", f.lastStatus)
		}
		if got := rowOrder(body); strings.Join(got, ",") != "dpl_1,dpl_4" {
			t.Errorf("rows = %v", got)
		}
		if !strings.Contains(body, `<span class="fc-badge">1</span>`) || !strings.Contains(body, `href="/ui/dispatch-pools" class="fc-btn fc-btn-text"`) {
			t.Error("active filter count badge or Clear All missing")
		}
	})
	t.Run("an unknown status is ignored, not an error", func(t *testing.T) {
		f := fixturePools()
		if rec := serve(t, f, anchorAdmin(), "/ui/dispatch-pools?status=BOGUS", false); rec.Code != http.StatusOK || f.lastStatus != nil {
			t.Fatalf("status %d, filter %v", rec.Code, f.lastStatus)
		}
	})
	t.Run("search matches code, name and client identifier", func(t *testing.T) {
		body := serve(t, fixturePools(), anchorAdmin(), "/ui/dispatch-pools?q=GLOBEX", false).Body.String()
		if got := rowOrder(body); strings.Join(got, ",") != "dpl_4" {
			t.Errorf("rows = %v", got)
		}
		if !strings.Contains(body, "Showing 1 of 4 dispatch pools") || !strings.Contains(body, `value="GLOBEX"`) {
			t.Error("summary or search value missing")
		}
	})
	t.Run("sort by created descending, links keep the filters", func(t *testing.T) {
		body := serve(t, fixturePools(), anchorAdmin(), "/ui/dispatch-pools?sort=created&dir=desc&q=o", false).Body.String()
		if got := rowOrder(body); strings.Join(got, ",") != "dpl_2,dpl_4,dpl_3" {
			t.Errorf("rows = %v", got)
		}
		if !strings.Contains(body, `aria-sort="descending"`) {
			t.Error("aria-sort missing")
		}
		// The Created heading toggles back to ascending; the Name heading starts ascending; both keep q.
		for _, href := range []string{`/ui/dispatch-pools?dir=asc&amp;q=o&amp;sort=created`, `/ui/dispatch-pools?dir=asc&amp;q=o&amp;sort=name`} {
			if !strings.Contains(body, href) {
				t.Errorf("sort link %s missing", href)
			}
		}
		// The search form carries the sort so typing keeps it.
		if !strings.Contains(body, `<input type="hidden" name="sort" value="created">`) {
			t.Error("hidden sort field missing")
		}
	})
	t.Run("nothing found shows the empty state with Clear filters", func(t *testing.T) {
		body := serve(t, fixturePools(), anchorAdmin(), "/ui/dispatch-pools?q=nomatch", false).Body.String()
		if !strings.Contains(body, "No dispatch pools found") || !strings.Contains(body, `<a href="/ui/dispatch-pools" class="fc-btn fc-btn-text-primary">Clear filters</a>`) {
			t.Error("empty state missing")
		}
		if !strings.Contains(body, `colspan="8"`) {
			t.Error("empty row does not span the 8 columns")
		}
	})
}

func TestPoolsDatastarPatches(t *testing.T) {
	rec := serve(t, fixturePools(), anchorAdmin(), "/ui/dispatch-pools?q=acme&status=SUSPENDED", true)
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type %q, body %s", ct, rec.Body.String())
	}
	body := rec.Body.String()
	events := strings.Count(body, "event: datastar-patch-elements\n")
	if events != 4 {
		t.Fatalf("%d patch events, want 4 (table, summary, filters button, url):\n%s", events, body)
	}
	for _, want := range []string{
		`data: elements <div id="pools-table"`,
		`data: elements <div id="pools-summary"`,
		"data: selector #pools-list [popovertarget]\n",
		`<span class="fc-badge">1</span>`,
		`history.replaceState(history.state, &#39;&#39;, &#34;/ui/dispatch-pools?q=acme\u0026status=SUSPENDED&#34;)`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("SSE lacks %q", want)
		}
	}
	if strings.Contains(body, "fc-sidebar") {
		t.Error("a live update must not resend the shell")
	}
	if got := rowOrder(body); strings.Join(got, ",") != "dpl_2" {
		t.Errorf("rows = %v", got)
	}
}

func TestPoolsPermissions(t *testing.T) {
	noPerm := &auth.AuthContext{
		PrincipalID: "prn_2", Email: "x@y.z", Scope: auth.ScopeAnchor, Roles: []string{"platform:other"},
		Permissions: []string{"platform:messaging:event:view"}, PrincipalType: auth.PrincipalTypeUser,
	}
	t.Run("no dispatch-pool:view permission is a 403 page and no query", func(t *testing.T) {
		f := fixturePools()
		rec := serve(t, f, noPerm, "/ui/dispatch-pools", false)
		if rec.Code != http.StatusForbidden || f.calls != 0 {
			t.Fatalf("status %d, repository calls %d", rec.Code, f.calls)
		}
		if !strings.Contains(rec.Body.String(), "Access denied") || !strings.Contains(rec.Body.String(), "fc-sidebar") {
			t.Error("403 page should render in the shell")
		}
	})
	t.Run("Datastar request without permission gets a bare 403", func(t *testing.T) {
		rec := serve(t, fixturePools(), noPerm, "/ui/dispatch-pools", true)
		if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "<html") {
			t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
		}
	})
	t.Run("client-scoped viewer sees platform pools and their own client's only", func(t *testing.T) {
		viewer := &auth.AuthContext{
			PrincipalID: "prn_3", Email: "ops@acme.example", Scope: auth.ScopeClient, Clients: []string{"clt_acme"},
			Roles: []string{"acme:viewer"}, Permissions: []string{"platform:messaging:dispatch-pool:view"}, PrincipalType: auth.PrincipalTypeUser,
		}
		body := serve(t, fixturePools(), viewer, "/ui/dispatch-pools", false).Body.String()
		if got := rowOrder(body); strings.Join(got, ",") != "dpl_2,dpl_3,dpl_1" {
			t.Errorf("rows = %v (globex must be hidden)", got)
		}
		if strings.Contains(body, `href="/dispatch-pools/new"`) || strings.Contains(body, `aria-label="Edit `) {
			t.Error("a viewer without create/update must not get Create or Edit")
		}
		if strings.Contains(body, `title="Dashboard"`) {
			t.Error("anchor-only nav entry shown to a client user")
		}
	})
	t.Run("repository failure is a 500 without details", func(t *testing.T) {
		rec := serve(t, &fakePools{err: errors.New("pg: connection refused")}, anchorAdmin(), "/ui/dispatch-pools", false)
		if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "connection refused") {
			t.Fatalf("status %d, leaked: %v", rec.Code, strings.Contains(rec.Body.String(), "connection refused"))
		}
	})
}

func TestPoolQueryURLRoundTrip(t *testing.T) {
	q := parsePoolQuery(url.Values{"q": {" a&b "}, "status": {"ARCHIVED"}, "sort": {"name"}, "dir": {"desc"}})
	if q.URL() != "/ui/dispatch-pools?dir=desc&q=a%26b&sort=name&status=ARCHIVED" {
		t.Fatalf("URL = %s", q.URL())
	}
	if again := parsePoolQuery(mustQuery(t, q.URL())); again != q {
		t.Fatalf("round trip: %+v != %+v", again, q)
	}
	if def := parsePoolQuery(url.Values{"sort": {"evil"}, "dir": {"sideways"}}); def.URL() != "/ui/dispatch-pools" {
		t.Fatalf("defaults should encode to the bare path, got %s", def.URL())
	}
	long := parsePoolQuery(url.Values{"q": {strings.Repeat("x", 500)}})
	if len(long.Q) != maxQueryLen {
		t.Fatalf("search not capped: %d", len(long.Q))
	}
}

func mustQuery(t *testing.T, u string) url.Values {
	t.Helper()
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Query()
}
