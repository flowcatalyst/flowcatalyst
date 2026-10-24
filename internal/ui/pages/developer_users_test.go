package pages

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

type fakeDevUsers struct{ ps []principal.Principal }

func (f *fakeDevUsers) FindByRole(_ context.Context, roleName string) ([]principal.Principal, error) {
	var out []principal.Principal
	for _, p := range f.ps {
		if slices.Contains(devRoleNames(&p), roleName) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeDevUsers) FindAll(context.Context) ([]principal.Principal, error) {
	return append([]principal.Principal(nil), f.ps...), nil
}

func (f *fakeDevUsers) FindByID(_ context.Context, id ids.PrincipalID) (*principal.Principal, error) {
	for i := range f.ps {
		if f.ps[i].ID == id {
			p := f.ps[i]
			return &p, nil
		}
	}
	return nil, nil
}

// fakeDevUserOps records what ran; the secret it returns is the value the
// tests look for in responses.
type fakeDevUserOps struct {
	calls     []string
	secret    string
	err       error
	revokeErr error
}

func (o *fakeDevUserOps) record(call string) error {
	o.calls = append(o.calls, call)
	return o.err
}

func (o *fakeDevUserOps) GrantRole(_ context.Context, id string) error {
	return o.record("grant:" + id)
}

func (o *fakeDevUserOps) RemoveRole(_ context.Context, id string) error {
	return o.record("remove:" + id)
}

func (o *fakeDevUserOps) SetCredential(_ context.Context, id string) (string, error) {
	return o.secret, o.record("set:" + id)
}

func (o *fakeDevUserOps) RevokeCredential(_ context.Context, id string) error {
	if err := o.record("revoke:" + id); err != nil {
		return err
	}
	return o.revokeErr
}

const testSecret = "dev-secret-VALUE-0123456789"

func devUser(id, name, email string, roles []string, cred bool, client string) principal.Principal {
	p := principal.Principal{
		ID: ids.PrincipalID(id), Type: principal.TypeUser, Scope: principal.ScopeAnchor, Name: name, Active: true,
		UserIdentity: principal.NewUserIdentity(email),
	}
	for _, r := range roles {
		p.Roles = append(p.Roles, serviceaccount.RoleAssignment{Role: role.StoredName(r)})
	}
	if cred {
		ref, at := "ref", time.Date(2026, 10, 2, 8, 30, 0, 0, time.UTC)
		p.UserIdentity.DevClientSecretRef, p.UserIdentity.DevClientSecretUpdatedAt = &ref, &at
	}
	if client != "" {
		c := ids.ClientID(client)
		p.ClientID, p.Scope = &c, principal.ScopeClient
	}
	return p
}

type devUsersHarness struct {
	store *fakeDevUsers
	ops   *fakeDevUserOps
}

func newDevUsersHarness() *devUsersHarness {
	dev := []string{developerRole}
	return &devUsersHarness{
		store: &fakeDevUsers{ps: []principal.Principal{
			devUser("prn_ann", `Ann <Dev>`, "ann@x.test", dev, true, ""),
			devUser("prn_bob", "Bob Builder", "bob@x.test", dev, false, ""),
			devUser("prn_cy", "Cy Client", "cy@acme.test", dev, false, "clt_acme"),
			devUser("prn_dee", "Dee Plain", "dee@x.test", []string{"other:role"}, false, ""),
			devUser("prn_eve", "Eve Plain", "eve@acme.test", nil, false, "clt_acme"),
			devUser("prn_fay", "Fay Plain", "fay@other.test", nil, false, "clt_other"),
		}},
		ops: &fakeDevUserOps{secret: testSecret},
	}
}

func (h *devUsersHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, &DeveloperUsers{Store: h.store, Ops: h.ops, Shell: shellForTest()})
}

