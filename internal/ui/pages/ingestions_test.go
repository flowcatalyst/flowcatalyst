package pages

import (
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	ingops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// ── fakes ──────────────────────────────────────────────────────────────

type fakeIngStore struct {
	items    []ingestion.ListItem
	versions map[string][]ingestion.Version
	kinds    []ingestion.RuntimeKind
	// carried are the kinds some live runtime carries.
	carried []string
	// notices are the undismissed notices by ingestion id.
	notices map[string]ingestion.Notices
	err     error
	// lastFilter is the filter the last list asked for.
	lastFilter ingestion.ListFilter
}

func (f *fakeIngStore) List(_ context.Context, fl ingestion.ListFilter) (*ingestion.Page, error) {
	f.lastFilter = fl
	if f.err != nil {
		return nil, f.err
	}
	page := &ingestion.Page{}
	for _, it := range f.items {
		switch {
		case fl.ApplicationCode != nil && it.ApplicationCode != *fl.ApplicationCode:
		case fl.ClientID != nil && string(it.ClientID) != *fl.ClientID:
		case fl.Q != nil && !strings.Contains(it.Code+" "+it.Definition.Name, *fl.Q):
		case fl.Status != nil && (*fl.Status == "ACTIVE") != it.Active:
		case fl.Kind != nil && ingestion.RunKind(it.Definition.Source) != *fl.Kind:
		case fl.AccessibleClientIDs != nil && !slices.Contains(*fl.AccessibleClientIDs, string(it.ClientID)):
		case fl.AccessibleApplicationIDs != nil && !slices.Contains(*fl.AccessibleApplicationIDs, string(it.ApplicationID)):
		case fl.After != "" && it.ID <= fl.After:
		default:
			page.Items = append(page.Items, it)
		}
	}
	if fl.Limit > 0 && len(page.Items) > fl.Limit {
		page.Items = page.Items[:fl.Limit]
		page.Next = page.Items[fl.Limit-1].ID
	}
	return page, nil
}

func (f *fakeIngStore) FindByID(_ context.Context, id string) (*ingestion.Ingestion, error) {
	if f.err != nil {
		return nil, f.err
	}
	for i := range f.items {
		if f.items[i].ID == id {
			it := f.items[i].Ingestion
			return &it, nil
		}
	}
	return nil, nil
}

func (f *fakeIngStore) ListVersions(_ context.Context, id string) ([]ingestion.Version, error) {
	vs := slices.Clone(f.versions[id])
	slices.SortFunc(vs, func(a, b ingestion.Version) int { return b.Number - a.Number })
	return vs, f.err
}

func (f *fakeIngStore) LiveKinds(context.Context) ([]ingestion.RuntimeKind, error) {
	return f.kinds, f.err
}

func (f *fakeIngStore) AnyLiveRuntimeCarries(context.Context) (func(string) bool, error) {
	return func(kind string) bool { return slices.Contains(f.carried, kind) }, f.err
}

func (f *fakeIngStore) NoticesOf(_ context.Context, ids []string) (map[string]ingestion.Notices, error) {
	out := map[string]ingestion.Notices{}
	for _, id := range ids {
		if n, ok := f.notices[id]; ok {
			out[id] = n
		}
	}
	return out, f.err
}

// fakeIngOps records what the page runs. It checks a definition as the use
// cases do before anything else (Validate), so a page test sees the API's field
// errors.
type fakeIngOps struct {
	calls    []string
	created  []ingops.CreateCommand
	clients  []string
	versions []ingops.SaveVersionCommand
	err      error
	newID    string
}

func (o *fakeIngOps) rec(call string) { o.calls = append(o.calls, call) }

func (o *fakeIngOps) Create(_ context.Context, clientRef string, cmd ingops.CreateCommand) (string, error) {
	o.rec("create")
	o.created = append(o.created, cmd)
	o.clients = append(o.clients, clientRef)
	if err := append(ingestion.ValidateCode(cmd.Code), cmd.Definition.Validate()...).Err(); err != nil {
		return "", err
	}
	if o.err != nil {
		return "", o.err
	}
	return cmp.Or(o.newID, "ing_1"), nil
}

func (o *fakeIngOps) SaveVersion(_ context.Context, cmd ingops.SaveVersionCommand) error {
	o.rec("save:" + cmd.ID)
	o.versions = append(o.versions, cmd)
	if err := cmd.Definition.Validate().Err(); err != nil {
		return err
	}
	return o.err
}

func (o *fakeIngOps) MakeLive(_ context.Context, id string, v int) error {
	o.rec("live:" + id + ":" + strconv.Itoa(v))
	return o.err
}

func (o *fakeIngOps) Pause(_ context.Context, id string) error { o.rec("pause:" + id); return o.err }

func (o *fakeIngOps) Resume(_ context.Context, id string) error { o.rec("resume:" + id); return o.err }

func (o *fakeIngOps) Delete(_ context.Context, id string) error { o.rec("delete:" + id); return o.err }

func (o *fakeIngOps) DismissFailure(_ context.Context, id, failureID string) error {
	o.rec("dismiss:" + id + ":" + failureID)
	return o.err
}

// ── fixtures ───────────────────────────────────────────────────────────

const (
	ingPermView    = "platform:ingestion:ingestion:view"
	ingPermCreate  = "platform:ingestion:ingestion:create"
	ingPermUpdate  = "platform:ingestion:ingestion:update"
	ingPermDelete  = "platform:ingestion:ingestion:delete"
	ingPermPromote = "platform:ingestion:ingestion:promote"
	ingPermRoute   = "platform:ingestion:ingestion:route"
	// ingPermSecrets: a definition that names secrets (the create form's
	// bearer token); ingPermIdentity: an SQS or S3 source without a key.
	ingPermSecrets  = "platform:ingestion:ingestion:bind-secrets"
	ingPermIdentity = "platform:ingestion:ingestion:runtime-identity"
)

var ingAllPerms = []string{ingPermView, ingPermCreate, ingPermUpdate, ingPermDelete, ingPermPromote, ingPermRoute}

func ingDef(name string, src ingestion.Source, split ingestion.Split) ingestion.Definition {
	return ingestion.Definition{
		Name: name, Source: src, Split: split,
		Target: ingestion.Target{Code: "orders:sync:order:received", TargetURL: "https://app.example.com/hooks/orders", Pool: "orders", Mode: ingestion.ModeImmediate, BatchSize: 100},
	}
}

func ingSample(id, code, app, client string, def ingestion.Definition, active bool, live, latest int) ingestion.ListItem {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	return ingestion.ListItem{
		Ingestion: ingestion.Ingestion{ //nolint:modernize // embedlit: a struct literal needs its type
			ID: id, ApplicationID: ids.ApplicationID("app_" + app), ClientID: ids.ClientID("clt_" + client), Code: code, Active: active,
			Version: live, LatestVersion: latest, Definition: def, CreatedAt: day, UpdatedAt: day.Add(time.Hour),
		},
		ApplicationCode: app + "-sync", ClientIdentifier: client,
	}
}

func ingVersion(id string, n int, def ingestion.Definition, note string) ingestion.Version {
	v := ingestion.Version{ID: "iv_" + id + "_" + strconv.Itoa(n), IngestionID: id, Number: n, Definition: def, CreatedAt: time.Date(2026, 10, 1, 9, n, 0, 0, time.UTC)}
	if note != "" {
		v.Note = &note
	}
	by := "prn_1"
	v.CreatedBy = &by
	return v
}

func ingFixture() *fakeIngStore {
	httpDef := ingDef("Orders <in>", ingestion.HTTP{Auth: ingestion.BearerAuth{Token: ingestion.SecretRef{Name: "push-token"}}}, ingestion.JSON{ArrayPath: "$.orders"})
	httpDef2 := httpDef
	httpDef2.Name = "Orders in (v3)"
	httpDef2.Target.BatchSize = 250
	sftp := ingDef("Nightly feed", ingestion.Sftp{Host: "sftp.example.com", Port: 22, Directory: "/out", Username: "svc", Credential: ingestion.SecretRef{Name: "sftp-password"}, Every: ingestion.Duration(time.Minute)}, ingestion.CSV{Header: true})
	poll := ingDef("Stock poll", ingestion.APIPoll{URL: "https://api.example.com/stock", Method: "GET", Auth: ingestion.NoAuth{}, Results: ingestion.RecordResults{}, Every: ingestion.Duration(5 * time.Second), IdleDelay: ingestion.Duration(time.Minute)}, ingestion.None{})
	camel := ingDef("Camel route", ingestion.Custom{Type: "camel-route", Settings: map[string]any{"yaml": "- from:\n    uri: timer:x\n"}}, ingestion.None{})
	mix := ingDef("MiX feed", ingestion.Custom{Type: "mix", Settings: map[string]any{"region": "eu"}}, ingestion.None{})

	owned := ingSample("ing_3", "stock-poll", "other", "globex", poll, true, 1, 1)
	set := "ingset_1"
	owned.ManagedBy = &set
	return &fakeIngStore{
		items: []ingestion.ListItem{
			ingSample("ing_1", "orders-in", "orders", "acme", httpDef, true, 2, 3),
			ingSample("ing_2", "nightly-feed", "orders", "acme", sftp, false, 1, 1),
			owned,
			ingSample("ing_4", "route-1", "orders", "acme", camel, true, 1, 1),
			ingSample("ing_5", "ext-1", "other", "globex", mix, true, 1, 1),
		},
		versions: map[string][]ingestion.Version{
			"ing_1": {ingVersion("ing_1", 1, httpDef, "first"), ingVersion("ing_1", 2, httpDef, "live <one>"), ingVersion("ing_1", 3, httpDef2, "")},
			"ing_2": {ingVersion("ing_2", 1, sftp, "")},
			"ing_3": {ingVersion("ing_3", 1, poll, "")},
			"ing_4": {ingVersion("ing_4", 1, camel, "")},
			"ing_5": {ingVersion("ing_5", 1, mix, "")},
		},
		kinds:   []ingestion.RuntimeKind{{Kind: "camel-route", Description: "A Camel route", RequiredPermission: ingPermRoute}},
		carried: []string{"http", "sftp", "api-poll", "camel-route", "s3", "sqs", "rabbitmq", "database-poll"},
	}
}

type ingHarness struct {
	store *fakeIngStore
	ops   *fakeIngOps
}

func newIngHarness() *ingHarness {
	return &ingHarness{store: ingFixture(), ops: &fakeIngOps{}}
}

func (h *ingHarness) page() *Ingestions {
	return &Ingestions{
		Store: h.store, Ops: h.ops, Clients: someClients(),
		ClientNames: &fakeClientStore{clients: []client.Client{
			{ID: "clt_acme", Name: "Acme Corp", Identifier: "acme", Status: client.StatusActive},
			{ID: "clt_globex", Name: "Globex <Inc>", Identifier: "globex", Status: client.StatusActive},
		}},
		Apps: &fakeAppStore{apps: []application.Application{
			{ID: "app_orders", Code: "orders-sync", Name: "Orders sync", Type: application.TypeIntegration, Active: true},
			{ID: "app_other", Code: "other-sync", Name: "Other <sync>", Type: application.TypeIntegration, Active: true},
		}},
		Shell: shellForTest(),
	}
}

func (h *ingHarness) do(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	page := h.page()
	return serveMounted(t, ac, q, page, &ClientOptions{Clients: page.Clients, Shell: page.Shell})
}

// ingUser is an anchor user holding exactly the given permissions.
func ingUser(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_9", Email: "ops@flowcatalyst.local", Scope: auth.ScopeAnchor, Roles: []string{"platform:ops"}, Permissions: perms, PrincipalType: auth.PrincipalTypeUser, AllApplications: true}
}

// ingAdmin is the super admin as the authenticator builds one: all
// applications included (the ingestion reach rules read it).
func ingAdmin() *auth.AuthContext {
	ac := anchorAdmin()
	ac.AllApplications = true
	return ac
}

// ingWithout is a user holding every ingestion permission but the given one.
func ingWithout(perm string) *auth.AuthContext {
	return ingUser(slices.DeleteFunc(slices.Clone(ingAllPerms), func(p string) bool { return p == perm })...)
}

func ingBody(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// ── list ───────────────────────────────────────────────────────────────

func TestIngestionsListRendersInsideTheShell(t *testing.T) {
	h := newIngHarness()
	body := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions"}))
	mustContain(t, body,
		`<title>Ingestions · FlowCatalyst</title>`,
		`<h1 class="fc-page-title">Ingestions</h1>`,
		`href="/ui/ingestions/new"`,
		`id="ings-list"`, `id="ings-table"`, `id="ings-rows"`,
		`orders-in`, `nightly-feed`, `route-1`,
		// the name is escaped, the client is its name, the application its code
		`Orders &lt;in&gt;`, `Globex &lt;Inc&gt;`, `orders-sync`,
		// source kind and summary
		`HTTP push`, `SFTP`, `svc@sftp.example.com:22/out`, `GET https://api.example.com/stock (none)`,
		// status chips: paused, set-owned, saved-and-not-live version number
		`Paused`, `Set`, `/ 3`,
		`href="/ui/ingestions/ing_1"`, `href="/ui/ingestions/ing_1/edit"`,
		`data-signals="{selIng: &#39;&#39;, ingTab: &#39;overview&#39;}"`,
		`<option value="orders-sync">Orders sync (orders-sync)</option>`, `<option value="clt_acme">Acme Corp</option>`,
		`<option value="camel-route">camel-route</option>`, `<option value="sftp">SFTP</option>`,
	)
	mustNotContain(t, body, `Orders <in>`, `Globex <Inc>`, `Other <sync>`)
	// the editor island loads on the page, not in the drawer
	mustContain(t, body, `fc-code-editor.`)
}

func TestIngestionsListSaysWhenNoRuntimeCarriesAKind(t *testing.T) {
	h := newIngHarness()
	body := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions"}))
	// ing_5 is the only running ingestion of a kind no live runtime reports.
	if got := strings.Count(body, "No runtime carries this kind"); got != 1 {
		t.Errorf("the notice appears %d times, want 1", got)
	}
	// A paused ingestion of such a kind would be no one's concern: nothing runs it anyway.
	h.store.items[4].Active = false
	body = ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions"}))
	mustNotContain(t, body, "No runtime carries this kind")
}

func TestIngestionsListPermissions(t *testing.T) {
	h := newIngHarness()
	t.Run("without view the page is refused", func(t *testing.T) {
		rec := h.do(t, ingUser(ingPermCreate), req{target: "/ui/ingestions"})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status %d", rec.Code)
		}
		d := h.do(t, ingUser(ingPermCreate), req{target: "/ui/ingestions", datastar: true})
		mustContain(t, d.Body.String(), "permission required")
		mustNotContain(t, d.Body.String(), "orders-in")
	})
	t.Run("view only hides New and Change", func(t *testing.T) {
		body := ingBody(t, h.do(t, ingUser(ingPermView), req{target: "/ui/ingestions"}))
		mustContain(t, body, "orders-in", `href="/ui/ingestions/ing_1"`)
		mustNotContain(t, body, `href="/ui/ingestions/new"`, `/ui/ingestions/ing_1/edit`, `ings-pause`, `ings-delete`)
	})
	t.Run("a client user sees its clients' ingestions only", func(t *testing.T) {
		ac := &auth.AuthContext{PrincipalID: "prn_5", Scope: auth.ScopeClient, Clients: []string{"clt_acme"}, AllApplications: true, Roles: []string{"r"}, Permissions: []string{ingPermView}, PrincipalType: auth.PrincipalTypeUser}
		body := ingBody(t, h.do(t, ac, req{target: "/ui/ingestions"}))
		mustContain(t, body, "orders-in", "nightly-feed")
		mustNotContain(t, body, "stock-poll", "ext-1")
		if h.store.lastFilter.AccessibleClientIDs == nil {
			t.Error("the list was not confined to the caller's clients in SQL")
		}
		// Another client's ingestion is not readable by id either.
		for _, target := range []string{"/ui/ingestions/ing_3", "/ui/ingestions/ing_3/tabs/activity", "/ui/ingestions/ing_3/definition.yaml", "/ui/ingestions/ing_3/compare?a=1&b=1"} {
			if rec := h.do(t, ac, req{target: target, datastar: true}); !strings.Contains(rec.Body.String(), "no access") && rec.Code != http.StatusForbidden {
				t.Errorf("%s: status %d: %s", target, rec.Code, rec.Body.String())
			}
		}
		mustNotContain(t, h.do(t, ac, req{target: "/ui/ingestions/ing_3", datastar: true}).Body.String(), "stock-poll")
	})
	t.Run("an application's account sees its application's ingestions only", func(t *testing.T) {
		ac := ingUser(ingPermView)
		ac.AllApplications, ac.Applications = false, []string{"app_orders"}
		body := ingBody(t, h.do(t, ac, req{target: "/ui/ingestions"}))
		mustContain(t, body, "orders-in", "route-1")
		mustNotContain(t, body, "stock-poll", "ext-1")
		if h.store.lastFilter.AccessibleApplicationIDs == nil {
			t.Error("the list was not confined to the caller's applications in SQL")
		}
		mustNotContain(t, h.do(t, ac, req{target: "/ui/ingestions/ing_5", datastar: true}).Body.String(), "MiX feed")
	})
}

func TestIngestionsListFilters(t *testing.T) {
	h := newIngHarness()
	body := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions?q=orders&client=clt_acme&application=orders-sync&status=ACTIVE&kind=http", datastar: true}))
	f := h.store.lastFilter
	if deref(f.Q) != "orders" || deref(f.ClientID) != "clt_acme" || deref(f.ApplicationCode) != "orders-sync" || deref(f.Status) != "ACTIVE" || deref(f.Kind) != "http" {
		t.Errorf("filter = %+v", f)
	}
	mustContain(t, body, `<tr id="ings-ing_1"`, `/ui/ingestions?application=orders-sync\u0026client=clt_acme\u0026kind=http\u0026q=orders\u0026status=ACTIVE`)
	mustNotContain(t, body, `ings-ing_2`)
	// A client, application or kind the caller is not offered is dropped, not trusted.
	ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions?client=clt_nope&application=nope&kind=nope&status=BOGUS", datastar: true}))
	f = h.store.lastFilter
	if f.ClientID != nil || f.ApplicationCode != nil || f.Kind != nil || f.Status != nil {
		t.Errorf("unknown filters reached the query: %+v", f)
	}
}

func TestIngestionsListPagesByCursor(t *testing.T) {
	h := newIngHarness()
	h.store.items = nil
	for i := range 53 {
		h.store.items = append(h.store.items, ingSample("ing_b"+strconv.Itoa(100+i), "bulk-"+strconv.Itoa(i), "orders", "acme",
			ingDef("Bulk", ingestion.HTTP{Auth: ingestion.BearerAuth{Token: ingestion.SecretRef{Name: "t"}}}, ingestion.None{}), true, 1, 1))
	}
	first := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions"}))
	mustContain(t, first, `Showing 50 ingestions`, `href="/ui/ingestions?after=ing_b149"`)
	second := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions?after=ing_b149"}))
	mustContain(t, second, `Showing 3 ingestions`, `bulk-52`, `href="/ui/ingestions"`)
	mustNotContain(t, second, `bulk-0<`)
	// A cursor that could not be an id is ignored.
	ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions?after=%27%3Bx"}))
	if h.store.lastFilter.After != "" {
		t.Errorf("after = %q", h.store.lastFilter.After)
	}
}

// ── drawer ─────────────────────────────────────────────────────────────

func TestIngestionDrawerContent(t *testing.T) {
	h := newIngHarness()
	body := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_1", datastar: true}))
	mustContain(t, body,
		`data: selector #ings-drawer`, `"selIng":"ing_1"`,
		`orders-in`, `role="tablist"`, `id="ings-tabs-tab-versions"`, `id="ings-tabs-panel-definition"`,
		// overview
		`Orders &lt;in&gt;`, `Acme Corp`, `POST /ingest/acme/orders-in on each runtime`, `JSON array elements at $.orders`,
		`orders:sync:order:received`, `https://app.example.com/hooks/orders`, `Unsigned`,
		`3, saved and not live`,
		// definition: the file fcdev reads, in the read-only editor, and its export
		`<fc-code-editor`, `readonly`, `application: orders-sync`, `client: acme`, `href="/ui/ingestions/ing_1/definition.yaml"`,
		// versions
		`id="ings-version-1"`, `id="ings-version-3"`, `live &lt;one&gt;`, `/ui/ingestions/ing_1/versions/3/live`, `Compare`,
		// the other half's tabs fill when shown
		`id="ings-activity-slot"`, `data-url="/ui/ingestions/ing_1/tabs/activity"`, `data-on-intersect__once`, `id="ings-history-slot"`,
	)
	mustNotContain(t, body, `Orders <in>`, `live <one>`, `/versions/2/live`)
}

func TestIngestionDrawerTabFromTheURL(t *testing.T) {
	h := newIngHarness()
	nav := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_1?tab=versions"}))
	mustContain(t, nav, `ingTab: &#39;versions&#39;`, `id="ings-drawer"`, `aria-selected="true"`)
	other := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_1?tab=nope"}))
	mustContain(t, other, `ingTab: &#39;overview&#39;`)
	if rec := h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_1'x"}); rec.Code != http.StatusNotFound {
		t.Errorf("bad id: status %d", rec.Code)
	}
	if rec := h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_missing"}); rec.Code != http.StatusNotFound {
		t.Errorf("missing: status %d", rec.Code)
	}
}

