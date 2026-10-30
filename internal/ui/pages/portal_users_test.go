package pages

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestPortalUsersList(t *testing.T) {
	h := newPortalHarness()
	jit := portalidentity.SourceJIT
	h.store.identities[0].Source = jit
	h.store.identities[1].PasswordHash = nil // never signed in: invited
	exp := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Minute)
	h.store.identities[1].InviteExpiresAt = &exp
	h.store.identities[2].Status = portalidentity.StatusDisabled
	body := h.do(t, anchorAdmin(), req{target: "/ui/portal-users?client=clt_acme"})
	mustContain(t, body,
		`<title>Portal Users · FlowCatalyst</title>`,
		`href="/ui/portal-users" class="fc-nav-item" title="Portal Users" aria-current="page"`,
		`<h1 class="fc-page-title">Portal Users</h1>`,
		`<option value="clt_acme" selected>Acme Corp</option>`,
		"Search email or name (starts with)", `name="client" value="clt_acme"`,
		"ann@acme.test", "Ann &lt;Admin&gt;", "bob@acme.test", "cy@acme.test",
		"Invited", "Suspended", "Active", `title="Invite expires `+exp.Format("2006-01-02 15:04")+` UTC"`,
		"SSO sign-in", "Invite",
		"No portal app", // bob
		"Customer Portal", "Supplier Portal",
		`data-action="/ui/portal-users/ptu_ann/apps/customers/revoke"`,
		`<dialog id="portal-user-suspend"`, `<dialog id="portal-user-reactivate"`, `<dialog id="portal-user-revoke"`, `<dialog id="portal-user-delete"`,
		"Showing 1 to 3 of 3 portal users", `id="portal-users-pager"`,
	)
	mustNotContain(t, body, "gus@globex.test")
	// Newest first, as the API lists.
	if a, b, c := strings.Index(body, "ann@acme"), strings.Index(body, "bob@acme"), strings.Index(body, "cy@acme"); a >= b || b >= c {
		t.Errorf("rows are not newest first: %d %d %d", a, b, c)
	}
	mustContain(t, menuOfPortal(t, body, "portal-user-ptu_ann-actions"), "Suspend…", `data-action="/ui/portal-users/ptu_ann/suspend"`, "Delete…")
	mustContain(t, menuOfPortal(t, body, "portal-user-ptu_cy-actions"), "Reactivate…", `data-action="/ui/portal-users/ptu_cy/reactivate"`)
	mustNotContain(t, menuOfPortal(t, body, "portal-user-ptu_cy-actions"), "Suspend…")
	// Ann holds the only active app she can have (customers); suppliers is
	// inactive, so there is nothing to grant her; Bob can be granted customers.
	mustContain(t, body, `href="/ui/portal-users/ptu_bob/grant?client=clt_acme"`)
	mustNotContain(t, body, `href="/ui/portal-users/ptu_ann/grant`)
	mustContain(t, body, "There is no other active portal app to grant.")
}

func TestPortalUsersFilters(t *testing.T) {
	h := newPortalHarness()
	// The search is a prefix on email or name, as the API's.
	body := h.do(t, anchorAdmin(), req{target: "/ui/portal-users?client=clt_acme&q=C"})
	mustContain(t, body, "cy@acme.test", "Showing 1 to 1 of 1 portal users")
	mustNotContain(t, body, "ann@acme.test")
	// An app by code.
	body = h.do(t, anchorAdmin(), req{target: "/ui/portal-users?client=clt_acme&app=suppliers"})
	mustContain(t, body, "cy@acme.test")
	mustNotContain(t, body, "ann@acme.test", "bob@acme.test")
	// The unassigned sentinel.
	body = h.do(t, anchorAdmin(), req{target: "/ui/portal-users?client=clt_acme&app=__unassigned__"})
	mustContain(t, body, "bob@acme.test", `<option value="__unassigned__" selected>No portal app (unassigned)</option>`)
	mustNotContain(t, body, "ann@acme.test")
	// A code that is not the client's app (another client's, or typed) is
	// dropped, not an error.
	body = h.do(t, anchorAdmin(), req{target: "/ui/portal-users?client=clt_acme&app=globex-portal"})
	mustContain(t, body, "ann@acme.test", "bob@acme.test", "cy@acme.test")
	// Nothing matches.
	body = h.do(t, anchorAdmin(), req{target: "/ui/portal-users?client=clt_acme&q=zzz"})
	mustContain(t, body, "No portal users match.", "Clear filters")
	// No client chosen yet.
	body = h.do(t, anchorAdmin(), req{target: "/ui/portal-users"})
	mustContain(t, body, "Select a client to view its portal users.")
	mustNotContain(t, body, "ann@acme.test", `id="portal-users-pager"`)
}

