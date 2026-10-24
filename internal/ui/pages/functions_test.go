package pages

import (
	"cmp"
	"context"
	"errors"
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
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	fnops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// ── fakes ──────────────────────────────────────────────────────────────

// Values the fake store holds for a secret and a database: the real
// repository returns neither, and no response of the page may carry them.
const (
	fnsSecretValue = "TOPSECRET-VALUE-1"
	fnsDSNValue    = "postgres://user:hunter2-DSN@db.internal/app"
)

type fakeFnStore struct {
	fns      []function.Function
	versions map[string][]function.Version
	aliases  map[string][]function.Alias
	settings map[string][]function.Setting
	routes   map[string][]function.Route
	err      error
	// lastFilters is the filter the last list asked for.
	lastFilters function.ListFilters
}

func (f *fakeFnStore) match(fl function.ListFilters) []function.Function {
	var out []function.Function
	for _, fn := range f.fns {
		switch {
		case fl.ApplicationID != nil && fn.ApplicationID != *fl.ApplicationID:
		case fl.ClientID != nil && deref(fn.ClientID) != *fl.ClientID:
		case fl.AddressPrefix != nil && !strings.HasPrefix(fn.Address, *fl.AddressPrefix):
		case fl.AccessibleClientIDs != nil && fn.ClientID != nil && !slices.Contains(*fl.AccessibleClientIDs, *fn.ClientID):
		default:
			out = append(out, fn)
		}
	}
	return out
}

func (f *fakeFnStore) FindWithFilters(_ context.Context, fl function.ListFilters) ([]function.Function, error) {
	f.lastFilters = fl
	if f.err != nil {
		return nil, f.err
	}
	out := f.match(fl)
	if fl.Offset != nil {
		out = out[min(int(*fl.Offset), len(out)):]
	}
	if fl.Limit != nil {
		out = out[:min(int(*fl.Limit), len(out))]
	}
	return out, nil
}

func (f *fakeFnStore) CountWithFilters(_ context.Context, fl function.ListFilters) (int64, error) {
	return int64(len(f.match(fl))), f.err
}

func (f *fakeFnStore) FindByID(_ context.Context, id string) (*function.Function, error) {
	if f.err != nil {
		return nil, f.err
	}
	for i := range f.fns {
		if f.fns[i].ID == id {
			fn := f.fns[i]
			return &fn, nil
		}
	}
	return nil, nil
}

func (f *fakeFnStore) ListVersionsByFunction(_ context.Context, id string) ([]function.Version, error) {
	return slices.Clone(f.versions[id]), f.err
}

func (f *fakeFnStore) ListAliases(_ context.Context, id string) ([]function.Alias, error) {
	return slices.Clone(f.aliases[id]), f.err
}

func (f *fakeFnStore) ListSettings(_ context.Context, id string) ([]function.Setting, error) {
	return slices.Clone(f.settings[id]), f.err
}

func (f *fakeFnStore) ListRoutesByFunction(_ context.Context, id string) ([]function.Route, error) {
	return slices.Clone(f.routes[id]), f.err
}

func (f *fakeFnStore) ServiceAccountCodes(_ context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		if id == "sa_1" {
			out[id] = "orders-svc"
		}
	}
	return out, nil
}

// fakeFnOps records what the page runs; err is returned by every call.
type fakeFnOps struct {
	calls   []string
	created []fnops.CreateCommand
	updated []fnops.UpdateCommand
	routes  []fnops.PutRouteCommand
	// settings is what PutSetting was given, by "kind/key".
	settings map[string]string
	wiring   FunctionWiring
	err      error
	newID    string
	// store gets the created function, as the database would.
	store *fakeFnStore
}

func (o *fakeFnOps) rec(call string) { o.calls = append(o.calls, call) }

func (o *fakeFnOps) Create(_ context.Context, cmd fnops.CreateCommand) (string, error) {
	o.rec("create")
	o.created = append(o.created, cmd)
	id := cmp.Or(o.newID, "fn_9")
	if o.store != nil && o.err == nil {
		o.store.fns = append(o.store.fns, function.Function{ID: id, ApplicationID: cmd.ApplicationID, ApplicationCode: "billing", Name: cmd.Name, Address: "billing." + cmd.Name, ClientID: cmd.ClientID, Limits: function.DefaultLimits()})
	}
	return id, o.err
}

func (o *fakeFnOps) Update(_ context.Context, cmd fnops.UpdateCommand) error {
	o.rec("update:" + cmd.ID)
	o.updated = append(o.updated, cmd)
	return o.err
}

func (o *fakeFnOps) Delete(_ context.Context, id string) error {
	o.rec("delete:" + id)
	return o.err
}

func (o *fakeFnOps) RetireVersion(_ context.Context, id string, n int32) error {
	o.rec("retire:" + id + ":" + strconv.Itoa(int(n)))
	return o.err
}

func (o *fakeFnOps) PutAlias(_ context.Context, id, name string, n int32) (FunctionWiring, error) {
	o.rec("alias:" + id + ":" + name + ":" + strconv.Itoa(int(n)))
	return o.wiring, o.err
}

func (o *fakeFnOps) DeleteAlias(_ context.Context, id, name string) (FunctionWiring, error) {
	o.rec("unalias:" + id + ":" + name)
	return o.wiring, o.err
}

func (o *fakeFnOps) PutSetting(_ context.Context, id string, kind function.SettingKind, key, value string) error {
	o.rec("set:" + id + ":" + string(kind) + ":" + key)
	if o.settings == nil {
		o.settings = map[string]string{}
	}
	o.settings[string(kind)+"/"+key] = value
	return o.err
}

func (o *fakeFnOps) DeleteSetting(_ context.Context, id string, kind function.SettingKind, key string) error {
	o.rec("unset:" + id + ":" + string(kind) + ":" + key)
	return o.err
}

func (o *fakeFnOps) PutRoute(_ context.Context, cmd fnops.PutRouteCommand) error {
	o.rec("route:" + cmd.FunctionID)
	o.routes = append(o.routes, cmd)
	return o.err
}

func (o *fakeFnOps) DeleteRoute(_ context.Context, id, routeID string) error {
	o.rec("unroute:" + id + ":" + routeID)
	return o.err
}

type fakeFnZones struct {
	zones []functiondomain.FunctionDomain
}

func (f fakeFnZones) FindWithFilters(context.Context, functiondomain.ListFilters) ([]functiondomain.FunctionDomain, error) {
	return f.zones, nil
}

type fakeFnSAs struct{}

func (fakeFnSAs) FindAll(context.Context) ([]serviceaccount.ServiceAccount, error) {
	return []serviceaccount.ServiceAccount{
		{ID: "sa_1", Code: "orders-svc", Name: "Orders <service>", Active: true},
		{ID: "sa_2", Code: "billing-svc", Name: "Billing", Active: true},
		{ID: "sa_3", Code: "old-svc", Name: "Old", Active: false},
	}, nil
}

// ── fixtures ───────────────────────────────────────────────────────────

