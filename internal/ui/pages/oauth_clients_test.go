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

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The secret references the fixtures hold: they must never be rendered.
const (
	ocStoredSecret   = "hashed:v1:stored_secret_ref_do_not_render"
	ocStoredPrevious = "hashed:v1:previous_secret_ref_do_not_render"
)

type fakeOCStore struct {
	clients []platformauth.OAuthClient
}

func (f *fakeOCStore) FindAll(context.Context) ([]platformauth.OAuthClient, error) {
	return append([]platformauth.OAuthClient(nil), f.clients...), nil
}

func (f *fakeOCStore) FindByID(_ context.Context, id string) (*platformauth.OAuthClient, error) {
	for i := range f.clients {
		if f.clients[i].ID == id {
			c := f.clients[i]
			return &c, nil
		}
	}
	return nil, nil
}

type fakeOCPortal map[string][]portalidentity.App

func (f fakeOCPortal) Apps(_ context.Context, clientID string) ([]portalidentity.App, error) {
	return f[clientID], nil
}

type fakeOCOps struct {
	store    *fakeOCStore
	created  []authops.CreateOAuthClientCommand
	updated  []authops.UpdateOAuthClientCommand
	rotated  []*int64
	actions  []string
	err      error
	rotation time.Time
}

func (o *fakeOCOps) act(name, id string) error {
	o.actions = append(o.actions, name+":"+id)
	return o.err
}

func (o *fakeOCOps) Create(_ context.Context, cmd authops.CreateOAuthClientCommand) (IssuedOAuthClient, error) {
	o.created = append(o.created, cmd)
	if o.err != nil {
		return IssuedOAuthClient{}, o.err
	}
	c := ocFixture("oc_9", "oac_new_client", cmd.ClientName, platformauth.OAuthClientType(cmd.ClientType), true)
	o.store.clients = append(o.store.clients, c)
	out := IssuedOAuthClient{ID: "oc_9", ClientID: "oac_new_client"}
	if cmd.ClientType == "CONFIDENTIAL" {
		out.Secret = "brand-new-client-secret"
	}
	return out, nil
}

func (o *fakeOCOps) Update(_ context.Context, cmd authops.UpdateOAuthClientCommand) error {
	o.updated = append(o.updated, cmd)
	return o.err
}

func (o *fakeOCOps) Activate(_ context.Context, id string) error   { return o.act("activate", id) }
func (o *fakeOCOps) Deactivate(_ context.Context, id string) error { return o.act("deactivate", id) }
func (o *fakeOCOps) Delete(_ context.Context, id string) error     { return o.act("delete", id) }
func (o *fakeOCOps) RevokePreviousSecret(_ context.Context, id string) error {
	return o.act("revoke", id)
}

func (o *fakeOCOps) Rotate(_ context.Context, id string, grace *int64) (RotatedOAuthSecret, error) {
	o.rotated = append(o.rotated, grace)
	out := RotatedOAuthSecret{ClientID: "oac_orders", Secret: "rotated-client-secret"}
	if grace == nil {
		out.PreviousExpiresAt = &o.rotation
	}
	return out, o.act("rotate", id)
}

func ocFixture(id, clientID, name string, t platformauth.OAuthClientType, active bool) platformauth.OAuthClient {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	c := platformauth.NewOAuthClient(clientID, name, t)
	c.ID, c.Active, c.CreatedAt, c.UpdatedAt = id, active, day, day
	if t == platformauth.OAuthClientConfidential {
		ref := ocStoredSecret
		c.SecretRef = &ref
	}
	return *c
}

type ocHarness struct {
	store *fakeOCStore
	ops   *fakeOCOps
}

func newOCHarness() *ocHarness {
	soon := time.Now().UTC().Add(20 * time.Hour).Truncate(time.Second)
	orders := ocFixture("oc_1", "oac_orders", "Orders <web>", platformauth.OAuthClientConfidential, true)
	orders.RedirectURIs = []string{"https://orders.example.com/callback"}
	orders.PostLogoutRedirectURIs = []string{"https://orders.example.com/bye"}
	orders.AllowedOrigins = []string{"https://orders.example.com"}
	orders.GrantTypes = []string{"authorization_code", "refresh_token"}
	orders.Scopes = []string{"openid", "email"}
	orders.ApplicationIDs = []string{"app_1", "app_2", "app_3", "app_4"}
	prev := ocStoredPrevious
	orders.PreviousSecretRef, orders.PreviousSecretExpiresAt = &prev, &soon
	spa := ocFixture("oc_2", "oac_spa", "Dashboard SPA", platformauth.OAuthClientPublic, false)
	spa.RedirectURIs = []string{"https://spa.example.com/cb"}
	spa.GrantTypes = []string{"authorization_code"}
	owner, portalApp := "clt_1", "pta_1"
	portal := ocFixture("oc_3", "oac_portal", "Customer portal", platformauth.OAuthClientPublic, true)
	portal.PortalClientID, portal.PortalAppID = &owner, &portalApp
	portal.GrantTypes = []string{"authorization_code"}
	store := &fakeOCStore{clients: []platformauth.OAuthClient{orders, spa, portal}}
	return &ocHarness{store: store, ops: &fakeOCOps{store: store, rotation: time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)}}
}

