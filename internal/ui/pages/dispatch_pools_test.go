package pages

import (
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// ── fakes ──────────────────────────────────────────────────────────────

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

func (f *fakePools) FindByID(_ context.Context, id string) (*dispatchpool.DispatchPool, error) {
	if f.err != nil {
		return nil, f.err
	}
	for i := range f.pools {
		if f.pools[i].ID == id {
			p := f.pools[i]
			return &p, nil
		}
	}
	return nil, nil
}

// fakeOps records the commands the page runs; err is returned by every call.
type fakeOps struct {
	created []operations.CreateCommand
	updated []operations.UpdateCommand
	actions []string // "suspend:dpl_1"
	err     error
	newID   string
	pools   *fakePools
}

func (o *fakeOps) Create(_ context.Context, cmd operations.CreateCommand) (string, error) {
	o.created = append(o.created, cmd)
	if o.err != nil {
		return "", o.err
	}
	id := o.newID
	if id == "" {
		id = "dpl_9"
	}
	if o.pools != nil {
		o.pools.pools = append(o.pools.pools, pool(id, cmd.Code, cmd.Name, dispatchpool.StatusActive, "", time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)))
	}
	return id, nil
}

func (o *fakeOps) Update(_ context.Context, cmd operations.UpdateCommand) error {
	o.updated = append(o.updated, cmd)
	return o.err
}

func (o *fakeOps) act(name, id string) error {
	o.actions = append(o.actions, name+":"+id)
	return o.err
}

func (o *fakeOps) Suspend(_ context.Context, id string) error  { return o.act("suspend", id) }
func (o *fakeOps) Activate(_ context.Context, id string) error { return o.act("activate", id) }
func (o *fakeOps) Delete(_ context.Context, id string) error   { return o.act("delete", id) }

type fakeClients struct{ clients []client.Client }

