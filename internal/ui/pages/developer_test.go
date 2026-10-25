package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/openapispecs"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

type fakeDevApps struct{ apps []application.Application }

func (f fakeDevApps) FindActive(context.Context) ([]application.Application, error) {
	return append([]application.Application(nil), f.apps...), nil
}

func (f fakeDevApps) FindByID(_ context.Context, id string) (*application.Application, error) {
	for i := range f.apps {
		if string(f.apps[i].ID) == id {
			a := f.apps[i]
			return &a, nil
		}
	}
	return nil, nil
}

type fakeDevSpecs struct {
	specs map[string][]openapispecs.OpenApiSpec
}

func (f fakeDevSpecs) FindCurrentByApplication(_ context.Context, id string) (*openapispecs.OpenApiSpec, error) {
	for _, s := range f.specs[id] {
		if s.Status == openapispecs.StatusCurrent {
			return &s, nil
		}
	}
	return nil, nil
}

func (f fakeDevSpecs) FindAllByApplication(_ context.Context, id string) ([]openapispecs.OpenApiSpec, error) {
	return append([]openapispecs.OpenApiSpec(nil), f.specs[id]...), nil
}

type fakeDevETs struct{ ets []eventtype.EventType }

func (f fakeDevETs) FindByApplication(_ context.Context, code string) ([]eventtype.EventType, error) {
	var out []eventtype.EventType
	for _, e := range f.ets {
		if e.Application == code {
			out = append(out, e)
		}
	}
	return out, nil
}

const devOrdersSpec = `{"openapi":"3.0.3",
 "info":{"title":"Orders API","version":"1.2.0","description":"Orders **service**"},
 "tags":[{"name":"Orders","description":"Order operations"}],
 "paths":{
  "/orders":{
   "parameters":[{"name":"tenant","in":"query","schema":{"type":"string"}}],
   "get":{"tags":["Orders"],"summary":"List orders","operationId":"listOrders",
    "parameters":[{"name":"limit","in":"query","description":"Page <size>","schema":{"type":"integer","format":"int32"}},{"$ref":"#/components/parameters/Trace"}],
    "responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"$ref":"#/components/schemas/OrderList"}}}}}},
   "post":{"tags":["Orders"],"summary":"Create <order>","requestBody":{"required":true,"description":"The order","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Order"}}}},
    "responses":{"201":{"description":"created"},"400":{"description":"bad request"}}}},
  "/health":{"get":{"summary":"Health","deprecated":true,"responses":{"200":{"description":"up"}}}}},
 "components":{
  "parameters":{"Trace":{"name":"X-Trace","in":"header","required":true,"schema":{"type":"string"}}},
  "schemas":{
   "Order":{"type":"object","properties":{"id":{"type":"string"},"parent":{"$ref":"#/components/schemas/Order"}}},
   "OrderList":{"type":"array","items":{"$ref":"#/components/schemas/Order"}}}}}`

type devHarness struct {
	apps  fakeDevApps
	specs fakeDevSpecs
	ets   fakeDevETs
	procs *fakeProcStore
}