func (h *ocHarness) page() *OAuthClients {
	apps := newAppHarness().store
	for _, id := range []string{"app_3", "app_4"} {
		apps.apps = append(apps.apps, app(id, "code-"+id, "Extra "+id, true))
	}
	return &OAuthClients{
		Clients: h.store, Ops: h.ops, Apps: apps,
		ClientNames: &fakeClientStore{clients: []client.Client{cli("clt_1", "acme", "Acme <Corp>", client.StatusActive), cli("clt_2", "globex", "Globex", client.StatusActive)}},
		Portal: fakeOCPortal{"clt_1": {
			{ID: "pta_1", ClientID: "clt_1", Code: "customers", Name: "Customer <portal>", Active: true},
			{ID: "pta_2", ClientID: "clt_1", Code: "suppliers", Name: "Suppliers", Active: true},
		}},
		Shell: shellForTest(),
	}
}

func (h *ocHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, h.page())
}

func (h *ocHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

// ocUser is an anchor caller holding only perms.
func ocUser(perms ...string) *auth.AuthContext { return connUser(perms...) }

const (
	ocPermView   = "platform:auth:oauth-client:view"
	ocPermCreate = "platform:auth:oauth-client:create"
	ocPermUpdate = "platform:auth:oauth-client:update"
	ocPermDelete = "platform:auth:oauth-client:delete"
	ocPermRotate = "platform:auth:oauth-client:regenerate-secret"
)

var ocRowIDs = regexp.MustCompile(`id="oc-oc_\d"`)

func TestOAuthClientsList(t *testing.T) {
	h := newOCHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients"})
	mustContain(t, body, `<title>OAuth Clients · FlowCatalyst</title>`, "Manage OAuth2/OIDC client registrations", "Add OAuth Client",
		"Orders &lt;web&gt;", `<code class="font-mono text-[12px]">oac_orders</code>`, ">CONFIDENTIAL<", ">PUBLIC<", ">Inactive<", ">Active<",
		"Showing 1 to 3 of 3 OAuth clients", `href="/ui/oauth-clients/new"`, "No restrictions",
		// The SPA's first three application tags and a count.
		"Orders", "Extra app_3", "+1 more",
		`<dialog id="oc-rotate"`, `<dialog id="oc-delete"`, `<dialog id="oc-deactivate"`, `data-signals="{selOc: &#39;&#39;, ocCopied: &#39;&#39;}"`)
	mustContain(t, body, `href="/ui/oauth-clients" class="fc-nav-item" title="OAuth Clients" aria-current="page"`)
	mustNotContain(t, body, ocStoredSecret, ocStoredPrevious, "Extra app_4")

	// The search covers the name, the client id and the application names.
	for _, q := range []string{"dashboard", "oac_portal", "extra app_4"} {
		got := h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients?q=" + url.QueryEscape(q), datastar: true})
		mustContain(t, got, "replaceState", "q=")
		if n := len(ocRowIDs.FindAllString(got, -1)); n != 1 {
			t.Errorf("q=%q: %d rows, want 1", q, n)
		}
	}
	sorted := h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients?sort=clientId&dir=desc", datastar: true})
	if spa, portal, orders := strings.Index(sorted, "oac_spa"), strings.Index(sorted, "oac_portal"), strings.Index(sorted, "oac_orders"); spa > portal || portal > orders {
		t.Error("sort=clientId&dir=desc: spa, portal, orders expected")
	}
	// The row menu: deactivate an active client, activate an inactive one.
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients"}), `data-action="/ui/oauth-clients/oc_1/deactivate"`, `data-action="/ui/oauth-clients/oc_2/activate"`, `data-action="/ui/oauth-clients/oc_1/delete"`)
}

// TestOAuthClientsPermissionMatrix pins the gates of the SPA's API: every
// control is hidden without its permission and every route refuses a forged
// post, before the use case runs and without a secret in the answer.
func TestOAuthClientsPermissionMatrix(t *testing.T) {
	forged := []struct {
		name, target string
		form         url.Values
	}{
		{"create", "/ui/oauth-clients/new", url.Values{"clientName": {"X"}, "clientType": {"CONFIDENTIAL"}, "redirectUris": {"https://x.example.com/cb"}, "grantTypes": {"authorization_code"}}},
		{"edit", "/ui/oauth-clients/oc_1/edit", url.Values{"clientName": {"Hacked"}, "redirectUris": {"https://evil.example.com/cb"}, "grantTypes": {"authorization_code"}}},
		{"activate", "/ui/oauth-clients/oc_2/activate", url.Values{}},
		{"deactivate", "/ui/oauth-clients/oc_1/deactivate", url.Values{}},
		{"delete", "/ui/oauth-clients/oc_1/delete", url.Values{}},
		{"rotate", "/ui/oauth-clients/oc_1/rotate-secret", url.Values{}},
		{"rotate now", "/ui/oauth-clients/oc_1/rotate-secret-now", url.Values{}},
		{"revoke previous", "/ui/oauth-clients/oc_1/revoke-previous-secret", url.Values{}},
	}
	every := []string{ocPermView, ocPermCreate, ocPermUpdate, ocPermDelete, ocPermRotate}
	callers := []struct {
		name string
		ac   *auth.AuthContext
		// can lists the forged routes this caller may run (the permission for
		// the operation, which the API checks).
		can []string
	}{
		{"viewer", ocUser(ocPermView), nil},
		// Holding the permissions is not enough without anchor scope.
		{"non-anchor with every permission", &auth.AuthContext{
			PrincipalID: "prn_c", Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"r"}, Permissions: every, PrincipalType: auth.PrincipalTypeUser,
		}, nil},
		{"creator", ocUser(ocPermView, ocPermCreate), []string{"create"}},
		{"updater", ocUser(ocPermView, ocPermUpdate), []string{"edit", "activate", "deactivate"}},
		{"deleter", ocUser(ocPermView, ocPermDelete), []string{"delete"}},
		{"rotator", ocUser(ocPermView, ocPermRotate), []string{"rotate", "rotate now", "revoke previous"}},
	}
	for _, c := range callers {
		for _, f := range forged {
			t.Run(c.name+"/"+f.name, func(t *testing.T) {
				h := newOCHarness()
				rec := h.rec(t, c.ac, req{method: http.MethodPost, target: f.target, datastar: true, form: f.form})
				body := rec.Body.String()
				allowed := false
				for _, n := range c.can {
					allowed = allowed || n == f.name
				}
				ran := len(h.ops.created)+len(h.ops.updated)+len(h.ops.actions) > 0
				if ran != allowed {
					t.Fatalf("use case ran = %v, want %v (%s)", ran, allowed, body)
				}
				mustNotContain(t, body, ocStoredSecret, ocStoredPrevious)
				if allowed {
					return
				}
				// The refusal is shown (a flash, or the form's error region) and no
				// credentials dialog or secret comes with it.
				if !strings.Contains(body, "fc-flash-error") && !strings.Contains(body, `id="oc-form-error"`) {
					t.Errorf("a refused post shows no refusal: %s", body)
				}
				mustNotContain(t, body, "brand-new-client-secret", "rotated-client-secret", `id="oc-credentials"`)
			})
		}
	}

	// Controls are hidden without the permission; the server stays the guard.
	h := newOCHarness()
	view := h.do(t, ocUser(ocPermView), req{target: "/ui/oauth-clients"})
	mustNotContain(t, view, "Add OAuth Client", `aria-label="Edit `, `<dialog id="oc-delete"`, `<dialog id="oc-rotate"`, `<dialog id="oc-activate"`)
	drawer := h.do(t, ocUser(ocPermView), req{target: "/ui/oauth-clients/oc_1", datastar: true})
	mustContain(t, drawer, "Rotating the secret needs the regenerate-secret permission")
	mustNotContain(t, drawer, "Rotate Secret", "Revoke Previous Secret", `href="/ui/oauth-clients/oc_1/edit"`, "Deactivate Client")
	rotator := h.do(t, ocUser(ocPermView, ocPermRotate), req{target: "/ui/oauth-clients/oc_1", datastar: true})
	mustContain(t, rotator, `data-action="/ui/oauth-clients/oc_1/rotate-secret"`, `data-action="/ui/oauth-clients/oc_1/rotate-secret-now"`, `data-action="/ui/oauth-clients/oc_1/revoke-previous-secret"`)
	mustNotContain(t, rotator, "Deactivate Client")
	// Reading needs the view permission and anchor scope.
	for _, target := range []string{"/ui/oauth-clients", "/ui/oauth-clients/oc_1", "/ui/oauth-clients/oc_1/edit"} {
		if rec := h.rec(t, ocUser(ocPermCreate), req{target: target}); rec.Code != http.StatusForbidden {
			t.Errorf("GET %s without the permission: %d", target, rec.Code)
		}
	}
	if rec := h.rec(t, ocUser(ocPermView), req{target: "/ui/oauth-clients/new"}); rec.Code != http.StatusForbidden {
		t.Errorf("create form without the create permission: %d", rec.Code)
	}
	if rec := h.rec(t, ocUser(ocPermView), req{target: "/ui/oauth-clients/oc_1/edit"}); rec.Code != http.StatusForbidden {
		t.Errorf("edit form without the update permission: %d", rec.Code)
	}
}