const fnsDescribeJSON = `{
  "abi": 1,
  "endpoints": [{"method": "POST", "path": "/render", "auth": "webhook"}, {"path": "/health"}],
  "subscriptions": [{"eventType": "billing:invoice:created", "path": "/on-invoice", "mode": "BLOCK_ON_ERROR"}],
  "schedules": [{"cron": "0 * * * *", "path": "/hourly", "timezone": "UTC"}],
  "config": ["greeting", "region"],
  "secrets": ["api_key", "hook_secret"],
  "db": ["main"],
  "httpAllow": ["api.example.com"],
  "emits": ["billing:invoice:rendered"],
  "dispatches": [{"target": "billing.mailer", "path": "/send"}]
}`

func fnsFixture() *fakeFnStore {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	acme, globex := "clt_acme", "clt_globex"
	pool, sa := "heavy", "sa_1"
	desc := "Renders <b>invoices</b>"
	base := function.Function{
		ID: "fn_1", ApplicationID: "app_billing", ApplicationCode: "billing", Name: "invoice-pdf", Address: "billing.invoice-pdf",
		Pool: &pool, Warm: true, Description: &desc, ServiceAccountID: &sa, Limits: function.DefaultLimits(), CreatedAt: day, UpdatedAt: day,
	}
	acmeFn := function.Function{ID: "fn_2", ApplicationID: "app_acme", ApplicationCode: "acme", Name: "orders", Address: "acme.orders", ClientID: &acme, Limits: function.DefaultLimits(), CreatedAt: day.Add(time.Hour), UpdatedAt: day}
	globexFn := function.Function{ID: "fn_3", ApplicationID: "app_acme", ApplicationCode: "acme", Name: "feed", Address: "acme.feed", ClientID: &globex, Limits: function.DefaultLimits(), CreatedAt: day.Add(2 * time.Hour), UpdatedAt: day}
	ready := day.Add(time.Minute)
	version := func(n int32, status function.VersionStatus) function.Version {
		return function.Version{ID: "ver_" + strconv.Itoa(int(n)), FunctionID: "fn_1", Number: n, Digest: strings.Repeat("ab", 32), Runtime: "wasm", Status: status, CreatedAt: day, ReadyAt: &ready, Describe: []byte(fnsDescribeJSON)}
	}
	failed := version(3, function.VersionFailed)
	failed.Failure = []byte(`{"message":"module load boom <x>"}`)
	return &fakeFnStore{
		fns: []function.Function{base, acmeFn, globexFn},
		versions: map[string][]function.Version{"fn_1": {
			version(1, function.VersionReady), version(2, function.VersionReady), failed,
			version(4, function.VersionRetired), version(5, function.VersionReady),
		}},
		aliases: map[string][]function.Alias{"fn_1": {
			{FunctionID: "fn_1", Name: "live", VersionID: "ver_2", UpdatedAt: day},
			{FunctionID: "fn_1", Name: "qa", VersionID: "ver_1", UpdatedAt: day},
		}},
		settings: map[string][]function.Setting{"fn_1": {
			{FunctionID: "fn_1", Kind: function.SettingConfig, Key: "greeting", Value: "hello <b>world</b>", UpdatedAt: day},
			{FunctionID: "fn_1", Kind: function.SettingConfig, Key: "extra", Value: "x", UpdatedAt: day},
			{FunctionID: "fn_1", Kind: function.SettingSecret, Key: "api_key", Value: fnsSecretValue, UpdatedAt: day},
			{FunctionID: "fn_1", Kind: function.SettingDB, Key: "main", Value: fnsDSNValue, UpdatedAt: day},
		}},
		routes: map[string][]function.Route{"fn_1": {
			{ID: "rt_1", FunctionID: "fn_1", Hostname: "api.example.com", PathPrefix: "/hooks", AliasPrefixes: []string{"qa"}, CreatedAt: day, UpdatedAt: day},
		}},
	}
}

func fnsHarnessNew() *fnsHarness {
	store := fnsFixture()
	return &fnsHarness{store: store, ops: &fakeFnOps{store: store}, zones: fakeFnZones{zones: []functiondomain.FunctionDomain{{ID: "dom_1", Zone: "example.com"}, {ID: "dom_2", Zone: "acme.test", ClientID: new("clt_acme")}}}}
}

type fnsHarness struct {
	store *fakeFnStore
	ops   *fakeFnOps
	zones FunctionZones
}

func (h *fnsHarness) page() *Functions {
	return &Functions{
		Functions: h.store, Ops: h.ops, Zones: h.zones, Clients: someClients(),
		ClientNames: &fakeClientStore{clients: []client.Client{
			{ID: "clt_acme", Name: "Acme Corp", Identifier: "acme", Status: client.StatusActive},
			{ID: "clt_globex", Name: "Globex <Inc>", Identifier: "globex", Status: client.StatusActive},
		}},
		Apps:            &fakeAppStore{apps: []application.Application{{ID: ids.ApplicationID("app_billing"), Code: "billing", Name: "Billing", Active: true}, {ID: ids.ApplicationID("app_acme"), Code: "acme", Name: "Acme <app>", Active: true}}},
		ServiceAccounts: fakeFnSAs{},
		Shell:           shellForTest(),
	}
}

func (h *fnsHarness) do(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	page := h.page()
	return serveMounted(t, ac, q, page, &ClientOptions{Clients: page.Clients, Shell: page.Shell})
}

// fnsUser is an anchor user holding exactly the given permissions.
func fnsUser(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_9", Email: "ops@flowcatalyst.local", Scope: auth.ScopeAnchor, Roles: []string{"platform:ops"}, Permissions: perms, PrincipalType: auth.PrincipalTypeUser}
}

const (
	fnPermView    = "platform:function:function:view"
	fnPermManage  = "platform:function:function:manage"
	fnPermPromote = "platform:function:alias:promote"
	fnPermSecrets = "platform:function:secret:manage"
	fnPermRoutes  = "platform:function:route:manage"
	fnPermDomains = "platform:function:domain:manage"
)

var fnAllPerms = []string{fnPermView, fnPermManage, fnPermPromote, fnPermSecrets, fnPermRoutes, fnPermDomains}

// fnsWithout is a user holding every function permission but the given one.
func fnsWithout(perm string) *auth.AuthContext {
	return fnsUser(slices.DeleteFunc(slices.Clone(fnAllPerms), func(p string) bool { return p == perm })...)
}

