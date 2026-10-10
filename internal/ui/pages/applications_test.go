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
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appdocs"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

type fakeAppStore struct{ apps []application.Application }

func (f *fakeAppStore) FindWithFilters(context.Context, *string, *string) ([]application.Application, error) {
	return append([]application.Application(nil), f.apps...), nil
}

func (f *fakeAppStore) FindByID(_ context.Context, id string) (*application.Application, error) {
	for i := range f.apps {
		if string(f.apps[i].ID) == id {
			a := f.apps[i]
			return &a, nil
		}
	}
	return nil, nil
}

type fakeLogins map[string]bool

func (f fakeLogins) HasLoginClientForApplication(_ context.Context, id string) (bool, error) {
	return f[id], nil
}

type fakeDocs struct{}

func (fakeDocs) ListByApplication(_ context.Context, id string) ([]appdocs.Summary, error) {
	if id == "app_1" {
		return []appdocs.Summary{{Slug: "intro", Title: "Getting <started>"}}, nil
	}
	return nil, nil
}

type fakeAppOps struct {
	created []appops.CreateCommand
	updated []appops.UpdateCommand
	logins  []appops.ProvisionLoginClientCommand
	actions []string
	err     error
	store   *fakeAppStore
}

func (o *fakeAppOps) Create(_ context.Context, cmd appops.CreateCommand) (string, error) {
	o.created = append(o.created, cmd)
	if o.err != nil {
		return "", o.err
	}
	o.store.apps = append(o.store.apps, app("app_9", cmd.Code, cmd.Name, true))
	return "app_9", nil
}

func (o *fakeAppOps) Update(_ context.Context, cmd appops.UpdateCommand) error {
	o.updated = append(o.updated, cmd)
	return o.err
}

func (o *fakeAppOps) act(name, id string) error {
	o.actions = append(o.actions, name+":"+id)
	return o.err
}

func (o *fakeAppOps) Activate(_ context.Context, id string) error   { return o.act("activate", id) }
func (o *fakeAppOps) Deactivate(_ context.Context, id string) error { return o.act("deactivate", id) }
func (o *fakeAppOps) Delete(_ context.Context, id string) error     { return o.act("delete", id) }

func (o *fakeAppOps) ProvisionServiceAccount(_ context.Context, id string) (appops.ProvisionServiceAccountResult, error) {
	if err := o.act("provision-sa", id); err != nil {
		return appops.ProvisionServiceAccountResult{}, err
	}
	return appops.ProvisionServiceAccountResult{ServiceAccountName: "Orders service", OAuthClientID: "oauth_client_1", OAuthClientSecret: "s3cr3t-value"}, nil
}

func (o *fakeAppOps) ProvisionLoginClient(_ context.Context, cmd appops.ProvisionLoginClientCommand) (appops.ProvisionLoginClientResult, error) {
	o.logins = append(o.logins, cmd)
	if o.err != nil {
		return appops.ProvisionLoginClientResult{}, o.err
	}
	return appops.ProvisionLoginClientResult{ClientType: "PUBLIC", RedirectURIs: cmd.RedirectURIs, ClientID: "login_client_1"}, nil
}

func app(id, code, name string, active bool) application.Application {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	return application.Application{ID: ids.ApplicationID(id), Code: code, Name: name, Type: application.TypeApplication, Active: active, CreatedAt: day, UpdatedAt: day}
}

type appHarness struct {
	store *fakeAppStore
	ops   *fakeAppOps
}

func newAppHarness() *appHarness {
	desc := "Order <management>"
	orders := app("app_1", "orders", "Orders", true)
	orders.Description = &desc
	sa := "sac_1"
	orders.ServiceAccountID = &sa
	billing := app("app_2", "billing", "Billing", false)
	billing.Type = application.TypeIntegration
	s := &fakeAppStore{apps: []application.Application{orders, billing}}
	return &appHarness{store: s, ops: &fakeAppOps{store: s}}
}

