package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	saops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// The webhook secrets the fixtures hold: they must never be rendered.
const (
	saStoredToken  = "fc_stored_token_do_not_render"
	saStoredSecret = "stored_signing_secret_do_not_render"
)

type fakeSAStore struct {
	accounts []serviceaccount.ServiceAccount
}

func (f *fakeSAStore) FindAll(context.Context) ([]serviceaccount.ServiceAccount, error) {
	return append([]serviceaccount.ServiceAccount(nil), f.accounts...), nil
}

func (f *fakeSAStore) FindByID(_ context.Context, id string) (*serviceaccount.ServiceAccount, error) {
	for i := range f.accounts {
		if f.accounts[i].ID == id {
			a := f.accounts[i]
			return &a, nil
		}
	}
	return nil, nil
}

type fakeSAPrincipals map[string]*principal.Principal

func (f fakeSAPrincipals) FindByServiceAccount(_ context.Context, id string) (*principal.Principal, error) {
	return f[id], nil
}

type fakeSAOAuth struct{}

func (fakeSAOAuth) FindByPrincipalID(_ context.Context, id string) ([]platformauth.OAuthClient, error) {
	if id == "prn_sa1" {
		return []platformauth.OAuthClient{{ClientID: "oauth_orders_public"}}, nil
	}
	return nil, nil
}

type fakeSARoles []role.Role

func (f fakeSARoles) FindAll(context.Context) ([]role.Role, error) { return []role.Role(f), nil }

type appAccessCall struct {
	principalID string
	apps        []string
	all         *bool
}

type fakeSAOps struct {
	store     *fakeSAStore
	created   []saops.CreateCommand
	updated   []saops.UpdateCommand
	roles     [][]string
	actions   []string
	appAccess []appAccessCall
	err       error
}

func (o *fakeSAOps) act(name, id string) error {
	o.actions = append(o.actions, name+":"+id)
	return o.err
}

func (o *fakeSAOps) Create(_ context.Context, cmd saops.CreateCommand) (saops.CreateWithCredentialsResult, error) {
	o.created = append(o.created, cmd)
	if o.err != nil {
		return saops.CreateWithCredentialsResult{}, o.err
	}
	sa := saFixture("sa_9", cmd.Code, cmd.Name, true)
	o.store.accounts = append(o.store.accounts, sa)
	return saops.CreateWithCredentialsResult{
		ServiceAccount: &sa, PrincipalID: "prn_sa9", OAuthClientID: "oauth_new_client", OAuthClientSecret: "new-oauth-secret",
		AuthToken: "fc_new_auth_token", SigningSecret: "new-signing-secret",
	}, nil
}

func (o *fakeSAOps) Update(_ context.Context, cmd saops.UpdateCommand) error {
	o.updated = append(o.updated, cmd)
	return o.err
}

func (o *fakeSAOps) Delete(_ context.Context, id string) error { return o.act("delete", id) }

func (o *fakeSAOps) AssignRoles(_ context.Context, id string, roles []string) error {
	o.roles = append(o.roles, roles)
	return o.act("roles", id)
}

func (o *fakeSAOps) RegenerateToken(_ context.Context, id string) (string, error) {
	return "fc_regenerated_token", o.act("regen-token", id)
}

func (o *fakeSAOps) RegenerateSecret(_ context.Context, id string) (string, error) {
	return "regenerated-signing-secret", o.act("regen-secret", id)
}

func (o *fakeSAOps) MintToken(_ context.Context, id string) (saops.MintedToken, error) {
	return saops.MintedToken{AccessToken: "minted.bearer.token", ExpiresIn: 3600}, o.act("mint", id)
}

func (o *fakeSAOps) SetApplicationAccess(_ context.Context, principalID string, apps []string, all *bool) error {
	o.appAccess = append(o.appAccess, appAccessCall{principalID, apps, all})
	return o.err
}

func saFixture(id, code, name string, active bool) serviceaccount.ServiceAccount {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	token, secret := saStoredToken, saStoredSecret
	return serviceaccount.ServiceAccount{
		ID: id, Code: code, Name: name, Active: active, ClientIDs: []string{}, CreatedAt: day, UpdatedAt: day,
		WebhookCredentials: serviceaccount.WebhookCredentials{AuthType: serviceaccount.AuthBearer, Token: &token, SigningSecret: &secret},
	}
}

func saRoleName(s string) role.Name {
	n, err := role.ParseName(s)
	if err != nil {
		panic(err)
	}
	return n
}

type saHarness struct {
	store      *fakeSAStore
	ops        *fakeSAOps
	principals fakeSAPrincipals
	conns      *fakeConns
	connOps    *fakeConnOps
}