func TestOAuthClientDrawer(t *testing.T) {
	h := newOCHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients/oc_1", datastar: true})
	mustContain(t, body, "Orders &lt;web&gt;", "oac_orders", "https://orders.example.com/callback", "https://orders.example.com/bye", ">authorization_code<", ">refresh_token<",
		">openid<", "Yes", ">Extra app_3<", "Not a portal client", "Identity tokens only", "Client Secret",
		"The client secret is encrypted and cannot be displayed",
		`data-action="/ui/oauth-clients/oc_1/rotate-secret"`, `id="oc-client-id"`, "navigator.clipboard.writeText",
		"Previous secret still accepted", "It has not been used since the rotation.", "Revoke Previous Secret",
		"Rotate Without Overlap", "Deactivate Client", `data-action="/ui/oauth-clients/oc_1/deactivate"`)
	mustNotContain(t, body, ocStoredSecret, ocStoredPrevious)
	mustContain(t, body, `"selOc":"oc_1"`)

	// A public client has no secret to rotate; the portal fields name their targets.
	pub := h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients/oc_3", datastar: true})
	mustNotContain(t, pub, "Rotate Secret", "Client Secret")
	mustContain(t, pub, "Acme &lt;Corp&gt;", "Customer &lt;portal&gt; (customers)", "No post-logout redirects configured", "No CORS origins configured", "No application restrictions")
	inactive := h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients/oc_2", datastar: true})
	mustContain(t, inactive, "Activate Client", `data-action="/ui/oauth-clients/oc_2/activate"`)

	// An overlap that has lapsed is the same as none.
	lapsed := time.Now().UTC().Add(-time.Hour)
	h.store.clients[0].PreviousSecretExpiresAt = &lapsed
	mustNotContain(t, h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients/oc_1", datastar: true}), "Previous secret still accepted", "Revoke Previous Secret")

	// Without the client list the owner's id is shown, not its name (an anchor
	// reads the owner's portal apps, as the API lets it).
	bare := h.do(t, ocUser(ocPermView), req{target: "/ui/oauth-clients/oc_3", datastar: true})
	mustContain(t, bare, "clt_1", "Customer &lt;portal&gt; (customers)")
	mustNotContain(t, bare, "Acme &lt;Corp&gt;")

	// A full page navigation renders the list with the drawer open.
	page := h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients/oc_1"})
	mustContain(t, page, `<title>OAuth Clients · FlowCatalyst</title>`, `data-signals="{selOc: &#39;oc_1&#39;, ocCopied: &#39;&#39;}"`, "Orders &lt;web&gt;")
	if rec := h.rec(t, anchorAdmin(), req{target: "/ui/oauth-clients/oc_nope"}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown client: %d", rec.Code)
	}
	if rec := h.rec(t, anchorAdmin(), req{target: "/ui/oauth-clients/a'b"}); rec.Code != http.StatusNotFound {
		t.Errorf("malformed id: %d", rec.Code)
	}
}

func TestOAuthClientCreateConfidentialShowsTheSecretOnce(t *testing.T) {
	h := newOCHarness()
	form := h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients/new", datastar: true})
	mustContain(t, form, `id="oc-form"`, "Create OAuth Client", "Public (SPA, Mobile)", "Confidential (Server)", "Authorization Code", "Client Credentials", "offline_access",
		"Extra app_3 (code-app_3)", "Acme &lt;Corp&gt;", `name="redirectUris"`, `name="allowedOrigins"`, `name="postLogoutRedirectUris"`, "one per line")
	// Defaults: public, PKCE on, authorization code and refresh token, the three scopes.
	mustContain(t, form, `value="authorization_code" checked`, `value="openid" checked`)

	rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/new", datastar: true, form: url.Values{
		"clientName": {"  Billing server "}, "clientType": {"CONFIDENTIAL"},
		"redirectUris":           {"https://billing.example.com/cb\n\n https://billing.example.com/cb \nhttp://localhost:3000/cb"},
		"postLogoutRedirectUris": {"https://billing.example.com/bye"}, "allowedOrigins": {"https://billing.example.com/ \nhttps://billing.example.com"},
		"grantTypes": {"authorization_code", "client_credentials"}, "defaultScopes": {"openid"}, "pkceRequired": {"true"}, "applicationIds": {"app_1"},
	}})
	body := rec.Body.String()
	if len(h.ops.created) != 1 {
		t.Fatalf("created: %v\n%s", h.ops.created, body)
	}
	cmd := h.ops.created[0]
	// The SPA's POST body.
	if cmd.ClientName != "Billing server" || cmd.ClientType != "CONFIDENTIAL" || len(cmd.RedirectURIs) != 2 || cmd.RedirectURIs[1] != "http://localhost:3000/cb" ||
		len(cmd.PostLogoutRedirectURIs) != 1 || len(cmd.AllowedOrigins) != 1 || cmd.AllowedOrigins[0] != "https://billing.example.com" ||
		len(cmd.GrantTypes) != 2 || len(cmd.Scopes) != 1 || len(cmd.ApplicationIDs) != 1 || cmd.PKCERequired == nil || !*cmd.PKCERequired ||
		cmd.PortalClientID != nil || cmd.PortalAppID != nil || cmd.APIAccess != nil {
		t.Errorf("command: %+v", cmd)
	}
	mustContain(t, body, "Client Secret Generated", "oac_new_client", "brand-new-client-secret", "will not be shown again", `id="oc-cred-0"`, `id="oc-cred-1"`,
		"navigator.clipboard.writeText", "mode replace", "OAuth client created successfully", "I've copied the secret")
	if strings.Count(body, "brand-new-client-secret") != 1 {
		t.Errorf("the secret appears %d times, want once", strings.Count(body, "brand-new-client-secret"))
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	// The create dialog cannot be dismissed except by its button, and nothing
	// about the secret reaches a signal or the new client's drawer.
	mustContain(t, body, `data-on:cancel="evt.preventDefault()"`, `"selOc":"oc_9"`)
	if strings.Contains(strings.Join(signalsOf(body), "\n"), "brand-new-client-secret") {
		t.Error("the secret reached a signal")
	}
	mustNotContain(t, h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients/oc_9", datastar: true}), "brand-new-client-secret")
	mustNotContain(t, h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients"}), "brand-new-client-secret")

	// A native post gets the page with the dialog (post/redirect/get would lose the secret).
	h2 := newOCHarness()
	native := h2.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/new", form: url.Values{
		"clientName": {"Native"}, "clientType": {"CONFIDENTIAL"}, "redirectUris": {"https://n.example.com/cb"}, "grantTypes": {"authorization_code"},
	}})
	if native.Code != http.StatusOK || native.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("native create: %d %v", native.Code, native.Header())
	}
	mustContain(t, native.Body.String(), "brand-new-client-secret", "Client Secret Generated", `<title>OAuth Clients`)
}