func (h *appHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	page := &Applications{Apps: h.store, Ops: h.ops, Logins: fakeLogins{"app_1": true}, Docs: fakeDocs{}, Shell: shellForTest()}
	return serveMounted(t, ac, q, page)
}

func (h *appHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

const appViewPerm = "platform:admin:application:view"

func TestApplicationsList(t *testing.T) {
	h := newAppHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/applications"})
	mustContain(t, body, `<title>Applications · FlowCatalyst</title>`, "Create Application", "Order &lt;management&gt;",
		">Integration<", ">Inactive<", "Showing 1 to 2 of 2 applications", `<dialog id="app-delete"`)
	typed := h.do(t, anchorAdmin(), req{target: "/ui/applications?type=INTEGRATION&q=bill", datastar: true})
	mustContain(t, typed, "Billing", "replaceState", "/ui/applications?q=bill", "type=INTEGRATION")
	mustNotContain(t, typed, "Order &lt;management&gt;")
	bogus := h.do(t, anchorAdmin(), req{target: "/ui/applications?type=NOPE"})
	mustContain(t, bogus, "Showing 1 to 2 of 2 applications")
}

func TestApplicationsPermissions(t *testing.T) {
	h := newAppHarness()
	if rec := h.rec(t, connUser("platform:messaging:event:view"), req{target: "/ui/applications"}); rec.Code != http.StatusForbidden {
		t.Errorf("no view: %d", rec.Code)
	}
	viewer := h.do(t, connUser(appViewPerm), req{target: "/ui/applications"})
	mustNotContain(t, viewer, "Create Application", `aria-label="Edit `, `<dialog id="app-delete"`)
	deleter := connUser(appViewPerm, "platform:admin:application:delete")
	h.do(t, deleter, req{method: http.MethodPost, target: "/ui/applications/app_2/activate", datastar: true, form: url.Values{}})
	if len(h.ops.actions) != 0 {
		t.Fatal("a delete-only user activated an application")
	}
	client := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_a"}, Roles: []string{"r"}, Permissions: []string{appViewPerm, "platform:admin:application:update"}, PrincipalType: auth.PrincipalTypeUser}
	drawer := h.do(t, client, req{target: "/ui/applications/app_2", datastar: true})
	mustContain(t, drawer, "Provisioning one needs the anchor scope.")
	mustNotContain(t, drawer, "Provision Login Client")
	h.do(t, client, req{method: http.MethodPost, target: "/ui/applications/app_2/provision-service-account", datastar: true, form: url.Values{}})
	if len(h.ops.actions) != 0 {
		t.Fatal("a client user provisioned a service account")
	}
}

func TestApplicationDrawer(t *testing.T) {
	h := newAppHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/applications/app_1", datastar: true})
	mustContain(t, body, "Application Details", "<code>sac_1</code>", "Provisioned", "Getting &lt;started&gt;",
		`href="/platform/docs/orders/intro"`, "Deactivate Application", "Deactivate the application first.", `"selApp":"app_1"`)
	mustNotContain(t, body, "Provision Login Client", `data-action="/ui/applications/app_1/delete"`)
	inactive := h.do(t, anchorAdmin(), req{target: "/ui/applications/app_2", datastar: true})
	mustContain(t, inactive, "Provision Service Account", "Provision Login Client", `data-action="/ui/applications/app_2/delete"`,
		"Activate Application", "/docs/sync")
}

func TestApplicationCreateAndEdit(t *testing.T) {
	h := newAppHarness()
	dlg := h.do(t, anchorAdmin(), req{target: "/ui/applications/new", datastar: true})
	mustContain(t, dlg, "Create Application", `type="radio" name="type" value="APPLICATION" class="sr-only" checked`, "Logo MIME Type")
	body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/applications/new", datastar: true, form: url.Values{
		"type": {"INTEGRATION"}, "code": {"shipping"}, "name": {"Shipping"}, "description": {""}, "defaultBaseUrl": {"https://ship.example.com"},
		"iconUrl": {""}, "website": {""}, "logo": {""}, "logoMimeType": {""},
	}})
	c := h.ops.created[0]
	if c.Code != "shipping" || c.Type != "INTEGRATION" || deref(c.DefaultBaseURL) != "https://ship.example.com" || c.Description != nil || c.Logo != nil {
		t.Errorf("create = %+v", c)
	}
	mustContain(t, body, `"selApp":"app_9"`, "Application created")
	h.ops.err = usecase.Conflict("CODE_EXISTS", "Application with code 'shipping' already exists")
	dup := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/applications/new", datastar: true, form: url.Values{"type": {"APPLICATION"}, "code": {"shipping"}, "name": {"S"}}})
	mustContain(t, dup, `id="app-form-code-err"`, "already exists")

	h.ops.err = nil
	edit := h.do(t, anchorAdmin(), req{target: "/ui/applications/app_1/edit", datastar: true})
	mustContain(t, edit, "Edit Application", "Orders (orders)")
	mustNotContain(t, edit, `name="code"`)
	h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/applications/app_1/edit", datastar: true, form: url.Values{"name": {"Orders 2"}, "description": {""}, "website": {"https://o.example.com"}}})
	if u := h.ops.updated[0]; *u.Name != "Orders 2" || u.Description != nil || deref(u.Website) != "https://o.example.com" {
		t.Errorf("update = %+v", u)
	}
}