func TestPortalUsersPaging(t *testing.T) {
	h := newPortalHarness()
	for n := range 30 {
		id := strconv.Itoa(n)
		h.store.identities = append(h.store.identities, portalUser("ptu_p"+id, "clt_acme", "p"+id+"@acme.test", "", portalDay.Add(-time.Duration(n+1)*time.Hour), "pta_cust"))
	}
	// 33 users, 25 a page: the SPA's default page size.
	body := h.do(t, anchorAdmin(), req{target: "/ui/portal-users?client=clt_acme"})
	mustContain(t, body, "ann@acme.test", "p21@acme.test", "page=2")
	mustNotContain(t, body, "p22@acme.test")
	body = h.do(t, anchorAdmin(), req{target: "/ui/portal-users?client=clt_acme&page=2"})
	mustContain(t, body, "p22@acme.test", "p29@acme.test")
	mustNotContain(t, body, "ann@acme.test")
	// Past the end shows the last page; a bigger page size takes it all.
	body = h.do(t, anchorAdmin(), req{target: "/ui/portal-users?client=clt_acme&page=9"})
	mustContain(t, body, "p29@acme.test")
	body = h.do(t, anchorAdmin(), req{target: "/ui/portal-users?client=clt_acme&rows=50"})
	mustContain(t, body, "ann@acme.test", "p29@acme.test")
	// A live (Datastar) answer patches the table, the pager and the URL.
	rec := h.serve(t, anchorAdmin(), req{target: "/ui/portal-users?client=clt_acme&app=customers&q=p1", datastar: true})
	mustContain(t, rec.Body.String(), `id="portal-users-region"`, `id="portal-users-pager"`, "p10@acme.test", "datastar-patch-elements")
	mustContain(t, rec.Body.String(), "app=customers", "client=clt_acme")
	_ = ds.RequestHeader
}

func TestPortalUsersPermissions(t *testing.T) {
	h := newPortalHarness()
	if rec := h.serve(t, portalCaller([]string{"clt_acme"}, "platform:messaging:event:view"), req{target: "/ui/portal-users?client=clt_acme"}); rec.Code != http.StatusForbidden {
		t.Errorf("no portal permission: status %d", rec.Code)
	}
	if rec := h.serve(t, portalCaller([]string{"clt_globex"}, permPortalView, permPortalManage), req{target: "/ui/portal-users?client=clt_acme"}); rec.Code != http.StatusForbidden {
		t.Errorf("another client: status %d", rec.Code)
	}
	viewer := h.do(t, portalCaller([]string{"clt_acme"}, permPortalView), req{target: "/ui/portal-users?client=clt_acme"})
	mustContain(t, viewer, "ann@acme.test", "Customer Portal")
	mustNotContain(t, viewer, `<dialog id="portal-user-`, "Delete…", "Suspend…", "Grant portal app", "Actions", `aria-label="Remove `)
	manager := h.do(t, portalCaller([]string{"clt_acme"}, permPortalManage), req{target: "/ui/portal-users?client=clt_acme"})
	mustContain(t, manager, `<dialog id="portal-user-delete"`, "Delete…", `aria-label="Remove Customer Portal access of ann@acme.test"`)
	// A client user lands on their client without choosing.
	own := h.do(t, portalCaller([]string{"clt_globex"}, permPortalView), req{target: "/ui/portal-users"})
	mustContain(t, own, "gus@globex.test")
	mustNotContain(t, own, "ann@acme.test")
}