func newSAHarness() *saHarness {
	desc := "Order <service>"
	scope, anchorScope := "CLIENT", "ANCHOR"
	used := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	orders := saFixture("sa_1", "orders-svc", "Orders <service>", true)
	orders.Description, orders.Scope, orders.ClientIDs, orders.LastUsedAt = &desc, &scope, []string{"clt_1"}, &used
	billing := saFixture("sa_2", "billing-svc", "Billing service", false)
	billing.Scope = &anchorScope
	app := ids.ApplicationID("app_1")
	bound := saFixture("sa_3", "app-bound", "App bound", true)
	bound.ApplicationID = &app
	store := &fakeSAStore{accounts: []serviceaccount.ServiceAccount{orders, billing, bound}}

	assigned := "ADMIN_ASSIGNED"
	day := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	p := principal.NewService("sa_1", "Orders")
	p.ID = "prn_sa1"
	p.AllApplications = false
	p.AccessibleApplicationIDs = []ids.ApplicationID{"app_1"}
	p.Roles = []serviceaccount.RoleAssignment{
		{Role: saRoleName("orders:reader"), AssignmentSource: &assigned, AssignedAt: day},
		{Role: saRoleName("platform:ops"), AssignmentSource: &assigned, AssignedAt: day},
	}
	p2 := principal.NewService("sa_2", "Billing")
	p2.ID = "prn_sa2"
	p3 := principal.NewService("sa_3", "App bound")
	p3.ID = "prn_sa3"
	p3.AllApplications = false

	conns := &fakeConns{conns: []connection.Connection{
		conn("con_1", "hooks", "Webhooks", connection.StatusActive, ""),
		conn("con_2", "paused-hooks", "Paused hooks", connection.StatusPaused, ""),
	}}
	other := conn("con_3", "other-hooks", "Other hooks", connection.StatusActive, "")
	other.ServiceAccountID = "sa_2"
	conns.conns = append(conns.conns, other)
	return &saHarness{
		store: store, ops: &fakeSAOps{store: store}, conns: conns, connOps: &fakeConnOps{conns: conns},
		principals: fakeSAPrincipals{"sa_1": p, "sa_2": p2, "sa_3": p3},
	}
}

func (h *saHarness) page() *ServiceAccounts {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	return &ServiceAccounts{
		Accounts: h.store, Ops: h.ops, Principals: h.principals, OAuthClients: fakeSAOAuth{},
		Roles: fakeSARoles{
			{Name: "orders:reader", DisplayName: "Orders reader", Permissions: []string{"orders:order:view"}, CreatedAt: day},
			{Name: "platform:ops", DisplayName: "Operations", Permissions: []string{"platform:messaging:event:view"}},
			{Name: "platform:admin", DisplayName: "Administrator", Permissions: []string{"platform:admin:client:update"}},
		},
		Clients: &fakeClientStore{clients: []client.Client{cli("clt_1", "acme", "Acme <Corp>", client.StatusActive), cli("clt_2", "globex", "Globex", client.StatusActive)}},
		Apps:    newAppHarness().store, Connections: h.conns, ConnectionOps: h.connOps, Shell: shellForTest(),
	}
}

func (h *saHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, h.page())
}