func (h *devUsersHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

const (
	permUserView   = "platform:iam:user:view"
	permUserUpdate = "platform:iam:user:update"
	permAssign     = "platform:iam:user:assign-roles"
	permOwnDevCred = "platform:developer:api-credential:manage"
)

// clientAdmin is a non-anchor administrator of one client.
func clientAdmin(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{
		PrincipalID: "prn_ca", Scope: auth.ScopeClient, Roles: []string{"r"}, Clients: []string{"clt_acme"},
		Permissions: perms, PrincipalType: auth.PrincipalTypeUser,
	}
}

func TestDeveloperUsersList(t *testing.T) {
	h := newDevUsersHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/developer-users"})
	mustContain(t, body, `<title>Developer Users · FlowCatalyst</title>`, "Ann &lt;Dev&gt;", "ann@x.test", "Bob Builder", "Cy Client",
		">Set<", ">Not set<", "2026-10-02 08:30:00 UTC", "Showing 1 to 3 of 3 developer users", "Grant Developer Role",
		`href="/ui/developer-users" class="fc-nav-item" title="Developer Users" aria-current="page"`)
	// Only developers; sorted by name, either way.
	mustNotContain(t, body, "Dee Plain", "Eve Plain")
	if strings.Index(body, "Ann &lt;") > strings.Index(body, "Bob Builder") {
		t.Error("not sorted by name")
	}
	desc := h.do(t, anchorAdmin(), req{target: "/ui/developer-users?sort=name&dir=desc"})
	if strings.Index(desc, "Cy Client") > strings.Index(desc, "Bob Builder") {
		t.Error("not sorted descending")
	}
	byEmail := h.do(t, anchorAdmin(), req{target: "/ui/developer-users?sort=email&dir=desc"})
	if strings.Index(byEmail, "Cy Client") > strings.Index(byEmail, "Ann &lt;") {
		t.Error("not sorted by email")
	}

	// Search covers name and email, live.
	live := h.do(t, anchorAdmin(), req{target: "/ui/developer-users?q=ACME", datastar: true})
	mustContain(t, live, "Cy Client", "Showing 1 to 1 of 1 developer users", "replaceState", "/ui/developer-users?q=ACME")
	mustNotContain(t, live, "Ann &lt;")
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/developer-users?q=builder"}), "Bob Builder")
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/developer-users?q=zzz"}), "No developer users found")

	h.store.ps = nil
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/developer-users"}), "No developer users yet")
}

// The list makes the principal API's check and, like the principal list,
// shows a non-anchor caller only the principals of the clients they reach.
func TestDeveloperUsersListChecks(t *testing.T) {
	h := newDevUsersHarness()
	if rec := h.rec(t, connUser(permUserUpdate), req{target: "/ui/developer-users"}); rec.Code != http.StatusForbidden {
		t.Errorf("no user:view: %d", rec.Code)
	}
	for _, path := range []string{"/ui/developer-users/grant", "/ui/developer-users/grant/search?q=x", "/ui/developer-users/prn_ann/confirm/remove"} {
		if rec := h.rec(t, connUser(permUserUpdate), req{target: path}); rec.Code != http.StatusForbidden {
			t.Errorf("%s without user:view or its permission: %d", path, rec.Code)
		}
	}
	body := h.do(t, clientAdmin(permUserView), req{target: "/ui/developer-users"})
	mustContain(t, body, "Cy Client")
	mustNotContain(t, body, "Ann &lt;", "Bob Builder", "Grant Developer Role")
}

func TestDeveloperUsersControls(t *testing.T) {
	h := newDevUsersHarness()
	const (
		rotate = `/ui/developer-users/prn_ann/confirm/rotate`
		revoke = `/ui/developer-users/prn_ann/confirm/revoke`
		remove = `/ui/developer-users/prn_ann/confirm/remove`
		set    = `data-action="/ui/developer-users/prn_bob/credential"`
		grant  = `/ui/developer-users/grant`
	)
	cases := []struct {
		name    string
		ac      *auth.AuthContext
		present []string
		absent  []string
	}{
		{"viewer", connUser(permUserView), nil, []string{rotate, revoke, remove, set, grant}},
		{"updater", connUser(permUserView, permUserUpdate), []string{rotate, revoke, set}, []string{remove, grant}},
		{"role assigner", connUser(permUserView, permAssign), []string{remove, grant}, []string{rotate, revoke, set}},
		{"administrator", anchorAdmin(), []string{rotate, revoke, remove, set, grant}, nil},
		// The self-service permission is for the caller's own credential only.
		{"own credential", func() *auth.AuthContext {
			ac := connUser(permUserView, permOwnDevCred)
			ac.PrincipalID = "prn_ann"
			return ac
		}(), []string{rotate, revoke}, []string{remove, grant, set}},
		// A client administrator may hold user:assign-roles; the platform role is the anchor's to give.
		{"client administrator", clientAdmin(permUserView, permAssign, permUserUpdate), nil, []string{remove, grant}},
	}
	for _, c := range cases {
		body := h.do(t, c.ac, req{target: "/ui/developer-users"})
		for _, p := range c.present {
			if !strings.Contains(body, p) {
				t.Errorf("%s lacks %s", c.name, p)
			}
		}
		for _, a := range c.absent {
			if strings.Contains(body, a) {
				t.Errorf("%s has %s", c.name, a)
			}
		}
	}
}

// Hiding a control is not the guard: a post by a caller without the
// permission is refused before anything is read or run.
func TestDeveloperUsersForgedPosts(t *testing.T) {
	h := newDevUsersHarness()
	rolePosts := []string{"/ui/developer-users/prn_bob/grant", "/ui/developer-users/prn_ann/remove-role"}
	credPosts := []string{"/ui/developer-users/prn_ann/credential", "/ui/developer-users/prn_ann/revoke"}
	refused := func(name string, ac *auth.AuthContext, posts ...string) {
		t.Helper()
		for _, post := range posts {
			for _, datastar := range []bool{true, false} {
				rec := h.rec(t, ac, req{method: http.MethodPost, target: post, datastar: datastar, form: url.Values{}})
				if datastar && !strings.Contains(rec.Body.String(), "fc-flash-error") {
					t.Errorf("%s, %s: no error answer: %s", name, post, rec.Body.String())
				}
				if !datastar && rec.Code != http.StatusForbidden {
					t.Errorf("%s, %s: status %d", name, post, rec.Code)
				}
				mustNotContain(t, rec.Body.String(), testSecret, "client_id=")
			}
		}
	}
	allPosts := append(slices.Clone(rolePosts), credPosts...)
	refused("viewer", connUser(permUserView), allPosts...)
	refused("no permission at all", connUser("platform:messaging:event:view"), allPosts...)
	// Role changes need user:assign-roles, not user:update (owner ruling 2026-10-11)...
	refused("updater", connUser(permUserView, permUserUpdate), rolePosts...)
	// ...and the anchor scope on top: the platform role is the anchor's to give.
	refused("client administrator", clientAdmin(permUserView, permAssign, permUserUpdate), rolePosts...)
	// Credentials need user:update (or the own-credential permission, for one's own).
	refused("role assigner", connUser(permUserView, permAssign), credPosts...)
	// The self-service permission does not reach another user's credential.
	own := connUser(permOwnDevCred)
	own.PrincipalID = "prn_ann"
	h.do(t, own, req{method: http.MethodPost, target: "/ui/developer-users/prn_bob/credential", datastar: true, form: url.Values{}})
	if len(h.ops.calls) != 0 {
		t.Fatalf("an operation ran without its permission: %v", h.ops.calls)
	}

	// Each one with its permission (the own-credential caller on their own record).
	admin := connUser(permUserView, permUserUpdate, permAssign)
	h.do(t, admin, req{method: http.MethodPost, target: "/ui/developer-users/prn_bob/grant", datastar: true, form: url.Values{}})
	h.do(t, admin, req{method: http.MethodPost, target: "/ui/developer-users/prn_ann/revoke", datastar: true, form: url.Values{}})
	h.do(t, own, req{method: http.MethodPost, target: "/ui/developer-users/prn_ann/credential", datastar: true, form: url.Values{}})
	if got := strings.Join(h.ops.calls, ","); got != "grant:prn_bob,revoke:prn_ann,set:prn_ann" {
		t.Errorf("calls = %s", got)
	}

	// A user the caller cannot reach is not found, as in the API; a malformed id never reaches the store.
	h.ops.calls = nil
	rec := h.rec(t, clientAdmin(permUserView, permUserUpdate), req{method: http.MethodPost, target: "/ui/developer-users/prn_ann/revoke", datastar: true, form: url.Values{}})
	if !strings.Contains(rec.Body.String(), "fc-flash-error") || len(h.ops.calls) != 0 {
		t.Errorf("out of scope: %s %v", rec.Body.String(), h.ops.calls)
	}
	if rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/developer-users/x'y/revoke", form: url.Values{}}); rec.Code != http.StatusNotFound {
		t.Errorf("malformed id: %d", rec.Code)
	}
}