func fnsBody(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// ── list ───────────────────────────────────────────────────────────────

func TestFunctionsListRendersInsideTheShell(t *testing.T) {
	h := fnsHarnessNew()
	body := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions"}))
	mustContain(t, body,
		`<title>Functions · FlowCatalyst</title>`,
		`href="/ui/functions" class="fc-nav-item" title="Functions" aria-current="page"`,
		`<h1 class="fc-page-title">Functions</h1>`,
		`href="/ui/functions/new"`,
		`id="fns-list"`, `id="fns-table"`, `id="fns-rows"`,
		`billing.invoice-pdf`, `acme.orders`, `acme.feed`,
		`heavy`, `Platform`, `Acme Corp`, `Globex &lt;Inc&gt;`,
		`href="/ui/functions/fn_1"`, `href="/ui/functions/fn_1/edit"`,
		`Showing 1 to 3 of 3 functions`,
		`<option value="app_billing">Billing (billing)</option>`,
		`data-signals="{selFn: &#39;&#39;, fnTab: &#39;overview&#39;}"`,
	)
	mustNotContain(t, body, `Globex <Inc>`)
}

func TestFunctionsListPermissions(t *testing.T) {
	h := fnsHarnessNew()
	t.Run("without view the page is refused", func(t *testing.T) {
		rec := h.do(t, fnsUser(fnPermManage), req{target: "/ui/functions"})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status %d", rec.Code)
		}
		d := h.do(t, fnsUser(fnPermManage), req{target: "/ui/functions", datastar: true})
		mustContain(t, d.Body.String(), "permission required")
		mustNotContain(t, d.Body.String(), "billing.invoice-pdf")
	})
	t.Run("view only hides Create and Edit", func(t *testing.T) {
		body := fnsBody(t, h.do(t, fnsUser(fnPermView), req{target: "/ui/functions"}))
		mustContain(t, body, "billing.invoice-pdf", `href="/ui/functions/fn_1"`)
		mustNotContain(t, body, `href="/ui/functions/new"`, `/ui/functions/fn_1/edit`)
	})
	t.Run("a client user sees platform functions and its own clients' only", func(t *testing.T) {
		ac := &auth.AuthContext{PrincipalID: "prn_5", Scope: auth.ScopeClient, Clients: []string{"clt_acme"}, Roles: []string{"r"}, Permissions: []string{fnPermView}, PrincipalType: auth.PrincipalTypeUser}
		body := fnsBody(t, h.do(t, ac, req{target: "/ui/functions"}))
		mustContain(t, body, "billing.invoice-pdf", "acme.orders")
		mustNotContain(t, body, "acme.feed")
		if h.store.lastFilters.AccessibleClientIDs == nil {
			t.Error("the list was not confined to the caller's clients in SQL")
		}
		// Another client's function is not readable by id either.
		rec := h.do(t, ac, req{target: "/ui/functions/fn_3"})
		if rec.Code != http.StatusForbidden {
			t.Errorf("another client's function: status %d", rec.Code)
		}
	})
}

func TestFunctionsListFilters(t *testing.T) {
	h := fnsHarnessNew()
	body := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions?q=acme.&client=clt_acme", datastar: true}))
	if got := deref(h.store.lastFilters.AddressPrefix); got != "acme." {
		t.Errorf("address prefix = %q", got)
	}
	if got := deref(h.store.lastFilters.ClientID); got != "clt_acme" {
		t.Errorf("client = %q", got)
	}
	mustContain(t, body, "acme.orders", `<tr id="fns-fn_2"`, `/ui/functions?client=clt_acme\u0026q=acme.`)
	mustNotContain(t, body, "acme.feed", "billing.invoice-pdf")
	// An application or client the caller is not offered is dropped, not trusted.
	fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions?application=app_nope&client=clt_nope", datastar: true}))
	if h.store.lastFilters.ApplicationID != nil || h.store.lastFilters.ClientID != nil {
		t.Errorf("unknown facets reached the query: %+v", h.store.lastFilters)
	}
}

// ── drawer ─────────────────────────────────────────────────────────────

func TestFunctionDrawerContent(t *testing.T) {
	h := fnsHarnessNew()
	body := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_1", datastar: true}))
	mustContain(t, body,
		`data: selector #fns-drawer`, `"selFn":"fn_1"`,
		// header and tabs
		`billing.invoice-pdf`, `role="tablist"`, `id="fns-tabs-tab-versions"`, `id="fns-tabs-panel-settings"`,
		// overview
		`Renders &lt;b&gt;invoices&lt;/b&gt;`, `orders-svc`, `href="/identity/service-accounts/sa_1"`, `64 MB`, `30000 ms`, `1048576 bytes`,
		// versions: aliases on the rows, the failure text escaped, the digest short
		`id="fns-version-2"`, `module load boom &lt;x&gt;`, `abababababab…`,
		// aliases
		`id="fns-alias-live"`, `id="fns-alias-qa"`,
		// declared surface, read from the live version (2)
		`From version 2 (live)`, `/render`, `billing:invoice:created`, `BLOCK_ON_ERROR`, `0 * * * *`, `api.example.com`, `billing:invoice:rendered`, `billing.mailer/send`,
		// settings: config values shown, declared-but-unset keys flagged
		`hello &lt;b&gt;world&lt;/b&gt;`, `extra`, `One or more declared keys have no value set.`,
		// routes
		`api.example.com`, `/hooks`, `id="fns-route-rt_1"`,
	)
	mustNotContain(t, body, `Renders <b>`, `module load boom <x>`)
}

func TestFunctionDrawerTabFromTheURL(t *testing.T) {
	h := fnsHarnessNew()
	nav := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_1?tab=settings"}))
	mustContain(t, nav, `data-signals="{selFn: &#39;fn_1&#39;, fnTab: &#39;settings&#39;}"`, `id="fns-drawer"`, `aria-selected="true"`)
	// An unknown tab is the first one.
	other := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_1?tab=nope"}))
	mustContain(t, other, `fnTab: &#39;overview&#39;`)
	// A bad id is not a function.
	if rec := h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_1'x"}); rec.Code != http.StatusNotFound {
		t.Errorf("status %d", rec.Code)
	}
	if rec := h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_missing"}); rec.Code != http.StatusNotFound {
		t.Errorf("missing: status %d", rec.Code)
	}
}

// A secret and a database DSN are never in a response, whatever the store
// holds, on any tab and in any dialog.
func TestFunctionPagesNeverCarrySecretValues(t *testing.T) {
	h := fnsHarnessNew()
	ac := anchorAdmin()
	for _, target := range []string{
		"/ui/functions/fn_1", "/ui/functions/fn_1?tab=settings",
		"/ui/functions/fn_1/settings/secrets/edit?key=api_key", "/ui/functions/fn_1/settings/db/edit?key=main",
		"/ui/functions/fn_1/settings/secrets/new?key=hook_secret", "/ui/functions/fn_1/settings/config/edit?key=greeting",
	} {
		for _, datastar := range []bool{false, true} {
			body := fnsBody(t, h.do(t, ac, req{target: target, datastar: datastar}))
			mustNotContain(t, body, fnsSecretValue, fnsDSNValue, "hunter2")
		}
	}
	// The settings tab shows them as set.
	body := fnsBody(t, h.do(t, ac, req{target: "/ui/functions/fn_1?tab=settings", datastar: true}))
	mustContain(t, body, `<code>api_key</code>`, `<code>main</code>`, "Write-only — values are never shown once set.", "Write-only — DSNs are never shown once set.")
}