func (h *saHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

// saAdmin holds every permission, anchor scope and all-applications access.
func saAdmin() *auth.AuthContext {
	ac := anchorAdmin()
	ac.AllApplications = true
	return ac
}

// saUser is an anchor caller holding only perms.
func saUser(perms ...string) *auth.AuthContext {
	ac := connUser(perms...)
	ac.AllApplications = true
	return ac
}

const (
	saPermView   = "platform:iam:service-account:view"
	saPermCreate = "platform:iam:service-account:create"
	saPermUpdate = "platform:iam:service-account:update"
	saPermDelete = "platform:iam:service-account:delete"
)

func TestServiceAccountsList(t *testing.T) {
	h := newSAHarness()
	body := h.do(t, saAdmin(), req{target: "/ui/service-accounts"})
	mustContain(t, body, `<title>Service Accounts · FlowCatalyst</title>`, "Manage service accounts and webhook credentials", "Add Service Account",
		"Orders &lt;service&gt;", `<code class="font-mono text-[13px]">orders-svc</code>`, ">BEARER_TOKEN<", ">Inactive<",
		"Acme &lt;Corp&gt;", ">All<", "Showing 1 to 3 of 3 service accounts", `href="/ui/service-accounts/new"`,
		`<dialog id="sa-regen-token"`, `<dialog id="sa-delete"`, `data-signals="{selSa: &#39;&#39;}"`)
	mustContain(t, body, `href="/ui/service-accounts" class="fc-nav-item" title="Service Accounts" aria-current="page"`)
	mustNotContain(t, body, saStoredToken, saStoredSecret)

	filtered := h.do(t, saAdmin(), req{target: "/ui/service-accounts?q=bill&status=INACTIVE", datastar: true})
	mustContain(t, filtered, "Billing service", "replaceState", "q=bill", "status=INACTIVE")
	mustNotContain(t, filtered, "Orders &lt;service&gt;")
	byClient := h.do(t, saAdmin(), req{target: "/ui/service-accounts?client=clt_1", datastar: true})
	mustContain(t, byClient, "Orders &lt;service&gt;", "client=clt_1")
	mustNotContain(t, byClient, "Billing service")
	bogus := h.do(t, saAdmin(), req{target: "/ui/service-accounts?client=clt_nope&status=NOPE"})
	mustContain(t, bogus, "Showing 1 to 3 of 3 service accounts")
	sorted := h.do(t, saAdmin(), req{target: "/ui/service-accounts?sort=code&dir=desc", datastar: true})
	if strings.Index(sorted, "orders-svc") > strings.Index(sorted, "billing-svc") {
		t.Error("sort=code&dir=desc: orders-svc must come before billing-svc")
	}
}

func TestServiceAccountsListWithoutClientNames(t *testing.T) {
	h := newSAHarness()
	// Listing clients is anchor with client:view: without it the column shows
	// ids and the Client filter is left out.
	body := h.do(t, saUser(saPermView), req{target: "/ui/service-accounts"})
	mustContain(t, body, "clt_1")
	mustNotContain(t, body, "Acme &lt;Corp&gt;", `id="filter-client"`, "Add Service Account", `aria-label="Edit `, `<dialog id="sa-delete"`, `<dialog id="sa-regen-token"`)
}

// TestServiceAccountsPermissionMatrix pins the gates of the SPA's API: every
// control is hidden without its permission and every route refuses a forged
// post, before the use case runs and without a secret in the answer.
func TestServiceAccountsPermissionMatrix(t *testing.T) {
	forged := []struct {
		name, target string
		form         url.Values
	}{
		{"create", "/ui/service-accounts/new", url.Values{"name": {"X"}, "code": {"x"}, "scope": {"ANCHOR"}}},
		{"edit", "/ui/service-accounts/sa_1/edit", url.Values{"name": {"Hacked"}, "scope": {"ANCHOR"}}},
		{"roles", "/ui/service-accounts/sa_1/roles", url.Values{"roles": {"platform:ops"}}},
		{"regenerate token", "/ui/service-accounts/sa_1/regenerate-token", url.Values{}},
		{"regenerate secret", "/ui/service-accounts/sa_1/regenerate-secret", url.Values{}},
		{"mint", "/ui/service-accounts/sa_1/token", url.Values{}},
		{"delete", "/ui/service-accounts/sa_1/delete", url.Values{}},
		{"applications", "/ui/service-accounts/sa_1/applications", url.Values{"applicationIds": {"app_1"}}},
		{"all applications", "/ui/service-accounts/sa_1/all-applications", url.Values{"allApplications": {"true"}}},
	}
	callers := []struct {
		name string
		ac   *auth.AuthContext
	}{
		{"viewer", saUser(saPermView, "platform:iam:user:view", "platform:iam:role:view")},
		// Holding the permission is not enough without anchor scope (owner decision #19).
		{"non-anchor with every permission", &auth.AuthContext{
			PrincipalID: "prn_c", Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"r"}, AllApplications: true,
			Permissions: []string{saPermView, saPermCreate, saPermUpdate, saPermDelete, "platform:iam:user:update", "platform:iam:role:view"}, PrincipalType: auth.PrincipalTypeUser,
		}},
		// An application-scoped caller reaches no platform-wide account.
		{"application-scoped", &auth.AuthContext{
			PrincipalID: "prn_app", Scope: auth.ScopeAnchor, Roles: []string{"r"}, Applications: []string{"app_9"},
			Permissions: []string{"platform:*:*:*"}, PrincipalType: "SERVICE",
		}},
	}
	for _, c := range callers {
		t.Run(c.name, func(t *testing.T) {
			h := newSAHarness()
			for _, f := range forged {
				body := h.do(t, c.ac, req{method: http.MethodPost, target: f.target, datastar: true, form: f.form})
				mustNotContain(t, body, "fc_regenerated_token", "regenerated-signing-secret", "minted.bearer.token", "new-oauth-secret", "fc_new_auth_token", saStoredToken, saStoredSecret)
				if len(h.ops.created)+len(h.ops.updated)+len(h.ops.roles)+len(h.ops.actions)+len(h.ops.appAccess) != 0 {
					t.Fatalf("%s: a forged post by the %s reached the use case: %+v", f.name, c.name, h.ops)
				}
			}
			// The same through a native form post.
			rec := h.rec(t, c.ac, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/regenerate-secret", form: url.Values{}})
			if rec.Code < 400 || len(h.ops.actions) != 0 {
				t.Errorf("native forged post: status %d, actions %v", rec.Code, h.ops.actions)
			}
		})
	}

	// The controls follow the gates.
	h := newSAHarness()
	viewer := h.do(t, saUser(saPermView), req{target: "/ui/service-accounts/sa_1", datastar: true})
	mustNotContain(t, viewer, "Regenerate Token", "Generate Token", "Manage Roles", `aria-label="Edit`, ">Delete<", "Danger Zone")
	mustContain(t, viewer, "Regenerating credentials needs the service-account update permission and anchor scope.")
	// Update without delete.
	updater := h.do(t, saUser(saPermView, saPermUpdate, "platform:iam:role:view"), req{target: "/ui/service-accounts/sa_1", datastar: true})
	mustContain(t, updater, "Regenerate Token", "Regenerate Secret", "Generate Token", "Manage Roles", `href="/ui/service-accounts/sa_1/edit"`)
	mustNotContain(t, updater, ">Delete<", "Danger Zone")
	// Update without the role list cannot manage roles.
	noRoles := h.do(t, saUser(saPermView, saPermUpdate), req{target: "/ui/service-accounts/sa_1", datastar: true})
	mustNotContain(t, noRoles, "Manage Roles")
	mustContain(t, noRoles, "Changing roles needs permission to view roles.")
	// The application-scoped caller sees no write control.
	scoped := h.do(t, callers[2].ac, req{target: "/ui/service-accounts/sa_1", datastar: true})
	mustNotContain(t, scoped, "Regenerate Token", "Generate Token", "Danger Zone")
	// An application-scoped caller bound to the account's application reaches it.
	bound := *callers[2].ac
	bound.Applications = []string{"app_1"}
	reach := h.do(t, &bound, req{target: "/ui/service-accounts/sa_3", datastar: true})
	mustContain(t, reach, "Regenerate Token", "Danger Zone")
	other := h.do(t, &bound, req{target: "/ui/service-accounts/sa_1", datastar: true})
	mustNotContain(t, other, "Regenerate Token")
	// Create is offered with the create permission and anchor scope only.
	if got := h.do(t, saUser(saPermView, saPermCreate), req{target: "/ui/service-accounts"}); !strings.Contains(got, "Add Service Account") {
		t.Error("no Add Service Account for a creator")
	}
	if rec := h.rec(t, saUser(saPermView), req{target: "/ui/service-accounts/new"}); rec.Code != http.StatusForbidden {
		t.Errorf("create dialog without the permission: %d", rec.Code)
	}
	if rec := h.rec(t, saUser(saPermView), req{target: "/ui/service-accounts/sa_1/edit"}); rec.Code != http.StatusForbidden {
		t.Errorf("edit dialog without the permission: %d", rec.Code)
	}
	if rec := h.rec(t, saUser("platform:messaging:event:view"), req{target: "/ui/service-accounts"}); rec.Code != http.StatusForbidden {
		t.Errorf("list without view: %d", rec.Code)
	}
}

func TestServiceAccountDrawer(t *testing.T) {
	h := newSAHarness()
	body := h.do(t, saAdmin(), req{target: "/ui/service-accounts/sa_1", datastar: true})
	mustContain(t, body, "Service Account Information", "Orders &lt;service&gt;", "<code>orders-svc</code>", "<code>prn_sa1</code>", "<code>oauth_orders_public</code>",
		"Order &lt;service&gt;", ">CLIENT<", "Acme &lt;Corp&gt;", ">BEARER_TOKEN<", "2026-10-01", "2026-10-05",
		"Credentials are encrypted and cannot be viewed", "Regenerate Token", "Regenerate Secret",
		"Mint a bearer token", "Generate Token", "Manage Roles", `<span class="block font-medium">reader</span>`, "orders:reader", ">ADMIN_ASSIGNED<",
		"Access to all applications", `name="applicationIds" value="app_1" checked`, "Connections", `href="/ui/connections/con_1"`, "paused-hooks",
		"Delete Service Account", `data-action="/ui/service-accounts/sa_1/delete"`, `"selSa":"sa_1"`)
	// The stored secrets are encrypted at rest and never leave the repository.
	mustNotContain(t, body, saStoredToken, saStoredSecret, "other-hooks")

	inactive := h.do(t, saAdmin(), req{target: "/ui/service-accounts/sa_2", datastar: true})
	mustContain(t, inactive, ">Inactive<", "The service account is deactivated.", "No roles assigned", "other-hooks")
	mustContain(t, inactive, `aria-describedby="sa-mint-why"`)
	mustNotContain(t, inactive, `data-action="/ui/service-accounts/sa_2/token"`)

	if rec := h.rec(t, saAdmin(), req{target: "/ui/service-accounts/sa_nope", datastar: true}); !strings.Contains(rec.Body.String(), "fc-flash-error") {
		t.Errorf("a missing account is an error flash: %s", rec.Body.String())
	}
	// Reads of sections are the principal API's and the connection API's permissions.
	limited := h.do(t, saUser(saPermView), req{target: "/ui/service-accounts/sa_1", datastar: true})
	mustContain(t, limited, "Viewing application access needs permission to view users.", "Viewing connections needs permission to view connections.")
	mustNotContain(t, limited, "paused-hooks")
	// The principal API's client-scope check on the application access reads.
	other := ids.ClientID("clt_9")
	h.principals["sa_1"].ClientID = &other
	clientUser := &auth.AuthContext{
		PrincipalID: "prn_c", Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"r"}, PrincipalType: auth.PrincipalTypeUser,
		Permissions: []string{saPermView, "platform:iam:user:view"},
	}
	scoped := h.do(t, clientUser, req{target: "/ui/service-accounts/sa_1", datastar: true})
	mustContain(t, scoped, "belongs to a client you cannot access")
	mustNotContain(t, scoped, `name="applicationIds"`)
}