func (f fakeClients) Search(_ context.Context, term string) ([]client.Client, error) {
	var out []client.Client
	for _, c := range f.clients {
		if strings.Contains(strings.ToLower(c.Name+" "+c.Identifier), strings.ToLower(term)) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f fakeClients) FindByIdentifier(_ context.Context, identifier string) (*client.Client, error) {
	for _, c := range f.clients {
		if c.Identifier == identifier {
			return &c, nil
		}
	}
	return nil, nil
}

func someClients() fakeClients {
	return fakeClients{clients: []client.Client{
		{ID: "clt_acme", Name: "Acme Corp", Identifier: "acme"},
		{ID: "clt_globex", Name: "Globex <Inc>", Identifier: "globex"},
	}}
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
	desc := "Invoices <b>and</b> receipts"
	billing.Description = &desc
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

// poolWriter may create/update dispatch pools but not read clients or delete.
func poolWriter() *auth.AuthContext {
	return &auth.AuthContext{
		PrincipalID: "prn_4", Email: "ops@flowcatalyst.local", Scope: auth.ScopeAnchor, Roles: []string{"platform:ops"},
		Permissions:   []string{"platform:messaging:dispatch-pool:view", "platform:messaging:dispatch-pool:create", "platform:messaging:dispatch-pool:update"},
		PrincipalType: auth.PrincipalTypeUser,
	}
}

// ── harness ────────────────────────────────────────────────────────────

type harness struct {
	pools   *fakePools
	ops     *fakeOps
	clients ClientDirectory
}

func newHarness() *harness {
	h := &harness{pools: fixturePools(), clients: someClients()}
	h.ops = &fakeOps{pools: h.pools}
	return h
}

func shellForTest() *shell.Shell { return &shell.Shell{Version: "test", Flashes: FlashMessages} }

type req struct {
	method, target string
	form           url.Values
	datastar       bool
	referer        string
	cookie         *http.Cookie
	// multipart sends the form as Datastar does ({contentType: 'form'}
	// posts FormData), instead of urlencoded as a native form.
	multipart bool
	// headers are extra request headers.
	headers map[string]string
}

// do serves one request through the page built from the fakes.
func (h *harness) do(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return h.serveWith(t, &DispatchPools{Pools: h.pools, Ops: h.ops, Clients: h.clients, Shell: shellForTest()}, ac, q)
}

// serveWith serves one request through page (and the client options
// endpoint) mounted under /ui, as ui.Mount does (without the guards).
func (h *harness) serveWith(t *testing.T, page *DispatchPools, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, page, &ClientOptions{Clients: h.clients, Shell: page.Shell})
}

// mounter is a page (or endpoint) that mounts its routes under /ui.
type mounter interface{ Mount(chi.Router) }

// serveMounted serves one request through pages mounted under /ui, as
// ui.Mount does (without the guards).
func serveMounted(t *testing.T, ac *auth.AuthContext, q req, pages ...mounter) *httptest.ResponseRecorder {
	t.Helper()
	mux := chi.NewRouter()
	mux.Route("/ui", func(r chi.Router) {
		for _, p := range pages {
			p.Mount(r)
		}
	})
	method := q.method
	if method == "" {
		method = http.MethodGet
	}
	body, contentType := "", "application/x-www-form-urlencoded"
	if q.form != nil {
		body = q.form.Encode()
		if q.multipart {
			var b strings.Builder
			mw := multipart.NewWriter(&b)
			for k, vs := range q.form {
				for _, v := range vs {
					if err := mw.WriteField(k, v); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := mw.Close(); err != nil {
				t.Fatal(err)
			}
			body, contentType = b.String(), mw.FormDataContentType()
		}
	}
	r := httptest.NewRequest(method, q.target, strings.NewReader(body))
	if q.form != nil {
		r.Header.Set("Content-Type", contentType)
	}
	r.Header.Set("Accept", "text/html")
	if q.datastar {
		r.Header.Set(ds.RequestHeader, "true")
		r.Header.Set("Accept", "text/event-stream, text/html, application/json")
	}
	if q.referer != "" {
		r.Header.Set("Referer", q.referer)
	}
	for k, v := range q.headers {
		r.Header.Set(k, v)
	}
	if q.cookie != nil {
		r.AddCookie(q.cookie)
	}
	if ac != nil {
		r = r.WithContext(auth.WithContext(r.Context(), ac))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}

// serve is the list harness the list tests use.
func serve(t *testing.T, finder *fakePools, ac *auth.AuthContext, target string, datastar bool) *httptest.ResponseRecorder {
	t.Helper()
	h := newHarness()
	h.pools = finder
	h.ops.pools = finder
	return h.do(t, ac, req{target: target, datastar: datastar})
}

var rowIDs = regexp.MustCompile(`id="pool-(dpl_\d)"`)

func rowOrder(body string) []string {
	var out []string
	for _, m := range rowIDs.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}

func mustContain(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("response lacks %s", want)
		}
	}
}

func mustNotContain(t *testing.T, body string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(body, u) {
			t.Errorf("response must not contain %s", u)
		}
	}
}

// ── list ───────────────────────────────────────────────────────────────

func TestPoolsPageRendersInsideTheShell(t *testing.T) {
	f := fixturePools()
	rec := serve(t, f, anchorAdmin(), "/ui/dispatch-pools", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	mustContain(t, body,
		`<title>Dispatch Pools · FlowCatalyst</title>`,
		`class="fc-sidebar"`,
		`href="/ui/dispatch-pools" class="fc-nav-item" title="Dispatch Pools" aria-current="page"`, // ported entry, current
		`href="/ui/connections" class="fc-nav-item"`,                                               // ported entry
		`href="/functions" class="fc-nav-item"`,                                                    // unported entry → SPA
		`<h1 class="fc-page-title">Dispatch Pools</h1>`,
		`href="/ui/dispatch-pools/new"`, // Create opens the form dialog
		`id="pools-list"`, `id="pools-table"`, `id="pools-rows"`, `id="pools-summary"`,
		`600/min`, `Anchor-level`, `acme`,
		`Old &lt;b&gt;pool&lt;/b&gt;`, // escaped
		`aria-sort="ascending"`,
		`4 dispatch pools`,
		`<div id="pool-dialog-slot"></div>`,
		`data-signals="{selPool: &#39;&#39;}"`,
		`<aside id="pool-drawer" class="fc-drawer"`,
		`<dialog id="pool-activate"`, `<dialog id="pool-suspend"`, `<dialog id="pool-delete"`,
	)
	mustNotContain(t, body, "<b>pool</b>", "Not in the new UI yet")
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
		mustContain(t, body, `<span class="fc-badge">1</span>`, `href="/ui/dispatch-pools" class="fc-btn fc-btn-text"`)
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
		mustContain(t, body, "Showing 1 of 4 dispatch pools", `value="GLOBEX"`)
	})
	t.Run("sort by created descending, links keep the filters", func(t *testing.T) {
		body := serve(t, fixturePools(), anchorAdmin(), "/ui/dispatch-pools?sort=created&dir=desc&q=o", false).Body.String()
		if got := rowOrder(body); strings.Join(got, ",") != "dpl_2,dpl_4,dpl_3" {
			t.Errorf("rows = %v", got)
		}
		mustContain(t, body, `aria-sort="descending"`,
			`/ui/dispatch-pools?dir=asc&amp;q=o&amp;sort=created`, `/ui/dispatch-pools?dir=asc&amp;q=o&amp;sort=name`,
			`<input type="hidden" name="sort" value="created">`)
	})
	t.Run("nothing found shows the empty state with Clear filters", func(t *testing.T) {
		body := serve(t, fixturePools(), anchorAdmin(), "/ui/dispatch-pools?q=nomatch", false).Body.String()
		mustContain(t, body, "No dispatch pools found", `<a href="/ui/dispatch-pools" class="fc-btn fc-btn-text-primary">Clear filters</a>`, `colspan="8"`)
	})
}