func TestDeveloperUsersConfirmations(t *testing.T) {
	h := newDevUsersHarness()
	rotate := h.do(t, anchorAdmin(), req{target: "/ui/developer-users/prn_ann/confirm/rotate", datastar: true})
	mustContain(t, rotate, `data: mode replace`, `<div id="dev-users-dialog-slot">`, "Rotate developer secret?",
		`The current developer client secret for &#34;Ann &lt;Dev&gt;&#34; stops working immediately.`, "The new value is shown once, so copy it before closing.",
		`action="/ui/developer-users/prn_ann/credential"`, "Rotate secret", `role="alertdialog"`, "autofocus", `data-on:close="el.remove()"`)
	revoke := h.do(t, anchorAdmin(), req{target: "/ui/developer-users/prn_ann/confirm/revoke", datastar: true})
	mustContain(t, revoke, `Revoke the developer API credential for &#34;Ann &lt;Dev&gt;&#34;? Any scripts using it will stop working immediately.`, "Revoke Credential", `action="/ui/developer-users/prn_ann/revoke"`)
	remove := h.do(t, anchorAdmin(), req{target: "/ui/developer-users/prn_ann/confirm/remove", datastar: true})
	mustContain(t, remove, "Remove Developer Role", `Remove the developer role from &#34;Ann &lt;Dev&gt;&#34;? Their API credential (if set) will be revoked too`, `action="/ui/developer-users/prn_ann/remove-role"`)

	// Without scripts the dialog is open on the list.
	page := h.do(t, anchorAdmin(), req{target: "/ui/developer-users/prn_ann/confirm/revoke"})
	mustContain(t, page, "Revoke Credential", "Showing 1 to 3 of 3 developer users")

	// The dialog is the action's own check, and only for an action there is.
	for _, c := range []struct {
		ac     *auth.AuthContext
		action string
	}{{connUser(permUserView), "revoke"}, {connUser(permUserView, permUserUpdate), "remove"}, {connUser(permUserView, permAssign), "rotate"}} {
		if rec := h.rec(t, c.ac, req{target: "/ui/developer-users/prn_ann/confirm/" + c.action}); rec.Code != http.StatusForbidden {
			t.Errorf("%s: %d", c.action, rec.Code)
		}
	}
	if rec := h.rec(t, anchorAdmin(), req{target: "/ui/developer-users/prn_ann/confirm/delete"}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown action: %d", rec.Code)
	}
	if rec := h.rec(t, anchorAdmin(), req{target: "/ui/developer-users/prn_nope/confirm/revoke"}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown user: %d", rec.Code)
	}
}

