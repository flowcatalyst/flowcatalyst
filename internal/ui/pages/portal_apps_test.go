package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// ── fakes ──────────────────────────────────────────────────────────────

// fakePortal is an in-memory PortalStore with the repositories' filters.
type fakePortal struct {
	apps       []portalidentity.App
	identities []portalidentity.Identity
	oauth      map[string][]portalidentity.LinkedOAuthClient
}

func (f *fakePortal) Apps(_ context.Context, clientID string) ([]portalidentity.App, error) {
	var out []portalidentity.App
	for _, a := range f.apps {
		if a.ClientID == clientID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakePortal) AppByCode(_ context.Context, clientID, code string) (*portalidentity.App, error) {
	for _, a := range f.apps {
		if a.ClientID == clientID && a.Code == portalidentity.NormalizeAppCode(code) {
			return &a, nil
		}
	}
	return nil, nil
}

func (f *fakePortal) GrantCounts(_ context.Context, appIDs []string) (map[string]int, error) {
	out := map[string]int{}
	for _, i := range f.identities {
		for _, g := range i.Apps {
			if slices.Contains(appIDs, g.AppID) {
				out[g.AppID]++
			}
		}
	}
	return out, nil
}

func (f *fakePortal) LinkedOAuthClients(_ context.Context, appIDs []string) (map[string][]portalidentity.LinkedOAuthClient, error) {
	out := map[string][]portalidentity.LinkedOAuthClient{}
	for _, id := range appIDs {
		out[id] = f.oauth[id]
	}
	return out, nil
}

func (f *fakePortal) CountUnassigned(_ context.Context, clientID string) (int64, error) {
	var n int64
	for _, i := range f.identities {
		if i.ClientID == clientID && len(i.Apps) == 0 {
			n++
		}
	}
	return n, nil
}

func (f *fakePortal) Search(_ context.Context, flt portalidentity.SearchFilter) ([]portalidentity.Identity, int64, error) {
	var matched []portalidentity.Identity
	for _, i := range f.identities {
		if i.ClientID != flt.ClientID {
			continue
		}
		q := strings.ToLower(flt.Query)
		if q != "" && !strings.HasPrefix(strings.ToLower(i.Email), q) && !strings.HasPrefix(strings.ToLower(i.Name), q) {
			continue
		}
		if flt.Unassigned && len(i.Apps) > 0 {
			continue
		}
		if flt.AppID != "" && !slices.ContainsFunc(i.Apps, func(g portalidentity.AppGrant) bool { return g.AppID == flt.AppID }) {
			continue
		}
		matched = append(matched, i)
	}
	slices.SortStableFunc(matched, func(a, b portalidentity.Identity) int { return b.CreatedAt.Compare(a.CreatedAt) })
	lo := min(flt.Offset, len(matched))
	hi := min(lo+flt.Limit, len(matched))
	return matched[lo:hi], int64(len(matched)), nil
}

func (f *fakePortal) Identity(_ context.Context, id string) (*portalidentity.Identity, error) {
	for _, i := range f.identities {
		if i.ID == id {
			return &i, nil
		}
	}
	return nil, nil
}

// fakePortalOps records the commands and applies their effect on the store.
type fakePortalOps struct {
	store *fakePortal
	calls []string
	err   error
	// secret is what a CONFIDENTIAL create hands back.
	secret string
}

func (o *fakePortalOps) note(call string) error {
	o.calls = append(o.calls, call)
	return o.err
}

func (o *fakePortalOps) CreateApp(_ context.Context, cmd portalidentity.CreateAppWithOAuthClientCommand) (portalidentity.CreateAppWithOAuthClientResult, error) {
	if err := o.note("create:" + cmd.ClientID + ":" + cmd.Code + ":" + cmd.Name + ":" + strings.Join(cmd.RedirectURIs, ",") + ":" + cmd.ClientType + ":" + deref(cmd.Description)); err != nil {
		return portalidentity.CreateAppWithOAuthClientResult{}, err
	}
	if o.store.findCode(cmd.ClientID, cmd.Code) != nil {
		return portalidentity.CreateAppWithOAuthClientResult{}, usecase.Conflict("CODE_EXISTS", "portal app code '"+cmd.Code+"' already exists for this client")
	}
	app := portalidentity.App{ID: "pta_new", ClientID: cmd.ClientID, Code: cmd.Code, Name: cmd.Name, Active: true, CreatedAt: time.Now()}
	o.store.apps = append(o.store.apps, app)
	res := portalidentity.CreateAppWithOAuthClientResult{AppID: app.ID, OAuthClientRowID: "oac_new", OAuthClientID: "client_new", ClientType: cmd.ClientType}
	if cmd.ClientType != "PUBLIC" {
		res.ClientType, res.ClientSecret = "CONFIDENTIAL", o.secret
	}
	return res, nil
}

func (f *fakePortal) findCode(clientID, code string) *portalidentity.App {
	for i := range f.apps {
		if f.apps[i].ClientID == clientID && f.apps[i].Code == code {
			return &f.apps[i]
		}
	}
	return nil
}

func (o *fakePortalOps) UpdateApp(_ context.Context, cmd portalidentity.UpdateAppCommand) error {
	call := "update:" + cmd.ClientID + ":" + cmd.ID + ":" + deref(cmd.Name) + ":" + deref(cmd.Description)
	if cmd.Active != nil {
		call += ":" + strconv.FormatBool(*cmd.Active)
	}
	return o.note(call)
}

func (o *fakePortalOps) DeleteApp(_ context.Context, cmd portalidentity.DeleteAppCommand) error {
	if err := o.note("delete-app:" + cmd.ClientID + ":" + cmd.ID); err != nil {
		return err
	}
	o.store.apps = slices.DeleteFunc(o.store.apps, func(a portalidentity.App) bool { return a.ID == cmd.ID })
	return nil
}

func (o *fakePortalOps) AssignUnassigned(_ context.Context, cmd portalidentity.AssignUnassignedCommand) (portalidentity.AssignUnassignedResult, error) {
	if err := o.note("assign:" + cmd.ClientID + ":" + cmd.PortalAppID); err != nil {
		return portalidentity.AssignUnassignedResult{}, err
	}
	var res portalidentity.AssignUnassignedResult
	for n := range o.store.identities {
		i := &o.store.identities[n]
		if i.ClientID == cmd.ClientID && len(i.Apps) == 0 {
			i.Apps = append(i.Apps, portalidentity.AppGrant{AppID: cmd.PortalAppID, Source: portalidentity.SourceAdmin, GrantedAt: time.Now()})
			res.IdentityIDs = append(res.IdentityIDs, i.ID)
		}
	}
	return res, nil
}

func (o *fakePortalOps) SetStatus(_ context.Context, cmd portalidentity.SetStatusCommand) error {
	if err := o.note("status:" + cmd.ClientID + ":" + cmd.ID + ":" + cmd.Status); err != nil {
		return err
	}
	for n := range o.store.identities {
		if o.store.identities[n].ID == cmd.ID {
			o.store.identities[n].Status = portalidentity.Status(cmd.Status)
		}
	}
	return nil
}

func (o *fakePortalOps) DeleteUser(_ context.Context, cmd portalidentity.DeleteCommand) error {
	if err := o.note("delete-user:" + cmd.ClientID + ":" + cmd.ID); err != nil {
		return err
	}
	o.store.identities = slices.DeleteFunc(o.store.identities, func(i portalidentity.Identity) bool { return i.ID == cmd.ID })
	return nil
}

func (o *fakePortalOps) GrantApp(_ context.Context, cmd portalidentity.AppGrantCommand) error {
	if err := o.note("grant:" + cmd.ClientID + ":" + cmd.IdentityID + ":" + cmd.PortalAppID); err != nil {
		return err
	}
	for n := range o.store.identities {
		if o.store.identities[n].ID == cmd.IdentityID {
			o.store.identities[n].Apps = append(o.store.identities[n].Apps, portalidentity.AppGrant{AppID: cmd.PortalAppID, Source: portalidentity.SourceAdmin, GrantedAt: time.Now()})
		}
	}
	return nil
}

func (o *fakePortalOps) RevokeApp(_ context.Context, cmd portalidentity.AppGrantCommand) error {
	if err := o.note("revoke:" + cmd.ClientID + ":" + cmd.IdentityID + ":" + cmd.PortalAppID); err != nil {
		return err
	}
	for n := range o.store.identities {
		i := &o.store.identities[n]
		if i.ID == cmd.IdentityID {
			i.Apps = slices.DeleteFunc(i.Apps, func(g portalidentity.AppGrant) bool { return g.AppID == cmd.PortalAppID })
		}
	}
	return nil
}

// ── fixtures ───────────────────────────────────────────────────────────

var portalDay = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func portalApp(id, client, code, name string, active bool) portalidentity.App {
	desc := name + " <portal>"
	return portalidentity.App{ID: id, ClientID: client, Code: code, Name: name, Description: &desc, Active: active, CreatedAt: portalDay, UpdatedAt: portalDay}
}

func portalUser(id, client, email, name string, created time.Time, appIDs ...string) portalidentity.Identity {
	hash := "x"
	i := portalidentity.Identity{
		ID: id, ClientID: client, Email: email, Name: name, PasswordHash: &hash, Status: portalidentity.StatusActive,
		Source: portalidentity.SourceInvite, CreatedAt: created, UpdatedAt: created,
	}
	for _, a := range appIDs {
		i.Apps = append(i.Apps, portalidentity.AppGrant{AppID: a, Source: portalidentity.SourceInvite, GrantedAt: portalDay})
	}
	return i
}

func portalFixture() *fakePortal {
	return &fakePortal{
		apps: []portalidentity.App{
			portalApp("pta_cust", "clt_acme", "customers", "Customer Portal", true),
			portalApp("pta_supp", "clt_acme", "suppliers", "Supplier Portal", false),
			portalApp("pta_glob", "clt_globex", "globex-portal", "Globex Portal", true),
		},
		identities: []portalidentity.Identity{
			portalUser("ptu_ann", "clt_acme", "ann@acme.test", "Ann <Admin>", portalDay.Add(3*time.Hour), "pta_cust"),
			portalUser("ptu_bob", "clt_acme", "bob@acme.test", "", portalDay.Add(2*time.Hour)),
			portalUser("ptu_cy", "clt_acme", "cy@acme.test", "Cy", portalDay.Add(time.Hour), "pta_cust", "pta_supp"),
			portalUser("ptu_gus", "clt_globex", "gus@globex.test", "Gus", portalDay, "pta_glob"),
		},
		oauth: map[string][]portalidentity.LinkedOAuthClient{
			"pta_cust": {{ID: "oac_1", ClientID: "client_cust", ClientName: "Customer Portal (portal)"}},
		},
	}
}

func portalClients() *fakeClientStore {
	return &fakeClientStore{clients: []client.Client{
		{ID: ids.ClientID("clt_acme"), Identifier: "acme", Name: "Acme Corp", Status: client.StatusActive},
		{ID: ids.ClientID("clt_globex"), Identifier: "globex", Name: "Globex <Inc>", Status: client.StatusActive},
	}}
}

// portalCaller is a client-scoped user of the given clients with the given
// portal permissions.
func portalCaller(clients []string, perms ...string) *auth.AuthContext {
	return &auth.AuthContext{
		PrincipalID: "prn_pc", Email: "pc@acme.test", Scope: auth.ScopeClient, Clients: clients,
		Roles: []string{"r"}, Permissions: perms, PrincipalType: auth.PrincipalTypeUser,
	}
}

const (
	permPortalView   = "platform:iam:portal-user:view"
	permPortalManage = "platform:iam:portal-user:manage"
)

type portalHarness struct {
	store *fakePortal
	ops   *fakePortalOps
}

func newPortalHarness() *portalHarness {
	s := portalFixture()
	return &portalHarness{store: s, ops: &fakePortalOps{store: s, secret: "s3cr3t-SECRET-value"}}
}

func (h *portalHarness) serve(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	sh := shellForTest()
	clients := portalClients()
	return serveMounted(t, ac, q,
		&PortalApps{Store: h.store, Ops: h.ops, Clients: clients, Shell: sh},
		&PortalUsers{Store: h.store, Ops: h.ops, Clients: clients, Shell: sh})
}

func (h *portalHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.serve(t, ac, q).Body.String()
}

func postForm(target string, kv ...string) req {
	f := url.Values{}
	for n := 0; n+1 < len(kv); n += 2 {
		f.Add(kv[n], kv[n+1])
	}
	return req{method: http.MethodPost, target: target, datastar: true, form: f}
}

// menuOfPortal is one row's action menu in a portal table.
func menuOfPortal(t *testing.T, body, rowActionsID string) string {
	t.Helper()
	_, menu, ok := strings.Cut(body, `id="`+rowActionsID+`-menu"`)
	if !ok {
		t.Fatalf("no menu %s", rowActionsID)
	}
	menu, _, _ = strings.Cut(menu, "</td>")
	return menu
}

// ── portal apps ────────────────────────────────────────────────────────

func TestPortalAppsList(t *testing.T) {
	h := newPortalHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/portal-apps?client=clt_acme"})
	mustContain(t, body,
		`<title>Portal Apps · FlowCatalyst</title>`,
		`href="/ui/portal-apps" class="fc-nav-item" title="Portal Apps" aria-current="page"`,
		`<h1 class="fc-page-title">Portal Apps</h1>`,
		`<option value="clt_acme" selected>Acme Corp</option>`, `Globex &lt;Inc&gt;`,
		"Customer Portal", "Customer Portal &lt;portal&gt;", `<code class="font-mono text-[13px]">customers</code>`,
		`client_cust`, `href="/authentication/oauth-clients/oac_1"`, "Not linked",
		"Active", "Inactive",
		"1 portal user(s) have no portal app", // bob has none; ann and cy hold apps
		`href="/ui/portal-apps/new?client=clt_acme"`, "New Portal App",
		`<dialog id="portal-app-assign"`, `<dialog id="portal-app-delete"`,
		`name="client" value="clt_acme"`,
	)
	// Another client's app is not listed.
	mustNotContain(t, body, "Globex Portal")
	mustContain(t, menuOfPortal(t, body, "portal-app-pta_cust-actions"), `data-action="/ui/portal-apps/pta_cust/assign-unassigned"`, `data-action="/ui/portal-apps/pta_cust/delete"`)
}

func TestPortalAppsListCountsAndAssignRules(t *testing.T) {
	h := newPortalHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/portal-apps?client=clt_acme"})
	// 1 user without an app (bob): the banner counts him, an inactive app
	// refuses new grants.
	mustContain(t, body, "1 portal user(s) have no portal app")
	mustContain(t, menuOfPortal(t, body, "portal-app-pta_supp-actions"), "An inactive portal app takes no new grants.")
	mustNotContain(t, menuOfPortal(t, body, "portal-app-pta_supp-actions"), `data-action="/ui/portal-apps/pta_supp/assign-unassigned"`)

	// Nobody without an app: nothing to assign, and no banner.
	h.store.identities[1].Apps = []portalidentity.AppGrant{{AppID: "pta_cust"}}
	body = h.do(t, anchorAdmin(), req{target: "/ui/portal-apps?client=clt_acme"})
	mustContain(t, menuOfPortal(t, body, "portal-app-pta_cust-actions"), "No portal users are without a portal app.")
	mustNotContain(t, body, "have no portal app, so they")
}

func TestPortalAppsNoClientChosen(t *testing.T) {
	h := newPortalHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/portal-apps"})
	mustContain(t, body, "Select a client to view its portal apps.", `<option value="clt_acme">Acme Corp</option>`)
	mustNotContain(t, body, "New Portal App", `<dialog id="portal-app-delete"`)

	// A client user lands on their own client.
	body = h.do(t, portalCaller([]string{"clt_globex"}, permPortalView), req{target: "/ui/portal-apps"})
	mustContain(t, body, "Globex Portal", `<option value="clt_globex" selected>Globex &lt;Inc&gt;</option>`)
	mustNotContain(t, body, "Customer Portal", "Acme Corp")
}

func TestPortalAppsPermissions(t *testing.T) {
	h := newPortalHarness()
	// No portal permission, or no access to the client: refused.
	if rec := h.serve(t, portalCaller([]string{"clt_acme"}, "platform:messaging:event:view"), req{target: "/ui/portal-apps?client=clt_acme"}); rec.Code != http.StatusForbidden {
		t.Errorf("no portal permission: status %d", rec.Code)
	}
	if rec := h.serve(t, portalCaller([]string{"clt_globex"}, permPortalView, permPortalManage), req{target: "/ui/portal-apps?client=clt_acme"}); rec.Code != http.StatusForbidden {
		t.Errorf("another client's apps: status %d", rec.Code)
	}

	// The viewer reads, and has no control that changes anything.
	viewer := h.do(t, portalCaller([]string{"clt_acme"}, permPortalView), req{target: "/ui/portal-apps?client=clt_acme"})
	mustContain(t, viewer, "Customer Portal")
	mustNotContain(t, viewer, "New Portal App", `<dialog id="portal-app-assign"`, `<dialog id="portal-app-delete"`, "Delete…", `aria-label="Edit `, "Actions")
	// Manage alone reads too (the API's requireAny) and changes.
	manager := h.do(t, portalCaller([]string{"clt_acme"}, permPortalManage), req{target: "/ui/portal-apps?client=clt_acme"})
	mustContain(t, manager, "New Portal App", `<dialog id="portal-app-delete"`, "Delete…", `aria-label="Edit Customer Portal"`)
}

func TestPortalAppsForgedPostsAreRefused(t *testing.T) {
	h := newPortalHarness()
	viewer := portalCaller([]string{"clt_acme"}, permPortalView)
	otherManager := portalCaller([]string{"clt_globex"}, permPortalView, permPortalManage)
	for name, ac := range map[string]*auth.AuthContext{"viewer": viewer, "manager of another client": otherManager, "no permission": portalCaller([]string{"clt_acme"})} {
		for _, q := range []req{
			postForm("/ui/portal-apps/new", "client", "clt_acme", "name", "X", "code", "x"),
			postForm("/ui/portal-apps/pta_cust/edit", "client", "clt_acme", "name", "Renamed"),
			postForm("/ui/portal-apps/pta_cust/delete", "client", "clt_acme"),
			postForm("/ui/portal-apps/pta_cust/assign-unassigned", "client", "clt_acme"),
		} {
			body := h.do(t, ac, q)
			if len(h.ops.calls) != 0 {
				t.Fatalf("%s: %s ran %v", name, q.target, h.ops.calls)
			}
			if lower := strings.ToLower(body); !strings.Contains(lower, "permission required") && !strings.Contains(lower, "no access to this client") {
				t.Errorf("%s: %s: the refusal is not shown:\n%s", name, q.target, body)
			}
		}
		// And the GET dialogs of a change.
		for _, target := range []string{"/ui/portal-apps/new?client=clt_acme", "/ui/portal-apps/pta_cust/edit?client=clt_acme"} {
			if rec := h.serve(t, ac, req{target: target}); rec.Code != http.StatusForbidden {
				t.Errorf("%s: GET %s: status %d", name, target, rec.Code)
			}
		}
	}
	// An app of another client is not found, even for its manager.
	manager := portalCaller([]string{"clt_acme"}, permPortalView, permPortalManage)
	body := h.do(t, manager, postForm("/ui/portal-apps/pta_glob/delete", "client", "clt_acme"))
	if len(h.ops.calls) != 0 || !strings.Contains(body, "fc-flash-error") {
		t.Errorf("another client's app: ran %v:\n%s", h.ops.calls, body)
	}
}

func TestPortalAppCreateShowsCredentialsOnce(t *testing.T) {
	h := newPortalHarness()
	rec := h.serve(t, anchorAdmin(), postForm("/ui/portal-apps/new",
		"client", "clt_acme", "name", "  Partner Portal ", "code", " Partner-Portal ", "description", " For partners ",
		"redirectUris", "https://p.example.com/cb\n\nhttps://p.example.com/cb\nhttp://localhost:3000/cb", "clientType", "CONFIDENTIAL"))
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, body)
	}
	// The SPA's body: trimmed, the code lower-cased, one callback per line.
	want := "create:clt_acme:partner-portal:Partner Portal:https://p.example.com/cb,http://localhost:3000/cb:CONFIDENTIAL:For partners"
	if len(h.ops.calls) != 1 || h.ops.calls[0] != want {
		t.Fatalf("calls %v, want %s", h.ops.calls, want)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control %q, want no-store", got)
	}
	before, dialog, found := strings.Cut(body, `id="portal-app-credentials"`)
	if !found {
		t.Fatalf("no credentials dialog:\n%s", body)
	}
	mustContain(t, dialog,
		"Portal app created", "<strong>Partner Portal</strong> is ready", "only once",
		`id="portal-cred-code" class="fc-mono-chip break-all">partner-portal<`, `id="portal-cred-client-id" class="fc-mono-chip break-all">client_new<`, `id="portal-cred-secret" class="fc-mono-chip break-all">s3cr3t-SECRET-value<`,
		`id="portal-cred-endpoint-0" class="fc-mono-chip break-all">http://example.com/portal/authorize<`, "/oauth/token", "/.well-known/jwks.json", "/.well-known/openid-configuration",
		"FLOWCATALYST_OIDC_CLIENT_SECRET=s3cr3t-SECRET-value", "FLOWCATALYST_OIDC_PORTAL=true", "# portalAppCode for /api/portal-users: partner-portal",
		`data-on:cancel="evt.preventDefault()"`, "Done",
	)
	mustNotContain(t, dialog, "No callback URL registered")
	// The secret is in the dialog and nowhere else in the answer: not in
	// the table, a flash or a signal.
	mustNotContain(t, before, "s3cr3t-SECRET-value")
	if n := strings.Count(body, "s3cr3t-SECRET-value"); n != 2 {
		t.Errorf("the secret appears %d times, want 2 (the field and the environment)", n)
	}
	mustContain(t, body, "Partner Portal")
	// A later read of the page does not have it.
	later := h.do(t, anchorAdmin(), req{target: "/ui/portal-apps?client=clt_acme"})
	mustNotContain(t, later, "s3cr3t-SECRET-value", "portal-app-credentials")
	mustContain(t, later, "Partner Portal")
}