func TestPoolsDatastarPatches(t *testing.T) {
	rec := serve(t, fixturePools(), anchorAdmin(), "/ui/dispatch-pools?q=acme&status=SUSPENDED", true)
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type %q, body %s", ct, rec.Body.String())
	}
	body := rec.Body.String()
	if events := strings.Count(body, "event: datastar-patch-elements\n"); events != 4 {
		t.Fatalf("%d patch events, want 4 (table, summary, toolbar state, url):\n%s", events, body)
	}
	mustContain(t, body,
		`data: elements <div id="pools-table"`,
		`data: elements <div id="pools-summary"`,
		`data: elements <div id="pools-list-state" class="fc-toolbar-state">`,
		`<span class="fc-badge">1</span>`,
		`Clear All</a>`,
		`history.replaceState(history.state, &#39;&#39;, &#34;/ui/dispatch-pools?q=acme\u0026status=SUSPENDED&#34;)`,
	)
	mustNotContain(t, body, "fc-sidebar", `name="q"`, `popover class="fc-filter-popover"`)
	if got := rowOrder(body); strings.Join(got, ",") != "dpl_2" {
		t.Errorf("rows = %v", got)
	}
}

func TestPoolsToolbarStateFollowsSortAndClear(t *testing.T) {
	body := serve(t, fixturePools(), anchorAdmin(), "/ui/dispatch-pools?sort=name&dir=desc&q=o", true).Body.String()
	mustContain(t, body,
		`<input type="hidden" name="sort" value="name">`,
		`<input type="hidden" name="dir" value="desc">`,
		`<a href="/ui/dispatch-pools?dir=desc&amp;sort=name" class="fc-btn fc-btn-text"`)
	cleared := serve(t, fixturePools(), anchorAdmin(), "/ui/dispatch-pools?q=&status=&sort=name&dir=desc", true).Body.String()
	if !strings.Contains(cleared, `data: elements <div id="pools-list-state"`) || strings.Contains(cleared, "Clear All") || strings.Contains(cleared, "fc-badge") {
		t.Errorf("cleared state patch wrong:\n%s", cleared)
	}
	mustContain(t, cleared, `/ui/dispatch-pools?dir=desc\u0026sort=name`)
	page := serve(t, fixturePools(), anchorAdmin(), "/ui/dispatch-pools?status=ACTIVE", false).Body.String()
	mustContain(t, page, `data-on:click__prevent="const f = el.closest(&#39;form&#39;);`)
}

// menuOf returns the markup of a row's overflow menu.
func menuOf(t *testing.T, body, id string) string {
	t.Helper()
	_, menu, ok := strings.Cut(body, `id="pool-`+id+`-actions-menu"`)
	if !ok {
		t.Fatalf("pool %s has no menu", id)
	}
	menu, _, _ = strings.Cut(menu, "</td>")
	return menu
}

func TestPoolsRowActions(t *testing.T) {
	body := serve(t, fixturePools(), anchorAdmin(), "/ui/dispatch-pools", false).Body.String()
	mustContain(t, body,
		// inline Edit opens the edit dialog through Datastar (a link without it)
		`<a href="/ui/dispatch-pools/dpl_1/edit" class="fc-icon-btn" title="Edit" aria-label="Edit Billing" aria-haspopup="dialog" data-on:click__prevent="@get(el.href)">`,
		`aria-label="More actions for Billing" popovertarget="pool-dpl_1-actions-menu"`,
		// the name opens the drawer
		`<a href="/ui/dispatch-pools/dpl_1" class="font-medium text-foreground no-underline hover:underline" data-on:click__prevent="@get(el.href)">Billing</a>`,
		// archived: Edit disabled with the reason
		`aria-label="Edit Old &lt;b&gt;pool&lt;/b&gt;" aria-disabled="true"`, `Archived pools cannot be edited.`,
	)
	active := menuOf(t, body, "dpl_1")
	mustContain(t, active,
		`<a href="/ui/dispatch-pools/dpl_1" class="fc-menu-item"`, "View details",
		`data-action="/ui/dispatch-pools/dpl_1/suspend"`, "Suspend…",
		`d.querySelector(&#39;form&#39;).action = el.dataset.action`,
		`<div role="separator" class="fc-menu-separator"></div>`,
		`class="fc-menu-item danger"`, `data-action="/ui/dispatch-pools/dpl_1/delete"`, "Delete…",
	)
	mustNotContain(t, active, "Activate…", "aria-disabled")
	mustContain(t, menuOf(t, body, "dpl_2"), `data-action="/ui/dispatch-pools/dpl_2/activate"`, "Activate…")
	archived := menuOf(t, body, "dpl_3")
	if strings.Contains(archived, "Delete") || strings.Contains(archived, "Activate") || strings.Contains(archived, "Suspend") || !strings.Contains(archived, "View details") {
		t.Errorf("archived pool menu = %s", archived)
	}
}