func TestApplicationActionsAndProvisioning(t *testing.T) {
	h := newAppHarness()
	for _, a := range []string{"app_2/activate", "app_1/deactivate", "app_2/delete"} {
		out := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/applications/" + a, datastar: true, form: url.Values{}})
		mustContain(t, out, "fc-flash-success")
	}
	h.ops.err = usecase.Conflict("APPLICATION_HAS_REFERENCES", "Cannot delete application 'billing' — 2 access grants still reference it.")
	mustContain(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/applications/app_2/delete", datastar: true, form: url.Values{}}), "fc-flash-error", "access grants")
	h.ops.err = nil

	sa := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/applications/app_2/provision-service-account", datastar: true, form: url.Values{"drawer": {"app_2"}}})
	mustContain(t, sa, "data: mode replace\n", `data: elements <div id="app-dialog-slot">`, "Service Account Provisioned", "s3cr3t-value", "oauth_client_1", "Orders service", "will not be shown again")
	native := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/applications/app_2/provision-service-account", form: url.Values{}})
	if native.Code != http.StatusOK || !strings.Contains(native.Body.String(), "s3cr3t-value") || native.Header().Get("Location") != "" {
		t.Errorf("native provisioning: %d", native.Code)
	}

	empty := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/applications/app_2/provision-login-client", datastar: true, form: url.Values{"clientType": {"PUBLIC"}, "redirectUris": {"  \n "}}})
	mustContain(t, empty, `id="app-login-redirect-uris-err"`, "At least one redirect URI is required")
	if len(h.ops.logins) != 0 {
		t.Fatal("provisioned without redirect URIs")
	}
	login := h.do(t, anchorAdmin(), req{
		method: http.MethodPost, target: "/ui/applications/app_2/provision-login-client", datastar: true,
		form: url.Values{"clientType": {"PUBLIC"}, "redirectUris": {"https://a.example.com/cb\nhttps://a.example.com/cb\n https://b.example.com/cb "}},
	})
	if l := h.ops.logins[0]; strings.Join(l.RedirectURIs, ",") != "https://a.example.com/cb,https://b.example.com/cb" || l.ClientType != "PUBLIC" {
		t.Errorf("login = %+v", l)
	}
	mustContain(t, login, "Login Client Provisioned", "login_client_1", "PUBLIC clients use PKCE")
	mustNotContain(t, login, "Client Secret")
}