func TestPortalAppCreatePublicClientAndNoCallback(t *testing.T) {
	h := newPortalHarness()
	body := h.do(t, anchorAdmin(), postForm("/ui/portal-apps/new", "client", "clt_acme", "name", "SPA", "code", "spa", "clientType", "PUBLIC"))
	mustContain(t, body, "None — public client (PKCE only)", "No callback URL registered yet", `href="/authentication/oauth-clients/oac_new"`)
	mustNotContain(t, body, "s3cr3t-SECRET-value", "FLOWCATALYST_OIDC_CLIENT_SECRET", "only once")
}

func TestPortalAppCreateNativePostRendersThePageNoStore(t *testing.T) {
	h := newPortalHarness()
	q := postForm("/ui/portal-apps/new", "client", "clt_acme", "name", "Native", "code", "native")
	q.datastar = false
	rec := h.serve(t, anchorAdmin(), q)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status %d, Cache-Control %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	mustContain(t, rec.Body.String(), "<title>Portal Apps · FlowCatalyst</title>", `id="portal-cred-secret" class="fc-mono-chip break-all">s3cr3t-SECRET-value<`)
	if len(rec.Result().Cookies()) != 0 {
		t.Errorf("a cookie was set on the credentials answer: %v", rec.Result().Cookies())
	}
}

