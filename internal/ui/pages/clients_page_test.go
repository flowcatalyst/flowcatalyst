package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

type fakeClientStore struct{ clients []client.Client }

func (f *fakeClientStore) FindAll(context.Context) ([]client.Client, error) {
	return append([]client.Client(nil), f.clients...), nil
}

func (f *fakeClientStore) FindByID(_ context.Context, id string) (*client.Client, error) {
	for i := range f.clients {
		if string(f.clients[i].ID) == id {
			c := f.clients[i]
			return &c, nil
		}
	}
	return nil, nil
}

type fakeConfigs map[string][]application.ClientConfig

func (f fakeConfigs) FindByClient(_ context.Context, id string) ([]application.ClientConfig, error) {
	return f[id], nil
}

type fakeClientOps struct {
	created []clientops.CreateCommand
	updated []clientops.UpdateCommand
	apps    [][]string
	actions []string
	err     error
	store   *fakeClientStore
}

func (o *fakeClientOps) Create(_ context.Context, cmd clientops.CreateCommand) (string, error) {
	o.created = append(o.created, cmd)
	if o.err != nil {
		return "", o.err
	}
	o.store.clients = append(o.store.clients, cli("clt_9", cmd.Identifier, cmd.Name, client.StatusActive))
	return "clt_9", nil
}

func (o *fakeClientOps) Update(_ context.Context, cmd clientops.UpdateCommand) error {
	o.updated = append(o.updated, cmd)
	return o.err
}

func (o *fakeClientOps) act(name string) error {
	o.actions = append(o.actions, name)
	return o.err
}

func (o *fakeClientOps) Activate(_ context.Context, id string) error { return o.act("activate:" + id) }

func (o *fakeClientOps) Suspend(_ context.Context, id, reason string) error {
	return o.act("suspend:" + id + ":" + reason)
}

func (o *fakeClientOps) Deactivate(_ context.Context, id, reason string) error {
	return o.act("deactivate:" + id + ":" + reason)
}

func (o *fakeClientOps) UpdateApplications(_ context.Context, id string, enabled []string) error {
	o.apps = append(o.apps, enabled)
	return o.act("apps:" + id)
}

func cli(id, ident, name string, status client.Status) client.Client {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	return client.Client{ID: ids.ClientID(id), Identifier: ident, Name: name, Status: status, CreatedAt: day, UpdatedAt: day}
}

type cliHarness struct {
	store *fakeClientStore
	ops   *fakeClientOps
}

func newCliHarness() *cliHarness {
	s := &fakeClientStore{clients: []client.Client{cli("clt_1", "acme", "Acme <Corp>", client.StatusActive), cli("clt_2", "globex", "Globex", client.StatusSuspended)}}
	return &cliHarness{store: s, ops: &fakeClientOps{store: s}}
}

func (h *cliHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	apps := newAppHarness().store
	page := &ClientsPage{Clients: h.store, Ops: h.ops, Apps: apps, Configs: fakeConfigs{"clt_1": {{ApplicationID: "app_1", Enabled: true}}}, Shell: shellForTest()}
	return serveMounted(t, ac, q, page)
}

func (h *cliHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

func TestClientsPage(t *testing.T) {
	h := newCliHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/clients"})
	mustContain(t, body, `<title>Clients · FlowCatalyst</title>`, "Create Client", "Acme &lt;Corp&gt;", "Showing 1 to 2 of 2 clients", `<dialog id="cli-deactivate"`)
	mustNotContain(t, body, "Filters")
	search := h.do(t, anchorAdmin(), req{target: "/ui/clients?q=glob", datastar: true})
	mustContain(t, search, "Globex")
	mustNotContain(t, search, "Acme &lt;Corp&gt;")

	drawer := h.do(t, anchorAdmin(), req{target: "/ui/clients/clt_1", datastar: true})
	mustContain(t, drawer, "Client Details", `<code id="cli-id-value">clt_1</code>`, "Available (1)", "Enabled (<span",
		`name="applicationIds" value="app_1" checked`, `name="applicationIds" value="app_2"`, ">Inactive<",
		`href="/clients/clt_1/theme"`, "Suspend Client", "Deactivate Client", `"selCli":"clt_1"`)
	mustNotContain(t, drawer, "Activate Client")
}

func TestClientsPermissions(t *testing.T) {
	h := newCliHarness()
	clientUser := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"r"}, Permissions: []string{"platform:admin:client:view"}, PrincipalType: auth.PrincipalTypeUser}
	if rec := h.rec(t, clientUser, req{target: "/ui/clients"}); rec.Code != http.StatusForbidden {
		t.Errorf("a client user: %d", rec.Code)
	}
	viewer := connUser("platform:admin:client:view")
	body := h.do(t, viewer, req{target: "/ui/clients/clt_1", datastar: true})
	mustNotContain(t, body, "Create Client", "Suspend Client", "Deactivate Client", ">Edit<")
	mustContain(t, body, `name="applicationIds"`) // anchor: the picker (PUT .../applications is anchor-only)
	h.do(t, viewer, req{method: http.MethodPost, target: "/ui/clients/clt_1/suspend", datastar: true, form: url.Values{}})
	if len(h.ops.actions) != 0 {
		t.Fatal("suspended without the permission")
	}
}

func TestClientsActions(t *testing.T) {
	h := newCliHarness()
	for _, a := range []string{"clt_2/activate", "clt_1/suspend", "clt_1/deactivate"} {
		out := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/clients/" + a, datastar: true, form: url.Values{"drawer": {"clt_1"}}})
		mustContain(t, out, "fc-flash-success")
	}
	out := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/clients/clt_1/applications", datastar: true, multipart: true, form: url.Values{"applicationIds": {"app_1", "app_2"}, "drawer": {"clt_1"}}})
	mustContain(t, out, "Applications updated", "data: selector #cli-drawer\n")
	if got := strings.Join(h.ops.actions, ","); got != "activate:clt_2,suspend:clt_1:Manual suspension,deactivate:clt_1:Manual deactivation,apps:clt_1" {
		t.Errorf("actions = %s", got)
	}
	if strings.Join(h.ops.apps[0], ",") != "app_1,app_2" {
		t.Errorf("apps = %v", h.ops.apps)
	}

	dlg := h.do(t, anchorAdmin(), req{target: "/ui/clients/new", datastar: true})
	mustContain(t, dlg, "Create Client", `name="identifier"`, "data: mode replace\n", `data: elements <div id="cli-dialog-slot">`)
	created := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/clients/new", datastar: true, form: url.Values{"name": {"Initech"}, "identifier": {"initech"}}})
	mustContain(t, created, `"selCli":"clt_9"`, "Client created")
	h.ops.err = usecase.Conflict("IDENTIFIER_EXISTS", "Client with identifier 'initech' already exists")
	dup := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/clients/new", datastar: true, form: url.Values{"name": {"Initech"}, "identifier": {"initech"}}})
	mustContain(t, dup, `id="cli-form-identifier-err"`)
	h.ops.err = nil
	h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/clients/clt_1/edit", datastar: true, form: url.Values{"name": {"Acme 2"}}})
	if u := h.ops.updated[0]; u.ID != "clt_1" || *u.Name != "Acme 2" {
		t.Errorf("update = %+v", u)
	}
}