func TestPoolsPermissions(t *testing.T) {
	noPerm := &auth.AuthContext{
		PrincipalID: "prn_2", Email: "x@y.z", Scope: auth.ScopeAnchor, Roles: []string{"platform:other"},
		Permissions: []string{"platform:messaging:event:view"}, PrincipalType: auth.PrincipalTypeUser,
	}
	viewer := &auth.AuthContext{
		PrincipalID: "prn_3", Email: "ops@acme.example", Scope: auth.ScopeClient, Clients: []string{"clt_acme"},
		Roles: []string{"acme:viewer"}, Permissions: []string{"platform:messaging:dispatch-pool:view"}, PrincipalType: auth.PrincipalTypeUser,
	}
	t.Run("no dispatch-pool:view permission is a 403 page and no query", func(t *testing.T) {
		f := fixturePools()
		rec := serve(t, f, noPerm, "/ui/dispatch-pools", false)
		if rec.Code != http.StatusForbidden || f.calls != 0 {
			t.Fatalf("status %d, repository calls %d", rec.Code, f.calls)
		}
		mustContain(t, rec.Body.String(), "Access denied", "fc-sidebar")
	})
	t.Run("Datastar request without permission gets 200 and an error flash, no rows", func(t *testing.T) {
		f := fixturePools()
		rec := serve(t, f, noPerm, "/ui/dispatch-pools", true)
		body := rec.Body.String()
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/event-stream" || f.calls != 0 {
			t.Fatalf("status %d, Content-Type %q, repository calls %d", rec.Code, rec.Header().Get("Content-Type"), f.calls)
		}
		mustContain(t, body, "data: selector #flash-stack\n", "data: mode append\n", `class="fc-flash fc-flash-error" role="alert"`, "Access denied.")
		mustNotContain(t, body, "pools-table", "<html")
	})
	t.Run("client-scoped viewer sees platform pools and their own client's only, and no actions", func(t *testing.T) {
		body := serve(t, fixturePools(), viewer, "/ui/dispatch-pools", false).Body.String()
		if got := rowOrder(body); strings.Join(got, ",") != "dpl_2,dpl_3,dpl_1" {
			t.Errorf("rows = %v (globex must be hidden)", got)
		}
		mustNotContain(t, body, `href="/ui/dispatch-pools/new"`, `aria-label="Edit `, "Suspend…", "Activate…", "Delete…",
			`<dialog id="pool-suspend"`, `<dialog id="pool-delete"`, `title="Dashboard"`)
		mustContain(t, body, "View details")
	})
	t.Run("write without delete: Create, Edit and Suspend, no Delete", func(t *testing.T) {
		body := serve(t, fixturePools(), poolWriter(), "/ui/dispatch-pools", false).Body.String()
		mustContain(t, body, `href="/ui/dispatch-pools/new"`, `aria-label="Edit Billing"`, "Suspend…", `<dialog id="pool-suspend"`)
		mustNotContain(t, body, "Delete…", `<dialog id="pool-delete"`)
	})
	t.Run("repository failure is a 500 without details", func(t *testing.T) {
		rec := serve(t, &fakePools{err: errors.New("pg: connection refused")}, anchorAdmin(), "/ui/dispatch-pools", false)
		if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "connection refused") {
			t.Fatalf("status %d, leaked: %v", rec.Code, strings.Contains(rec.Body.String(), "connection refused"))
		}
	})
	t.Run("repository failure on a live request is an error flash without details", func(t *testing.T) {
		rec := serve(t, &fakePools{err: errors.New("pg: connection refused")}, anchorAdmin(), "/ui/dispatch-pools", true)
		body := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(body, "Something went wrong.") || strings.Contains(body, "connection refused") {
			t.Fatalf("status %d body %s", rec.Code, body)
		}
	})
}