func TestServiceAccountCreate(t *testing.T) {
	h := newSAHarness()
	dlg := h.do(t, saAdmin(), req{target: "/ui/service-accounts/new", datastar: true})
	mustContain(t, dlg, "Create Service Account", "Anchor (all clients)", "Partner (assigned clients)", "Client (single client)",
		"Access to all applications", `name="clientIds" value="clt_1"`, "Acme &lt;Corp&gt;", "$saFormCode")

	rec := h.rec(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, multipart: true, form: url.Values{
		"name": {" Shipping svc "}, "code": {"shipping-svc"}, "description": {""}, "scope": {"PARTNER"}, "clientIds": {"clt_1", "clt_2", "clt_1"}, "allApplications": {"true"},
	}})
	body := rec.Body.String()
	if len(h.ops.created) != 1 {
		t.Fatalf("created = %+v\n%s", h.ops.created, body)
	}
	c := h.ops.created[0]
	if c.Code != "shipping-svc" || c.Name != "Shipping svc" || c.Description != nil || deref(c.Scope) != "PARTNER" ||
		strings.Join(c.ClientIDs, ",") != "clt_1,clt_2" || c.AllApplications == nil || !*c.AllApplications {
		t.Errorf("create command = %+v", c)
	}
	mustContain(t, body, "Service Account Created", "will not be shown again", "OAuth Credentials", "Webhook Credentials",
		"oauth_new_client", "new-oauth-secret", "fc_new_auth_token", "new-signing-secret", "Authorization: Bearer &lt;token&gt;",
		"I've saved the credentials", `id="sa-credentials"`, "mode replace", "Service account created successfully", `"selSa":"sa_9"`)
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	// Escape and the mask do not close the create dialog.
	mustContain(t, body, `data-on:cancel="evt.preventDefault()"`, "el.dataset.saved || el.showModal()")
	// The secrets are in the dialog only: not in a signal, not in the table, not in the drawer.
	if strings.Count(body, "new-oauth-secret") != 1 {
		t.Errorf("the OAuth secret appears %d times, want once", strings.Count(body, "new-oauth-secret"))
	}
	if strings.Contains(strings.Join(signalsOf(body), "\n"), "secret") {
		t.Errorf("a secret reached a signal: %v", signalsOf(body))
	}
	// A later read of the account does not repeat them.
	again := h.do(t, saAdmin(), req{target: "/ui/service-accounts/sa_9", datastar: true})
	mustNotContain(t, again, "new-oauth-secret", "fc_new_auth_token", "new-signing-secret")

	// An anchor account sends no clients (the platform refuses a scope that
	// disagrees with the links), no description and no allApplications.
	h.do(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, form: url.Values{
		"name": {"Plain"}, "code": {"plain"}, "description": {"d"}, "scope": {"ANCHOR"}, "clientIds": {"clt_1"},
	}})
	p := h.ops.created[1]
	if p.ClientIDs != nil || p.AllApplications != nil || deref(p.Description) != "d" || deref(p.Scope) != "ANCHOR" {
		t.Errorf("anchor create command = %+v", p)
	}
}