func TestFunctionDrawerPermissionMatrix(t *testing.T) {
	h := fnsHarnessNew()
	marks := map[string]string{
		"edit":        `href="/ui/functions/fn_1/edit"`,
		"delete":      `href="/ui/functions/fn_1/delete"`,
		"promote":     `/ui/functions/fn_1/versions/1/promote`,
		"demote":      `data-action="/ui/functions/fn_1/aliases/live/delete"`,
		"point alias": `/ui/functions/fn_1/aliases/new?version=1`,
		"alias add":   `href="/ui/functions/fn_1/aliases/new"`,
		"retire":      `data-action="/ui/functions/fn_1/versions/5/retire"`,
		"config add":  `href="/ui/functions/fn_1/settings/config/new"`,
		"config set":  `/ui/functions/fn_1/settings/config/new?key=region`,
		"secret add":  `href="/ui/functions/fn_1/settings/secrets/new"`,
		"secret set":  `/ui/functions/fn_1/settings/secrets/new?key=hook_secret`,
		"secret drop": `data-action="/ui/functions/fn_1/settings/secrets/delete?key=api_key"`,
		"db add":      `href="/ui/functions/fn_1/settings/db/new"`,
		"route add":   `href="/ui/functions/fn_1/routes/new"`,
		"route edit":  `href="/ui/functions/fn_1/routes/rt_1/edit"`,
		"route drop":  `data-action="/ui/functions/fn_1/routes/rt_1/delete"`,
		"zones":       `Claimed Zones`,
	}
	cases := []struct {
		name  string
		perms []string
		want  []string
	}{
		{"view only", []string{fnPermView}, nil},
		{"manage", []string{fnPermView, fnPermManage}, []string{"edit", "delete", "retire", "config add", "config set"}},
		{"promote", []string{fnPermView, fnPermPromote}, []string{"promote", "demote", "point alias", "alias add"}},
		{"secrets", []string{fnPermView, fnPermSecrets}, []string{"secret add", "secret set", "secret drop", "db add"}},
		{"routes", []string{fnPermView, fnPermRoutes}, []string{"route add", "route edit", "route drop"}},
		{"domains", []string{fnPermView, fnPermDomains}, []string{"zones"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := fnsBody(t, h.do(t, fnsUser(c.perms...), req{target: "/ui/functions/fn_1", datastar: true}))
			for name, mark := range marks {
				if has := strings.Contains(body, mark); has != slices.Contains(c.want, name) {
					t.Errorf("%q present = %v, want %v", name, has, !has)
				}
			}
		})
	}
	t.Run("the live version offers Demote, not Promote", func(t *testing.T) {
		body := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_1", datastar: true}))
		mustNotContain(t, body, `/versions/2/promote`)
	})
	t.Run("claimed zones are the function owner's", func(t *testing.T) {
		body := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_1", datastar: true}))
		mustContain(t, body, `<code class="fc-mono-chip">example.com</code>`)
		mustNotContain(t, body, `acme.test`)
	})
	t.Run("an aliased or retired version cannot be retired", func(t *testing.T) {
		body := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_1", datastar: true}))
		mustNotContain(t, body, `/versions/1/retire`, `/versions/4/retire`)
		mustContain(t, body, `An alias still points at this version. Remove the alias first.`, `This version is retired.`)
	})
}

// ── forged posts ───────────────────────────────────────────────────────

// Every write checks the permission of the API endpoint it mirrors: a caller
// who holds every other function permission is refused, and nothing is run.
func TestFunctionForgedPostsAreRefused(t *testing.T) {
	cases := []struct {
		name, need, target string
		form               url.Values
		call               string
	}{
		{"create", fnPermManage, "/ui/functions/new", url.Values{"application": {"app_billing"}, "name": {"thumbs"}}, "create"},
		{"edit", fnPermManage, "/ui/functions/fn_1/edit", url.Values{"memoryMb": {"128"}}, "update:fn_1"},
		{"delete", fnPermManage, "/ui/functions/fn_1/delete", url.Values{"confirm": {"billing.invoice-pdf"}}, "delete:fn_1"},
		{"retire", fnPermManage, "/ui/functions/fn_1/versions/5/retire", nil, "retire:fn_1:5"},
		{"config", fnPermManage, "/ui/functions/fn_1/settings/config", url.Values{"key": {"region"}, "value": {"eu"}}, "set:fn_1:CONFIG:region"},
		{"config remove", fnPermManage, "/ui/functions/fn_1/settings/config/delete?key=greeting", nil, "unset:fn_1:CONFIG:greeting"},
		{"promote", fnPermPromote, "/ui/functions/fn_1/versions/1/promote", nil, "alias:fn_1:live:1"},
		{"alias", fnPermPromote, "/ui/functions/fn_1/aliases", url.Values{"name": {"qa"}, "version": {"5"}}, "alias:fn_1:qa:5"},
		{"alias remove", fnPermPromote, "/ui/functions/fn_1/aliases/qa/delete", nil, "unalias:fn_1:qa"},
		{"secret", fnPermSecrets, "/ui/functions/fn_1/settings/secrets", url.Values{"key": {"api_key"}, "value": {"n3w"}}, "set:fn_1:SECRET:api_key"},
		{"secret remove", fnPermSecrets, "/ui/functions/fn_1/settings/secrets/delete?key=api_key", nil, "unset:fn_1:SECRET:api_key"},
		{"db", fnPermSecrets, "/ui/functions/fn_1/settings/db", url.Values{"key": {"main"}, "value": {"postgres://x"}}, "set:fn_1:DB:main"},
		{"db remove", fnPermSecrets, "/ui/functions/fn_1/settings/db/delete?key=main", nil, "unset:fn_1:DB:main"},
		{"route", fnPermRoutes, "/ui/functions/fn_1/routes", url.Values{"hostname": {"api.example.com"}, "pathPrefix": {"/"}}, "route:fn_1"},
		{"route remove", fnPermRoutes, "/ui/functions/fn_1/routes/rt_1/delete", nil, "unroute:fn_1:rt_1"},
	}
	for _, c := range cases {
		for _, datastar := range []bool{true, false} {
			t.Run(c.name+map[bool]string{true: " datastar", false: " native"}[datastar], func(t *testing.T) {
				h := fnsHarnessNew()
				form := c.form
				if form == nil {
					form = url.Values{}
				}
				rec := h.do(t, fnsWithout(c.need), req{method: http.MethodPost, target: c.target, form: form, datastar: datastar})
				if len(h.ops.calls) != 0 {
					t.Fatalf("a caller without %s ran %v", c.need, h.ops.calls)
				}
				if !datastar && rec.Code != http.StatusForbidden && rec.Code != http.StatusUnprocessableEntity {
					t.Errorf("native status %d", rec.Code)
				}
				if !strings.Contains(strings.ToLower(rec.Body.String()), "permission required") {
					t.Errorf("no refusal in the answer: %s", rec.Body.String())
				}
				// With the permission the same post runs the operation.
				h = fnsHarnessNew()
				h.do(t, fnsUser(fnPermView, c.need), req{method: http.MethodPost, target: c.target, form: form, datastar: datastar})
				if !slices.Contains(h.ops.calls, c.call) {
					t.Errorf("calls = %v, want %s", h.ops.calls, c.call)
				}
			})
		}
	}
	t.Run("a dialog is refused with the same permission", func(t *testing.T) {
		for _, c := range []struct{ need, target string }{
			{fnPermManage, "/ui/functions/new"},
			{fnPermManage, "/ui/functions/fn_1/edit"},
			{fnPermManage, "/ui/functions/fn_1/delete"},
			{fnPermManage, "/ui/functions/fn_1/settings/config/new"},
			{fnPermSecrets, "/ui/functions/fn_1/settings/secrets/new"},
			{fnPermSecrets, "/ui/functions/fn_1/settings/db/edit?key=main"},
			{fnPermPromote, "/ui/functions/fn_1/aliases/new"},
			{fnPermRoutes, "/ui/functions/fn_1/routes/new"},
			{fnPermRoutes, "/ui/functions/fn_1/routes/rt_1/edit"},
		} {
			h := fnsHarnessNew()
			body := h.do(t, fnsWithout(c.need), req{target: c.target, datastar: true}).Body.String()
			if !strings.Contains(body, "permission required") || strings.Contains(body, "<dialog") {
				t.Errorf("%s without %s: %s", c.target, c.need, body)
			}
		}
	})
}