func TestOAuthClientCreatePublicOpensTheDrawer(t *testing.T) {
	h := newOCHarness()
	rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/new", datastar: true, form: url.Values{
		"clientName": {"Mobile"}, "clientType": {"PUBLIC"}, "redirectUris": {"com.example.app://callback"}, "grantTypes": {"authorization_code"},
		"portalClientId": {"clt_1"}, "portalAppId": {"pta_2"},
	}})
	body := rec.Body.String()
	cmd := h.ops.created[0]
	// PKCE is always on for a public client (its checkbox is disabled, so it is not posted).
	if cmd.PKCERequired == nil || !*cmd.PKCERequired {
		t.Errorf("PKCE for a public client: %v", cmd.PKCERequired)
	}
	if cmd.PortalClientID == nil || *cmd.PortalClientID != "clt_1" || cmd.PortalAppID == nil || *cmd.PortalAppID != "pta_2" {
		t.Errorf("portal fields: %+v", cmd)
	}
	mustContain(t, body, "OAuth client created successfully", `"selOc":"oc_9"`, "Mobile")
	mustNotContain(t, body, `id="oc-credentials"`, "brand-new-client-secret")
	// The portal app is sent only with its owner.
	h.ops.created = nil
	h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/new", datastar: true, form: url.Values{
		"clientName": {"No owner"}, "clientType": {"PUBLIC"}, "redirectUris": {"https://a.example.com/cb"}, "grantTypes": {"authorization_code"},
		"portalAppId": {"pta_2"}, "apiAccess": {"true"},
	}})
	if c := h.ops.created[0]; c.PortalAppID != nil || c.PortalClientID != nil || c.APIAccess == nil || !*c.APIAccess {
		t.Errorf("command: %+v", c)
	}
	// A native post redirects to the new client's drawer.
	native := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/new", form: url.Values{
		"clientName": {"Native"}, "clientType": {"PUBLIC"}, "redirectUris": {"https://n.example.com/cb"}, "grantTypes": {"authorization_code"},
	}})
	if native.Code != http.StatusSeeOther || native.Header().Get("Location") != "/ui/oauth-clients/oc_9" {
		t.Errorf("native create: %d %v", native.Code, native.Header())
	}
}