func TestPortalUsersForgedPostsAreRefused(t *testing.T) {
	h := newPortalHarness()
	callers := map[string]*auth.AuthContext{
		"viewer":                    portalCaller([]string{"clt_acme"}, permPortalView),
		"manager of another client": portalCaller([]string{"clt_globex"}, permPortalView, permPortalManage),
		"no permission":             portalCaller([]string{"clt_acme"}),
	}
	for name, ac := range callers {
		for _, q := range []req{
			postForm("/ui/portal-users/ptu_ann/suspend", "client", "clt_acme"),
			postForm("/ui/portal-users/ptu_cy/reactivate", "client", "clt_acme"),
			postForm("/ui/portal-users/ptu_ann/delete", "client", "clt_acme"),
			postForm("/ui/portal-users/ptu_ann/apps/customers/revoke", "client", "clt_acme"),
			postForm("/ui/portal-users/ptu_bob/grant", "client", "clt_acme", "app", "customers"),
		} {
			body := h.do(t, ac, q)
			if len(h.ops.calls) != 0 {
				t.Fatalf("%s: %s ran %v", name, q.target, h.ops.calls)
			}
			if lower := strings.ToLower(body); !strings.Contains(lower, "permission required") && !strings.Contains(lower, "no access to this client") {
				t.Errorf("%s: %s: the refusal is not shown:\n%s", name, q.target, body)
			}
		}
		if rec := h.serve(t, ac, req{target: "/ui/portal-users/ptu_bob/grant?client=clt_acme"}); rec.Code != http.StatusForbidden {
			t.Errorf("%s: GET grant dialog: status %d", name, rec.Code)
		}
	}
	// A manager of acme naming globex as the client is refused too, and an
	// identity of another client is not found under the client named.
	manager := portalCaller([]string{"clt_acme"}, permPortalView, permPortalManage)
	h.do(t, manager, postForm("/ui/portal-users/ptu_ann/delete", "client", "clt_globex"))
	body := h.do(t, manager, postForm("/ui/portal-users/ptu_gus/delete", "client", "clt_acme"))
	if len(h.ops.calls) != 0 || !strings.Contains(body, "fc-flash-error") {
		t.Errorf("another client's identity: ran %v:\n%s", h.ops.calls, body)
	}
}

func TestPortalUserStatusAndDelete(t *testing.T) {
	h := newPortalHarness()
	body := h.do(t, anchorAdmin(), postForm("/ui/portal-users/ptu_ann/suspend", "client", "clt_acme"))
	mustContain(t, body, "ann@acme.test suspended", "Suspended", `id="portal-users-region"`)
	body = h.do(t, anchorAdmin(), postForm("/ui/portal-users/ptu_ann/reactivate", "client", "clt_acme"))
	mustContain(t, body, "ann@acme.test reactivated")
	body = h.do(t, anchorAdmin(), postForm("/ui/portal-users/ptu_bob/delete", "client", "clt_acme"))
	mustContain(t, body, "bob@acme.test deleted")
	mustNotContain(t, body, `id="portal-user-ptu_bob"`)
	want := []string{"status:clt_acme:ptu_ann:DISABLED", "status:clt_acme:ptu_ann:ACTIVE", "delete-user:clt_acme:ptu_bob"}
	if strings.Join(h.ops.calls, " ") != strings.Join(want, " ") {
		t.Errorf("calls %v, want %v", h.ops.calls, want)
	}
	// The list the user was on is the one re-rendered (its filter kept).
	q := postForm("/ui/portal-users/ptu_cy/suspend", "client", "clt_acme")
	q.referer = "http://x/ui/portal-users?client=clt_acme&app=suppliers"
	body = h.do(t, anchorAdmin(), q)
	mustContain(t, body, "cy@acme.test")
	mustNotContain(t, body, "ann@acme.test")
	// A refusal of the use case is shown.
	h.ops.err = usecase.NotFound("IDENTITY_NOT_FOUND", "portal identity not found")
	mustContain(t, h.do(t, anchorAdmin(), postForm("/ui/portal-users/ptu_ann/delete", "client", "clt_acme")), "fc-flash-error", "portal identity not found")
	// A native post redirects to the list on the client.
	h.ops.err = nil
	n := postForm("/ui/portal-users/ptu_ann/suspend", "client", "clt_acme")
	n.datastar = false
	rec := h.serve(t, anchorAdmin(), n)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/portal-users?client=clt_acme" {
		t.Errorf("native suspend: %d %s", rec.Code, rec.Header().Get("Location"))
	}
}