func TestIngestionLazyTabsAreFilledOnDemand(t *testing.T) {
	h := newIngHarness()
	for _, tab := range []string{"activity", "failures", "history"} {
		body := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_1/tabs/" + tab, datastar: true}))
		mustContain(t, body, `data: selector #ings-`+tab+`-slot`, `data: mode inner`, `/ui/ingestions/ing_1/`+tab)
	}
	if rec := h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_1/tabs/overview", datastar: true}); !strings.Contains(rec.Body.String(), "not found") && rec.Code != http.StatusNotFound {
		t.Errorf("a tab that is not lazy: %d %s", rec.Code, rec.Body.String())
	}
	// A tab is read with the view permission, and only for an ingestion the caller reaches.
	d := h.do(t, ingUser(ingPermCreate), req{target: "/ui/ingestions/ing_1/tabs/history", datastar: true})
	mustContain(t, d.Body.String(), "permission required")
	mustNotContain(t, d.Body.String(), `/ui/ingestions/ing_1/history`)
}

func TestIngestionCompareShowsBothDefinitions(t *testing.T) {
	h := newIngHarness()
	body := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_1/compare?a=2&b=3", datastar: true}))
	mustContain(t, body, `id="ings-compare"`, `Version 2 (live)`, `Version 3`, `batchSize: 100`, `batchSize: 250`, `Orders in (v3)`)
	same := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_1/compare?a=1&b=2", datastar: true}))
	mustContain(t, same, `Both versions hold the same definition.`)
	// A pair that is not two saved versions is told so.
	bad := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_1/compare?a=1&b=9", datastar: true}))
	mustContain(t, bad, "Choose two saved versions to compare.")
	// Without Datastar the drawer opens on the Versions tab with the pair.
	nav := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_1?tab=versions&a=2&b=3"}))
	mustContain(t, nav, `Version 2 (live)`, `Orders in (v3)`)
}