func TestPortalAppCreateErrors(t *testing.T) {
	h := newPortalHarness()
	body := h.do(t, anchorAdmin(), postForm("/ui/portal-apps/new", "client", "clt_acme", "name", "Again", "code", "customers"))
	mustContain(t, body, `id="portal-app-form-code-err"`, "already exists for this client")
	mustNotContain(t, body, "portal-app-credentials")

	// A refusal of the use case for a callback shows on the callbacks field.
	h.ops.err = usecase.Validation("REDIRECT_URI_INVALID", "redirectUris must be absolute http(s) URLs without wildcards: ftp://x")
	body = h.do(t, anchorAdmin(), postForm("/ui/portal-apps/new", "client", "clt_acme", "name", "Bad", "code", "bad", "redirectUris", "ftp://x"))
	mustContain(t, body, `id="portal-app-form-redirect-uris-err"`, "absolute http(s) URLs")
	// The refused form keeps what was typed.
	mustContain(t, body, "ftp://x", `value="Bad"`)

	// Without Datastar the page comes back with the dialog open and the use case's status.
	q := postForm("/ui/portal-apps/new", "client", "clt_acme", "name", "Bad", "code", "bad", "redirectUris", "ftp://x")
	q.datastar = false
	if rec := h.serve(t, anchorAdmin(), q); rec.Code != http.StatusBadRequest {
		t.Errorf("native refusal: status %d, want the use case's 400", rec.Code)
	}
}