func newDevHarness() *devHarness {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	desc := "Orders & <things>"
	plat := application.Application{ID: "app_plat", Code: "platform", Name: "Platform", Active: true}
	orders := application.Application{ID: "app_orders", Code: "orders", Name: "Orders <App>", Description: &desc, Active: true}
	notes := "Removed /legacy"
	h := &devHarness{
		apps: fakeDevApps{apps: []application.Application{plat, orders}},
		specs: fakeDevSpecs{specs: map[string][]openapispecs.OpenApiSpec{
			"app_orders": {
				{ID: "spec_2", ApplicationID: "app_orders", Version: "1.2.0", Status: openapispecs.StatusCurrent, Spec: []byte(devOrdersSpec), SyncedAt: day},
				{
					ID: "spec_1", ApplicationID: "app_orders", Version: "1.1.0", Status: openapispecs.StatusArchived, Spec: []byte(`{}`), SyncedAt: day.Add(-time.Hour),
					ChangeNotesText: &notes, ChangeNotes: &openapispecs.ChangeNotes{HasBreaking: true},
				},
			},
		}},
	}
	h.ets = fakeDevETs{ets: []eventtype.EventType{
		et("evt_1", "orders:fulfilment:shipment:shipped", "Shipped <b>", eventtype.StatusCurrent, spec("1.0", eventtype.SpecCurrent, `{"type":"object","properties":{"sku":{"type":"string"}}}`)),
		et("evt_2", "orders:billing:invoice:issued", "Invoice issued", eventtype.StatusCurrent),
		et("evt_3", "orders:billing:invoice:voided", "Invoice voided", eventtype.StatusArchived, spec("1.0", eventtype.SpecFinalising, `{"type":"object"}`)),
		et("evt_9", "crm:accounts:account:created", "Account created", eventtype.StatusCurrent),
	}}
	ship := proc("prc_1", "orders:fulfilment:shipment-flow", "Shipment Flow", process.StatusCurrent)
	ship.Body = "graph TD\n  A[Start] --> B{<script>alert(1)</script>}\n  B --> C[\"<span data-init='alert(2)'>x</span>\"]"
	ship.Tags = []string{"fulfilment"}
	old := proc("prc_2", "orders:billing:refunds", "Refunds", process.StatusArchived)
	other := proc("prc_3", "crm:accounts:onboarding", "Onboarding", process.StatusCurrent)
	h.procs = &fakeProcStore{procs: []process.Process{ship, old, other}}
	return h
}

func (h *devHarness) page() *Developer {
	return &Developer{Apps: h.apps, Specs: h.specs, EventTypes: h.ets, Processes: &appFilteringProcs{h.procs}, Shell: shellForTest()}
}

// appFilteringProcs applies the application filter the repository does.
type appFilteringProcs struct{ *fakeProcStore }

func (f *appFilteringProcs) FindWithFilters(_ context.Context, app, _, _ *string) ([]process.Process, error) {
	var out []process.Process
	for _, p := range f.procs {
		if app == nil || p.Application == *app {
			out = append(out, p)
		}
	}
	return out, nil
}

func (h *devHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, h.page())
}