func TestIngestionExportDownloadsTheDefinitionFile(t *testing.T) {
	h := newIngHarness()
	rec := h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_2/definition.yaml"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("content type %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="nightly-feed.yaml"` {
		t.Errorf("content disposition %q", cd)
	}
	files, err := ingestion.ParseFiles(rec.Body.Bytes())
	if err != nil || len(files) != 1 {
		t.Fatalf("the file does not parse back: %v (%d files)", err, len(files))
	}
	if f := files[0]; f.Application != "orders-sync" || f.Client != "acme" || f.Code != "nightly-feed" || f.Active {
		t.Errorf("file = %+v", f)
	}
	// Exporting needs the view permission.
	if rec := h.do(t, ingUser(ingPermCreate), req{target: "/ui/ingestions/ing_2/definition.yaml"}); rec.Code != http.StatusForbidden {
		t.Errorf("without view: status %d", rec.Code)
	}
}

func TestIngestionDrawerPermissionMatrix(t *testing.T) {
	h := newIngHarness()
	marks := map[string]string{
		"change":    `href="/ui/ingestions/ing_1/edit"`,
		"new":       `New version…`,
		"pause":     `data-action="/ui/ingestions/ing_1/pause"`,
		"delete":    `data-action="/ui/ingestions/ing_1/delete"`,
		"make live": `data-action="/ui/ingestions/ing_1/versions/3/live"`,
		"from":      `href="/ui/ingestions/ing_1/edit?from=1"`,
	}
	cases := []struct {
		name  string
		perms []string
		want  []string
	}{
		{"view only", []string{ingPermView}, nil},
		{"update", []string{ingPermView, ingPermUpdate}, []string{"change", "new", "pause", "from"}},
		{"delete", []string{ingPermView, ingPermDelete}, []string{"delete"}},
		{"promote", []string{ingPermView, ingPermPromote}, []string{"make live"}},
		{"everything", ingAllPerms, []string{"change", "new", "pause", "delete", "make live", "from"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := ingBody(t, h.do(t, ingUser(c.perms...), req{target: "/ui/ingestions/ing_1", datastar: true}))
			for name, mark := range marks {
				if has := strings.Contains(body, mark); has != slices.Contains(c.want, name) {
					t.Errorf("%q present = %v, want %v", name, has, !has)
				}
			}
		})
	}
	t.Run("the live version offers no Make live", func(t *testing.T) {
		mustNotContain(t, ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_1", datastar: true})), `/versions/2/live`)
	})
	t.Run("a paused ingestion offers Resume", func(t *testing.T) {
		body := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_2", datastar: true}))
		mustContain(t, body, `data-action="/ui/ingestions/ing_2/resume"`)
		mustNotContain(t, body, `data-action="/ui/ingestions/ing_2/pause"`)
	})
}

// A set's ingestions are read-only: no change, no new version, no make live,
// no remove; pausing and resuming still work.
func TestIngestionsOwnedByASetAreReadOnly(t *testing.T) {
	h := newIngHarness()
	body := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_3", datastar: true}))
	mustContain(t, body, `id="ings-managed"`, `Managed by a set`, `data-action="/ui/ingestions/ing_3/pause"`)
	mustNotContain(t, body, `/ui/ingestions/ing_3/edit`, `/ui/ingestions/ing_3/delete`, `New version…`, `/versions/`)
	// Not offered in the list either: Change is disabled with the reason, Remove too.
	list := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions"}))
	mustContain(t, list, `Managed by a set`)
	mustNotContain(t, list, `data-action="/ui/ingestions/ing_3/delete"`, `href="/ui/ingestions/ing_3/edit"`)
	// And not by a forged request: the dialog is refused, the use case is the gate for the posts.
	if rec := h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_3/edit"}); rec.Code != http.StatusForbidden {
		t.Errorf("the change dialog of a set's ingestion: status %d", rec.Code)
	}
}

// ── kinds that need a permission ───────────────────────────────────────

func TestAKindWithARequiredPermissionIsHiddenWithoutIt(t *testing.T) {
	h := newIngHarness()
	t.Run("the create form offers it to those who hold the permission", func(t *testing.T) {
		body := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/new", datastar: true}))
		mustContain(t, body, `<option value="camel-route"`, `id="ings-kind-camel-route"`, `Route (YAML)`, `platform:ingestion:ingestion:route`)
	})
	t.Run("and not to those who do not", func(t *testing.T) {
		body := ingBody(t, h.do(t, ingWithout(ingPermRoute), req{target: "/ui/ingestions/new", datastar: true}))
		mustNotContain(t, body, `<option value="camel-route"`)
		mustContain(t, body, `<option value="http"`, `<option value="sftp"`)
	})
	t.Run("changing such an ingestion needs it", func(t *testing.T) {
		ac := ingWithout(ingPermRoute)
		drawer := ingBody(t, h.do(t, ac, req{target: "/ui/ingestions/ing_4", datastar: true}))
		mustContain(t, drawer, `Changing a camel-route ingestion needs platform:ingestion:ingestion:route.`)
		mustNotContain(t, drawer, `href="/ui/ingestions/ing_4/edit"`, `New version…`)
		d := h.do(t, ac, req{target: "/ui/ingestions/ing_4/edit", datastar: true})
		mustContain(t, d.Body.String(), "platform:ingestion:ingestion:route")
		mustNotContain(t, d.Body.String(), `ings-form`)
		// Removing needs only delete.
		mustContain(t, drawer, `data-action="/ui/ingestions/ing_4/delete"`)
	})
}

// ── the form ───────────────────────────────────────────────────────────

func TestIngestionCreateFormIsDrawnFromTheKinds(t *testing.T) {
	h := newIngHarness()
	body := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/new", datastar: true}))
	mustContain(t, body,
		`id="ings-form-body"`, `action="/ui/ingestions/new"`,
		// identity
		`name="application"`, `name="client"`, `name="code"`, `name="name"`, `name="keepFailedPayloads"`,
		// the kind select is bound to a signal, the kinds' fields are fieldsets gated by it
		`name="kind"`, `data-bind:ing-kind`, `id="ings-kind-http"`, `id="ings-kind-sftp"`, `id="ings-kind-s3"`, `id="ings-kind-sqs"`,
		`id="ings-kind-rabbitmq"`, `id="ings-kind-database-poll"`, `id="ings-kind-api-poll"`, `id="ings-kind-camel-route"`,
		`data-show="$ingKind === &#39;sftp&#39;"`, `data-attr:disabled`,
		// typed fields: a secret NAME with its reference input, a number, a duration, a choice
		`name="sftp.credential"`, `pattern="([a-z0-9]+(-[a-z0-9]+)*:)?[a-z0-9]+(-[a-z0-9]+)*"`, `name="sftp.port"`, `type="number"`,
		`name="sftp.every"`, `name="http.auth"`, `data-bind:ing-http-auth`, `name="target.mode"`, `name="split.kind"`,
		// the code editor for an extension's YAML
		`<fc-code-editor`, `language="yaml"`,
		// split and dispatch
		`name="target.pool"`, `name="target.connection"`, `name="target.serviceAccount"`, `name="target.batchSize"`,
		// signals of the controls other fields depend on
		`ingHttpAuth`, `ingSplitKind`,
		// the application and client choices
		`<option value="orders-sync">Orders sync (orders-sync)</option>`, `Make live when saved`,
	)
	// Only the chosen kind (http) starts enabled.
	mustContain(t, body, `<fieldset id="ings-kind-sftp" class="m-0 flex min-w-0 flex-col gap-4 border-0 p-0" data-attr:disabled`)
	if strings.Contains(body, `<fieldset id="ings-kind-http" class="m-0 flex min-w-0 flex-col gap-4 border-0 p-0" data-attr:disabled="!($ingKind === &#39;http&#39;)" data-show="$ingKind === &#39;http&#39;" disabled`) {
		t.Error("the chosen kind starts disabled")
	}
	// No HTML required attribute on a kind's fields: a hidden required control would block the submit.
	for _, name := range []string{"sftp.host", "sftp.port", "http.token", "api.url"} {
		i := strings.Index(body, `name="`+name+`"`)
		tag := body[strings.LastIndex(body[:i], "<") : i+len(name)+8]
		if strings.Contains(tag, "required") {
			t.Errorf("%s carries required: %s", name, tag)
		}
	}
}

func TestIngestionFormNeverCarriesASecretValue(t *testing.T) {
	// A definition holds the NAME of a secret, never a value; the form shows names only.
	h := newIngHarness()
	body := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_2/edit", datastar: true}))
	mustContain(t, body, `value="sftp-password"`, `Change nightly-feed`, `Saved as version 2; runtimes keep running version 1 until a version is made live.`)
	mustNotContain(t, body, `type="password"`)
}

func TestIngestionEditFormStartsFromAVersion(t *testing.T) {
	h := newIngHarness()
	live := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_1/edit", datastar: true}))
	mustContain(t, live, `Change orders-in`, `value="Orders &lt;in&gt;"`, `Saved as version 4; runtimes keep running version 2`)
	from := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_1/edit?from=3", datastar: true}))
	mustContain(t, from, `value="Orders in (v3)"`, `Starting from version 3.`, `value="250"`)
	// Neither code nor client nor application can be changed.
	mustNotContain(t, live, `name="code"`, `name="client"`, `name="application"`)
	// A change is offered "make live" only to those who may promote.
	mustContain(t, live, `name="live"`)
	mustNotContain(t, ingBody(t, h.do(t, ingUser(ingPermView, ingPermUpdate), req{target: "/ui/ingestions/ing_1/edit", datastar: true})), `name="live"`)
}

// formValues is a valid create form for an HTTP source.
func ingCreateForm() url.Values {
	return url.Values{
		"application": {"orders-sync"}, "client": {"acme"}, "code": {"new-feed"}, "name": {"New feed"}, "kind": {"http"},
		"http.auth": {"bearer"}, "http.token": {"push-token"},
		"split.kind": {"json"}, "split.arrayPath": {"$"},
		"target.code": {"orders:sync:order:received"}, "target.targetUrl": {"https://app.example.com/hooks"}, "target.pool": {"orders"},
		"target.mode": {"IMMEDIATE"}, "target.batchSize": {"100"}, "live": {"on"},
	}
}

func ingPost(h *ingHarness, t *testing.T, ac *auth.AuthContext, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	return h.do(t, ac, req{method: http.MethodPost, target: target, form: form, datastar: true, multipart: true, referer: "https://fc.example/ui/ingestions"})
}

func TestIngestionCreateRunsTheUseCaseWithTheDefinition(t *testing.T) {
	h := newIngHarness()
	rec := ingPost(h, t, ingAdmin(), "/ui/ingestions/new", ingCreateForm())
	body := ingBody(t, rec)
	if len(h.ops.created) != 1 {
		t.Fatalf("created %d: %s", len(h.ops.created), body)
	}
	cmd := h.ops.created[0]
	if cmd.Application != "orders-sync" || cmd.Code != "new-feed" || !cmd.Live || h.ops.clients[0] != "acme" {
		t.Errorf("command = %+v client %q", cmd, h.ops.clients[0])
	}
	raw, _ := json.Marshal(cmd.Definition)
	want := `{"name":"New feed","keepFailedPayloads":false,"source":{"kind":"http","auth":{"kind":"bearer","token":{"name":"push-token"}}},"split":{"kind":"json","arrayPath":"$"},` +
		`"target":{"code":"orders:sync:order:received","targetUrl":"https://app.example.com/hooks","pool":"orders","mode":"IMMEDIATE","batchSize":100}}`
	if string(raw) != want {
		t.Errorf("definition =\n%s\nwant\n%s", raw, want)
	}
	// As the Nuxt form: the dialog closes, the new ingestion's drawer opens, the user is told.
	mustContain(t, body, `ings-form`, `Ingestion created`, `"selIng":"ing_1"`)
}

func TestIngestionCreateReportsEveryProblemOnItsField(t *testing.T) {
	h := newIngHarness()
	form := ingCreateForm()
	form.Set("kind", "sftp")
	form.Set("sftp.host", "bad host!")
	form.Set("sftp.port", "99999")
	form.Set("sftp.directory", "")
	form.Set("sftp.username", "svc")
	form.Set("sftp.credential", "Not A Name")
	form.Set("sftp.every", "soon")
	form.Set("code", "Bad Code")
	form.Set("target.code", "nope")
	body := ingBody(t, ingPost(h, t, ingAdmin(), "/ui/ingestions/new", form))
	if len(h.ops.created) != 0 {
		t.Fatal("a definition with a duration that is not one reached the use case")
	}
	mustContain(t, body, `id="ings-form-sftp-every-err"`, `Enter a duration such as 30s, 5m, 2h, 1d or PT30S.`)
	// Fix the duration: the API's own problems arrive on the controls their paths name.
	form.Set("sftp.every", "5m")
	body = ingBody(t, ingPost(h, t, ingAdmin(), "/ui/ingestions/new", form))
	mustContain(t, body,
		`id="ings-form-sftp-host-err"`, `id="ings-form-sftp-port-err"`, `id="ings-form-sftp-directory-err"`, `id="ings-form-sftp-credential-err"`,
		`id="ings-form-code-err"`, `id="ings-form-target-code-err"`, `Must be 1 to 65535`, `Must be application:subdomain:aggregate:action`,
		// the form is shown again whole, with what was typed
		`value="bad host!"`, `value="Bad Code"`, `id="ings-form-body"`,
	)
	if got := h.ops.created[len(h.ops.created)-1].Definition.Source.(ingestion.Sftp).Every; time.Duration(got) != 5*time.Minute {
		t.Errorf("every = %v", time.Duration(got))
	}
}

func TestIngestionCreateErrorsOfTheUseCaseLandOnTheirFields(t *testing.T) {
	for _, c := range []struct {
		err   error
		field string
	}{
		{usecase.Conflict("INGESTION_CODE_EXISTS", "an ingestion with code 'new-feed' already exists"), "code"},
		{usecase.Validation("INGESTION_APPLICATION_INVALID", `no application with code "x"`), "application"},
		{usecase.Validation("INGESTION_CLIENT_INVALID", `no client "x"`), "client"},
		{usecase.Authorization("INGESTION_KIND_PERMISSION_REQUIRED", "permission required for source kind camel-route"), "kind"},
		{usecase.Validation("INGESTION_TARGET_INVALID", `target.pool: no dispatch pool with code "orders" usable by this client`), "target-pool"},
		{usecase.Validation("INGESTION_TARGET_INVALID", `target.connection: no connection with code "c" usable by this application and client`), "target-connection"},
	} {
		h := newIngHarness()
		h.ops.err = c.err
		body := ingBody(t, ingPost(h, t, ingAdmin(), "/ui/ingestions/new", ingCreateForm()))
		mustContain(t, body, `id="ings-form-`+c.field+`-err"`)
	}
	// Anything else is the form's own message, not a field's.
	h := newIngHarness()
	h.ops.err = usecase.Conflict("INGESTION_SET_OWNED", "the ingestion belongs to a set")
	body := ingBody(t, ingPost(h, t, ingAdmin(), "/ui/ingestions/new", ingCreateForm()))
	mustContain(t, body, "The ingestion belongs to a set", `ings-form-error`)
}

func TestIngestionSaveVersionRunsTheUseCase(t *testing.T) {
	h := newIngHarness()
	form := ingCreateForm()
	for _, k := range []string{"application", "client", "code", "live"} {
		form.Del(k)
	}
	form.Set("name", "Orders in, changed")
	form.Set("note", "why <this>")
	body := ingBody(t, ingPost(h, t, ingAdmin(), "/ui/ingestions/ing_1/edit", form))
	if len(h.ops.versions) != 1 {
		t.Fatalf("saved %d: %s", len(h.ops.versions), body)
	}
	cmd := h.ops.versions[0]
	if cmd.ID != "ing_1" || cmd.Live || deref(cmd.Note) != "why <this>" || cmd.Definition.Name != "Orders in, changed" {
		t.Errorf("command = %+v", cmd)
	}
	mustContain(t, body, `Version saved`)
	// Make live when saved is a promote.
	form.Set("live", "on")
	ingBody(t, ingPost(h, t, ingAdmin(), "/ui/ingestions/ing_1/edit", form))
	if !h.ops.versions[1].Live {
		t.Error("live was not passed")
	}
}

// ── forged posts ───────────────────────────────────────────────────────

func TestIngestionForgedPostsAreRefused(t *testing.T) {
	post := func(ac *auth.AuthContext, target string, form url.Values) (*ingHarness, string) {
		h := newIngHarness()
		return h, ingPost(h, t, ac, target, form).Body.String()
	}
	t.Run("create needs create", func(t *testing.T) {
		h, body := post(ingWithout(ingPermCreate), "/ui/ingestions/new", ingCreateForm())
		mustContain(t, body, "ermission required")
		if len(h.ops.calls) != 0 {
			t.Errorf("the use case ran: %v", h.ops.calls)
		}
	})
	t.Run("a new version needs update", func(t *testing.T) {
		form := ingCreateForm()
		h, body := post(ingWithout(ingPermUpdate), "/ui/ingestions/ing_1/edit", form)
		mustContain(t, body, "ermission required")
		if len(h.ops.calls) != 0 {
			t.Errorf("the use case ran: %v", h.ops.calls)
		}
	})
	t.Run("a live version needs promote", func(t *testing.T) {
		form := ingCreateForm()
		h, body := post(ingWithout(ingPermPromote), "/ui/ingestions/ing_1/edit", form)
		mustContain(t, body, "ermission required")
		if len(h.ops.calls) != 0 {
			t.Errorf("the use case ran: %v", h.ops.calls)
		}
	})
	for _, c := range []struct {
		name, target, perm string
	}{
		{"pause needs update", "/ui/ingestions/ing_1/pause", ingPermUpdate},
		{"resume needs update", "/ui/ingestions/ing_2/resume", ingPermUpdate},
		{"remove needs delete", "/ui/ingestions/ing_1/delete", ingPermDelete},
		{"make live needs promote", "/ui/ingestions/ing_1/versions/3/live", ingPermPromote},
	} {
		t.Run(c.name, func(t *testing.T) {
			h, body := post(ingWithout(c.perm), c.target, url.Values{"drawer": {"ing_1"}})
			mustContain(t, body, "ermission required")
			if len(h.ops.calls) != 0 {
				t.Errorf("the use case ran: %v", h.ops.calls)
			}
		})
	}
	t.Run("another client's ingestion is out of reach", func(t *testing.T) {
		ac := &auth.AuthContext{PrincipalID: "prn_5", Scope: auth.ScopeClient, Clients: []string{"clt_acme"}, AllApplications: true, Roles: []string{"r"}, Permissions: ingAllPerms, PrincipalType: auth.PrincipalTypeUser}
		h, body := post(ac, "/ui/ingestions/ing_3/edit", ingCreateForm())
		mustContain(t, body, "no access")
		if len(h.ops.calls) != 0 {
			t.Errorf("the use case ran: %v", h.ops.calls)
		}
	})
	t.Run("a forged kind is not a kind", func(t *testing.T) {
		form := ingCreateForm()
		form.Set("kind", "ftp'; alert(1); '")
		h, body := post(ingAdmin(), "/ui/ingestions/new", form)
		mustContain(t, body, "Choose where the data comes from.")
		if len(h.ops.calls) != 0 {
			t.Errorf("the use case ran: %v", h.ops.calls)
		}
	})
	t.Run("a bad id is not an ingestion", func(t *testing.T) {
		h, body := post(ingAdmin(), "/ui/ingestions/ing_1'x/pause", nil)
		mustContain(t, body, "not found")
		if len(h.ops.calls) != 0 {
			t.Errorf("the use case ran: %v", h.ops.calls)
		}
	})
}

func TestIngestionActionsRunTheirUseCase(t *testing.T) {
	for _, c := range []struct {
		target, call, flash string
	}{
		{"/ui/ingestions/ing_1/pause", "pause:ing_1", "Ingestion paused"},
		{"/ui/ingestions/ing_2/resume", "resume:ing_2", "Ingestion resumed"},
		{"/ui/ingestions/ing_1/delete", "delete:ing_1", "Ingestion removed"},
		{"/ui/ingestions/ing_1/versions/3/live", "live:ing_1:3", "Version live"},
	} {
		h := newIngHarness()
		body := ingBody(t, ingPost(h, t, ingAdmin(), c.target, url.Values{"drawer": {"ing_1"}, "tab": {"versions"}}))
		if !slices.Equal(h.ops.calls, []string{c.call}) {
			t.Errorf("%s ran %v", c.target, h.ops.calls)
		}
		mustContain(t, body, c.flash)
	}
	// The drawer is refreshed where the user is, and closed after a remove.
	h := newIngHarness()
	body := ingBody(t, ingPost(h, t, ingAdmin(), "/ui/ingestions/ing_1/pause", url.Values{"drawer": {"ing_1"}, "tab": {"versions"}}))
	mustContain(t, body, `data: selector #ings-drawer`, `"ingTab":"versions"`, `<tr id="ings-ing_1"`)
	del := ingBody(t, ingPost(h, t, ingAdmin(), "/ui/ingestions/ing_1/delete", url.Values{"drawer": {"ing_1"}}))
	mustContain(t, del, `"selIng":""`)
	// A refusal of the use case is an error flash, and nothing else changes.
	h.ops.err = usecase.Conflict("INGESTION_SET_OWNED", "the ingestion belongs to set ingset_1")
	refused := ingBody(t, ingPost(h, t, ingAdmin(), "/ui/ingestions/ing_1/delete", url.Values{"drawer": {"ing_1"}}))
	mustContain(t, refused, "belongs to set ingset_1")
	// A native post redirects to the drawer on its tab with a flash key.
	h.do(t, ingAdmin(), req{method: http.MethodPost, target: "/ui/ingestions/ing_1/resume", form: url.Values{"tab": {"versions"}}})
	h.ops.err = nil
	native := h.do(t, ingAdmin(), req{method: http.MethodPost, target: "/ui/ingestions/ing_1/resume", form: url.Values{"tab": {"versions"}}})
	if native.Code != http.StatusSeeOther || native.Header().Get("Location") != "/ui/ingestions/ing_1?tab=versions" {
		t.Errorf("native post: %d %s", native.Code, native.Header().Get("Location"))
	}
}

