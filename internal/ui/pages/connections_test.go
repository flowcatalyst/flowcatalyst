package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	connops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

type fakeConns struct{ conns []connection.Connection }

func (f *fakeConns) FindWithFilters(_ context.Context, status, _ *string) ([]connection.Connection, error) {
	var out []connection.Connection
	for _, c := range f.conns {
		if status == nil || string(c.Status) == *status {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeConns) FindByID(_ context.Context, id string) (*connection.Connection, error) {
	for i := range f.conns {
		if f.conns[i].ID == id {
			c := f.conns[i]
			return &c, nil
		}
	}
	return nil, nil
}

type fakeConnOps struct {
	created []connops.CreateCommand
	updated []connops.UpdateCommand
	actions []string
	err     error
	conns   *fakeConns
}

func (o *fakeConnOps) Create(_ context.Context, cmd connops.CreateCommand) (string, error) {
	o.created = append(o.created, cmd)
	if o.err != nil {
		return "", o.err
	}
	o.conns.conns = append(o.conns.conns, conn("con_9", cmd.Code, cmd.Name, connection.StatusActive, ""))
	return "con_9", nil
}

func (o *fakeConnOps) Update(_ context.Context, cmd connops.UpdateCommand) error {
	o.updated = append(o.updated, cmd)
	return o.err
}

func (o *fakeConnOps) act(name, id string) error {
	o.actions = append(o.actions, name+":"+id)
	return o.err
}

func (o *fakeConnOps) Pause(_ context.Context, id string) error    { return o.act("pause", id) }
func (o *fakeConnOps) Activate(_ context.Context, id string) error { return o.act("activate", id) }
func (o *fakeConnOps) Delete(_ context.Context, id string) error   { return o.act("delete", id) }

type fakeSAs struct{}

func (fakeSAs) FindAll(context.Context) ([]serviceaccount.ServiceAccount, error) {
	return []serviceaccount.ServiceAccount{{ID: "sa_1", Code: "orders-svc", Name: "Orders <service>"}}, nil
}

func conn(id, code, name string, status connection.Status, client string) connection.Connection {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	c := connection.Connection{ID: id, Code: code, Name: name, Status: status, ServiceAccountID: "sa_1", Source: connection.SourceUI, CreatedAt: day, UpdatedAt: day}
	if client != "" {
		cid := ids.ClientID("clt_" + client)
		ident := client
		c.ClientID, c.ClientIdentifier = &cid, &ident
	}
	return c
}

type connHarness struct {
	conns *fakeConns
	ops   *fakeConnOps
}

func newConnHarness() *connHarness {
	desc, ext := "Primary <b>hook</b>", "ext-42"
	hooks := conn("con_1", "hooks", "Webhooks", connection.StatusActive, "")
	hooks.Description, hooks.ExternalID = &desc, &ext
	f := &fakeConns{conns: []connection.Connection{
		hooks,
		conn("con_2", "acme-hooks", "Acme hooks", connection.StatusPaused, "acme"),
		conn("con_3", "globex-hooks", "Globex hooks", connection.StatusActive, "globex"),
	}}
	return &connHarness{conns: f, ops: &fakeConnOps{conns: f}}
}

func (h *connHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

func (h *connHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	page := &Connections{Connections: h.conns, Ops: h.ops, Clients: someClients(), ServiceAccounts: fakeSAs{}, Shell: shellForTest()}
	return serveConnPage(t, page, ac, q)
}

func serveConnPage(t *testing.T, page *Connections, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	mux := chi.NewRouter()
	mux.Route("/ui", func(r chi.Router) { page.Mount(r) })
	method := q.method
	if method == "" {
		method = http.MethodGet
	}
	body := ""
	if q.form != nil {
		body = q.form.Encode()
	}
	r := httptest.NewRequest(method, q.target, strings.NewReader(body))
	if q.form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	r.Header.Set("Accept", "text/html")
	if q.datastar {
		r.Header.Set(ds.RequestHeader, "true")
	}
	r = r.WithContext(auth.WithContext(r.Context(), ac))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}

func connUser(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_c", Email: "c@x.y", Scope: auth.ScopeAnchor, Roles: []string{"r"}, Permissions: perms, PrincipalType: auth.PrincipalTypeUser}
}

func TestConnectionsList(t *testing.T) {
	h := newConnHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/connections"})
	mustContain(t, body,
		`<title>Connections · FlowCatalyst</title>`,
		`href="/ui/connections" class="fc-nav-item" title="Connections" aria-current="page"`,
		`<h1 class="fc-page-title">Connections</h1>`, "Manage webhook connections for event delivery",
		`href="/ui/connections/new"`, "Create Connection",
		`id="conns-table"`, `id="conns-summary"`, "3 connections",
		`Search connections...`, `<option value="PAUSED">Paused</option>`,
		`<dialog id="conn-activate"`, `<dialog id="conn-pause"`, `<dialog id="conn-delete"`,
		`<aside id="conn-drawer"`, `data-signals="{selConn: &#39;&#39;}"`,
	)
	mustContain(t, menuOfConn(t, body, "con_1"), "Pause…", `data-action="/ui/connections/con_1/pause"`, "Delete…")
	mustContain(t, menuOfConn(t, body, "con_2"), "Activate…", `data-action="/ui/connections/con_2/activate"`)
	mustNotContain(t, menuOfConn(t, body, "con_2"), "Pause…")

	filtered := h.do(t, anchorAdmin(), req{target: "/ui/connections?status=PAUSED&q=acme"})
	mustContain(t, filtered, "1 connection<")
	mustNotContain(t, filtered, `id="conn-con_1"`)
}

func menuOfConn(t *testing.T, body, id string) string {
	t.Helper()
	_, menu, ok := strings.Cut(body, `id="conn-`+id+`-actions-menu"`)
	if !ok {
		t.Fatalf("connection %s has no menu", id)
	}
	menu, _, _ = strings.Cut(menu, "</td>")
	return menu
}

func TestConnectionsPermissions(t *testing.T) {
	h := newConnHarness()
	if rec := h.rec(t, connUser("platform:messaging:event:view"), req{target: "/ui/connections"}); rec.Code != http.StatusForbidden {
		t.Errorf("no view permission: status %d", rec.Code)
	}
	viewer := h.do(t, connUser("platform:messaging:connection:view"), req{target: "/ui/connections"})
	mustNotContain(t, viewer, "Create Connection", `aria-label="Edit `, "Pause…", "Delete…", `<dialog id="conn-pause"`)
	updater := h.do(t, connUser("platform:messaging:connection:view", "platform:messaging:connection:update"), req{target: "/ui/connections"})
	mustContain(t, updater, `aria-label="Edit Webhooks"`, "Pause…")
	mustNotContain(t, updater, "Create Connection", "Delete…")
	creator := h.do(t, connUser("platform:messaging:connection:view", "platform:messaging:connection:create"), req{target: "/ui/connections"})
	mustContain(t, creator, "Create Connection")
	mustNotContain(t, creator, `aria-label="Edit `)

	h.do(t, connUser("platform:messaging:connection:view", "platform:messaging:connection:update"), req{method: http.MethodPost, target: "/ui/connections/con_1/delete", datastar: true, form: url.Values{}})
	if len(h.ops.actions) != 0 {
		t.Fatal("deleted without the delete permission")
	}
	scoped := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_acme"}, Roles: []string{"r"}, Permissions: []string{"platform:messaging:connection:view"}, PrincipalType: auth.PrincipalTypeUser}
	if rec := h.rec(t, scoped, req{target: "/ui/connections/con_3"}); rec.Code != http.StatusForbidden {
		t.Errorf("another client's connection: status %d", rec.Code)
	}
}

func TestConnectionsDrawer(t *testing.T) {
	h := newConnHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/connections/con_1", datastar: true})
	mustContain(t, body, "data: selector #conn-drawer\n", "Connection Details", "Primary &lt;b&gt;hook&lt;/b&gt;",
		"<code>ext-42</code>", "Service Account", "sa_1", "Anchor-level (no client)",
		"Pause Connection", "Delete Connection", `"selConn":"con_1"`, `href="/ui/connections/con_1/edit"`)
	mustNotContain(t, body, "Activate Connection")
	paused := h.do(t, anchorAdmin(), req{target: "/ui/connections/con_2", datastar: true})
	mustContain(t, paused, "Activate Connection", ">acme<")
}

func connCreateForm(kv ...string) url.Values {
	v := url.Values{"code": {"orders-hook"}, "name": {"Orders hook"}, "description": {""}, "externalId": {""}, "serviceAccountId": {"sa_1"}, "client": {""}}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return v
}

func TestConnectionCreate(t *testing.T) {
	t.Run("the dialog lists service accounts and clients", func(t *testing.T) {
		h := newConnHarness()
		body := h.do(t, anchorAdmin(), req{target: "/ui/connections/new", datastar: true})
		mustContain(t, body, "data: mode replace\n", `data: elements <div id="conn-dialog-slot"><dialog id="conn-form"`, "Create Connection", `<option value="">Select a service account</option>`,
			`<option value="sa_1">Orders &lt;service&gt; (orders-svc)</option>`, `<option value="acme">Acme Corp</option>`,
			"Anchor-level (leave empty) or select a client")
	})
	t.Run("without service-account view permission the select is disabled and says why", func(t *testing.T) {
		h := newConnHarness()
		body := h.do(t, connUser("platform:messaging:connection:create"), req{target: "/ui/connections/new", datastar: true})
		mustContain(t, body, "Choosing a service account needs permission to view service accounts.")
		mustNotContain(t, body, "orders-svc")
	})
	t.Run("success: the SPA's POST body, dialog closed, drawer opened, flash", func(t *testing.T) {
		h := newConnHarness()
		body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/connections/new", datastar: true, form: connCreateForm("externalId", "x-1", "client", "acme")})
		if len(h.ops.created) != 1 {
			t.Fatalf("creates %d", len(h.ops.created))
		}
		cmd := h.ops.created[0]
		if cmd.Code != "orders-hook" || cmd.ServiceAccountID != "sa_1" || deref(cmd.ExternalID) != "x-1" || cmd.Description != nil || deref(cmd.ClientID) != "clt_acme" || cmd.ApplicationCode != nil {
			t.Errorf("command = %+v", cmd)
		}
		mustContain(t, body, `document.getElementById(&#34;conn-form&#34;)?.close()`, `data: elements <div id="conns-table"`, `"selConn":"con_9"`, "Connection created")
	})
	t.Run("use-case errors go to their fields", func(t *testing.T) {
		h := newConnHarness()
		h.ops.err = usecase.Validation("SERVICE_ACCOUNT_REQUIRED", "serviceAccountId is required")
		body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/connections/new", datastar: true, form: connCreateForm("serviceAccountId", "")})
		mustContain(t, body, `<div id="conn-form-service-account-id-err" role="alert" class="text-sm text-destructive">ServiceAccountId is required</div>`,
			`document.getElementById(&#34;conn-form-service-account-id&#34;)?.focus()`)
		h.ops.err = usecase.Conflict("CODE_EXISTS", "Connection with code 'orders-hook' already exists")
		body = h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/connections/new", datastar: true, form: connCreateForm()})
		mustContain(t, body, `id="conn-form-code-err"`, "already exists")
	})
}

func TestConnectionEditAndActions(t *testing.T) {
	h := newConnHarness()
	dlg := h.do(t, anchorAdmin(), req{target: "/ui/connections/con_1/edit", datastar: true})
	mustContain(t, dlg, "Edit Connection", "Webhooks (hooks)", `name="externalId" value="ext-42"`, `data-attr:value="$selConn"`)
	mustNotContain(t, dlg, `name="serviceAccountId"`, `name="code"`)

	body := h.do(t, anchorAdmin(), req{
		method: http.MethodPost, target: "/ui/connections/con_1/edit", datastar: true,
		form: url.Values{"name": {"Webhooks 2"}, "description": {""}, "externalId": {""}, "drawer": {"con_1"}},
	})
	cmd := h.ops.updated[0]
	if cmd.ID != "con_1" || cmd.Name != "Webhooks 2" || cmd.Description != nil || cmd.ExternalID != nil || cmd.Status != nil || cmd.ApplicationCode != nil {
		t.Errorf("update = %+v", cmd)
	}
	mustContain(t, body, "Connection updated", "data: selector #conn-drawer\n")

	body = h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/connections/con_1/pause", datastar: true, form: url.Values{"drawer": {"con_1"}}})
	mustContain(t, body, "Connection paused", "data: selector #conn-drawer\n")
	body = h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/connections/con_2/activate", datastar: true, form: url.Values{}})
	mustContain(t, body, "Connection activated")
	body = h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/connections/con_1/delete", datastar: true, form: url.Values{"drawer": {"con_1"}}})
	mustContain(t, body, "Connection deleted", `"selConn":""`)
	if strings.Join(h.ops.actions, ",") != "pause:con_1,activate:con_2,delete:con_1" {
		t.Errorf("actions = %v", h.ops.actions)
	}

	h.ops.err = usecase.Conflict("HAS_SUBSCRIPTIONS", "Cannot delete a connection that has subscriptions. Remove all subscriptions first.")
	refused := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/connections/con_1/delete", datastar: true, form: url.Values{}})
	mustContain(t, refused, "fc-flash-error", "Remove all subscriptions first.")
	mustNotContain(t, refused, "conns-table")

	h.ops.err = nil
	rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/connections/con_2/activate", form: url.Values{}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Set-Cookie"), "fc_ui_flash=connection-activated") {
		t.Errorf("native: status %d, cookie %q", rec.Code, rec.Header().Get("Set-Cookie"))
	}
}