func TestPortalAppEditDialogAndUpdate(t *testing.T) {
	h := newPortalHarness()
	dialog := h.do(t, anchorAdmin(), req{target: "/ui/portal-apps/pta_cust/edit?client=clt_acme", datastar: true})
	mustContain(t, dialog,
		"Edit Portal App", "Customer Portal (customers)", `action="/ui/portal-apps/pta_cust/edit"`,
		`name="client" value="clt_acme"`, `value="Customer Portal"`, "The code is fixed",
		`role="switch"`, "Active — inactive portals refuse sign-ins and new grants",
	)
	mustNotContain(t, dialog, `name="redirectUris"`, `name="clientType"`, `name="code"`)

	// The switch off (an unchecked switch posts nothing) and a blank
	// description: the SPA's PUT body.
	h.do(t, anchorAdmin(), postForm("/ui/portal-apps/pta_cust/edit", "client", "clt_acme", "name", " Renamed ", "description", ""))
	if want := "update:clt_acme:pta_cust:Renamed::false"; len(h.ops.calls) != 1 || h.ops.calls[0] != want {
		t.Fatalf("calls %v, want %s", h.ops.calls, want)
	}
	body := h.do(t, anchorAdmin(), postForm("/ui/portal-apps/pta_cust/edit", "client", "clt_acme", "name", "Customer Portal", "active", "true"))
	mustContain(t, body, "Portal app &#34;Customer Portal&#34; updated", "portal-apps-region")
	if got := h.ops.calls[len(h.ops.calls)-1]; got != "update:clt_acme:pta_cust:Customer Portal::true" {
		t.Errorf("last call %s", got)
	}
}