// ── the form's reader ──────────────────────────────────────────────────

// Every kind's fields round-trip: a definition read into the form and built
// again is the same definition.
func TestIngestionFormRoundTripsEveryKind(t *testing.T) {
	cat := ingsNewCatalogue([]ingestion.RuntimeKind{{Kind: "camel-route", RequiredPermission: ingPermRoute}, {Kind: "mix", Settings: json.RawMessage(
		`[{"name":"region","label":"Region","type":"TEXT","required":true},{"name":"batch","type":"INTEGER"},{"name":"fast","type":"BOOLEAN"},` +
			`{"name":"poll","type":"DURATION"},{"name":"secretName","type":"SECRET"},{"name":"mode","type":"CHOICE","choices":["a","b"]},{"name":"tags","type":"LIST"},{"name":"bad name","type":"TEXT"}]`)}}, "legacy-kind")
	day := ingestion.Duration(time.Minute)
	prefix := "orders/"
	hosts := "sftp.example.com ssh-rsa AAAA"
	scope := "read"
	body := "{}"
	expires := "$.expires_in"
	cases := map[string]ingestion.Source{
		"http bearer":  ingestion.HTTP{Auth: ingestion.BearerAuth{Token: ingestion.SecretRef{Name: "t"}}},
		"http basic":   ingestion.HTTP{Auth: ingestion.BasicAuth{Username: "u", Password: ingestion.SecretRef{Name: "p"}}, RateLimit: &ingestion.RateLimit{Requests: 10, Per: day}},
		"http fc":      ingestion.HTTP{Auth: ingestion.FlowCatalystToken{Application: "crm", PrincipalID: new("prn_abc")}},
		"sftp":         ingestion.Sftp{Host: "h", Port: 22, Directory: "/out", Username: "u", Credential: ingestion.SecretRef{Name: "c"}, KnownHosts: &hosts, Include: new("*.csv"), Every: day},
		"s3 own key":   ingestion.S3{Region: "eu-west-1", Bucket: "bkt", Prefix: &prefix, Every: day, Credential: &ingestion.AWSCredential{AccessKeyID: "AKIA1234", SecretAccessKey: ingestion.SecretRef{Name: "aws"}}},
		"s3 identity":  ingestion.S3{Region: "eu-west-1", Bucket: "bkt", Every: day},
		"sqs":          ingestion.Sqs{Region: "eu-west-1", Queue: "q"},
		"rabbitmq":     ingestion.RabbitMQ{URI: "amqps://mq", Queue: "q", Username: "u", Password: ingestion.SecretRef{Name: "p"}},
		"dbpoll mark":  ingestion.DatabasePoll{JDBCURL: "jdbc:postgresql://x/y", Username: "u", Password: ingestion.SecretRef{Name: "p"}, Table: "t", Track: ingestion.Watermark{Column: "id", Type: "NUMBER"}, MaxRows: 100, Every: day},
		"dbpoll state": ingestion.DatabasePoll{JDBCURL: "jdbc:postgresql://x/y", Username: "u", Password: ingestion.SecretRef{Name: "p"}, Table: "t", Track: ingestion.StatusTrack{Column: "s", KeyColumn: "id", Pending: "N", Processed: "Y"}, MaxRows: 100, Every: day},
		"api none": ingestion.APIPoll{
			URL: "https://a.example.com/x", Method: "GET", Auth: ingestion.NoAuth{}, Results: ingestion.RecordResults{}, Every: day, IdleDelay: day,
			Headers: []ingestion.Header{{Name: "Accept", Value: "application/json"}},
		},
		"api oauth": ingestion.APIPoll{
			URL: "https://a.example.com/x", Method: "POST", Body: &body, Auth: ingestion.ClientCredentials{TokenURL: "https://a.example.com/t", ClientID: "id", ClientSecret: ingestion.SecretRef{Name: "s"}, Scope: &scope, CredentialsIn: "BODY"},
			Results: ingestion.JSONPathResults{Path: "$.items"}, Cursor: &ingestion.Cursor{From: "$.next", Param: "cursor", Initial: new("0")}, Every: day, IdleDelay: day,
		},
		"api login": ingestion.APIPoll{
			URL: "https://a.example.com/x", Method: "GET", Auth: ingestion.Login{
				URL: "https://a.example.com/l", Format: "FORM", UsernameField: "u", PasswordField: "p", Username: "me", Password: ingestion.SecretRef{Name: "pw"},
				ExtraFields: map[string]string{"grant_type": "password"}, TokenPath: "$.token", ExpiresInPath: &expires, Lifetime: day, Header: "Authorization", Prefix: "Bearer ",
			},
			Results: ingestion.HeaderResults{Name: "X-More", Equals: new("yes")}, Every: day, IdleDelay: day,
		},
		"api key": ingestion.APIPoll{URL: "https://a.example.com/x", Method: "GET", Auth: ingestion.APIKey{Header: "X-Key", Value: ingestion.SecretRef{Name: "k"}}, Results: ingestion.RecordResults{}, Every: day, IdleDelay: day},
		"camel":   ingestion.Custom{Type: "camel-route", Settings: map[string]any{"yaml": "- from:\n    uri: timer:x\n"}},
		"mix":     ingestion.Custom{Type: "mix", Settings: map[string]any{"region": "eu", "batch": float64(5), "fast": true, "poll": "PT5M", "secretName": "mix-secret", "mode": "b", "tags": []any{"x", "y"}}},
		"legacy":  ingestion.Custom{Type: "legacy-kind", Settings: map[string]any{"any": "thing", "n": float64(2)}},
	}
	splits := []ingestion.Split{ingestion.None{}, ingestion.CSV{Header: false}, ingestion.CSV{Header: true}, ingestion.JSON{ArrayPath: "$.a.b"}, ingestion.XML{Element: "row"}}
	n := 0
	for name, src := range cases {
		split := splits[n%len(splits)]
		n++
		if _, poll := src.(ingestion.DatabasePoll); poll {
			split = ingestion.None{}
		}
		def := ingDef("Round trip", src, split)
		def.KeepFailedPayloads = n%2 == 0
		def.Target.Connection = new("conn-1")
		values, err := ingsValuesOf(def, cat)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		built := ingsBuildDefinition(values, cat)
		if len(built.Errs) != 0 {
			t.Fatalf("%s: %v", name, built.Errs)
		}
		raw, _ := json.Marshal(built.Doc)
		back, err := ingestion.ParseDefinition(raw)
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, raw)
		}
		if !def.Equal(back) {
			want, _ := json.Marshal(def)
			t.Errorf("%s:\n got %s\nwant %s", name, raw, want)
		}
		if problems := back.Validate(); len(problems) != 0 && name != "mix" && name != "legacy" {
			t.Errorf("%s: %v", name, problems)
		}
	}
}