func TestPoolQueryURLRoundTrip(t *testing.T) {
	q := parsePoolQuery(url.Values{"q": {" a&b "}, "status": {"ARCHIVED"}, "sort": {"name"}, "dir": {"desc"}})
	if q.URL() != "/ui/dispatch-pools?dir=desc&q=a%26b&sort=name&status=ARCHIVED" {
		t.Fatalf("URL = %s", q.URL())
	}
	if again := parsePoolQuery(mustQuery(t, q.URL())); !reflect.DeepEqual(again, q) {
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

// ── drawer ─────────────────────────────────────────────────────────────

func TestPoolDrawer(t *testing.T) {
	h := newHarness()
	t.Run("Datastar: content patched into the frame, signal set, focus on the title", func(t *testing.T) {
		body := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-pools/dpl_1", datastar: true}).Body.String()
		mustContain(t, body,
			"data: selector #pool-drawer\n", "data: mode inner\n",
			`<h2 id="pool-drawer-title" tabindex="-1" class="fc-drawer-title">Billing</h2>`,
			`<p class="fc-drawer-subtitle">billing</p>`,
			"Pool Details", "600 / minute", "Anchor-level (no client)", "Invoices &lt;b&gt;and&lt;/b&gt; receipts",
			`href="/ui/dispatch-pools/dpl_1/edit"`, // Edit
			"Suspend Pool", `data-action="/ui/dispatch-pools/dpl_1/suspend"`, "Delete Pool",
			"event: datastar-patch-signals\n", `"selPool":"dpl_1"`,
			`document.getElementById(&#34;pool-drawer-title&#34;)?.focus()`,
		)
		mustNotContain(t, body, "Activate Pool", "<b>and</b>")
	})
	t.Run("suspended: Activate instead of Suspend", func(t *testing.T) {
		body := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-pools/dpl_2", datastar: true}).Body.String()
		mustContain(t, body, "Activate Pool", "Unlimited (concurrency-only)", ">acme<")
		mustNotContain(t, body, "Suspend Pool")
	})
	t.Run("archived: read-only, no Edit, no Actions", func(t *testing.T) {
		body := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-pools/dpl_3", datastar: true}).Body.String()
		mustNotContain(t, body, "/edit", "Actions</h3>", "Delete Pool")
	})
	t.Run("navigation: the list page with the drawer open", func(t *testing.T) {
		rec := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-pools/dpl_1"})
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		mustContain(t, rec.Body.String(), `data-signals="{selPool: &#39;dpl_1&#39;}"`, `id="pool-drawer-title"`, "Pool Details", `id="pools-table"`)
	})
	t.Run("another client's pool is refused, an unknown or malformed id is not found", func(t *testing.T) {
		viewer := &auth.AuthContext{
			PrincipalID: "prn_3", Email: "ops@acme.example", Scope: auth.ScopeClient, Clients: []string{"clt_acme"},
			Roles: []string{"acme:viewer"}, Permissions: []string{"platform:messaging:dispatch-pool:view"}, PrincipalType: auth.PrincipalTypeUser,
		}
		if rec := h.do(t, viewer, req{target: "/ui/dispatch-pools/dpl_4"}); rec.Code != http.StatusForbidden {
			t.Errorf("globex pool for an acme viewer: status %d", rec.Code)
		}
		body := h.do(t, viewer, req{target: "/ui/dispatch-pools/dpl_2", datastar: true}).Body.String()
		mustNotContain(t, body, "/edit", "Suspend Pool", "Activate Pool", "Delete Pool")
		if rec := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-pools/dpl_404"}); rec.Code != http.StatusNotFound {
			t.Errorf("unknown pool: status %d", rec.Code)
		}
		if rec := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-pools/x'y"}); rec.Code != http.StatusNotFound {
			t.Errorf("malformed id: status %d", rec.Code)
		}
	})
}

// ── create ─────────────────────────────────────────────────────────────

func createForm(kv ...string) url.Values {
	v := url.Values{"code": {"orders"}, "name": {"Orders"}, "description": {""}, "rateLimit": {""}, "concurrency": {"10"}, "client": {""}}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return v
}

func TestPoolCreateDialog(t *testing.T) {
	h := newHarness()
	t.Run("Datastar: the dialog is patched into the slot, open, with client suggestions", func(t *testing.T) {
		body := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-pools/new", datastar: true}).Body.String()
		mustContain(t, body,
			"data: mode replace\n", `data: elements <div id="pool-dialog-slot"><dialog id="pool-form"`,
			`<dialog id="pool-form"`, ` open>`, `data-init="el.open &amp;&amp; el.close(); el.showModal()"`,
			`<form id="pool-form-form" method="post" action="/ui/dispatch-pools/new"`,
			`id="pool-form-code-field"`, `id="pool-form-concurrency" name="concurrency" value="10"`,
			`<option value="acme">Acme Corp</option>`, `<option value="globex">Globex &lt;Inc&gt;</option>`,
			`/ui/clients/options?list=pool-form-client-options&amp;q=`,
			`group-has-[#pool-form-anchor:checked]/scope:hidden`,
			"Create Pool",
		)
	})
	t.Run("without permission to view clients the client field is disabled and says why", func(t *testing.T) {
		body := h.do(t, poolWriter(), req{target: "/ui/dispatch-pools/new", datastar: true}).Body.String()
		mustContain(t, body, "Choosing a client needs permission to view clients.", `list="pool-form-client-options" autocomplete="off" disabled`)
		mustNotContain(t, body, "Acme Corp", "/ui/clients/options?")
	})
	t.Run("navigation: the list page with the dialog open", func(t *testing.T) {
		body := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-pools/new"}).Body.String()
		mustContain(t, body, `<div id="pool-dialog-slot"><dialog id="pool-form"`, `id="pools-table"`)
	})
	t.Run("a viewer cannot open it", func(t *testing.T) {
		viewer := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeAnchor, Roles: []string{"r"}, Permissions: []string{"platform:messaging:dispatch-pool:view"}, PrincipalType: auth.PrincipalTypeUser}
		if rec := h.do(t, viewer, req{target: "/ui/dispatch-pools/new"}); rec.Code != http.StatusForbidden {
			t.Errorf("status %d", rec.Code)
		}
	})
}