func TestOAuthClientCreateValidation(t *testing.T) {
	h := newOCHarness()
	post := func(f url.Values) string {
		t.Helper()
		return h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/new", datastar: true, form: f})
	}
	body := post(url.Values{"clientName": {" "}, "clientType": {"NOPE"}, "redirectUris": {"not a url"}, "postLogoutRedirectUris": {"nope"}, "allowedOrigins": {"https://a.example.com/path"}})
	mustContain(t, body, "Client name is required", "Choose a client type.", "At least one grant type is required", "Not a valid URL: not a url", "Not a valid URL: nope",
		"Origin should not include a path", `id="oc-form-client-name-field"`, `id="oc-form-redirect-uris-field"`, `id="oc-form-grant-types-wrap"`, "oc-form-client-name")
	// The text typed is given back.
	mustContain(t, body, "not a url")
	if len(h.ops.created) != 0 {
		t.Fatal("an invalid form reached the use case")
	}
	mustContain(t, post(url.Values{"clientName": {"X"}, "clientType": {"PUBLIC"}, "grantTypes": {"client_credentials"}}), "At least one redirect URI is required")
	mustContain(t, post(url.Values{"clientName": {"X"}, "clientType": {"PUBLIC"}, "redirectUris": {"https://a.example.com"}, "grantTypes": {"client_credentials"}, "allowedOrigins": {"ftp://a.example.com"}}), "Not a valid origin: ftp://a.example.com")

	// The use case's refusals land on their field or the form.
	h.ops.err = usecase.Validation("PORTAL_API_ACCESS_CONFLICT", "a portal client cannot have apiAccess")
	conflict := post(url.Values{"clientName": {"X"}, "clientType": {"PUBLIC"}, "redirectUris": {"https://a.example.com/cb"}, "grantTypes": {"authorization_code"}, "portalClientId": {"clt_1"}, "apiAccess": {"true"}})
	mustContain(t, conflict, "A portal client cannot have apiAccess", `id="oc-form-api-access-field"`)
	h.ops.err = usecase.Validation("PORTAL_APP_CLIENT_MISMATCH", "portalAppId belongs to a different client than portalClientId")
	mismatch := post(url.Values{"clientName": {"X"}, "clientType": {"PUBLIC"}, "redirectUris": {"https://a.example.com/cb"}, "grantTypes": {"authorization_code"}, "portalClientId": {"clt_2"}, "portalAppId": {"pta_1"}})
	mustContain(t, mismatch, "belongs to a different client", `id="oc-form-portal-app-id-field"`)
	h.ops.err = usecase.Internal("REPO", "boom: db password hunter2", nil)
	failed := post(url.Values{"clientName": {"X"}, "clientType": {"PUBLIC"}, "redirectUris": {"https://a.example.com/cb"}, "grantTypes": {"authorization_code"}})
	mustContain(t, failed, "The request could not be completed")
	mustNotContain(t, failed, "hunter2")
	// A native post re-renders the page with the dialog and the status.
	native := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/new", form: url.Values{"clientName": {""}, "clientType": {"PUBLIC"}}})
	if native.Code != http.StatusUnprocessableEntity {
		t.Errorf("native invalid post: %d", native.Code)
	}
}

