package pages

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/bff"
)

func TestPermissionsList(t *testing.T) {
	h := newRolesHarness()
	h.cat.entries = append(h.cat.entries, rolesPerm("platform:messaging:dispatch-job:retry", "Retry <dispatch> jobs"))
	body := h.do(t, anchorAdmin(), req{target: "/ui/permissions"})
	mustContain(t, body,
		`<title>Permissions · FlowCatalyst</title>`, `<h1 class="fc-page-title">Permissions</h1>`, "View all available permissions in the system",
		"Create Permission", "orders:sales:order:view", "View orders", "Retry &lt;dispatch&gt; jobs",
		`id="perm-filter-app-summary" class="fc-multiselect-summary">All Applications<`, `name="action" value="retry"`,
		"Showing 1 to 6 of 6 permissions", `href="/ui/permissions" class="fc-nav-item" title="Permissions" aria-current="page"`,
	)
}

func TestPermissionsFilters(t *testing.T) {
	h := newRolesHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/permissions?app=orders&action=create&action=approve"})
	mustContain(t, body, "orders:sales:order:create", "orders:sales:order:approve", "Showing 1 to 2 of 2 permissions")
	mustNotContain(t, body, "platform:iam:role:view</td>")
	// The context options follow the chosen applications; a chosen context
	// that is no longer an option is dropped.
	cascade := h.do(t, anchorAdmin(), req{target: "/ui/permissions?app=platform&ctx=sales&ctx=iam"})
	mustContain(t, cascade, `name="ctx" value="iam"`, "platform:iam:role:view")
	mustNotContain(t, cascade, `name="ctx" value="sales"`, "orders:sales:order:view")
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/permissions?q=audit+log"}), "platform:admin:audit:view", "Showing 1 to 1 of 1 permissions")
	live := h.do(t, anchorAdmin(), req{target: "/ui/permissions?app=orders", datastar: true})
	mustContain(t, live, `id="perms-table"`, `id="perm-filter-ctx-options"`, `id="perms-list-state"`)
}

func TestPermissionsPermissionMatrix(t *testing.T) {
	h := newRolesHarness()
	if rec := h.rec(t, rolesUser("platform:iam:user:view"), req{target: "/ui/permissions"}); rec.Code != http.StatusForbidden {
		t.Errorf("no role:view: %d", rec.Code)
	}
	mustNotContain(t, h.do(t, rolesUser(rolesViewPerm), req{target: "/ui/permissions"}), "Create Permission")
	mustNotContain(t, h.do(t, rolesClientUser(), req{target: "/ui/permissions"}), "Create Permission")
	mustContain(t, h.do(t, rolesUser(rolesViewPerm, rolesCreatePerm), req{target: "/ui/permissions"}), "Create Permission")

	form := url.Values{"application": {"orders"}, "context": {"sales"}, "aggregate": {"order"}, "action": {"refund"}}
	for _, ac := range []*auth.AuthContext{rolesUser(rolesViewPerm), rolesClientUser(), rolesUser(rolesViewPerm, rolesUpdatePerm, rolesDeletePerm)} {
		mustContain(t, h.do(t, ac, rolesPost("/ui/permissions/new", form)), "perm-form-error", "required")
		if rec := h.rec(t, ac, req{method: http.MethodPost, target: "/ui/permissions/new", form: form}); rec.Code != http.StatusForbidden {
			t.Errorf("native forged create: %d", rec.Code)
		}
		if rec := h.rec(t, ac, req{target: "/ui/permissions/new"}); rec.Code != http.StatusForbidden {
			t.Errorf("create dialog: %d", rec.Code)
		}
	}
	if len(h.cat.defined) != 0 {
		t.Fatalf("defined %v without the permission", h.cat.defined)
	}
}

func TestPermissionsCreate(t *testing.T) {
	h := newRolesHarness()
	open := h.do(t, rolesManager(), req{target: "/ui/permissions/new", datastar: true})
	mustContain(t, open, `id="perm-form"`, "Create Permission", `<option value="orders" selected>Orders</option>`, "Permission code")
	mustContain(t, h.do(t, rolesManager(), req{target: "/ui/permissions/new"}), `<dialog id="perm-form"`, "orders:sales:order:view")

	form := url.Values{"application": {"orders"}, "context": {" billing "}, "aggregate": {"invoice"}, "action": {"approve"}, "description": {" Approve <invoices> "}}
	body := h.do(t, rolesManager(), rolesPost("/ui/permissions/new", form))
	d, ok := h.cat.defined["orders:billing:invoice:approve"]
	if !ok || d == nil || *d != "Approve <invoices>" {
		t.Fatalf("defined %v", h.cat.defined)
	}
	mustContain(t, body, "Permission orders:billing:invoice:approve created", `id="perms-table"`, "perm-form")
	// No description: none is stored.
	h.do(t, rolesManager(), rolesPost("/ui/permissions/new", url.Values{"application": {"orders"}, "context": {"a"}, "aggregate": {"b"}, "action": {"c"}}))
	if d, ok := h.cat.defined["orders:a:b:c"]; !ok || d != nil {
		t.Errorf("description %v", d)
	}
	rec := h.rec(t, rolesManager(), req{method: http.MethodPost, target: "/ui/permissions/new", form: url.Values{"application": {"orders"}, "context": {"x"}, "aggregate": {"y"}, "action": {"z"}}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Set-Cookie"), "fc_ui_flash=permission-created") {
		t.Errorf("native: %d %q", rec.Code, rec.Header().Get("Set-Cookie"))
	}
}

func TestPermissionsCreateValidation(t *testing.T) {
	h := newRolesHarness()
	bad := h.do(t, rolesManager(), rolesPost("/ui/permissions/new", url.Values{"application": {"orders"}, "context": {"Bill ing"}, "aggregate": {"in_voice"}, "action": {""}}))
	mustContain(t, bad, "perm-form-context-field", "perm-form-aggregate-field", "perm-form-action-field", "Use lowercase letters, numbers and hyphens.")
	exists := h.do(t, rolesManager(), rolesPost("/ui/permissions/new", url.Values{"application": {"orders"}, "context": {"sales"}, "aggregate": {"order"}, "action": {"view"}}))
	mustContain(t, exists, "This permission already exists.")
	if len(h.cat.defined) != 0 {
		t.Fatalf("defined %v", h.cat.defined)
	}
	// A native post re-renders the list with the dialog open and the errors.
	rec := h.rec(t, rolesManager(), req{method: http.MethodPost, target: "/ui/permissions/new", form: url.Values{"application": {"orders"}, "context": {"UP"}, "aggregate": {"b"}, "action": {"c"}}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Use lowercase letters") {
		t.Errorf("native: %d", rec.Code)
	}
	h.cat.err = http.ErrAbortHandler
	mustContain(t, h.do(t, rolesManager(), rolesPost("/ui/permissions/new", url.Values{"application": {"orders"}, "context": {"a"}, "aggregate": {"b"}, "action": {"c"}})), "could not be completed")
}

func TestPermissionsSegmentRule(t *testing.T) {
	for s, want := range map[string]bool{"billing": true, "a-b-1": true, "": false, "Up": false, "a_b": false, "a b": false, "a:b": false} {
		if got := bff.ValidPermissionSegment(s); got != want {
			t.Errorf("%q: %v", s, got)
		}
	}
}