func TestIngestionDurationsAreReadInShortAndISOForm(t *testing.T) {
	for in, want := range map[string]string{"30s": "PT30S", "5m": "PT5M", "2H": "PT2H", "1d": "PT24H", "90s": "PT1M30S", "pt15m": "PT15M", "PT1H30M": "PT1H30M"} {
		got, err := ingsParseDuration(in)
		if err != nil || got != want {
			t.Errorf("%q = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "soon", "5", "-5m", "P", "PT"} {
		if got, err := ingsParseDuration(in); err == nil {
			t.Errorf("%q = %q, want an error", in, got)
		}
	}
}

// What a runtime reports of a kind's settings never reaches an attribute or a
// path unchecked.
func TestIngestionExtensionSettingsAreDrawnSafely(t *testing.T) {
	reported := []ingestion.RuntimeKind{
		{Kind: "good-kind", Settings: json.RawMessage(`{"fields":[{"name":"a","type":"text"},{"name":"b\" onclick=\"x","type":"TEXT"},{"name":"c","type":"CHOICE","choices":["<x>"]}]}`)},
		{Kind: "bad kind'", Settings: json.RawMessage(`[]`)},
		{Kind: "http", Description: "cannot redefine a platform kind"},
		{Kind: "nothing"},
	}
	cat := ingsNewCatalogue(reported)
	if cat.find("bad kind'") != nil {
		t.Error("a kind with an unsafe name was drawn")
	}
	if k := cat.find("http"); k == nil || k.Description == "cannot redefine a platform kind" {
		t.Error("a runtime redefined a platform kind")
	}
	k := cat.find("good-kind")
	if k == nil || len(k.Fields) != 2 {
		t.Fatalf("fields = %+v", k)
	}
	if n := cat.find("nothing"); n == nil || len(n.Fields) != 1 || n.Fields[0].Type != ingsJSON {
		t.Errorf("a kind with no description is edited as JSON: %+v", n)
	}
	for _, name := range []string{"good-kind", "nothing"} {
		if cat.find(name).Fields[0].Path == "" {
			t.Errorf("%s: no path", name)
		}
	}
}

func TestIngestionErrorPathsFindTheirFields(t *testing.T) {
	cat := ingsNewCatalogue(nil)
	values := ingsDefaults(cat)
	values["kind"] = "api-poll"
	values["api.auth"] = "login"
	values["api.headers"] = "Accept: x\nHost: y"
	built := ingsBuildDefinition(values, cat)
	for path, name := range map[string]string{
		"source.url": "api.url", "source.auth.password": "api.loginPassword", "source.auth.password.name": "api.loginPassword",
		"source.headers.1.name": "api.headers", "source.every": "api.every", "target.pool": "target.pool", "split.kind": "split.kind",
	} {
		f, ok := built.fieldFor(path)
		if !ok || f.Name != name {
			t.Errorf("%s -> %q (%v), want %s", path, f.Name, ok, name)
		}
	}
	// A branch not chosen has no field: basic's username is not login's.
	if f, ok := built.fieldFor("source.auth.token"); ok {
		t.Errorf("source.auth.token found %s in the login branch", f.Name)
	}
}

// The platform does not check an extension's settings further than their shape,
// so the form asks for the ones the runtime says are required.
func TestIngestionRequiredExtensionSettingsAreAsked(t *testing.T) {
	cat := ingsNewCatalogue([]ingestion.RuntimeKind{
		{Kind: "camel-route"},
		{Kind: "mix", Settings: json.RawMessage(`[{"name":"region","type":"TEXT","required":true},{"name":"zone","type":"TEXT","required":true,"defaultValue":"eu"}]`)},
	})
	values := ingsDefaults(cat)
	values["kind"] = "mix"
	built := ingsBuildDefinition(values, cat)
	if built.Errs["mix.s.region"] == "" || built.Errs["mix.s.zone"] != "" {
		t.Errorf("errors = %v", built.Errs)
	}
	values["kind"] = "camel-route"
	if built = ingsBuildDefinition(values, cat); built.Errs["camel-route.s.yaml"] == "" {
		t.Errorf("a route with no YAML: %v", built.Errs)
	}
}

func TestIngestionDatabasePollForcesNoSplit(t *testing.T) {
	cat := ingsNewCatalogue(nil)
	values := ingsDefaults(cat)
	values["kind"] = "database-poll"
	values["split.kind"] = "json"
	built := ingsBuildDefinition(values, cat)
	if got, _ := ingsGetPath(built.Doc, "split.kind"); got != "none" {
		t.Errorf("split = %v", got)
	}
}

func TestEveryBuiltInKindHasAForm(t *testing.T) {
	cat := ingsNewCatalogue(nil)
	for _, name := range ingestion.BuiltInKinds() {
		if name == ingestion.KindCustom {
			continue
		}
		if cat.find(name) == nil {
			t.Errorf("no form for the %s kind", name)
		}
	}
	for _, k := range cat {
		if k.Label == "" {
			t.Errorf("%s has no label", k.Name)
		}
	}
}