// ── create and edit ────────────────────────────────────────────────────

func TestFunctionCreate(t *testing.T) {
	good := func(kv ...string) url.Values {
		v := url.Values{"application": {"app_billing"}, "name": {"thumbs"}, "client": {""}, "pool": {""}, "serviceAccountId": {""}, "memoryMb": {""}, "maxConcurrency": {""}, "timeoutMs": {""}, "maxBodyBytes": {""}}
		for i := 0; i+1 < len(kv); i += 2 {
			v.Set(kv[i], kv[i+1])
		}
		return v
	}
	t.Run("the dialog offers the SPA's fields", func(t *testing.T) {
		h := fnsHarnessNew()
		body := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions/new", datastar: true}))
		mustContain(t, body,
			`<div id="fns-dialog-slot"><dialog id="fns-form"`, `action="/ui/functions/new"`, `Create Function`,
			`id="fns-form-application"`, `Billing (billing)`, `Acme &lt;app&gt;`, `id="fns-form-name"`, `id="fns-form-client"`,
			`id="fns-form-pool"`, `role="switch"`, `id="fns-form-service-account-id"`, `Orders &lt;service&gt; (orders-svc)`, `Billing (billing-svc)`,
			`id="fns-form-memory-mb"`, `placeholder="64"`, `placeholder="1048576"`,
		)
		mustNotContain(t, body, `old-svc`, `Orders <service>`)
	})
	t.Run("a valid post runs the use case with the SPA's body and opens the drawer", func(t *testing.T) {
		h := fnsHarnessNew()
		body := fnsBody(t, h.do(t, anchorAdmin(), req{
			method: http.MethodPost, target: "/ui/functions/new", datastar: true, multipart: true,
			form: good("client", "acme", "pool", "heavy", "warm", "true", "serviceAccountId", "sa_2", "memoryMb", "128", "maxBodyBytes", "2048"),
		}))
		if len(h.ops.created) != 1 {
			t.Fatalf("created = %v", h.ops.created)
		}
		cmd := h.ops.created[0]
		if cmd.ApplicationID != "app_billing" || cmd.Name != "thumbs" || deref(cmd.ClientID) != "clt_acme" || deref(cmd.Pool) != "heavy" || deref(cmd.ServiceAccount) != "sa_2" || cmd.Warm == nil || !*cmd.Warm {
			t.Errorf("command = %+v", cmd)
		}
		if cmd.Limits == nil || *cmd.Limits.MemoryMB != 128 || *cmd.Limits.MaxBodyBytes != 2048 || cmd.Limits.TimeoutMs != nil {
			t.Errorf("limits = %+v", cmd.Limits)
		}
		mustContain(t, body, "Function created", `"selFn":"fn_9"`)
	})
	t.Run("the SPA's checks are field errors and nothing runs", func(t *testing.T) {
		h := fnsHarnessNew()
		body := fnsBody(t, h.do(t, anchorAdmin(), req{
			method: http.MethodPost, target: "/ui/functions/new", datastar: true,
			form: good("application", "", "name", "Bad_Name", "memoryMb", "lots"),
		}))
		mustContain(t, body, `id="fns-form-application-err"`, `id="fns-form-name-err"`, `Lowercase letters, numbers, hyphens only.`, `id="fns-form-memory-mb-err"`, `Enter a whole number.`)
		if len(h.ops.calls) != 0 {
			t.Errorf("ran %v", h.ops.calls)
		}
	})
	t.Run("an unknown client is the client field's error", func(t *testing.T) {
		h := fnsHarnessNew()
		body := fnsBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/new", datastar: true, form: good("client", "nobody")}))
		mustContain(t, body, `id="fns-form-client-err"`, noSuchClient)
	})
	t.Run("the use case's refusals land on their fields", func(t *testing.T) {
		for _, c := range []struct {
			err   *usecase.Error
			field string
		}{
			{usecase.Conflict("ADDRESS_EXISTS", "Function with address 'billing.thumbs' already exists"), "name"},
			{usecase.Validation("SERVICE_ACCOUNT_INACTIVE", "service account 'x' is inactive"), "service-account-id"},
			{usecase.Validation("TIMEOUT_MS_TOO_LARGE", "timeoutMs exceeds the platform ceiling"), "timeout-ms"},
			{usecase.Authorization("SCOPE_FORBIDDEN", "no access to service account 'x'"), "service-account-id"},
			{usecase.Authorization("SCOPE_FORBIDDEN", "no access to client"), "client"},
		} {
			h := fnsHarnessNew()
			h.ops.err = c.err
			body := fnsBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/new", datastar: true, form: good("client", "acme")}))
			mustContain(t, body, `id="fns-form-`+c.field+`-err"`)
		}
	})
	t.Run("a native post redirects with a flash key", func(t *testing.T) {
		h := fnsHarnessNew()
		rec := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/new", form: good()})
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/functions/fn_9?tab=overview" {
			t.Fatalf("status %d, location %q", rec.Code, rec.Header().Get("Location"))
		}
		if !strings.Contains(rec.Header().Get("Set-Cookie"), "fc_ui_flash=function-created") {
			t.Errorf("cookie: %s", rec.Header().Get("Set-Cookie"))
		}
	})
	t.Run("a native post with errors re-renders the page with the dialog open", func(t *testing.T) {
		h := fnsHarnessNew()
		rec := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/new", form: good("name", "")})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status %d", rec.Code)
		}
		mustContain(t, rec.Body.String(), `<dialog id="fns-form"`, ` open>`, `Name is required.`, `class="fc-sidebar"`)
	})
}