func TestServiceAccountCreateRefusals(t *testing.T) {
	h := newSAHarness()
	// A caller without all-applications access may not grant it.
	noAll := saUser(saPermView, saPermCreate)
	noAll.AllApplications = false
	form := url.Values{"name": {"X"}, "code": {"x"}, "scope": {"ANCHOR"}, "allApplications": {"true"}}
	body := h.do(t, noAll, req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, form: form})
	mustContain(t, body, "Only an all-applications administrator may grant all-applications access")
	// An application-scoped caller reaches no platform-wide account at all.
	delete(form, "allApplications")
	body = h.do(t, noAll, req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, form: form})
	mustContain(t, body, "Not authorised for this service account")
	if len(h.ops.created) != 0 {
		t.Fatalf("refused creates reached the use case: %+v", h.ops.created)
	}
	// A scope that is not one of the three is refused before the use case.
	body = h.do(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, form: url.Values{"name": {"X"}, "code": {"x"}, "scope": {"ROOT"}}})
	mustContain(t, body, "Choose a scope.")
	if len(h.ops.created) != 0 {
		t.Fatal("an unknown scope reached the use case")
	}
	// The use case's errors land on the fields.
	h.ops.err = usecase.Conflict("CODE_EXISTS", "Service account with code 'x' already exists")
	body = h.do(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, form: url.Values{"name": {"X"}, "code": {"x"}, "scope": {"ANCHOR"}}})
	mustContain(t, body, "Service account with code &#39;x&#39; already exists", `id="sa-form-code-field"`)
	mustNotContain(t, body, "Service Account Created")
	h.ops.err = usecase.Validation("SCOPE_REACH_MISMATCH", "scope CLIENT does not match the account's client links")
	body = h.do(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, form: url.Values{"name": {"X"}, "code": {"x"}, "scope": {"CLIENT"}}})
	mustContain(t, body, "Scope CLIENT does not match", `id="sa-form-client-ids`)
	// A native post answers the page with the error, not a redirect.
	rec := h.rec(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/new", form: url.Values{"name": {"X"}, "code": {"x"}, "scope": {"ANCHOR"}}})
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("native refused create: %d", rec.Code)
	}
}

func TestServiceAccountCreateNativeShowsTheCredentialsOnce(t *testing.T) {
	h := newSAHarness()
	rec := h.rec(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/new", form: url.Values{"name": {"Native"}, "code": {"native"}, "scope": {"ANCHOR"}}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d (post/redirect/get would lose the secrets): %s", rec.Code, rec.Body.String())
	}
	mustContain(t, rec.Body.String(), "new-oauth-secret", "Service Account Created", `<aside id="sa-drawer"`)
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Location") != "" {
		t.Errorf("headers: %v", rec.Header())
	}
}

