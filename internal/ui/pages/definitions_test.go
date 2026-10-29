package pages

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
)

// Definition export and import (definitions.go): the shared fixtures in
// testdata/definitions are the contract with the SPA, so each kind is
// exported to exactly the fixture's bytes and the fixture is imported back
// into the create dialog.

func defFixture(t *testing.T, kind string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "definitions", kind+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// defCase is one kind's page, built around records equivalent to its fixture.
type defCase struct {
	kind, base, id       string
	viewPerm, createPerm string
	user                 func(perms ...string) *auth.AuthContext
	admin                func() *auth.AuthContext
	// build makes a fresh page; serve runs a request through it and created
	// counts the create calls its use cases saw.
	build func(t *testing.T) (serve func(ac *auth.AuthContext, q req) *httptest.ResponseRecorder, created func() int)
	// filled are fragments of the dialog once the fixture is imported.
	filled []string
	// notes are exactly the notes the fixture gets when every reference
	// exists (what the form cannot take); nil for none.
	notes []string
	// broken maps a fixture fragment to its replacement and the note it must
	// then produce.
	broken []defBroken
}

type defBroken struct{ old, new, note string }

func fixtureTime() time.Time { return time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC) }

func defCases() []defCase {
	return []defCase{
		{
			kind: defKindApplication, base: "/ui/applications", id: "app_1",
			viewPerm: appViewPerm, createPerm: "platform:admin:application:create", user: connUser, admin: anchorAdmin,
			build: func(t *testing.T) (func(*auth.AuthContext, req) *httptest.ResponseRecorder, func() int) {
				a := app("app_1", "orders", "Orders", true)
				desc, base, icon, site := "Order capture and fulfilment", "https://orders.qa.example.com", "https://orders.qa.example.com/icon.png", "https://orders.example.com"
				logo, mime := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"/>`, "image/svg+xml"
				a.Description, a.DefaultBaseURL, a.IconURL, a.Website, a.Logo, a.LogoMimeType = &desc, &base, &icon, &site, &logo, &mime
				s := &fakeAppStore{apps: []application.Application{a}}
				ops := &fakeAppOps{store: s}
				page := &Applications{Apps: s, Ops: ops, Logins: fakeLogins{}, Docs: fakeDocs{}, Shell: shellForTest()}
				return func(ac *auth.AuthContext, q req) *httptest.ResponseRecorder { return serveMounted(t, ac, q, page) },
					func() int { return len(ops.created) }
			},
			filled: []string{`value="orders"`, `value="Orders"`, "Order capture and fulfilment", `value="https://orders.qa.example.com"`,
				`value="https://orders.example.com"`, `value="image/svg+xml"`, `&lt;svg xmlns=`},
			broken: []defBroken{{`"APPLICATION"`, `"BOGUS"`, "Type `BOGUS` is not Application or Integration"}},
		},
		{
			kind: defKindServiceAccount, base: "/ui/service-accounts", id: "sa_1",
			viewPerm: saPermView, createPerm: saPermCreate, user: saUser, admin: saAdmin,
			build: func(t *testing.T) (func(*auth.AuthContext, req) *httptest.ResponseRecorder, func() int) {
				h := newSAHarness()
				desc, scope, appID := "Signs order webhooks", "CLIENT", ids.ApplicationID("app_1")
				sa := &h.store.accounts[0]
				sa.Name, sa.Description, sa.Scope, sa.ClientIDs, sa.ApplicationID = "Orders service", &desc, &scope, []string{"clt_1"}, &appID
				h.principals["sa_1"].Roles = h.principals["sa_1"].Roles[:0]
				h.principals["sa_1"].Roles = append(h.principals["sa_1"].Roles, serviceaccount.RoleAssignment{Role: saRoleName("orders:dispatcher"), AssignedAt: fixtureTime()})
				page := h.page()
				return func(ac *auth.AuthContext, q req) *httptest.ResponseRecorder { return serveMounted(t, ac, q, page) },
					func() int { return len(h.ops.created) }
			},
			filled: []string{`value="orders-svc"`, `value="Orders service"`, "Signs order webhooks", `&#34;saFormScope&#34;:&#34;CLIENT&#34;`,
				// The code signal is seeded and kept: a stale page signal or the
				// name field's blur would otherwise replace the imported code.
				`&#34;saFormCode&#34;:&#34;orders-svc&#34;`, `&#34;saFormCodeKept&#34;:true`},
			notes: []string{
				"Application not applied by this form: `orders`.",
				"Assign roles after creating: `orders:dispatcher`.",
			},
			broken: []defBroken{{`"acme"`, `"nope"`, "Client `nope` was not found here. Import or create it first, then choose it."}},
		},
		{
			kind: defKindConnection, base: "/ui/connections", id: "con_1",
			viewPerm: "platform:messaging:connection:view", createPerm: "platform:messaging:connection:create", user: connUser, admin: anchorAdmin,
			build: func(t *testing.T) (func(*auth.AuthContext, req) *httptest.ResponseRecorder, func() int) {
				c := conn("con_1", "billing-hooks", "Billing hooks", connection.StatusActive, "acme")
				desc, ext := "Webhooks into billing", "ext-42"
				c.Description, c.ExternalID = &desc, &ext
				store := &fakeConns{conns: []connection.Connection{c}}
				ops := &fakeConnOps{conns: store}
				page := &Connections{Connections: store, Ops: ops, Clients: someClients(), ServiceAccounts: fakeSAs{}, Shell: shellForTest()}
				return func(ac *auth.AuthContext, q req) *httptest.ResponseRecorder { return serveMounted(t, ac, q, page) },
					func() int { return len(ops.created) }
			},
			filled: []string{`value="billing-hooks"`, `value="Billing hooks"`, "Webhooks into billing", `value="ext-42"`, `value="sa_1" selected`, `value="acme"`},
			broken: []defBroken{
				{`"orders-svc"`, `"nope-svc"`, "Service account `nope-svc` was not found here."},
				{`"acme"`, `"nope"`, "Client `nope` was not found here."},
			},
		},
		{
			kind: defKindDispatchPool, base: "/ui/dispatch-pools", id: "dpl_1",
			viewPerm: "platform:messaging:dispatch-pool:view", createPerm: "platform:messaging:dispatch-pool:create", user: connUser, admin: anchorAdmin,
			build: func(t *testing.T) (func(*auth.AuthContext, req) *httptest.ResponseRecorder, func() int) {
				p := pool("dpl_1", "orders-pool", "Orders pool", dispatchpool.StatusActive, "acme", fixtureTime())
				desc, rate := "Order deliveries", int32(600)
				p.Description, p.RateLimit = &desc, &rate
				store := &fakePools{pools: []dispatchpool.DispatchPool{p}}
				ops := &fakeOps{pools: store}
				page := &DispatchPools{Pools: store, Ops: ops, Clients: someClients(), Shell: shellForTest()}
				return func(ac *auth.AuthContext, q req) *httptest.ResponseRecorder { return serveMounted(t, ac, q, page) },
					func() int { return len(ops.created) }
			},
			filled: []string{`value="orders-pool"`, `value="Orders pool"`, "Order deliveries", `value="600"`, `value="10"`, `value="acme"`},
			broken: []defBroken{{`"acme"`, `"nope"`, "Client `nope` was not found here."}},
		},
		{
			kind: defKindSubscription, base: "/ui/subscriptions", id: "sub_1",
			viewPerm: subViewPerm, createPerm: "platform:messaging:subscription:create", user: connUser, admin: anchorAdmin,
			build: func(t *testing.T) (func(*auth.AuthContext, req) *httptest.ResponseRecorder, func() int) {
				s := sub("sub_1", "orders-hook", "Orders hook", subscription.StatusActive, "orders", "acme")
				desc, endpoint, connID, queue := "Order events to billing", "https://billing.example.com/hooks/orders", "con_1", "DEFAULT"
				s.Description, s.Endpoint, s.ConnectionID, s.Queue, s.ClientScoped = &desc, endpoint, &connID, &queue, true
				s.DelaySeconds = 0
				subs := &fakeSubs{subs: []subscription.Subscription{s}}
				ops := &fakeSubOps{subs: subs}
				conns := &fakeConns{conns: []connection.Connection{conn("con_1", "billing-hooks", "Billing hooks", connection.StatusActive, "")}}
				pools := &fakePools{pools: []dispatchpool.DispatchPool{pool("dpl_1", "orders-pool", "Orders pool", dispatchpool.StatusActive, "", fixtureTime())}}
				placed := et("evt_1", "orders:sales:order:placed", "Order placed", eventtype.StatusCurrent, spec("1.0", eventtype.SpecCurrent, orderSchema))
				placed.ClientScoped = true
				page := &Subscriptions{Subs: subs, Ops: ops, Pools: pools, Connections: conns, ServiceAccounts: fakeSAs{}, EventTypes: &fakeETs{ets: []eventtype.EventType{placed}}, Apps: fakeApps{}, Clients: someClients(), Shell: shellForTest()}
				return func(ac *auth.AuthContext, q req) *httptest.ResponseRecorder { return serveMounted(t, ac, q, page) },
					func() int { return len(ops.created) }
			},
			filled: []string{`value="orders-hook"`, `value="Orders hook"`, "Order events to billing", `value="https://billing.example.com/hooks/orders"`,
				`value="evt_1"`, `value="con_1" selected`, `value="dpl_1" selected`, `value="acme"`},
			notes: []string{"Application not applied by this form: `orders`."},
			broken: []defBroken{
				{`"billing-hooks"`, `"nope-conn"`, "Connection `nope-conn` was not found here."},
				{`"orders-pool"`, `"nope-pool"`, "Dispatch pool `nope-pool` was not found here."},
				{`"orders:sales:order:placed"`, `"orders:sales:order:nope"`, "Event type `orders:sales:order:nope` was not found here."},
				{`"acme"`, `"nope"`, "Client `nope` was not found here."},
			},
		},
	}
}