func TestFunctionEdit(t *testing.T) {
	post := func(h *fnsHarness, ac *auth.AuthContext, form url.Values) string {
		t.Helper()
		return fnsBody(t, h.do(t, ac, req{method: http.MethodPost, target: "/ui/functions/fn_1/edit", datastar: true, form: form, multipart: true}))
	}
	same := func(kv ...string) url.Values {
		v := url.Values{
			"description": {"Renders <b>invoices</b>"}, "pool": {"heavy"}, "warm": {"true"}, "serviceAccountId": {"sa_1"},
			"memoryMb": {"64"}, "maxConcurrency": {"16"}, "timeoutMs": {"30000"}, "maxBodyBytes": {"1048576"}, "drawer": {"fn_1"}, "tab": {"versions"},
		}
		for i := 0; i+1 < len(kv); i += 2 {
			if kv[i+1] == "-" {
				v.Del(kv[i])
			} else {
				v.Set(kv[i], kv[i+1])
			}
		}
		return v
	}
	t.Run("the dialog is prefilled and names the address", func(t *testing.T) {
		h := fnsHarnessNew()
		body := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_1/edit", datastar: true}))
		mustContain(t, body, `action="/ui/functions/fn_1/edit"`, `Edit Function`, `billing.invoice-pdf`, `value="heavy"`, `Renders &lt;b&gt;invoices&lt;/b&gt;`, `checked`,
			`<option value="sa_1" selected>`, `value="30000"`, `name="drawer"`, `name="tab"`)
		mustNotContain(t, body, `id="fns-form-name"`, `id="fns-form-application"`)
	})
	t.Run("an untouched form sends only the warm flag and the limits", func(t *testing.T) {
		h := fnsHarnessNew()
		body := post(h, anchorAdmin(), same())
		cmd := h.ops.updated[0]
		if cmd.Description != nil || cmd.Pool != nil || cmd.ClearPool || cmd.ServiceAccount != nil || cmd.ClearServiceAccount || cmd.Warm == nil || !*cmd.Warm {
			t.Errorf("command = %+v", cmd)
		}
		// The drawer is refreshed on the tab it is on, with a flash.
		mustContain(t, body, "Function updated", `selector #fns-drawer`, `data-signals:fn-tab="&#39;versions&#39;"`, `"fnTab":"versions"`)
	})
	t.Run("changes are sent: description, pool cleared, warm off, account cleared", func(t *testing.T) {
		h := fnsHarnessNew()
		post(h, anchorAdmin(), same("description", "", "pool", "", "warm", "-", "serviceAccountId", "", "timeoutMs", "5000"))
		cmd := h.ops.updated[0]
		if cmd.Description == nil || *cmd.Description != "" || !cmd.ClearPool || cmd.Pool != nil || *cmd.Warm || !cmd.ClearServiceAccount || cmd.ServiceAccount != nil {
			t.Errorf("command = %+v", cmd)
		}
		if *cmd.Limits.TimeoutMs != 5000 {
			t.Errorf("limits = %+v", cmd.Limits)
		}
	})
	t.Run("a new pool and a new service account", func(t *testing.T) {
		h := fnsHarnessNew()
		post(h, anchorAdmin(), same("pool", "light", "serviceAccountId", "sa_2"))
		cmd := h.ops.updated[0]
		if deref(cmd.Pool) != "light" || deref(cmd.ServiceAccount) != "sa_2" || cmd.ClearServiceAccount {
			t.Errorf("command = %+v", cmd)
		}
	})
	t.Run("a caller who cannot view service accounts leaves the account alone", func(t *testing.T) {
		h := fnsHarnessNew()
		ac := fnsUser(fnPermView, fnPermManage)
		dialog := fnsBody(t, h.do(t, ac, req{target: "/ui/functions/fn_1/edit", datastar: true}))
		mustContain(t, dialog, `Choosing a service account needs permission to view service accounts.`, `orders-svc`)
		post(h, ac, same("serviceAccountId", "-"))
		if cmd := h.ops.updated[0]; cmd.ServiceAccount != nil || cmd.ClearServiceAccount {
			t.Errorf("the account changed: %+v", cmd)
		}
	})
	t.Run("a refusal lands on its field", func(t *testing.T) {
		h := fnsHarnessNew()
		h.ops.err = usecase.Validation("MEMORY_MB_TOO_LARGE", "memoryMb exceeds the platform ceiling")
		mustContain(t, post(h, anchorAdmin(), same("memoryMb", "99999")), `id="fns-form-memory-mb-err"`, `exceeds the platform ceiling`)
	})
	t.Run("an unknown function is not found", func(t *testing.T) {
		h := fnsHarnessNew()
		rec := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_nope/edit", form: same()})
		if rec.Code != http.StatusNotFound {
			t.Errorf("status %d", rec.Code)
		}
	})
}

// ── versions and aliases ───────────────────────────────────────────────

func TestFunctionPromoteAndAliases(t *testing.T) {
	t.Run("promote is immediate and reports the wiring", func(t *testing.T) {
		h := fnsHarnessNew()
		h.ops.wiring = FunctionWiring{DispatchPoolCode: "fn-billing-invoice-pdf", SubscriptionsCreated: 2, SchedulesUpdated: 1}
		body := fnsBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/versions/5/promote", datastar: true, form: url.Values{"drawer": {"fn_1"}, "tab": {"versions"}}}))
		if !slices.Equal(h.ops.calls, []string{"alias:fn_1:live:5"}) {
			t.Fatalf("calls = %v", h.ops.calls)
		}
		mustContain(t, body, "Promoted", "Version 5 is now live — pool fn-billing-invoice-pdf · 2 subscriptions created · 1 schedule updated", `selector #fns-drawer`)
	})
	t.Run("a refused promote is an error flash", func(t *testing.T) {
		h := fnsHarnessNew()
		h.ops.err = usecase.BusinessRule("SETTINGS_MISSING", "settings missing: secret hook_secret")
		body := fnsBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/versions/5/promote", datastar: true, form: url.Values{}}))
		mustContain(t, body, "settings missing: secret hook_secret", "fc-flash")
		mustNotContain(t, body, "Promoted")
	})
	t.Run("a version number must be a number", func(t *testing.T) {
		h := fnsHarnessNew()
		rec := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/versions/x/promote", form: url.Values{}})
		if rec.Code != http.StatusNotFound || len(h.ops.calls) != 0 {
			t.Errorf("status %d calls %v", rec.Code, h.ops.calls)
		}
	})
	t.Run("demote removes the live alias", func(t *testing.T) {
		h := fnsHarnessNew()
		h.ops.wiring = FunctionWiring{SubscriptionsDeleted: 1}
		body := fnsBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/aliases/live/delete", datastar: true, form: url.Values{}}))
		mustContain(t, body, "Alias removed — 1 subscription deleted")
		if !slices.Equal(h.ops.calls, []string{"unalias:fn_1:live"}) {
			t.Errorf("calls = %v", h.ops.calls)
		}
	})
	t.Run("an alias name must be an alias name", func(t *testing.T) {
		h := fnsHarnessNew()
		rec := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/aliases/Bad_Name/delete", form: url.Values{}})
		if rec.Code != http.StatusNotFound || len(h.ops.calls) != 0 {
			t.Errorf("status %d calls %v", rec.Code, h.ops.calls)
		}
	})
	t.Run("the point-alias dialog lists the ready versions and starts at live", func(t *testing.T) {
		h := fnsHarnessNew()
		body := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_1/aliases/new?version=5", datastar: true}))
		mustContain(t, body, `<dialog id="fns-alias-form"`, `value="live"`, `<option value="5" selected>Version 5</option>`, `<option value="1">Version 1</option>`)
		mustNotContain(t, body, `Version 3`, `Version 4`)
	})
	t.Run("saving an alias", func(t *testing.T) {
		h := fnsHarnessNew()
		body := fnsBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/aliases", datastar: true, form: url.Values{"name": {"canary"}, "version": {"5"}}}))
		mustContain(t, body, "Alias updated", "canary → version 5")
		if !slices.Equal(h.ops.calls, []string{"alias:fn_1:canary:5"}) {
			t.Errorf("calls = %v", h.ops.calls)
		}
		// The SPA disables Save for a blank name or no version: field errors.
		bad := fnsBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/aliases", datastar: true, form: url.Values{"name": {""}, "version": {""}}}))
		mustContain(t, bad, `id="fns-alias-form-name-err"`, `id="fns-alias-form-version-err"`)
		// The use case's refusal.
		h.ops.err = usecase.BusinessRule("VERSION_NOT_READY", "version 3 is FAILED, not READY")
		refused := fnsBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/aliases", datastar: true, form: url.Values{"name": {"qa"}, "version": {"3"}}}))
		mustContain(t, refused, `id="fns-alias-form-version-err"`, "is FAILED, not READY")
	})
	t.Run("retire", func(t *testing.T) {
		h := fnsHarnessNew()
		body := fnsBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/versions/5/retire", datastar: true, form: url.Values{}}))
		mustContain(t, body, "Version 5 retired")
		h.ops.err = usecase.Conflict("RETIRE_IN_USE", "version 5 is still pointed at by alias 'qa'")
		refused := fnsBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/versions/5/retire", datastar: true, form: url.Values{}}))
		mustContain(t, refused, "still pointed at by alias")
	})
	t.Run("a native promote redirects to the drawer on the tab", func(t *testing.T) {
		h := fnsHarnessNew()
		rec := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/versions/5/promote", form: url.Values{"tab": {"versions"}}})
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/functions/fn_1?tab=versions" {
			t.Errorf("status %d location %q", rec.Code, rec.Header().Get("Location"))
		}
	})
}