func TestServiceAccountEdit(t *testing.T) {
	h := newSAHarness()
	dlg := h.do(t, saAdmin(), req{target: "/ui/service-accounts/sa_1/edit", datastar: true})
	mustContain(t, dlg, "Edit Service Account", "Orders &lt;service&gt; (orders-svc)", `value="Orders &lt;service&gt;"`, `<option value="CLIENT" selected>`,
		`name="clientIds" value="clt_1" checked`)
	mustNotContain(t, dlg, `name="code"`, "Access to all applications")
	// An account with no stored scope starts from the tier its links give, so
	// saving does not clear them.
	h.store.accounts[0].Scope = nil
	dlg = h.do(t, saAdmin(), req{target: "/ui/service-accounts/sa_1/edit", datastar: true})
	mustContain(t, dlg, `<option value="CLIENT" selected>`)

	body := h.do(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/edit", datastar: true, form: url.Values{
		"name": {"Orders 2"}, "description": {""}, "scope": {"PARTNER"}, "clientIds": {"clt_1", "clt_2"}, "drawer": {"sa_1"},
	}})
	u := h.ops.updated[0]
	if u.ID != "sa_1" || deref(u.Name) != "Orders 2" || u.Description != nil || deref(u.Scope) != "PARTNER" || strings.Join(u.ClientIDs, ",") != "clt_1,clt_2" {
		t.Errorf("update command = %+v", u)
	}
	mustContain(t, body, "Service account updated successfully", "sa-drawer", "Service Account Information")
	// An anchor account is saved with an empty, non-nil client list: the
	// links are cleared (nil would leave them alone).
	h.do(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/edit", datastar: true, form: url.Values{"name": {"Orders"}, "scope": {"ANCHOR"}, "clientIds": {"clt_1"}}})
	a := h.ops.updated[1]
	if a.ClientIDs == nil || len(a.ClientIDs) != 0 {
		t.Errorf("anchor update must send an empty, non-nil client list: %#v", a.ClientIDs)
	}
	// The use case's refusal stays in the dialog.
	h.ops.err = usecase.Validation("SCOPE_REACH_MISMATCH", "scope CLIENT does not match the account's client links")
	refused := h.do(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/edit", datastar: true, form: url.Values{"name": {"Orders"}, "scope": {"CLIENT"}}})
	mustContain(t, refused, "does not match the account")
	mustNotContain(t, refused, "Service account updated successfully")
	h.ops.err = nil
	blank := h.do(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/edit", datastar: true, form: url.Values{"name": {"  "}, "scope": {"ANCHOR"}}})
	if len(h.ops.updated) != 4 || deref(h.ops.updated[3].Name) != "" {
		t.Errorf("a blank name is the use case's to refuse: %+v", h.ops.updated)
	}
	_ = blank
}

func TestServiceAccountRoles(t *testing.T) {
	h := newSAHarness()
	// A caller who holds platform:messaging:event:view but not platform:admin:client:update.
	caller := saUser(saPermView, saPermUpdate, "platform:iam:role:view", "platform:messaging:event:view")
	dlg := h.do(t, caller, req{target: "/ui/service-accounts/sa_1/roles", datastar: true})
	mustContain(t, dlg, "Manage Roles", `value="orders:reader" checked`, `value="platform:ops" checked`, "Operations (platform:ops)")
	// platform:admin holds a permission the caller lacks: locked, not offered.
	mustNotContain(t, dlg, `value="platform:admin"`, "Locked roles")
	// A held role above the caller is locked: shown, posted back unchanged.
	h.principals["sa_1"].Roles = append(h.principals["sa_1"].Roles, serviceaccount.RoleAssignment{Role: saRoleName("platform:admin")})
	dlg = h.do(t, caller, req{target: "/ui/service-accounts/sa_1/roles", datastar: true})
	mustContain(t, dlg, "Locked roles", `<input type="hidden" name="roles" value="platform:admin">`, "you cannot add or remove them")
	mustNotContain(t, dlg, `type="checkbox" name="roles" value="platform:admin"`)
	// The super admin holds everything: nothing is locked.
	all := h.do(t, saAdmin(), req{target: "/ui/service-accounts/sa_1/roles", datastar: true})
	mustNotContain(t, all, "Locked roles")
	mustContain(t, all, `value="platform:admin" checked`)

	body := h.do(t, caller, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/roles", datastar: true, form: url.Values{"roles": {"orders:reader", "platform:admin", "orders:reader", " "}, "drawer": {"sa_1"}}})
	if len(h.ops.roles) != 1 || strings.Join(h.ops.roles[0], ",") != "orders:reader,platform:admin" {
		t.Errorf("roles = %v", h.ops.roles)
	}
	mustContain(t, body, "Roles updated")
	// The role ceiling is the use case's: its refusal is shown in the dialog.
	h.ops.err = usecase.Authorization("ROLE_ABOVE_CALLER", "You may only assign or remove roles whose permissions you hold yourself: platform:admin")
	refused := h.do(t, caller, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/roles", datastar: true, form: url.Values{"roles": {"orders:reader"}}})
	mustContain(t, refused, "You may only assign or remove roles whose permissions you hold yourself: platform:admin", "sa-roles-error")
	mustNotContain(t, refused, "Roles updated")
	// Roles need the role list.
	h.ops.err = nil
	n := len(h.ops.roles)
	noList := saUser(saPermView, saPermUpdate)
	if rec := h.rec(t, noList, req{target: "/ui/service-accounts/sa_1/roles", datastar: true}); !strings.Contains(rec.Body.String(), "needs permission to view roles") {
		t.Errorf("roles dialog without the role list: %s", rec.Body.String())
	}
	h.do(t, noList, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/roles", datastar: true, form: url.Values{"roles": {"orders:reader"}}})
	if len(h.ops.roles) != n {
		t.Error("roles were saved without the role list permission")
	}
}

func TestServiceAccountCredentials(t *testing.T) {
	h := newSAHarness()
	page := h.do(t, saAdmin(), req{target: "/ui/service-accounts/sa_1", datastar: true})
	// The SPA's confirmations, with their warnings.
	list := h.do(t, saAdmin(), req{target: "/ui/service-accounts"})
	mustContain(t, list, "Regenerate auth token?", "Webhook deliveries from this service account will carry the new token",
		"Regenerate signing secret?", "Generate a bearer token?", "Existing tokens are not revoked, and the mint is recorded in the audit log", "The new value is shown once")
	mustContain(t, page, `data-action="/ui/service-accounts/sa_1/regenerate-token"`, `data-action="/ui/service-accounts/sa_1/regenerate-secret"`, `data-action="/ui/service-accounts/sa_1/token"`)

	for _, tc := range []struct {
		route, title, secret, action string
	}{
		{"regenerate-token", "New Auth Token", "fc_regenerated_token", "regen-token:sa_1"},
		{"regenerate-secret", "New Signing Secret", "regenerated-signing-secret", "regen-secret:sa_1"},
		{"token", "Bearer Token", "minted.bearer.token", "mint:sa_1"},
	} {
		rec := h.rec(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/" + tc.route, datastar: true, form: url.Values{"drawer": {"sa_1"}}})
		body := rec.Body.String()
		if h.ops.actions[len(h.ops.actions)-1] != tc.action {
			t.Fatalf("%s: actions %v", tc.route, h.ops.actions)
		}
		mustContain(t, body, tc.title, tc.secret, "will not be shown again", `id="sa-cred-0-0"`, "navigator.clipboard.writeText", "mode replace")
		if strings.Count(body, tc.secret) != 1 {
			t.Errorf("%s: the secret appears %d times, want once", tc.route, strings.Count(body, tc.secret))
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: Cache-Control = %q", tc.route, rec.Header().Get("Cache-Control"))
		}
		// These dialogs can be dismissed, and every way of closing removes them.
		mustContain(t, body, `data-on:close="el.remove()"`)
		mustNotContain(t, body, `data-on:cancel="evt.preventDefault()"`)
		// Nothing about the secret is in a signal or stays on the account's page.
		if strings.Contains(strings.Join(signalsOf(body), "\n"), tc.secret) {
			t.Errorf("%s: the secret reached a signal", tc.route)
		}
		after := h.do(t, saAdmin(), req{target: "/ui/service-accounts/sa_1", datastar: true})
		mustNotContain(t, after, tc.secret)
	}
	// The mint is an action of its own, not a flash: no toast text carries the token.
	// A refusal (the role ceiling, an inactive account) shows no dialog.
	h.ops.err = usecase.Authorization("ROLE_ABOVE_CALLER", "You may only mint a token for a service account whose roles' permissions you hold yourself: platform:admin")
	refused := h.do(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/token", datastar: true, form: url.Values{}})
	mustContain(t, refused, "fc-flash-error", "You may only mint a token")
	mustNotContain(t, refused, "minted.bearer.token", `id="sa-credentials"`)
}

func TestServiceAccountDelete(t *testing.T) {
	h := newSAHarness()
	list := h.do(t, saAdmin(), req{target: "/ui/service-accounts"})
	mustContain(t, list, "Delete Service Account", "the OAuth client (client_credentials will stop working)", "This action cannot be undone.")
	body := h.do(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/delete", datastar: true, form: url.Values{"drawer": {"sa_1"}}})
	if len(h.ops.actions) != 1 || h.ops.actions[0] != "delete:sa_1" {
		t.Fatalf("actions %v", h.ops.actions)
	}
	mustContain(t, body, "Service account deleted successfully", `"selSa":""`)
	rec := h.rec(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/sa_2/delete", form: url.Values{}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/service-accounts" {
		t.Errorf("native delete: %d %v", rec.Code, rec.Header())
	}
	if rec := h.rec(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/sa_nope/delete", form: url.Values{}}); rec.Code != http.StatusNotFound {
		t.Errorf("delete of a missing account: %d", rec.Code)
	}
}

func TestServiceAccountApplicationAccess(t *testing.T) {
	h := newSAHarness()
	admin := saAdmin()
	body := h.do(t, admin, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/applications", datastar: true, form: url.Values{"applicationIds": {"app_1", "app_2"}}})
	mustContain(t, body, "Application access updated")
	if len(h.ops.appAccess) != 1 || h.ops.appAccess[0].principalID != "prn_sa1" || strings.Join(h.ops.appAccess[0].apps, ",") != "app_1,app_2" || h.ops.appAccess[0].all != nil {
		t.Errorf("application access = %+v", h.ops.appAccess)
	}
	// The all-applications switch keeps the explicit grants.
	h.do(t, admin, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/all-applications", datastar: true, form: url.Values{"allApplications": {"true"}}})
	on := h.ops.appAccess[1]
	if on.all == nil || !*on.all || strings.Join(on.apps, ",") != "app_1" {
		t.Errorf("grant all = %+v", on)
	}
	h.principals["sa_1"].AllApplications = true
	body = h.do(t, admin, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/all-applications", datastar: true, form: url.Values{}})
	mustContain(t, body, "Restricted to specific applications")
	off := h.ops.appAccess[2]
	if off.all == nil || *off.all {
		t.Errorf("restrict = %+v", off)
	}
	drawer := h.do(t, admin, req{target: "/ui/service-accounts/sa_1", datastar: true})
	mustNotContain(t, drawer, `name="applicationIds"`)

	// Only a caller holding all-applications access may grant it.
	noAll := saUser(saPermView, saPermUpdate, "platform:iam:user:view", "platform:iam:user:update")
	noAll.AllApplications = false
	n := len(h.ops.appAccess)
	// (An application-scoped caller reaches no platform-wide account, so the
	// refusal is the reach; the grant rule is checked with a bound account.)
	noAll.Applications = []string{"app_1"}
	h.principals["sa_3"].AllApplications = false
	refused := h.do(t, noAll, req{method: http.MethodPost, target: "/ui/service-accounts/sa_3/all-applications", datastar: true, form: url.Values{"allApplications": {"true"}}})
	mustContain(t, refused, "Only an all-applications administrator may grant all-applications access")
	// The principal API's permissions: user:update writes, user:view reads.
	viewer := saUser(saPermView, saPermUpdate, "platform:iam:user:view")
	h.do(t, viewer, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/applications", datastar: true, form: url.Values{"applicationIds": {"app_1"}}})
	h.do(t, viewer, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/all-applications", datastar: true, form: url.Values{"allApplications": {"true"}}})
	if len(h.ops.appAccess) != n {
		t.Fatalf("application access changed without user:update: %+v", h.ops.appAccess[n:])
	}
	// Removing one's own all-applications access needs a confirmation the
	// switch never sends, so it is refused.
	self := saAdmin()
	self.PrincipalID = "prn_sa1"
	refusedSelf := h.do(t, self, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/all-applications", datastar: true, form: url.Values{}})
	mustContain(t, refusedSelf, "You are removing your own all-applications access")
	if len(h.ops.appAccess) != n {
		t.Error("a caller removed its own all-applications access")
	}
	mustNotContain(t, h.do(t, admin, req{target: "/ui/service-accounts/sa_1", datastar: true}), saStoredToken)
}

func TestServiceAccountConnections(t *testing.T) {
	h := newSAHarness()
	drawer := h.do(t, saAdmin(), req{target: "/ui/service-accounts/sa_1", datastar: true})
	mustContain(t, drawer, `data-action="/ui/service-accounts/sa_1/connections/con_1/pause"`, `action="/ui/service-accounts/sa_1/connections/con_2/activate"`, `href="/ui/connections/new"`, "New Connection")
	body := h.do(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/connections/con_1/pause", datastar: true, form: url.Values{"drawer": {"sa_1"}}})
	mustContain(t, body, "Connection paused")
	h.do(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/connections/con_2/activate", datastar: true, form: url.Values{}})
	if strings.Join(h.connOps.actions, ",") != "pause:con_1,activate:con_2" {
		t.Errorf("connection actions = %v", h.connOps.actions)
	}
	// A connection of another account is not this account's to pause.
	h.do(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/connections/con_3/pause", datastar: true, form: url.Values{}})
	if len(h.connOps.actions) != 2 {
		t.Error("a connection of another service account was paused")
	}
	// connection:update is the connections API's check.
	h.do(t, saUser(saPermView, "platform:messaging:connection:view"), req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/connections/con_1/pause", datastar: true, form: url.Values{}})
	if len(h.connOps.actions) != 2 {
		t.Error("a connection was paused without connection:update")
	}
	viewer := h.do(t, saUser(saPermView, "platform:messaging:connection:view"), req{target: "/ui/service-accounts/sa_1", datastar: true})
	mustContain(t, viewer, "paused-hooks")
	mustNotContain(t, viewer, "/pause", "/activate", "New Connection")
}

var signalAttr = regexp.MustCompile(`data-signals="[^"]*"|data: signals [^\n]*`)

// signalsOf returns the signals a Datastar response declares or patches
// (data-signals attributes and signal patches): where page state lives.
func signalsOf(body string) []string { return signalAttr.FindAllString(body, -1) }