func TestOAuthClientPortalAppOptions(t *testing.T) {
	h := newOCHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients/portal-app-options?portalClientId=clt_1", datastar: true})
	mustContain(t, body, `id="oc-form-portal-app-id-field"`, "Customer &lt;portal&gt; (customers)", "Suppliers", "Legacy: whole client (no app)")
	none := h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients/portal-app-options?portalClientId=", datastar: true})
	mustContain(t, none, `id="oc-form-portal-app-id-field"`)
	mustNotContain(t, none, "Suppliers", "Legacy: whole client")
	// An id that could not be one is offered no apps.
	mustNotContain(t, h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients/portal-app-options?portalClientId=a%27b", datastar: true}), "Suppliers")
	if rec := h.rec(t, ocUser(ocPermView), req{target: "/ui/oauth-clients/portal-app-options?portalClientId=clt_1", datastar: true}); !strings.Contains(rec.Body.String(), "fc-flash-error") {
		t.Errorf("a caller who can neither create nor update must be refused: %s", rec.Body.String())
	}
}

func TestOAuthClientEdit(t *testing.T) {
	h := newOCHarness()
	form := h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients/oc_1/edit", datastar: true})
	mustContain(t, form, "Edit OAuth Client", "Orders &lt;web&gt; (oac_orders)", "https://orders.example.com/callback", "https://orders.example.com/bye",
		`data-attr:value="$selOc"`, `action="/ui/oauth-clients/oc_1/edit"`, `data-signals="{ocFormType: &#39;CONFIDENTIAL&#39;, ocFormPortal: &#39;&#39;}"`)
	mustNotContain(t, form, `name="clientType"`, ocStoredSecret)
	// The portal client's form carries its owner and app.
	portal := h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients/oc_3/edit", datastar: true})
	mustContain(t, portal, `value="pta_1" selected`, "Customer &lt;portal&gt; (customers)", "ocFormPortal: &#39;clt_1&#39;")
	// Without the client list only the held owner is offered, and a held value the form
	// cannot offer (a grant type outside the SPA's three) is kept.
	h.store.clients[0].GrantTypes = append(h.store.clients[0].GrantTypes, "password")
	bare := h.do(t, ocUser(ocPermView, ocPermUpdate), req{target: "/ui/oauth-clients/oc_3/edit", datastar: true})
	mustContain(t, bare, `value="clt_1" selected`, "Listing clients needs permission to view clients", "Listing applications needs permission to view applications")
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients/oc_1/edit", datastar: true}), `value="password" checked`)

	post := func(id string, f url.Values) string {
		t.Helper()
		return h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/" + id + "/edit", datastar: true, form: f})
	}
	body := post("oc_1", url.Values{
		"clientName": {"Orders renamed"}, "redirectUris": {"https://o.example.com/cb"}, "postLogoutRedirectUris": {""}, "allowedOrigins": {""},
		"grantTypes": {"authorization_code"}, "pkceRequired": {"true"}, "drawer": {"oc_1"},
	})
	if len(h.ops.updated) != 1 {
		t.Fatalf("updated: %v\n%s", h.ops.updated, body)
	}
	u := h.ops.updated[0]
	// The SPA's PUT body: everything, an emptied list clears (non-nil), the owner "" clears.
	if u.ID != "oc_1" || *u.ClientName != "Orders renamed" || u.PostLogoutRedirectURIs == nil || len(u.PostLogoutRedirectURIs) != 0 || u.AllowedOrigins == nil || len(u.AllowedOrigins) != 0 ||
		u.Scopes == nil || len(u.Scopes) != 0 || u.ApplicationIDs == nil || *u.PortalClientID != "" || *u.PortalAppID != "" || *u.APIAccess || !*u.PKCERequired {
		t.Errorf("command: %+v", u)
	}
	mustContain(t, body, "OAuth client updated successfully", "Orders &lt;web&gt;", "mode inner")
	// A public client's PKCE stays on although its checkbox is disabled.
	h.ops.updated = nil
	post("oc_2", url.Values{"clientName": {"SPA"}, "redirectUris": {"https://s.example.com/cb"}, "grantTypes": {"authorization_code"}})
	if !*h.ops.updated[0].PKCERequired {
		t.Error("PKCE of a public client was turned off")
	}
	// Portal app only with its owner.
	h.ops.updated = nil
	post("oc_3", url.Values{"clientName": {"P"}, "redirectUris": {"https://p.example.com/cb"}, "grantTypes": {"authorization_code"}, "portalClientId": {"clt_1"}, "portalAppId": {"pta_2"}})
	post("oc_3", url.Values{"clientName": {"P"}, "redirectUris": {"https://p.example.com/cb"}, "grantTypes": {"authorization_code"}, "portalAppId": {"pta_2"}})
	if *h.ops.updated[0].PortalAppID != "pta_2" || *h.ops.updated[1].PortalAppID != "" || *h.ops.updated[1].PortalClientID != "" {
		t.Errorf("portal fields: %+v %+v", h.ops.updated[0], h.ops.updated[1])
	}
	// The SPA's checks: redirect URIs are required for the authorization_code and
	// refresh_token grants (not for client_credentials alone), the name and a grant always.
	h.ops.updated = nil
	bad := post("oc_1", url.Values{"clientName": {""}, "grantTypes": {"refresh_token"}})
	mustContain(t, bad, "Client name is required", "At least one redirect URI is required for authorization_code or refresh_token grants")
	post("oc_1", url.Values{"clientName": {"M2M"}, "grantTypes": {"client_credentials"}})
	if len(h.ops.updated) != 1 || len(h.ops.updated[0].RedirectURIs) != 0 {
		t.Errorf("a client_credentials client needs no redirect URI: %+v", h.ops.updated)
	}
	if rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/oc_nope/edit", form: url.Values{"clientName": {"x"}, "grantTypes": {"client_credentials"}}}); rec.Code != http.StatusNotFound {
		t.Errorf("edit of a missing client: %d", rec.Code)
	}
	native := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/oc_1/edit", form: url.Values{"clientName": {"N"}, "grantTypes": {"client_credentials"}}})
	if native.Code != http.StatusSeeOther || native.Header().Get("Location") != "/ui/oauth-clients" {
		t.Errorf("native edit: %d %v", native.Code, native.Header())
	}
}