func TestPoolCreateSubmit(t *testing.T) {
	t.Run("Datastar success: the use case runs, the dialog closes, the new pool opens in the drawer, a flash", func(t *testing.T) {
		h := newHarness()
		rec := h.do(t, anchorAdmin(), req{
			method: http.MethodPost, target: "/ui/dispatch-pools/new", datastar: true,
			form: createForm("description", "For orders", "rateLimit", "120", "client", "acme"), referer: "http://x/ui/dispatch-pools?status=ACTIVE",
		})
		if len(h.ops.created) != 1 {
			t.Fatalf("create calls %d", len(h.ops.created))
		}
		cmd := h.ops.created[0]
		if cmd.Code != "orders" || cmd.Name != "Orders" || deref(cmd.Description) != "For orders" || cmd.RateLimit == nil || *cmd.RateLimit != 120 ||
			cmd.Concurrency == nil || *cmd.Concurrency != 10 || deref(cmd.ClientID) != "clt_acme" {
			t.Errorf("command = %+v", cmd)
		}
		body := rec.Body.String()
		mustContain(t, body,
			`document.getElementById(&#34;pool-form&#34;)?.close()`,
			`data: elements <div id="pools-table"`,
			"data: selector #pool-drawer\n", `"selPool":"dpl_9"`,
			`class="fc-flash fc-flash-success"`, "Dispatch pool created",
		)
		// The table is the view the user was on (the Referer's status filter).
		if h.pools.lastStatus == nil || *h.pools.lastStatus != "ACTIVE" {
			t.Errorf("table re-rendered without the page's filter: %v", h.pools.lastStatus)
		}
	})
	t.Run("anchor-level ignores the client field; blanks are not sent", func(t *testing.T) {
		h := newHarness()
		h.do(t, anchorAdmin(), req{
			method: http.MethodPost, target: "/ui/dispatch-pools/new", datastar: true,
			form: createForm("anchor", "true", "client", "acme", "concurrency", ""),
		})
		cmd := h.ops.created[0]
		if cmd.ClientID != nil || cmd.Description != nil || cmd.RateLimit != nil || cmd.Concurrency != nil {
			t.Errorf("command = %+v", cmd)
		}
	})
	t.Run("not a number: field errors patched, nothing run, focus on the first", func(t *testing.T) {
		h := newHarness()
		body := h.do(t, anchorAdmin(), req{
			method: http.MethodPost, target: "/ui/dispatch-pools/new", datastar: true,
			form: createForm("rateLimit", "fast", "concurrency", "1e3"),
		}).Body.String()
		if len(h.ops.created) != 0 {
			t.Fatal("use case ran with an invalid form")
		}
		mustContain(t, body,
			`id="pool-form-rate-limit-field">`,
			`<div id="pool-form-rate-limit-err" role="alert" class="text-sm text-destructive">Enter a whole number.</div>`,
			`id="pool-form-rate-limit" name="rateLimit" value="fast" placeholder="Unlimited" aria-invalid="true" aria-describedby="pool-form-rate-limit-err"`,
			`<div id="pool-form-concurrency-err"`,
			`id="pool-form-code-field">`, // every field is re-sent (clears old errors)
			`data: elements <div id="pool-form-error"></div>`,
			`document.getElementById(&#34;pool-form-rate-limit&#34;)?.focus()`,
		)
		mustNotContain(t, body, "pool-form-code-err", "fc-flash-success")
	})
	t.Run("use-case errors go to their field, others to the form", func(t *testing.T) {
		h := newHarness()
		h.ops.err = usecase.Conflict("CODE_EXISTS", "Dispatch pool with code 'orders' already exists")
		body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-pools/new", datastar: true, form: createForm()}).Body.String()
		mustContain(t, body, `<div id="pool-form-code-err" role="alert" class="text-sm text-destructive">Dispatch pool with code &#39;orders&#39; already exists</div>`,
			`document.getElementById(&#34;pool-form-code&#34;)?.focus()`)

		h.ops.err = usecase.Authorization("SCOPE_FORBIDDEN", "anchor scope required for this resource")
		body = h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-pools/new", datastar: true, form: createForm()}).Body.String()
		mustContain(t, body, `data: elements <div id="pool-form-error"><div class="grid`, `role="alert"`, "Anchor scope required for this resource")
		mustNotContain(t, body, "-err\" role")

		h.ops.err = usecase.Authorization("SCOPE_FORBIDDEN", "no access to this resource's client")
		body = h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-pools/new", datastar: true, form: createForm("client", "globex")}).Body.String()
		mustContain(t, body, `<div id="pool-form-client-err" role="alert" class="text-sm text-destructive">No access to this resource&#39;s client</div>`)

		h.ops.err = errors.New("pg: deadlock")
		body = h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-pools/new", datastar: true, form: createForm()}).Body.String()
		mustContain(t, body, "The request could not be completed.")
		mustNotContain(t, body, "deadlock")
	})
	t.Run("an unknown client identifier is the client field's error", func(t *testing.T) {
		h := newHarness()
		body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-pools/new", datastar: true, form: createForm("client", "nope")}).Body.String()
		if len(h.ops.created) != 0 {
			t.Fatal("use case ran")
		}
		mustContain(t, body, "No client with this identifier.", `value="nope"`)
	})
	t.Run("permission is checked before anything runs", func(t *testing.T) {
		h := newHarness()
		viewer := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeAnchor, Roles: []string{"r"}, Permissions: []string{"platform:messaging:dispatch-pool:view"}, PrincipalType: auth.PrincipalTypeUser}
		body := h.do(t, viewer, req{method: http.MethodPost, target: "/ui/dispatch-pools/new", datastar: true, form: createForm()}).Body.String()
		if len(h.ops.created) != 0 {
			t.Fatal("use case ran without permission")
		}
		mustContain(t, body, `<div id="pool-form-error"><div`)
	})
}