func TestDeveloperUsersActions(t *testing.T) {
	h := newDevUsersHarness()
	grant := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/developer-users/prn_dee/grant", datastar: true, multipart: true, form: url.Values{}})
	mustContain(t, grant, "Granted", "Ann &lt;Dev&gt;", "Dee Plain can now hold a developer API credential", `selector #dev-users-dialog`, "mode remove")
	revoke := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/developer-users/prn_ann/revoke", datastar: true, form: url.Values{}})
	mustContain(t, revoke, "Revoked", "Developer credential revoked for Ann &lt;Dev&gt;")

	// Removing the role revokes the credential too, best effort: the removal stands when the revoke fails.
	h.ops.revokeErr = usecase.Authorization("NOPE", "no")
	removed := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/developer-users/prn_ann/remove-role", datastar: true, form: url.Values{}})
	mustContain(t, removed, "Removed", "Developer role removed from Ann &lt;Dev&gt;")
	mustNotContain(t, removed, "fc-flash-error")
	h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/developer-users/prn_bob/remove-role", datastar: true, form: url.Values{}})
	if got := strings.Join(h.ops.calls, ","); got != "grant:prn_dee,revoke:prn_ann,remove:prn_ann,revoke:prn_ann,remove:prn_bob" {
		t.Errorf("calls = %s (no credential, no revoke)", got)
	}

	// A refusal by the use case is an error toast; the dialog is left.
	h.ops.err = usecase.BusinessRule("NOT_A_DEVELOPER", "Principal does not hold the developer role")
	refused := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/developer-users/prn_bob/revoke", datastar: true, form: url.Values{}})
	mustContain(t, refused, "fc-flash-error", "Principal does not hold the developer role")

	// A native post is redirected with a flash key.
	h.ops.err, h.ops.revokeErr = nil, nil
	rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/developer-users/prn_ann/revoke", form: url.Values{}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/developer-users" {
		t.Errorf("native post: %d %s", rec.Code, rec.Header().Get("Location"))
	}
}