func (h *devHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

const (
	devViewPerm  = "platform:developer:application-openapi:view"
	devOrdersURL = "/ui/developer/applications/app_orders"
)

func devPortalUser() *auth.AuthContext { return connUser(devViewPerm, procViewPerm) }

func TestDeveloperApplicationsList(t *testing.T) {
	h := newDevHarness()
	body := h.do(t, devPortalUser(), req{target: "/ui/developer"})
	mustContain(t, body, `<title>Developer Portal · FlowCatalyst</title>`, `href="/ui/developer/applications/app_orders"`, "Orders &lt;App&gt;",
		"Orders &amp; &lt;things&gt;", `title="This platform"`, ">1.2.0<", "2026-10-01 09:00:00 UTC",
		// The sidebar entry for the area is current.
		`href="/ui/developer" class="fc-nav-item" title="Applications" aria-current="page"`)

	h.apps.apps = nil
	mustContain(t, h.do(t, devPortalUser(), req{target: "/ui/developer"}), "No applications visible. Ask an admin to grant access to one.")
}

// Every route makes the BFF's check: the permission and anchor scope.
func TestDeveloperPermissionMatrix(t *testing.T) {
	h := newDevHarness()
	routes := []string{
		"/ui/developer", devOrdersURL, devOrdersURL + "?tab=event-types", devOrdersURL + "/tabs/event-types", devOrdersURL + "/tabs/processes", devOrdersURL + "/tabs/api-docs",
		devOrdersURL + "/operation?m=get&p=/orders", devOrdersURL + "/openapi.json", devOrdersURL + "/versions",
	}
	otherPerm := connUser("platform:developer:application-openapi:sync", procViewPerm)
	clientScoped := &auth.AuthContext{PrincipalID: "prn_x", Scope: auth.ScopeClient, Roles: []string{"r"}, Permissions: []string{devViewPerm, procViewPerm}, PrincipalType: auth.PrincipalTypeUser}
	for _, route := range routes {
		for name, ac := range map[string]*auth.AuthContext{"no permission": otherPerm, "not anchor": clientScoped} {
			for _, datastar := range []bool{false, true} {
				rec := h.rec(t, ac, req{target: route, datastar: datastar})
				body := rec.Body.String()
				if datastar && !strings.Contains(body, "fc-flash-error") {
					t.Errorf("%s, %s (datastar): no error answer", name, route)
				}
				if !datastar && rec.Code != http.StatusForbidden {
					t.Errorf("%s, %s: status %d", name, route, rec.Code)
				}
				mustNotContain(t, body, "Orders API", "Shipment Flow", "Shipped", "1.2.0", "list orders")
			}
		}
	}
	if rec := h.rec(t, devPortalUser(), req{target: "/ui/developer/applications/app_nope"}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown application: %d", rec.Code)
	}
}

func TestDeveloperApplicationPage(t *testing.T) {
	h := newDevHarness()
	body := h.do(t, devPortalUser(), req{target: devOrdersURL})
	mustContain(t, body, `<title>Orders &lt;App&gt; · FlowCatalyst</title>`, "orders · Orders &amp; &lt;things&gt;", ">API v1.2.0<",
		`href="`+devOrdersURL+`/versions"`, `role="tablist"`, ">API Docs<", ">Event Types<", ">Processes<",
		`src="/ui/assets/fc-diagram.`,
		// The API docs tab is the default and is rendered with the page.
		`id="dev-docs"`, "Orders API", "Download OpenAPI", `href="`+devOrdersURL+`/openapi.json"`, "Order operations")
	// The other tabs load when first shown, through Datastar, from an attribute (no data in an expression).
	mustContain(t, body, `id="dev-et" data-src="`+devOrdersURL+`/tabs/event-types" data-on-intersect__once="@get(el.dataset.src)"`,
		`id="dev-proc" data-src="`+devOrdersURL+`/tabs/processes"`)
	mustNotContain(t, body, "Shipped", "Shipment Flow")

	// ?tab= opens a tab, rendered in place; the others are then lazy.
	et := h.do(t, devPortalUser(), req{target: devOrdersURL + "?tab=event-types"})
	mustContain(t, et, "Shipped &lt;b&gt;", `data-src="`+devOrdersURL+`/tabs/api-docs"`)
	mustNotContain(t, et, "Orders API")
	// An unknown tab is the default one.
	mustContain(t, h.do(t, devPortalUser(), req{target: devOrdersURL + "?tab=nope"}), "Download OpenAPI")
}

func TestDeveloperEventTypesTab(t *testing.T) {
	h := newDevHarness()
	body := h.do(t, devPortalUser(), req{target: devOrdersURL + "?tab=event-types"})
	// CURRENT only by default, this application's only, by code; the first is selected.
	mustContain(t, body, "Shipped &lt;b&gt;", "Invoice issued", `<span class="fc-code-seg app">orders</span>`,
		`<option value="fulfilment">`, `<option value="billing">`, `<option value="invoice">`, `id="dev-et-viewer"`,
		`id="dev-et-evt_2" class="`, "No schema published for orders:billing:invoice:issued yet.", ">CURRENT<")
	mustNotContain(t, body, "Invoice voided", "Account created", `id="et-code-typescript"`)

	// The chosen one's schema is in the viewer, with the generated types.
	viewer := h.do(t, devPortalUser(), req{target: devOrdersURL + "/tabs/event-types?et=evt_1", datastar: true})
	mustContain(t, viewer, `id="dev-et-evt_1" class="`, "sku", `id="et-code-typescript"`, `id="et-code-java"`, `id="et-code-php"`, ">Example<")

	// Filters: the live answer patches the panel and the address.
	live := h.do(t, devPortalUser(), req{target: devOrdersURL + "/tabs/event-types?etsub=billing&et=evt_2", datastar: true})
	mustContain(t, live, `data: elements <div id="dev-et"`, "replaceState", "etsub=billing", "Invoice issued")
	mustContain(t, live, "No schema published for orders:billing:invoice:issued yet.")
	mustNotContain(t, live, "Shipped")

	// All statuses (an empty parameter): the archived one is back; a status of its own.
	all := h.do(t, devPortalUser(), req{target: devOrdersURL + "/tabs/event-types?etstatus=", datastar: true})
	mustContain(t, all, "Invoice voided", "Shipped &lt;b&gt;", "etstatus=")
	arch := h.do(t, devPortalUser(), req{target: devOrdersURL + "/tabs/event-types?etstatus=ARCHIVED", datastar: true})
	mustContain(t, arch, "Invoice voided", "FINALISING")
	mustNotContain(t, arch, "Shipped", "Invoice issued")

	// A subdomain that is not one of the application's is dropped; so is a selection outside the view.
	pruned := h.do(t, devPortalUser(), req{target: devOrdersURL + "/tabs/event-types?etsub=accounts&et=evt_9", datastar: true})
	mustContain(t, pruned, "Shipped &lt;b&gt;")
	mustNotContain(t, pruned, "Account created")

	none := h.do(t, devPortalUser(), req{target: devOrdersURL + "/tabs/event-types?etsub=billing&etagg=shipment", datastar: true})
	mustContain(t, none, "No event types match the current filters.", "Select an event type above to see its schema and sample code.")

	// Without scripts the filter form lands on the page itself.
	rec := h.rec(t, devPortalUser(), req{target: devOrdersURL + "/tabs/event-types?etsub=billing"})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "etsub=billing&tab=event-types") {
		t.Errorf("native filter: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if rec := h.rec(t, devPortalUser(), req{target: devOrdersURL + "/tabs/nope", datastar: true}); !strings.Contains(rec.Body.String(), "fc-flash-error") {
		t.Errorf("unknown tab: %s", rec.Body.String())
	}
}

func TestDeveloperProcessesTab(t *testing.T) {
	h := newDevHarness()
	body := h.do(t, devPortalUser(), req{target: devOrdersURL + "?tab=processes"})
	mustContain(t, body, "Shipment Flow", `href="`+devOrdersURL+`/tabs/processes?proc=prc_1"`, ">fulfilment<")
	mustNotContain(t, body, "Onboarding", "Refunds", "<fc-diagram", `aria-label="Edit `)

	// Selecting a process draws its diagram (the Mermaid island) with the source escaped.
	sel := h.do(t, devPortalUser(), req{target: devOrdersURL + "/tabs/processes?proc=prc_1", datastar: true})
	mustContain(t, sel, `<fc-diagram class="block" data-bundle="/ui/assets/mermaid.`, "B{&lt;script&gt;alert(1)&lt;/script&gt;}",
		"&lt;span data-init=&#39;alert(2)&#39;&gt;x&lt;/span&gt;", ` hidden data-ignore></div>`,
		`href="/ui/processes/prc_1"`, "Open process")
	mustNotContain(t, sel, "<script>alert", "<span data-init", "/ui/processes/prc_1/edit")

	// Edit appears with the process area's permission; an archived process cannot be edited.
	editor := connUser(devViewPerm, procViewPerm, "platform:messaging:process:update")
	ed := h.do(t, editor, req{target: devOrdersURL + "/tabs/processes?proc=prc_1&pstatus=", datastar: true})
	mustContain(t, ed, `href="/ui/processes/prc_1/edit"`, procArchivedNoEdit)

	arch := h.do(t, editor, req{target: devOrdersURL + "/tabs/processes?pstatus=ARCHIVED&proc=prc_2", datastar: true})
	mustContain(t, arch, "Refunds", "No diagram body yet.")
	mustNotContain(t, arch, `href="/ui/processes/prc_2/edit"`)

	// The process list has its own permission: without it the tab says so, with no process in it.
	denied := h.do(t, connUser(devViewPerm), req{target: devOrdersURL + "?tab=processes"})
	mustContain(t, denied, "Processes are not available to you")
	mustNotContain(t, denied, "Shipment Flow", "orders:fulfilment")
	denied = h.do(t, connUser(devViewPerm), req{target: devOrdersURL + "/tabs/processes?proc=prc_1", datastar: true})
	mustContain(t, denied, "Processes are not available to you")
	mustNotContain(t, denied, "Shipment Flow", "graph TD")

	h.procs.procs = nil
	mustContain(t, h.do(t, devPortalUser(), req{target: devOrdersURL + "/tabs/processes", datastar: true}), "No processes defined for this application yet.")
}

func TestDeveloperAPIDocs(t *testing.T) {
	h := newDevHarness()
	body := h.do(t, devPortalUser(), req{target: devOrdersURL})
	mustContain(t, body, "Version 1.2.0", "2026-10-01 09:00:00 UTC", "3 operations", "<strong>service</strong>",
		// Groups: the declared tag first, then the untagged ones.
		"Order operations", ">GET<", ">POST<", ">deprecated<",
		"/orders", "/health", "List orders", "Create &lt;order&gt;",
		// Each operation loads its detail from an attribute.
		`data-detail-url="`+devOrdersURL+`/operation?m=get&amp;n=1&amp;p=%2Forders"`, `data-on:toggle__once="@get(el.dataset.detailUrl)"`, `id="dev-op-1-body"`)
	mustNotContain(t, body, "X-Trace", "limit")

	// The detail: parameters (path level too, refs followed), the body and the responses, schemas as highlighted JSON.
	d := h.do(t, devPortalUser(), req{target: devOrdersURL + "/operation?m=get&p=/orders&n=1", datastar: true})
	mustContain(t, d, `data: elements <div id="dev-op-1-body"`, "listOrders", "X-Trace", "limit", "tenant", "int32", "Page &lt;size&gt;", `title="required"`,
		"application/json", `<span class="hl-`, `<span class="hl-key">"type"</span>`, ">200<", "ok")
	post := h.do(t, devPortalUser(), req{target: devOrdersURL + "/operation?m=post&p=/orders&n=2", datastar: true})
	mustContain(t, post, "Request body", "The order", ">201<", ">400<", "bad request", `<span class="hl-key">"$ref"</span>: <span class="hl-str">"#/components/schemas/Order"</span>`)

	// Without scripts the operation is a page of its own.
	page := h.do(t, devPortalUser(), req{target: devOrdersURL + "/operation?m=get&p=/orders"})
	mustContain(t, page, "<title>get /orders · FlowCatalyst</title>", "GET /orders", "X-Trace", `href="`+devOrdersURL+`?tab=api-docs"`)

	for _, bad := range []string{"m=get&p=/nope", "m=trace&p=/orders", "m=constructor&p=/orders", "p=/orders"} {
		if rec := h.rec(t, devPortalUser(), req{target: devOrdersURL + "/operation?" + bad + "&n=1"}); rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d", bad, rec.Code)
		}
	}
	if rec := h.rec(t, devPortalUser(), req{target: devOrdersURL + "/operation?m=get&p=/orders&n=x", datastar: true}); !strings.Contains(rec.Body.String(), "fc-flash-error") {
		t.Errorf("bad row number: %s", rec.Body.String())
	}

	// A closed application document: nothing published, then unreadable.
	h.specs.specs["app_orders"] = nil
	mustContain(t, h.do(t, devPortalUser(), req{target: devOrdersURL}), "API documentation not available")
	h.specs.specs["app_orders"] = []openapispecs.OpenApiSpec{{ID: "s", ApplicationID: "app_orders", Version: "1", Status: openapispecs.StatusCurrent, Spec: []byte(`[1,2`)}}
	mustContain(t, h.do(t, devPortalUser(), req{target: devOrdersURL}), "The OpenAPI document could not be read")
}