func importReq(text string, datastar bool) req {
	return req{method: http.MethodPost, form: url.Values{"definition": {text}}, datastar: datastar}
}

func TestDefinitionExportIsTheFixture(t *testing.T) {
	for _, c := range defCases() {
		t.Run(c.kind, func(t *testing.T) {
			serve, _ := c.build(t)
			rec := serve(c.admin(), req{target: c.base + "/" + c.id + "/export.json"})
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("content type %q", got)
			}
			code := strings.TrimPrefix(strings.SplitN(defFixture(t, c.kind), `"code": "`, 2)[1], "")
			code, _, _ = strings.Cut(code, `"`)
			if got, want := rec.Header().Get("Content-Disposition"), `attachment; filename="`+c.kind+"-"+code+`.json"`; got != want {
				t.Errorf("disposition %q, want %q", got, want)
			}
			if got, want := rec.Body.String(), defFixture(t, c.kind); got != want {
				t.Errorf("export differs from the fixture\n--- got\n%s\n--- want\n%s", got, want)
			}

			// The Datastar view shows the same JSON read-only with Copy and Download.
			view := serve(c.admin(), req{target: c.base + "/" + c.id + "/export", datastar: true}).Body.String()
			mustContain(t, view, `id="definition-export"`, "Copy", "Download", `href="`+c.base+"/"+c.id+`/export.json"`, `&#34;flowcatalyst&#34;: &#34;definition&#34;`)
			mustNotContain(t, view, "Not included", "not included")
		})
	}
}