// ── settings ───────────────────────────────────────────────────────────

func TestFunctionSettings(t *testing.T) {
	post := func(h *fnsHarness, kind string, form url.Values) string {
		t.Helper()
		return fnsBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/settings/" + kind, datastar: true, form: form, multipart: true}))
	}
	t.Run("a config value is shown for editing, a secret and a DSN never are", func(t *testing.T) {
		h := fnsHarnessNew()
		cfg := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_1/settings/config/edit?key=greeting", datastar: true}))
		mustContain(t, cfg, `Set config value`, `hello &lt;b&gt;world&lt;/b&gt;`, `readonly`, `<textarea`)
		sec := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_1/settings/secrets/edit?key=api_key", datastar: true}))
		mustContain(t, sec, `Set secret`, `type="password"`, `Write-only: this value is stored encrypted`, `readonly`)
		mustNotContain(t, sec, `<textarea`, `value="`+fnsSecretValue)
		db := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_1/settings/db/new", datastar: true}))
		mustContain(t, db, `Set database binding`, `Name`, `type="password"`)
		mustNotContain(t, db, `readonly`)
	})
	t.Run("saving each kind runs the use case with the value as typed", func(t *testing.T) {
		h := fnsHarnessNew()
		post(h, "config", url.Values{"key": {"region"}, "value": {" eu-west "}})
		post(h, "secrets", url.Values{"key": {"hook_secret"}, "value": {"n3w-s3cret-VALUE"}})
		post(h, "db", url.Values{"key": {"main"}, "value": {"postgres://u:p@h/db"}})
		want := map[string]string{"CONFIG/region": " eu-west ", "SECRET/hook_secret": "n3w-s3cret-VALUE", "DB/main": "postgres://u:p@h/db"}
		for k, v := range want {
			if h.ops.settings[k] != v {
				t.Errorf("%s = %q, want %q", k, h.ops.settings[k], v)
			}
		}
	})
	t.Run("no answer to a secret save echoes the secret", func(t *testing.T) {
		const typed = "TYPED-SECRET-ZZ9"
		h := fnsHarnessNew()
		var bodies []string
		bodies = append(bodies, post(h, "secrets", url.Values{"key": {"hook_secret"}, "value": {typed}, "drawer": {"fn_1"}, "tab": {"settings"}}))
		h.ops.err = usecase.Validation("ENCRYPTION_NOT_CONFIGURED", "cannot store a SECRET or DB setting")
		bodies = append(bodies, post(h, "secrets", url.Values{"key": {"hook_secret"}, "value": {typed}}))
		h.ops.err = errors.New("boom: " + "db unavailable")
		bodies = append(bodies, post(h, "db", url.Values{"key": {"main"}, "value": {typed}}))
		// A native post that fails re-renders the whole page.
		h.ops.err = usecase.Validation("INVALID_KEY_FORMAT", "bad key")
		native := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/settings/secrets", form: url.Values{"key": {"9bad"}, "value": {typed}}})
		bodies = append(bodies, native.Body.String())
		for i, b := range bodies {
			if strings.Contains(b, typed) {
				t.Errorf("answer %d carries the secret", i)
			}
		}
		mustContain(t, bodies[0], "Set secret &#34;hook_secret&#34;")
		mustContain(t, bodies[1], `id="fns-setting-form-value-err"`)
		mustContain(t, native.Body.String(), `id="fns-setting-form-key-err"`, `type="password"`)
	})
	t.Run("a config value is echoed on failure", func(t *testing.T) {
		h := fnsHarnessNew()
		h.ops.err = usecase.Validation("INVALID_KEY_FORMAT", "bad key")
		body := post(h, "config", url.Values{"key": {"9bad"}, "value": {"keep-me"}})
		mustContain(t, body, `id="fns-setting-form-key-err"`, `keep-me`)
	})
	t.Run("a blank key or value is a field error", func(t *testing.T) {
		h := fnsHarnessNew()
		body := post(h, "secrets", url.Values{"key": {""}, "value": {""}})
		mustContain(t, body, `id="fns-setting-form-key-err"`, `id="fns-setting-form-value-err"`)
		if len(h.ops.calls) != 0 {
			t.Errorf("ran %v", h.ops.calls)
		}
	})
	t.Run("an unknown kind is not found", func(t *testing.T) {
		h := fnsHarnessNew()
		rec := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/settings/other", form: url.Values{"key": {"a"}, "value": {"b"}}})
		if rec.Code != http.StatusNotFound || len(h.ops.calls) != 0 {
			t.Errorf("status %d calls %v", rec.Code, h.ops.calls)
		}
	})
	t.Run("removing a setting names the key", func(t *testing.T) {
		h := fnsHarnessNew()
		body := fnsBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/settings/secrets/delete?key=api_key", datastar: true, form: url.Values{}}))
		mustContain(t, body, "Removed secret &#34;api_key&#34;")
		if !slices.Equal(h.ops.calls, []string{"unset:fn_1:SECRET:api_key"}) {
			t.Errorf("calls = %v", h.ops.calls)
		}
	})
}

// ── routes ─────────────────────────────────────────────────────────────