func TestPortalAppDeleteAndAssign(t *testing.T) {
	h := newPortalHarness()
	body := h.do(t, anchorAdmin(), postForm("/ui/portal-apps/pta_cust/assign-unassigned", "client", "clt_acme"))
	mustContain(t, body, "1 user(s) can now sign in to Customer Portal", "portal-apps-region")
	mustNotContain(t, body, "have no portal app, so they") // the banner is gone
	if h.ops.calls[0] != "assign:clt_acme:pta_cust" {
		t.Errorf("calls %v", h.ops.calls)
	}

	body = h.do(t, anchorAdmin(), postForm("/ui/portal-apps/pta_cust/delete", "client", "clt_acme"))
	mustContain(t, body, "Portal app &#34;Customer Portal&#34; deleted")
	mustNotContain(t, body, `id="portal-app-pta_cust"`)
	if h.ops.calls[1] != "delete-app:clt_acme:pta_cust" {
		t.Errorf("calls %v", h.ops.calls)
	}

	// A refusal of the use case is shown, not swallowed.
	h.ops.err = usecase.Validation("PORTAL_APP_INACTIVE", "portal app 'suppliers' is inactive")
	body = h.do(t, anchorAdmin(), postForm("/ui/portal-apps/pta_supp/assign-unassigned", "client", "clt_acme"))
	mustContain(t, body, "fc-flash-error", "is inactive")

	// A native post redirects with a flash key.
	h.ops.err = nil
	q := postForm("/ui/portal-apps/pta_supp/delete", "client", "clt_acme")
	q.datastar = false
	rec := h.serve(t, anchorAdmin(), q)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/portal-apps?client=clt_acme" {
		t.Errorf("native delete: %d %s", rec.Code, rec.Header().Get("Location"))
	}
}

func TestPortalAppsListEscapesData(t *testing.T) {
	h := newPortalHarness()
	h.store.apps[0].Name = `<script>alert(1)</script>`
	body := h.do(t, anchorAdmin(), req{target: "/ui/portal-apps?client=clt_acme"})
	mustNotContain(t, body, `<script>alert(1)</script>`)
	mustContain(t, body, `&lt;script&gt;alert(1)&lt;/script&gt;`)
}