// The new secret is in the response that creates it, once: no-store, in the
// dialog and nowhere else (no flash, signal, URL or later page).
func TestDeveloperUsersSecretShownOnce(t *testing.T) {
	h := newDevUsersHarness()
	rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/developer-users/prn_bob/credential", datastar: true, form: url.Values{}})
	body := rec.Body.String()
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
	if n := strings.Count(body, testSecret); n != 1 {
		t.Errorf("the secret is in the response %d times: %s", n, body)
	}
	mustContain(t, body, `id="dev-secret-dialog"`, "Developer Client Secret", "Copy this secret now. It will not be shown again.",
		"client_id=prn_bob", `id="dev-secret-value"`, `data-on:cancel="evt.preventDefault(); el.remove()"`, "d.close(); d.remove()",
		"Developer credential set for Bob Builder", "Client secret copied to clipboard")
	// It is removed from the page however it is closed, and is in no signal.
	mustNotContain(t, body, "devSecret:", "secret="+testSecret, "Location")

	rotated := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/developer-users/prn_ann/credential", datastar: true, form: url.Values{}})
	mustContain(t, rotated, "Developer credential rotated for Ann &lt;Dev&gt;")

	// A native post gets the page itself, no-store, not a redirect.
	nat := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/developer-users/prn_bob/credential", form: url.Values{}})
	if nat.Code != http.StatusOK || nat.Header().Get("Cache-Control") != "no-store" || strings.Count(nat.Body.String(), testSecret) != 1 {
		t.Errorf("native post: %d %q", nat.Code, nat.Header().Get("Cache-Control"))
	}

	// Nothing else shows it: not the list, a dialog, the search.
	for _, target := range []string{"/ui/developer-users", "/ui/developer-users/grant", "/ui/developer-users/prn_bob/confirm/revoke", "/ui/developer-users/grant/search?q=bob"} {
		for _, datastar := range []bool{false, true} {
			mustNotContain(t, h.do(t, anchorAdmin(), req{target: target, datastar: datastar}), testSecret)
		}
	}

	// A failure shows no secret and no dialog.
	h.ops.err = errors.New("boom")
	failed := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/developer-users/prn_bob/credential", datastar: true, form: url.Values{}})
	mustContain(t, failed, "fc-flash-error")
	mustNotContain(t, failed, testSecret, "dev-secret-dialog", "boom")
}

func TestDeveloperUsersGrantDialog(t *testing.T) {
	h := newDevUsersHarness()
	dlg := h.do(t, anchorAdmin(), req{target: "/ui/developer-users/grant", datastar: true})
	mustContain(t, dlg, `<div id="dev-users-dialog-slot">`, "Grant Developer Role", "Search for an existing user", `action="/ui/developer-users/grant/search"`,
		"Type a name or an email to find a user.", `data-on:input__debounce.300ms`)

	found := h.do(t, anchorAdmin(), req{target: "/ui/developer-users/grant/search?q=plain", datastar: true})
	// Users who are not developers yet, matching the text; developers and other types are left out.
	mustContain(t, found, `id="dev-grant-results"`, "Dee Plain", "Eve Plain", "dee@x.test", ">DP<", `action="/ui/developer-users/prn_dee/grant"`)
	mustNotContain(t, found, "Ann &lt;", "Bob Builder")
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/developer-users/grant/search?q=nobody", datastar: true}), "No matching users")
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/developer-users/grant/search?q=", datastar: true}), "Type a name or an email")
	// The match is on the name or the email, any case.
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/developer-users/grant/search?q=FAY@OTHER", datastar: true}), "Fay Plain")

	h.store.ps = append(h.store.ps, func() principal.Principal {
		p := devUser("prn_svc", "Plain Service", "svc@x.test", nil, false, "")
		p.Type = principal.TypeService
		return p
	}())
	mustNotContain(t, h.do(t, anchorAdmin(), req{target: "/ui/developer-users/grant/search?q=plain", datastar: true}), "Plain Service")

	rec := h.rec(t, anchorAdmin(), req{target: "/ui/developer-users/grant/search?q=plain"})
	if rec.Code != http.StatusSeeOther {
		t.Errorf("search without scripts: %d", rec.Code)
	}
	// Opening it needs the right to grant.
	for _, ac := range []*auth.AuthContext{connUser(permUserView), connUser(permUserView, permUserUpdate), clientAdmin(permUserView, permAssign)} {
		if rec := h.rec(t, ac, req{target: "/ui/developer-users/grant", datastar: true}); !strings.Contains(rec.Body.String(), "fc-flash-error") {
			t.Errorf("dialog opened for %v", ac.Permissions)
		}
	}
}