func TestPortalUserGrantAndRevoke(t *testing.T) {
	h := newPortalHarness()
	h.store.apps[1].Active = true // suppliers
	dialog := h.do(t, anchorAdmin(), req{target: "/ui/portal-users/ptu_ann/grant?client=clt_acme", datastar: true})
	mustContain(t, dialog, "Grant portal app", "ann@acme.test", `action="/ui/portal-users/ptu_ann/grant"`, `name="client" value="clt_acme"`,
		`<option value="suppliers" selected>Supplier Portal (suppliers)</option>`)
	mustNotContain(t, dialog, `value="customers"`, "globex-portal") // held, and another client's

	body := h.do(t, anchorAdmin(), postForm("/ui/portal-users/ptu_ann/grant", "client", "clt_acme", "app", "suppliers"))
	mustContain(t, body, "ann@acme.test can now sign in to Supplier Portal", "Supplier Portal")
	if h.ops.calls[0] != "grant:clt_acme:ptu_ann:pta_supp" {
		t.Errorf("calls %v", h.ops.calls)
	}

	body = h.do(t, anchorAdmin(), postForm("/ui/portal-users/ptu_cy/apps/suppliers/revoke", "client", "clt_acme"))
	mustContain(t, body, "cy@acme.test no longer has access to Supplier Portal")
	if h.ops.calls[1] != "revoke:clt_acme:ptu_cy:pta_supp" {
		t.Errorf("calls %v", h.ops.calls)
	}

	// The API's rules: an unknown app is not found, an inactive one refused.
	n := len(h.ops.calls)
	mustContain(t, h.do(t, anchorAdmin(), postForm("/ui/portal-users/ptu_bob/grant", "client", "clt_acme", "app", "nope")), "PortalApp not found")
	h.store.apps[0].Active = false
	mustContain(t, h.do(t, anchorAdmin(), postForm("/ui/portal-users/ptu_bob/grant", "client", "clt_acme", "app", "customers")), `id="portal-grant-form-app-err"`, "is inactive")
	mustContain(t, h.do(t, anchorAdmin(), postForm("/ui/portal-users/ptu_bob/grant", "client", "clt_acme", "app", "globex-portal")), "PortalApp not found")
	if len(h.ops.calls) != n {
		t.Errorf("a refused grant ran %v", h.ops.calls[n:])
	}
	// Nothing to grant: no dialog.
	h.store.apps[1].Active = false
	if rec := h.serve(t, anchorAdmin(), req{target: "/ui/portal-users/ptu_bob/grant?client=clt_acme"}); !strings.Contains(rec.Body.String(), "no other active portal app") {
		t.Errorf("no grantable app: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPortalUsersEscapeData(t *testing.T) {
	h := newPortalHarness()
	h.store.identities[0].Email = `"><img src=x onerror=alert(1)>@acme.test`
	body := h.do(t, anchorAdmin(), req{target: "/ui/portal-users?client=clt_acme"})
	mustNotContain(t, body, `<img src=x`)
}