// The native (no Datastar) post/redirect/get, with the fixed-key flash.
func TestPoolCreateNativeFlash(t *testing.T) {
	h := newHarness()
	rec := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-pools/new", form: createForm()})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/dispatch-pools" {
		t.Fatalf("status %d location %q", rec.Code, rec.Header().Get("Location"))
	}
	var flash *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == shell.FlashCookie {
			flash = c
		}
	}
	if flash == nil || flash.Value != "pool-created" || !flash.HttpOnly || flash.Path != "/ui" || flash.MaxAge <= 0 {
		t.Fatalf("flash cookie = %+v", flash)
	}
	page := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-pools", cookie: &http.Cookie{Name: shell.FlashCookie, Value: flash.Value}})
	mustContain(t, page.Body.String(), `<div class="fc-flash fc-flash-success" data-init__delay.3s="el.remove()">`, "Dispatch pool created")
	cleared := false
	for _, c := range page.Result().Cookies() {
		if c.Name == shell.FlashCookie && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("the page did not clear the flash cookie")
	}
	for _, v := range []string{"<script>alert(1)</script>", "pool-unknown", "Dispatch pool created"} {
		body := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-pools", cookie: &http.Cookie{Name: shell.FlashCookie, Value: url.QueryEscape(v)}}).Body.String()
		mustNotContain(t, body, "fc-flash-success", "alert(1)")
	}

	t.Run("a validation failure re-renders the page with the dialog open, 422", func(t *testing.T) {
		rec := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-pools/new", form: createForm("concurrency", "x")})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status %d", rec.Code)
		}
		mustContain(t, rec.Body.String(), `<dialog id="pool-form"`, " open", "Enter a whole number.", `value="orders"`, `id="pools-table"`)
	})
}

// ── edit ───────────────────────────────────────────────────────────────

func TestPoolEdit(t *testing.T) {
	t.Run("the dialog carries the pool's values and the drawer field", func(t *testing.T) {
		h := newHarness()
		body := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-pools/dpl_1/edit", datastar: true}).Body.String()
		mustContain(t, body,
			`action="/ui/dispatch-pools/dpl_1/edit"`, "Edit Dispatch Pool", "Billing (billing)",
			`name="name" value="Billing"`, `name="rateLimit" value="600"`, `name="concurrency" value="10"`,
			`>Invoices &lt;b&gt;and&lt;/b&gt; receipts</textarea>`,
			`<input type="hidden" name="drawer" value="" data-attr:value="$selPool">`,
			`disabled data-attr:disabled="$poolFormBusy || !$poolFormDirty">Save</button>`,
			"Leave blank to keep the current setting.",
		)
		mustNotContain(t, body, `name="code"`, `name="client"`)
	})
	t.Run("archived pools are read-only", func(t *testing.T) {
		h := newHarness()
		body := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-pools/dpl_3/edit", datastar: true}).Body.String()
		mustContain(t, body, "fc-flash-error", "Archived pools cannot be edited.")
		h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-pools/dpl_3/edit", datastar: true, form: url.Values{"name": {"x"}}})
		if len(h.ops.updated) != 0 {
			t.Fatal("an archived pool was updated")
		}
	})
	t.Run("save: the SPA's PUT body (blanks clear), the dialog closes, the drawer refreshes when it shows the pool", func(t *testing.T) {
		h := newHarness()
		body := h.do(t, anchorAdmin(), req{
			method: http.MethodPost, target: "/ui/dispatch-pools/dpl_1/edit", datastar: true,
			form: url.Values{"name": {"Billing 2"}, "description": {""}, "rateLimit": {""}, "concurrency": {"0"}, "drawer": {"dpl_1"}},
		}).Body.String()
		if len(h.ops.updated) != 1 {
			t.Fatalf("update calls %d", len(h.ops.updated))
		}
		cmd := h.ops.updated[0]
		// A blank description and rate limit clear them ("" and 0); a blank
		// concurrency keeps it.
		if cmd.ID != "dpl_1" || deref(cmd.Name) != "Billing 2" || cmd.Description == nil || *cmd.Description != "" ||
			cmd.RateLimit == nil || *cmd.RateLimit != 0 || cmd.Concurrency != nil {
			t.Errorf("command = %+v", cmd)
		}
		mustContain(t, body, `document.getElementById(&#34;pool-form&#34;)?.close()`, `data: elements <div id="pools-table"`,
			"data: selector #pool-drawer\n", "Pool updated")
		other := h.do(t, anchorAdmin(), req{
			method: http.MethodPost, target: "/ui/dispatch-pools/dpl_1/edit", datastar: true,
			form: url.Values{"name": {"Billing 3"}, "drawer": {"dpl_2"}},
		}).Body.String()
		mustNotContain(t, other, "#pool-drawer")
	})
	t.Run("a use-case validation error goes to its field", func(t *testing.T) {
		h := newHarness()
		h.ops.err = usecase.Validation("NAME_REQUIRED", "name cannot be empty")
		body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-pools/dpl_1/edit", datastar: true, form: url.Values{"name": {" "}}}).Body.String()
		mustContain(t, body, `<div id="pool-form-name-err" role="alert" class="text-sm text-destructive">Name cannot be empty</div>`)
		mustNotContain(t, body, "pool-form-code-field") // edit has no code field
	})
}