func TestDefinitionImportFillsTheCreateDialog(t *testing.T) {
	for _, c := range defCases() {
		t.Run(c.kind, func(t *testing.T) {
			serve, created := c.build(t)
			for _, datastar := range []bool{true, false} {
				rec := serve(c.admin(), importReq(defFixture(t, c.kind), datastar).withPath(c.base+"/new/import"))
				if rec.Code != http.StatusOK {
					t.Fatalf("datastar=%v: status %d: %s", datastar, rec.Code, rec.Body)
				}
				body := rec.Body.String()
				mustContain(t, body, c.filled...)
				if len(c.notes) == 0 {
					mustContain(t, body, "Imported. Review and create.")
				} else {
					mustContain(t, body, append([]string{"Imported with notes."}, htmlEscaped(c.notes)...)...)
				}
				mustNotContain(t, body, "was not found here", "Ignored (unknown keys)")
			}
			if n := created(); n != 0 {
				t.Errorf("import created %d records", n)
			}
		})
	}
}

func (q req) withPath(p string) req { q.target = p; return q }

func htmlEscaped(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.NewReplacer("`", "`", "&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;").Replace(s)
	}
	return out
}

func TestDefinitionImportListsWhatItCannotResolve(t *testing.T) {
	for _, c := range defCases() {
		t.Run(c.kind, func(t *testing.T) {
			serve, created := c.build(t)
			for _, b := range c.broken {
				text := strings.Replace(defFixture(t, c.kind), b.old, b.new, 1)
				if text == defFixture(t, c.kind) {
					t.Fatalf("fixture lacks %s", b.old)
				}
				body := serve(c.admin(), importReq(text, true).withPath(c.base+"/new/import")).Body.String()
				mustContain(t, body, "Imported with notes.", b.note)
			}
			// Unknown keys are listed, not dropped.
			text := strings.Replace(defFixture(t, c.kind), `"code":`, `"zzUnknown": 1, "code":`, 1)
			mustContain(t, serve(c.admin(), importReq(text, true).withPath(c.base+"/new/import")).Body.String(), "Ignored (unknown keys): `zzUnknown`.")
			if n := created(); n != 0 {
				t.Errorf("import created %d records", n)
			}
		})
	}
}