func TestFunctionRoutes(t *testing.T) {
	post := func(h *fnsHarness, form url.Values) string {
		t.Helper()
		return fnsBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/routes", datastar: true, form: form}))
	}
	t.Run("the dialogs", func(t *testing.T) {
		h := fnsHarnessNew()
		add := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_1/routes/new", datastar: true}))
		mustContain(t, add, `Add Route`, `value="/"`, `<option value="qa">qa</option>`)
		mustNotContain(t, add, `<option value="live">`, `readonly`)
		edit := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_1/routes/rt_1/edit", datastar: true}))
		mustContain(t, edit, `Edit Route`, `value="api.example.com"`, `value="/hooks"`, `readonly`, `value="qa"`, `name="edit" value="1"`)
		if rec := h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_1/routes/rt_9/edit"}); rec.Code != http.StatusNotFound {
			t.Errorf("unknown route: status %d", rec.Code)
		}
	})
	t.Run("saving a route splits the alias prefixes", func(t *testing.T) {
		h := fnsHarnessNew()
		body := post(h, url.Values{"hostname": {"Api.Example.com"}, "pathPrefix": {""}, "alias": {""}, "aliasPrefixes": {"qa, canary  dev"}})
		mustContain(t, body, "Route saved")
		cmd := h.ops.routes[0]
		if cmd.FunctionID != "fn_1" || cmd.Hostname != "Api.Example.com" || cmd.PathPrefix != "/" || cmd.Alias != "" || !slices.Equal(cmd.AliasPrefixes, []string{"qa", "canary", "dev"}) {
			t.Errorf("command = %+v", cmd)
		}
	})
	t.Run("a hostname is required", func(t *testing.T) {
		h := fnsHarnessNew()
		mustContain(t, post(h, url.Values{"hostname": {""}}), `id="fns-route-form-hostname-err"`)
		if len(h.ops.calls) != 0 {
			t.Errorf("ran %v", h.ops.calls)
		}
	})
	t.Run("the use case's refusals land on their fields", func(t *testing.T) {
		for code, field := range map[string]string{
			"ROUTE_HOST_NOT_CLAIMED": "hostname", "ROUTE_TAKEN": "hostname", "INVALID_HOSTNAME": "hostname",
			"ALIAS_NOT_FOUND": "alias", "ALIAS_PREFIXES_REQUIRE_LIVE": "alias-prefixes", "INVALID_ALIAS_PREFIX": "alias-prefixes",
		} {
			h := fnsHarnessNew()
			h.ops.err = usecase.BusinessRule(code, "refused: "+code)
			mustContain(t, post(h, url.Values{"hostname": {"x.example.com"}}), `id="fns-route-form-`+field+`-err"`, "Refused: "+code)
		}
	})
	t.Run("removing a route", func(t *testing.T) {
		h := fnsHarnessNew()
		body := fnsBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/routes/rt_1/delete", datastar: true, form: url.Values{}}))
		mustContain(t, body, "Route removed")
		if !slices.Equal(h.ops.calls, []string{"unroute:fn_1:rt_1"}) {
			t.Errorf("calls = %v", h.ops.calls)
		}
	})
}

// ── delete ─────────────────────────────────────────────────────────────

func TestFunctionDeleteNeedsTheTypedAddress(t *testing.T) {
	post := func(h *fnsHarness, confirm string) string {
		t.Helper()
		return fnsBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/delete", datastar: true, form: url.Values{"confirm": {confirm}, "drawer": {"fn_1"}}, multipart: true}))
	}
	t.Run("the dialog summarises what goes with the function", func(t *testing.T) {
		h := fnsHarnessNew()
		body := fnsBody(t, h.do(t, anchorAdmin(), req{target: "/ui/functions/fn_1/delete", datastar: true}))
		mustContain(t, body, `Delete Function`, `5 versions`, `Live version: <strong>v2</strong>`, `Type billing.invoice-pdf to confirm`, `placeholder="billing.invoice-pdf"`, `cannot be undone`)
	})
	t.Run("a wrong or partial address deletes nothing", func(t *testing.T) {
		for _, typed := range []string{"", "billing", "billing.invoice-pd", "BILLING.INVOICE-PDF", "billing.invoice-pdf.", "yes"} {
			h := fnsHarnessNew()
			body := post(h, typed)
			mustContain(t, body, `id="fns-delete-form-confirm-err"`, "Type the function&#39;s address exactly to confirm.")
			if len(h.ops.calls) != 0 {
				t.Errorf("%q ran %v", typed, h.ops.calls)
			}
		}
	})
	t.Run("a wrong address on a native post is refused too", func(t *testing.T) {
		h := fnsHarnessNew()
		rec := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/delete", form: url.Values{"confirm": {"nope"}}})
		if rec.Code != http.StatusUnprocessableEntity || len(h.ops.calls) != 0 {
			t.Errorf("status %d calls %v", rec.Code, h.ops.calls)
		}
		mustContain(t, rec.Body.String(), `<dialog id="fns-delete-form"`)
	})
	t.Run("the address typed with surrounding whitespace deletes and closes the drawer", func(t *testing.T) {
		h := fnsHarnessNew()
		body := post(h, "  billing.invoice-pdf \n")
		if !slices.Equal(h.ops.calls, []string{"delete:fn_1"}) {
			t.Fatalf("calls = %v", h.ops.calls)
		}
		mustContain(t, body, "Function billing.invoice-pdf deleted", `"selFn":""`)
		mustNotContain(t, body, `selector #fns-drawer`)
	})
	t.Run("a refusal stays in the dialog", func(t *testing.T) {
		h := fnsHarnessNew()
		h.ops.err = usecase.Authorization("SCOPE_FORBIDDEN", "no access to this function")
		body := post(h, "billing.invoice-pdf")
		mustContain(t, body, "No access to this function")
		mustNotContain(t, body, `"selFn":""`)
	})
	t.Run("a native delete redirects to the list", func(t *testing.T) {
		h := fnsHarnessNew()
		rec := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/functions/fn_1/delete", form: url.Values{"confirm": {"billing.invoice-pdf"}}})
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/functions" {
			t.Errorf("status %d location %q", rec.Code, rec.Header().Get("Location"))
		}
	})
}

// ── helpers ────────────────────────────────────────────────────────────

func TestParseFnsDescribe(t *testing.T) {
	d := parseFnsDescribe([]byte(fnsDescribeJSON))
	if len(d.Endpoints) != 2 || d.Endpoints[1].Method != "GET" || d.Endpoints[1].Auth != "none" {
		t.Errorf("endpoints = %+v", d.Endpoints)
	}
	if !slices.Equal(d.Secrets, []string{"api_key", "hook_secret"}) || !slices.Equal(d.Dispatches, []string{"billing.mailer/send"}) {
		t.Errorf("declared = %+v", d)
	}
	for _, raw := range []string{"", "null", "[1,2]", `{"endpoints": "x", "config": [1, "ok", null]}`, "{bad"} {
		got := parseFnsDescribe([]byte(raw)) // never panics
		if len(got.Endpoints) != 0 || len(got.Secrets) != 0 {
			t.Errorf("%q = %+v", raw, got)
		}
	}
	if got := parseFnsDescribe([]byte(`{"config": [1, "ok"]}`)); !slices.Equal(got.Config, []string{"ok"}) {
		t.Errorf("config = %v", got.Config)
	}
}

func TestFnsFailureTextAndWiring(t *testing.T) {
	for in, want := range map[string]string{
		``: "", `null`: "", `"plain"`: "plain", `{"message":"m"}`: "m", `{"reason":"r"}`: "r", `{"other":1}`: `{"other":1}`, `not json`: "not json",
	} {
		if got := fnsFailureText([]byte(in)); got != want {
			t.Errorf("fnsFailureText(%q) = %q, want %q", in, got, want)
		}
	}
	if got := (FunctionWiring{}).text(); got != "" {
		t.Errorf("empty wiring = %q", got)
	}
	w := FunctionWiring{DispatchPoolCode: "p", SubscriptionsCreated: 1, SubscriptionsDeleted: 3, SchedulesCreated: 2}
	if got := w.text(); got != "pool p · 1 subscription created · 3 subscriptions deleted · 2 schedules created" {
		t.Errorf("wiring = %q", got)
	}
	if got := fnsShortDigest("0123456789abcdef"); got != "0123456789ab…" {
		t.Errorf("digest = %q", got)
	}
}