// ── actions ────────────────────────────────────────────────────────────

func TestPoolActions(t *testing.T) {
	t.Run("suspend: the use case runs, the table and the open drawer are patched, a flash", func(t *testing.T) {
		h := newHarness()
		body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-pools/dpl_1/suspend", datastar: true, form: url.Values{"drawer": {"dpl_1"}}}).Body.String()
		if strings.Join(h.ops.actions, ",") != "suspend:dpl_1" {
			t.Fatalf("actions %v", h.ops.actions)
		}
		mustContain(t, body, `data: elements <div id="pools-table"`, "data: selector #pool-drawer\n", "Pool suspended")
	})
	t.Run("a multipart post (what Datastar sends) is read: the open drawer refreshes", func(t *testing.T) {
		h := newHarness()
		body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-pools/dpl_1/suspend", datastar: true, multipart: true, form: url.Values{"drawer": {"dpl_1"}}}).Body.String()
		mustContain(t, body, "data: selector #pool-drawer\n", "Pool suspended")
		h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-pools/new", datastar: true, multipart: true, form: createForm("code", "multi")})
		if len(h.ops.created) != 1 || h.ops.created[0].Code != "multi" {
			t.Fatalf("multipart create = %+v", h.ops.created)
		}
	})
	t.Run("activate with the drawer on another pool leaves the drawer alone", func(t *testing.T) {
		h := newHarness()
		body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-pools/dpl_2/activate", datastar: true, form: url.Values{"drawer": {""}}}).Body.String()
		mustContain(t, body, "Pool activated")
		mustNotContain(t, body, "#pool-drawer")
	})
	t.Run("delete closes the drawer showing the pool", func(t *testing.T) {
		h := newHarness()
		body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-pools/dpl_1/delete", datastar: true, form: url.Values{"drawer": {"dpl_1"}}}).Body.String()
		mustContain(t, body, `"selPool":""`, "Pool deleted")
	})
	t.Run("delete needs the delete permission", func(t *testing.T) {
		h := newHarness()
		body := h.do(t, poolWriter(), req{method: http.MethodPost, target: "/ui/dispatch-pools/dpl_1/delete", datastar: true}).Body.String()
		if len(h.ops.actions) != 0 {
			t.Fatal("deleted without permission")
		}
		mustContain(t, body, "fc-flash-error", "Access denied.")
	})
	t.Run("a use-case error is an error flash", func(t *testing.T) {
		h := newHarness()
		h.ops.err = usecase.Authorization("SCOPE_FORBIDDEN", "anchor scope required for this resource")
		body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-pools/dpl_1/suspend", datastar: true}).Body.String()
		mustContain(t, body, "fc-flash-error", "anchor scope required for this resource")
		mustNotContain(t, body, "pools-table")
	})
	t.Run("native post: redirect with the flash key", func(t *testing.T) {
		h := newHarness()
		rec := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-pools/dpl_1/suspend", form: url.Values{}})
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status %d", rec.Code)
		}
		if c := rec.Header().Get("Set-Cookie"); !strings.Contains(c, shell.FlashCookie+"=pool-suspended") {
			t.Errorf("Set-Cookie %q", c)
		}
	})
	t.Run("GET on an action is not routed (state changes are POST only)", func(t *testing.T) {
		h := newHarness()
		if rec := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-pools/dpl_1/delete"}); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("status %d", rec.Code)
		}
	})
}

// ── client options ─────────────────────────────────────────────────────

func TestClientOptions(t *testing.T) {
	h := newHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/clients/options?list=pool-form-client-options&q=glob", datastar: true}).Body.String()
	mustContain(t, body, `data: elements <datalist id="pool-form-client-options"><option value="globex">Globex &lt;Inc&gt;</option></datalist>`)
	mustNotContain(t, body, "acme")

	empty := h.do(t, poolWriter(), req{target: "/ui/clients/options?list=pool-form-client-options&q=", datastar: true}).Body.String()
	mustContain(t, empty, `data: elements <datalist id="pool-form-client-options"></datalist>`)

	bad := h.do(t, anchorAdmin(), req{target: "/ui/clients/options?list=x'y&q=", datastar: true}).Body.String()
	mustContain(t, bad, "fc-flash-error")
	mustNotContain(t, bad, "<datalist")
}