func TestOAuthClientRotation(t *testing.T) {
	h := newOCHarness()
	// The SPA's confirmation, with its warnings; the 24-hour overlap is stated.
	list := h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients"})
	mustContain(t, list, "Rotate client secret?", "keeps working for a 24-hour overlap", "Applications using the old secret will fail to authenticate after that",
		"The new value is shown once, so copy it before closing", "Rotate client secret now?", "stops working immediately, with no overlap", "Revoke previous secret?")

	rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/oc_1/rotate-secret", datastar: true, form: url.Values{"drawer": {"oc_1"}}})
	body := rec.Body.String()
	if len(h.ops.rotated) != 1 || h.ops.rotated[0] != nil {
		t.Fatalf("rotations: %v", h.ops.rotated)
	}
	mustContain(t, body, "New Client Secret", "rotated-client-secret", "will not be shown again", `id="oc-cred-0"`, "navigator.clipboard.writeText", "mode replace",
		"keeps working until 2026-10-25 09:00:00 UTC", "Client secret rotated successfully")
	if strings.Count(body, "rotated-client-secret") != 1 {
		t.Errorf("the secret appears %d times, want once", strings.Count(body, "rotated-client-secret"))
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	// This dialog can be dismissed, and every way of closing removes it.
	mustContain(t, body, `data-on:close="el.remove()"`)
	mustNotContain(t, body, `data-on:cancel="evt.preventDefault()"`)
	if strings.Contains(strings.Join(signalsOf(body), "\n"), "rotated-client-secret") {
		t.Error("the secret reached a signal")
	}
	mustNotContain(t, h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients/oc_1", datastar: true}), "rotated-client-secret")

	// Without overlap: the old secret stops at once.
	now := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/oc_1/rotate-secret-now", datastar: true, form: url.Values{"drawer": {"oc_1"}}})
	if len(h.ops.rotated) != 2 || h.ops.rotated[1] == nil || *h.ops.rotated[1] != 0 {
		t.Fatalf("rotations: %v", h.ops.rotated)
	}
	mustContain(t, now, "rotated-client-secret", "The previous secret stopped working immediately.")

	// A refusal (a public client, a missing id) shows no dialog.
	h.ops.err = usecase.Conflict("NOT_CONFIDENTIAL", "Only CONFIDENTIAL clients have rotatable secrets")
	refused := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/oc_2/rotate-secret", datastar: true, form: url.Values{}})
	mustContain(t, refused, "fc-flash-error", "Only CONFIDENTIAL clients have rotatable secrets")
	mustNotContain(t, refused, "rotated-client-secret", `id="oc-credentials"`)

	// A rotator without the view permission gets the secret, not the list.
	h.ops.err = nil
	blind := h.rec(t, ocUser(ocPermRotate), req{method: http.MethodPost, target: "/ui/oauth-clients/oc_1/rotate-secret", datastar: true, form: url.Values{}})
	mustContain(t, blind.Body.String(), "rotated-client-secret")
	mustNotContain(t, blind.Body.String(), "Orders &lt;web&gt;", `id="oc-table"`)
	if blind.Header().Get("Cache-Control") != "no-store" {
		t.Error("the blind rotation answer is cacheable")
	}
	// A native post gets the page with the dialog.
	native := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/oc_1/rotate-secret", form: url.Values{}})
	if native.Code != http.StatusOK || native.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("native rotate: %d %v", native.Code, native.Header())
	}
	mustContain(t, native.Body.String(), "rotated-client-secret", "New Client Secret")

	// Revoke the previous secret: refreshes the drawer, flashes, no secret.
	revoke := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/oc_1/revoke-previous-secret", datastar: true, form: url.Values{"drawer": {"oc_1"}}})
	if got := h.ops.actions[len(h.ops.actions)-1]; got != "revoke:oc_1" {
		t.Errorf("actions %v", h.ops.actions)
	}
	mustContain(t, revoke, "Previous client secret revoked", "mode inner")
	mustNotContain(t, revoke, "rotated-client-secret")
}

func TestOAuthClientStateActions(t *testing.T) {
	h := newOCHarness()
	list := h.do(t, anchorAdmin(), req{target: "/ui/oauth-clients"})
	mustContain(t, list, "Delete OAuth Client", "Applications using this client will no longer be able to authenticate users.", "Inactive clients cannot be used to authenticate users.")

	for _, tc := range []struct {
		route, action, flash string
	}{
		{"activate", "activate:oc_2", "Activated"},
		{"deactivate", "deactivate:oc_1", "Deactivated"},
	} {
		_, id, _ := strings.Cut(tc.action, ":")
		body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/" + id + "/" + tc.route, datastar: true, form: url.Values{"drawer": {id}}})
		if got := h.ops.actions[len(h.ops.actions)-1]; got != tc.action {
			t.Fatalf("%s: actions %v", tc.route, h.ops.actions)
		}
		mustContain(t, body, tc.flash, "mode inner")
	}
	// Delete from a row leaves the drawer alone; from the drawer it closes it.
	row := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/oc_1/delete", datastar: true, form: url.Values{"drawer": {""}}})
	mustContain(t, row, "OAuth client deleted")
	mustNotContain(t, row, `"selOc":""`)
	drawer := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/oc_1/delete", datastar: true, form: url.Values{"drawer": {"oc_1"}}})
	mustContain(t, drawer, `"selOc":""`)
	// A delete is refused when the use case says so (and the page says why).
	h.ops.err = usecase.NotFound("OAUTH_CLIENT_NOT_FOUND", "OAuth client not found")
	missing := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/oc_9/delete", datastar: true, form: url.Values{}})
	mustContain(t, missing, "fc-flash-error", "OAuth client not found")
	// Native posts redirect to the list with a flash key.
	h.ops.err = nil
	native := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/oc_2/activate", form: url.Values{}})
	if native.Code != http.StatusSeeOther || native.Header().Get("Location") != "/ui/oauth-clients" {
		t.Errorf("native activate: %d %v", native.Code, native.Header())
	}
	if rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/oauth-clients/a'b/delete", form: url.Values{}}); rec.Code != http.StatusNotFound {
		t.Errorf("malformed id: %d", rec.Code)
	}
}