func TestDeveloperDownload(t *testing.T) {
	h := newDevHarness()
	rec := h.rec(t, devPortalUser(), req{target: devOrdersURL + "/openapi.json"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename=orders-api-1.2.0-openapi.json` {
		t.Errorf("Content-Disposition = %q", got)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
	mustContain(t, rec.Body.String(), "{\n  \"openapi\": \"3.0.3\",", `"title": "Orders API"`)

	h.specs.specs["app_orders"] = nil
	if rec := h.rec(t, devPortalUser(), req{target: devOrdersURL + "/openapi.json"}); rec.Code != http.StatusNotFound {
		t.Errorf("no document: %d", rec.Code)
	}
}

func TestDeveloperVersions(t *testing.T) {
	h := newDevHarness()
	body := h.do(t, devPortalUser(), req{target: devOrdersURL + "/versions"})
	mustContain(t, body, "<title>API Versions · FlowCatalyst</title>", "orders · Every sync that introduces a change creates a new row.",
		">1.2.0<", ">1.1.0<", "Initial version", "Removed /legacy", `title="Removed paths, schemas, or operations"`, ">breaking<", ">ARCHIVED<", ">CURRENT<")
	h.specs.specs["app_orders"] = nil
	mustContain(t, h.do(t, devPortalUser(), req{target: devOrdersURL + "/versions"}), "No OpenAPI versions yet.")
}

func TestOpenAPIParsing(t *testing.T) {
	doc, err := parseOpenAPI([]byte(devOrdersSpec))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Title != "Orders API" || doc.Version != "1.2.0" || doc.Total != 3 || len(doc.Groups) != 2 || doc.Groups[0].Name != "Orders" || doc.Groups[1].Name != "default" {
		t.Fatalf("doc = %+v", doc)
	}
	if got := doc.Groups[0].Ops; got[0].Method != "get" || got[1].Method != "post" || got[0].N != 1 || got[1].N != 2 || doc.Groups[1].Ops[0].N != 3 {
		t.Errorf("ops = %+v", got)
	}

	// A recursive schema is cut where it recurses; a missing target stays a reference.
	op, ok := doc.operation("post", "/orders")
	if !ok || len(op.Body.Content) != 1 {
		t.Fatalf("op = %+v", op)
	}
	html := op.Body.Content[0].SchemaHTML
	if strings.Count(html, "components/schemas/Order") != 1 {
		t.Errorf("the recursion was not cut once: %s", html)
	}
	r := &oaResolver{root: map[string]any{}}
	if got := r.resolve(map[string]any{"$ref": "#/components/schemas/Gone"}); got.(map[string]any)["$ref"] != "#/components/schemas/Gone" {
		t.Errorf("missing ref = %v", got)
	}
	if got := r.resolve(map[string]any{"$ref": "https://example.com/x.json"}); got.(map[string]any)["$ref"] != "https://example.com/x.json" {
		t.Errorf("remote ref = %v", got)
	}

	// The operation's parameter overrides the path-level one of the same name and place.
	op, _ = doc.operation("get", "/orders")
	if len(op.Params) != 3 || op.Params[0].Name != "tenant" || op.Params[1].Type != "integer (int32)" || !op.Params[2].Required {
		t.Errorf("params = %+v", op.Params)
	}

	if _, err := parseOpenAPI([]byte(`[]`)); err == nil {
		t.Error("an array is not an OpenAPI document")
	}
	for in, want := range map[string]string{"Orders API|app|1.2": "orders-api-1.2-openapi.json", "|app_1|": "app-1-openapi.json", "***||": "openapi-openapi.json"} {
		parts := strings.Split(in, "|")
		if got := downloadName(parts[0], parts[1], parts[2]); got != want {
			t.Errorf("downloadName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeveloperTabStateInURL(t *testing.T) {
	if got := devDetailURL("app_1", "event-types", url.Values{"etstatus": {""}, "etsub": {"a b"}}); got != "/ui/developer/applications/app_1?etstatus=&etsub=a+b&tab=event-types" {
		t.Errorf("devDetailURL = %q", got)
	}
}