func TestDefinitionImportRefusals(t *testing.T) {
	for _, c := range defCases() {
		t.Run(c.kind, func(t *testing.T) {
			serve, created := c.build(t)
			other := defKindConnection
			if c.kind == other {
				other = defKindDispatchPool
			}
			bad := map[string]string{
				"wrong kind":   defFixture(t, other),
				"bad version":  strings.Replace(defFixture(t, c.kind), `"version": 1`, `"version": 2`, 1),
				"invalid JSON": "{nope",
				"not ours":     `{"hello": "world"}`,
				"empty":        "  ",
			}
			want := map[string]string{
				"wrong kind":   "This file is a " + other + " definition; this form takes a " + c.kind + " definition.",
				"bad version":  "version is not supported",
				"invalid JSON": "This is not valid JSON.",
				"not ours":     "This is not a FlowCatalyst definition file.",
				"empty":        "Paste the definition JSON or choose a file.",
			}
			for name, text := range bad {
				// Datastar: only the import dialog's error region is patched.
				body := serve(c.admin(), importReq(text, true).withPath(c.base+"/new/import")).Body.String()
				mustContain(t, body, want[name], `id="`+c.importDialogID()+`-error"`)
				mustNotContain(t, body, "Imported")
				// A native post gets the page with the dialog open and the message.
				rec := serve(c.admin(), importReq(text, false).withPath(c.base+"/new/import"))
				mustContain(t, rec.Body.String(), want[name])
			}
			if n := created(); n != 0 {
				t.Errorf("a refused import created %d records", n)
			}
		})
	}
}

func (c defCase) importDialogID() string {
	return map[string]string{
		defKindApplication: appImportDialog, defKindServiceAccount: saImportDialog, defKindConnection: connImportDialog,
		defKindDispatchPool: poolImportDialog, defKindSubscription: subImportDialog,
	}[c.kind]
}

func TestDefinitionPermissions(t *testing.T) {
	for _, c := range defCases() {
		t.Run(c.kind, func(t *testing.T) {
			serve, created := c.build(t)
			text := defFixture(t, c.kind)

			// Import needs the create permission: a viewer is refused.
			viewer := c.user(c.viewPerm)
			if rec := serve(viewer, importReq(text, false).withPath(c.base+"/new/import")); rec.Code != http.StatusForbidden {
				t.Errorf("import without create: status %d", rec.Code)
			}
			// A creator who cannot view still imports (it is the create form's own reach).
			creator := c.user(c.createPerm)
			if rec := serve(creator, importReq(text, false).withPath(c.base+"/new/import")); rec.Code != http.StatusOK {
				t.Errorf("import with create: status %d: %s", rec.Code, rec.Body)
			}

			// Export needs the view permission, in both forms.
			for _, target := range []string{c.base + "/" + c.id + "/export", c.base + "/" + c.id + "/export.json"} {
				if rec := serve(creator, req{target: target}); rec.Code != http.StatusForbidden {
					t.Errorf("%s without view: status %d", target, rec.Code)
				}
				if rec := serve(viewer, req{target: target, datastar: true}); rec.Code != http.StatusOK {
					t.Errorf("%s with view: status %d: %s", target, rec.Code, rec.Body)
				}
			}
			// The drawer offers the export to a viewer.
			mustContain(t, serve(viewer, req{target: c.base + "/" + c.id, datastar: true}).Body.String(), "Export JSON", c.base+"/"+c.id+"/export")
			if n := created(); n != 0 {
				t.Errorf("created %d records", n)
			}
		})
	}
}

func TestDefinitionExportOmitsWhatTheViewerCannotRead(t *testing.T) {
	for _, tc := range []struct {
		kind, perm, key, want string
	}{
		{defKindConnection, "platform:messaging:connection:view", "serviceAccountCode", "Service account not included: you can't read service accounts."},
		{defKindSubscription, subViewPerm, "connectionCode", "Connection not included: you can't read connections."},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			var c defCase
			for _, k := range defCases() {
				if k.kind == tc.kind {
					c = k
				}
			}
			serve, _ := c.build(t)
			viewer := c.user(tc.perm)
			file := serve(viewer, req{target: c.base + "/" + c.id + "/export.json"})
			if file.Code != http.StatusOK {
				t.Fatalf("status %d", file.Code)
			}
			mustNotContain(t, file.Body.String(), `"`+tc.key+`"`, `"id"`, "sa_1", "con_1")
			mustContain(t, serve(viewer, req{target: c.base + "/" + c.id + "/export", datastar: true}).Body.String(), strings.ReplaceAll(tc.want, "'", "&#39;"))
		})
	}
}

func TestDefinitionExportNeverCarriesCredentials(t *testing.T) {
	h := newSAHarness()
	rec := serveMounted(t, saAdmin(), req{target: "/ui/service-accounts/sa_1/export.json"}, h.page())
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	mustNotContain(t, rec.Body.String(), saStoredToken, saStoredSecret, "oauth", "prn_", "principal", "clt_")
}

var _ = client.Client{}
