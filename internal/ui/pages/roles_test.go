package pages

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/bff"
	"github.com/flowcatalyst/go-sdk/usecase"
)

const (
	rolesViewPerm   = "platform:iam:role:view"
	rolesCreatePerm = "platform:iam:role:create"
	rolesUpdatePerm = "platform:iam:role:update"
	rolesDeletePerm = "platform:iam:role:delete"
)

type fakeRolesStore struct{ roles []role.Role }

func (f *fakeRolesStore) FindAll(context.Context) ([]role.Role, error) {
	return append([]role.Role(nil), f.roles...), nil
}

func (f *fakeRolesStore) FindByName(_ context.Context, name string) (*role.Role, error) {
	for i := range f.roles {
		if f.roles[i].Name == name {
			r := f.roles[i]
			return &r, nil
		}
	}
	return nil, nil
}

type fakeRolesApps struct{}

func (fakeRolesApps) FindActive(context.Context) ([]application.Application, error) {
	return []application.Application{{Code: "orders", Name: "Orders"}, {Code: "platform", Name: "Platform"}}, nil
}

type fakeRolesOps struct {
	created []operations.CreateCommand
	updated []operations.UpdateCommand
	deleted []string
	synced  int
	err     error
}

func (o *fakeRolesOps) Create(_ context.Context, cmd operations.CreateCommand) (string, error) {
	o.created = append(o.created, cmd)
	return "rol_new", o.err
}

func (o *fakeRolesOps) Update(_ context.Context, cmd operations.UpdateCommand) error {
	o.updated = append(o.updated, cmd)
	return o.err
}

func (o *fakeRolesOps) Delete(_ context.Context, id string) error {
	o.deleted = append(o.deleted, id)
	return o.err
}

func (o *fakeRolesOps) SyncPlatform(context.Context) (RolesSyncResult, error) {
	o.synced++
	return RolesSyncResult{Created: 2, Removed: 1, Total: 30}, o.err
}

type fakeCatalogue struct {
	entries []bff.PermissionEntry
	defined map[string]*string
	err     error
}

func (f *fakeCatalogue) Catalogue(_ context.Context, app string) ([]bff.PermissionEntry, error) {
	var out []bff.PermissionEntry
	for _, e := range f.entries {
		if app == "" || e.Application == app {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeCatalogue) Define(_ context.Context, code string, description *string) error {
	if f.defined == nil {
		f.defined = map[string]*string{}
	}
	f.defined[code] = description
	return f.err
}

func rolesPerm(code, description string) bff.PermissionEntry {
	parts := strings.Split(code, ":")
	return bff.PermissionEntry{Permission: code, Application: parts[0], Context: parts[1], Aggregate: parts[2], Action: parts[3], Description: description}
}

type rolesHarness struct {
	store *fakeRolesStore
	ops   *fakeRolesOps
	cat   *fakeCatalogue
}

func newRolesHarness() *rolesHarness {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	desc := "Runs <b>orders</b>"
	mk := func(id, name, display string, source role.Source, perms ...string) role.Role {
		app, _, _ := strings.Cut(name, ":")
		return role.Role{ID: id, Name: name, DisplayName: display, ApplicationCode: app, Permissions: perms, Source: source, CreatedAt: day, UpdatedAt: day}
	}
	admin := mk("rol_1", "orders:admin", "Orders admin", role.SourceDatabase, "orders:sales:order:view", "orders:sales:order:create")
	admin.Description = &desc
	return &rolesHarness{
		store: &fakeRolesStore{roles: []role.Role{
			admin,
			mk("rol_2", "orders:viewer", "Orders viewer", role.SourceSDK, "orders:sales:order:view"),
			mk("rol_3", "platform:super-admin", "Super admin", role.SourceCode, "platform:*:*:*"),
			mk("rol_4", "platform:auditor", "Auditor", role.SourceDatabase, "platform:admin:audit:view", "platform:iam:role:view"),
			mk("rol_5", "orders:empty", "", role.SourceDatabase),
		}},
		ops: &fakeRolesOps{},
		cat: &fakeCatalogue{entries: []bff.PermissionEntry{
			rolesPerm("orders:sales:order:view", "View orders"),
			rolesPerm("orders:sales:order:create", ""),
			rolesPerm("orders:sales:order:approve", ""),
			rolesPerm("platform:iam:role:view", "View roles"),
			rolesPerm("platform:admin:audit:view", "View the audit log"),
		}},
	}
}

func (h *rolesHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q,
		&RolesPage{Store: h.store, Ops: h.ops, Permissions: h.cat, Apps: fakeRolesApps{}, Shell: shellForTest()},
		&PermissionsPage{Catalogue: h.cat, Apps: fakeRolesApps{}, Shell: shellForTest()})
}

func (h *rolesHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

// rolesUser is an anchor user holding the given permissions.
func rolesUser(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_r", Email: "r@x.y", Scope: auth.ScopeAnchor, Roles: []string{"r"}, Permissions: perms, PrincipalType: auth.PrincipalTypeUser}
}

// rolesClientUser holds every role permission but is not an anchor user.
func rolesClientUser() *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_c", Scope: auth.ScopeClient, Clients: []string{"clt_a"}, Roles: []string{"r"}, Permissions: []string{rolesViewPerm, rolesCreatePerm, rolesUpdatePerm, rolesDeletePerm}, PrincipalType: auth.PrincipalTypeUser}
}

func rolesManager() *auth.AuthContext {
	return rolesUser(rolesViewPerm, rolesCreatePerm, rolesUpdatePerm, rolesDeletePerm)
}

func rolesPost(target string, form url.Values) req {
	return req{method: http.MethodPost, target: target, datastar: true, form: form}
}

func TestRolesList(t *testing.T) {
	h := newRolesHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/roles"})
	mustContain(t, body,
		`<title>Roles · FlowCatalyst</title>`, `<h1 class="fc-page-title">Roles</h1>`, "Manage roles and their permissions",
		"Create Role", "Sync Platform Roles",
		`href="/ui/roles/orders:admin"`, "Orders admin", `orders:viewer`, "Runs &lt;b&gt;orders&lt;/b&gt;",
		// An empty display name falls back to the short name, as the SPA.
		`>empty<`,
		`>Admin<`, `>SDK<`, `>Code<`,
		`id="role-filter-app-summary" class="fc-multiselect-summary">All applications<`, `name="app" value="orders"`,
		`name="source" value="DATABASE"`, "Showing 1 to 5 of 5 roles", `id="role-pager"`,
		`<dialog id="role-delete"`, `<aside id="role-drawer"`,
		// The nav entries point at /ui.
		`href="/ui/roles" class="fc-nav-item" title="Roles" aria-current="page"`, `href="/ui/permissions" class="fc-nav-item"`,
	)
}

func TestRolesFilters(t *testing.T) {
	h := newRolesHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/roles?app=platform&source=DATABASE"})
	mustContain(t, body, "Auditor", "Showing 1 to 1 of 1 roles")
	mustNotContain(t, body, "Orders admin", "Super admin")
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/roles?q=SUPER"}), "Super admin", "Showing 1 to 1 of 1 roles")
	// The description is searched as well.
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/roles?q=runs"}), "Orders admin", "Showing 1 to 1 of 1 roles")
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/roles?q=zzz"}), "No roles found")
	live := h.do(t, anchorAdmin(), req{target: "/ui/roles?source=SDK&source=CODE", datastar: true})
	mustContain(t, live, "Orders viewer", "Super admin", `id="roles-table"`, `id="roles-list-state"`, `/ui/roles?source=SDK\u0026source=CODE`)
	mustNotContain(t, live, "Orders admin")
}

func TestRolesPaging(t *testing.T) {
	h := newRolesHarness()
	for i := range 120 {
		n := fmt.Sprintf("bulk:r%03d", i)
		h.store.roles = append(h.store.roles, role.Role{ID: fmt.Sprintf("rol_x%03d", i), Name: n, ApplicationCode: "bulk", Source: role.SourceSDK})
	}
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/roles?page=2"}), "Showing 101 to 125 of 125 roles")
}

func TestRolesPermissionMatrix(t *testing.T) {
	h := newRolesHarness()
	if rec := h.rec(t, rolesUser("platform:iam:user:view"), req{target: "/ui/roles"}); rec.Code != http.StatusForbidden {
		t.Errorf("no role:view: status %d", rec.Code)
	}
	viewer := h.do(t, rolesUser(rolesViewPerm), req{target: "/ui/roles"})
	mustContain(t, viewer, "Orders admin")
	mustNotContain(t, viewer, "Create Role", "Sync Platform Roles", `aria-label="Edit `, `<dialog id="role-delete"`, "Delete…")

	// Anchor scope is part of every write gate (RequireAnchor): the same
	// permissions without it show no write control.
	client := h.do(t, rolesClientUser(), req{target: "/ui/roles"})
	mustContain(t, client, "Orders admin")
	mustNotContain(t, client, "Create Role", "Sync Platform Roles", `aria-label="Edit `, `<dialog id="role-delete"`)

	// The sync needs all three permissions.
	two := h.do(t, rolesUser(rolesViewPerm, rolesCreatePerm, rolesUpdatePerm), req{target: "/ui/roles"})
	mustContain(t, two, "Create Role", `aria-label="Edit Orders admin"`)
	mustNotContain(t, two, "Sync Platform Roles", `<dialog id="role-delete"`)
	mustContain(t, h.do(t, rolesManager(), req{target: "/ui/roles"}), "Sync Platform Roles", `<dialog id="role-delete"`)

	// Edit and delete are offered for admin-created roles only.
	mgr := h.do(t, rolesManager(), req{target: "/ui/roles"})
	mustContain(t, mgr, `aria-label="Edit Orders admin"`, `aria-label="Edit Auditor"`)
	mustNotContain(t, mgr, `aria-label="Edit Orders viewer"`, `aria-label="Edit Super admin"`)
}

func TestRolesForgedPostsAreRefused(t *testing.T) {
	h := newRolesHarness()
	body := url.Values{"application": {"orders"}, "roleName": {"x"}, "permission": {"orders:sales:order:view"}, "displayName": {"x"}}
	targets := []string{
		"/ui/roles/new", "/ui/roles/sync-platform", "/ui/roles/orders:admin/edit",
		"/ui/roles/orders:admin/edit/add-permission", "/ui/roles/orders:admin/delete",
	}
	// Reading is not writing; a client user has the permissions without anchor scope.
	for _, ac := range []*auth.AuthContext{rolesUser(rolesViewPerm), rolesClientUser()} {
		for _, target := range targets {
			if got := h.do(t, ac, rolesPost(target, body)); !strings.Contains(got, "fc-flash") && !strings.Contains(got, "role-form-error") {
				t.Errorf("%s: no refusal in %q", target, got)
			}
			if rec := h.rec(t, ac, req{method: http.MethodPost, target: target, form: body}); rec.Code != http.StatusForbidden {
				t.Errorf("native %s: status %d", target, rec.Code)
			}
		}
	}
	// Each permission alone is not enough for the others' endpoints.
	h.rec(t, rolesUser(rolesViewPerm, rolesUpdatePerm, rolesDeletePerm), rolesPost("/ui/roles/new", body))
	h.rec(t, rolesUser(rolesViewPerm, rolesCreatePerm, rolesDeletePerm), rolesPost("/ui/roles/orders:admin/edit", body))
	h.rec(t, rolesUser(rolesViewPerm, rolesCreatePerm, rolesUpdatePerm), rolesPost("/ui/roles/orders:admin/delete", body))
	h.rec(t, rolesUser(rolesViewPerm, rolesCreatePerm, rolesUpdatePerm), rolesPost("/ui/roles/sync-platform", body))
	if n := len(h.ops.created) + len(h.ops.updated) + len(h.ops.deleted) + h.ops.synced; n != 0 {
		t.Fatalf("a use case ran without the permission: %+v", h.ops)
	}
}

func TestRolesRowActions(t *testing.T) {
	h := newRolesHarness()
	// The auditor role holds a platform permission this user lacks: the
	// delete is a lock with the SPA's explanation; the other admin role is
	// deletable.
	body := h.do(t, rolesManager(), req{target: "/ui/roles"})
	mustContain(t, body, "This role holds permissions you do not have, so you cannot delete it.", `data-action="/ui/roles/orders:admin/delete"`)
	mustNotContain(t, body, `data-action="/ui/roles/platform:auditor/delete"`, `data-action="/ui/roles/platform:super-admin/delete"`, `data-action="/ui/roles/orders:viewer/delete"`)
	// A super-admin holds everything: no lock.
	mustNotContain(t, h.do(t, anchorAdmin(), req{target: "/ui/roles"}), "so you cannot delete it")
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/roles"}), `data-action="/ui/roles/platform:auditor/delete"`)
}

func TestRolesDrawer(t *testing.T) {
	h := newRolesHarness()
	body := h.do(t, rolesManager(), req{target: "/ui/roles/orders:admin", datastar: true})
	mustContain(t, body, "Role Details", `<code>orders:admin</code>`, "Runs &lt;b&gt;orders&lt;/b&gt;", "Admin-created", "Permissions (2)",
		"orders:sales:order:view", "orders:sales:order:create", `href="/ui/roles/orders:admin/edit"`, ">Edit<", "2026-10-01 09:00:00 UTC", "selRole")
	// The colon may arrive percent-encoded; the page opens with the drawer.
	page := h.do(t, rolesManager(), req{target: "/ui/roles/orders%3Aadmin"})
	mustContain(t, page, "Role Details", `selRole: &#39;rol_1&#39;`, `<h1 class="fc-page-title">Roles</h1>`)
	// No edit link for a code-defined role or without role:update.
	code := h.do(t, rolesManager(), req{target: "/ui/roles/platform:super-admin", datastar: true})
	mustContain(t, code, "Code-defined", "platform:*:*:*")
	mustNotContain(t, code, ">Edit<")
	mustNotContain(t, h.do(t, rolesUser(rolesViewPerm), req{target: "/ui/roles/orders:admin", datastar: true}), ">Edit<")
	mustContain(t, h.do(t, rolesManager(), req{target: "/ui/roles/orders:empty", datastar: true}), "This role has no permissions assigned", "Permissions (0)")
	if rec := h.rec(t, rolesUser("platform:iam:user:view"), req{target: "/ui/roles/orders:admin"}); rec.Code != http.StatusForbidden {
		t.Errorf("no role:view: %d", rec.Code)
	}
	for _, name := range []string{"orders:nope", "nocolon", "orders:ad%20min", "orders:%3Cscript%3E"} {
		if rec := h.rec(t, rolesManager(), req{target: "/ui/roles/" + name}); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d", name, rec.Code)
		}
	}
}

func TestRolesCreate(t *testing.T) {
	h := newRolesHarness()
	form := h.do(t, rolesManager(), req{target: "/ui/roles/new", datastar: true})
	mustContain(t, form, `id="role-form"`, "Create Role", `<option value="orders" selected>Orders</option>`, `Will be prefixed with application code`)
	native := h.do(t, rolesManager(), req{target: "/ui/roles/new"})
	mustContain(t, native, `<dialog id="role-form"`, "Orders admin")

	body := h.do(t, rolesManager(), rolesPost("/ui/roles/new", url.Values{"application": {"orders"}, "roleName": {"picker"}, "displayName": {""}, "description": {"Picks <orders>"}}))
	if len(h.ops.created) != 1 {
		t.Fatalf("created %v", h.ops.created)
	}
	c := h.ops.created[0]
	if c.ApplicationCode != "orders" || c.RoleName != "picker" || c.DisplayName != "picker" || c.Description == nil || *c.Description != "Picks <orders>" || len(c.Permissions) != 0 || c.ClientManaged {
		t.Errorf("command %+v", c)
	}
	mustContain(t, body, "Role created successfully", "role-form", `id="roles-table"`)

	// Native post: redirect to the list with the flash.
	rec := h.rec(t, rolesManager(), req{method: http.MethodPost, target: "/ui/roles/new", form: url.Values{"application": {"orders"}, "roleName": {"p2"}}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/roles" || !strings.Contains(rec.Header().Get("Set-Cookie"), "fc_ui_flash=role-created") {
		t.Errorf("native: %d %q %q", rec.Code, rec.Header().Get("Location"), rec.Header().Get("Set-Cookie"))
	}
}

func TestRolesCreateErrorsLandOnTheField(t *testing.T) {
	h := newRolesHarness()
	for _, c := range []struct{ code, field string }{
		{"ROLE_EXISTS", "role-form-role-name-field"},
		{"INVALID_ROLE_NAME", "role-form-role-name-field"},
		{"APPLICATION_NOT_FOUND", "role-form-application-field"},
		{"DISPLAY_NAME_REQUIRED", "role-form-display-name-field"},
	} {
		h.ops.err = usecase.Conflict(c.code, "The "+c.code+" message")
		body := h.do(t, rolesManager(), rolesPost("/ui/roles/new", url.Values{"application": {"orders"}, "roleName": {"admin"}}))
		mustContain(t, body, c.field, "The "+c.code+" message")
	}
	h.ops.err = usecase.Authorization("SOMETHING", "Not allowed here")
	mustContain(t, h.do(t, rolesManager(), rolesPost("/ui/roles/new", url.Values{"application": {"orders"}, "roleName": {"admin"}})), "Not allowed here")
	// A native post re-renders the list with the dialog open and the status.
	h.ops.err = usecase.Validation("ROLE_NAME_REQUIRED", "roleName is required")
	rec := h.rec(t, rolesManager(), req{method: http.MethodPost, target: "/ui/roles/new", form: url.Values{"application": {"orders"}}})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "RoleName is required") {
		t.Errorf("native error: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRolesDelete(t *testing.T) {
	h := newRolesHarness()
	body := h.do(t, rolesManager(), rolesPost("/ui/roles/orders:admin/delete", url.Values{"drawer": {"rol_1"}}))
	if len(h.ops.deleted) != 1 || h.ops.deleted[0] != "rol_1" {
		t.Fatalf("deleted %v", h.ops.deleted)
	}
	mustContain(t, body, "Role deleted successfully", `id="roles-table"`, "selRole")
	rec := h.rec(t, rolesManager(), req{method: http.MethodPost, target: "/ui/roles/orders:empty/delete", form: url.Values{}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Set-Cookie"), "fc_ui_flash=role-deleted") {
		t.Errorf("native: %d %q", rec.Code, rec.Header().Get("Set-Cookie"))
	}
	// Only admin-created roles are deleted (the SPA offers nothing else).
	h.ops.deleted = nil
	for _, name := range []string{"platform:super-admin", "orders:viewer"} {
		mustContain(t, h.do(t, rolesManager(), rolesPost("/ui/roles/"+name+"/delete", url.Values{})), "Only admin-created roles can be deleted")
	}
	h.do(t, rolesManager(), rolesPost("/ui/roles/orders:nope/delete", url.Values{}))
	if len(h.ops.deleted) != 0 {
		t.Errorf("deleted %v", h.ops.deleted)
	}
	// The use case's refusal (the permission ceiling) is shown as it is.
	h.ops.err = usecase.Authorization(role.PermissionCeilingCode, "You may only add or remove permissions you hold yourself: platform:admin:audit:view")
	mustContain(t, h.do(t, rolesManager(), rolesPost("/ui/roles/platform:auditor/delete", url.Values{})), "You may only add or remove permissions you hold yourself")
}

func TestRolesSyncPlatform(t *testing.T) {
	h := newRolesHarness()
	body := h.do(t, rolesManager(), rolesPost("/ui/roles/sync-platform", url.Values{}))
	if h.ops.synced != 1 {
		t.Fatalf("synced %d", h.ops.synced)
	}
	mustContain(t, body, "Platform Roles Synced", "2 created, 1 removed (30 total)")
	rec := h.rec(t, rolesManager(), req{method: http.MethodPost, target: "/ui/roles/sync-platform", form: url.Values{}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Set-Cookie"), "fc_ui_flash=roles-synced") {
		t.Errorf("native: %d %q", rec.Code, rec.Header().Get("Set-Cookie"))
	}
	if got := rolesSyncDetail(RolesSyncResult{Total: 30}); got != "30 roles up to date" {
		t.Errorf("no change: %q", got)
	}
}

func TestRolesEditPage(t *testing.T) {
	h := newRolesHarness()
	body := h.do(t, rolesManager(), req{target: "/ui/roles/orders:admin/edit"})
	mustContain(t, body,
		"Edit Role", `<p class="fc-page-subtitle">orders:admin</p>`, `value="Orders admin"`, "Runs &lt;b&gt;orders&lt;/b&gt;",
		`name="permission" value="orders:sales:order:view" checked`, `name="permission" value="orders:sales:order:approve"`,
		"New permission", `formaction="/ui/roles/orders:admin/edit/add-permission"`, "Select all", "Client Managed",
		`action="/ui/roles/orders:admin/edit"`,
	)
	// Only the role's application's permissions are offered.
	mustNotContain(t, body, "platform:iam:role:view")

	// A platform role has no new-permission builder; the permissions the user
	// does not hold are locked, shown, and the ones on the role are posted back.
	plat := h.do(t, rolesManager(), req{target: "/ui/roles/platform:auditor/edit"})
	mustNotContain(t, plat, "New permission", `type="checkbox" name="permission" value="platform:admin:audit:view"`)
	mustContain(t, plat, `name="permission" value="platform:iam:role:view" checked`, "1 locked permissions", "Locked permissions are ones you do not hold",
		`<input type="hidden" name="permission" value="platform:admin:audit:view">`, "on this role")
	// A super-admin holds everything: nothing is locked.
	mustNotContain(t, h.do(t, anchorAdmin(), req{target: "/ui/roles/platform:auditor/edit"}), "locked permissions")
}

func TestRolesEditPageRefusals(t *testing.T) {
	h := newRolesHarness()
	// The SPA warns and returns to the role.
	for _, c := range []struct {
		name string
		ac   *auth.AuthContext
		flag string
	}{
		{"orders:viewer", rolesManager(), "role-not-admin-created"},
		{"platform:super-admin", rolesManager(), "role-not-admin-created"},
		{"orders:admin", rolesUser(rolesViewPerm), "role-no-update"},
		{"orders:admin", rolesClientUser(), "role-no-update"},
	} {
		rec := h.rec(t, c.ac, req{target: "/ui/roles/" + c.name + "/edit"})
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/roles/"+c.name || !strings.Contains(rec.Header().Get("Set-Cookie"), "fc_ui_flash="+c.flag) {
			t.Errorf("%s: %d %q %q", c.name, rec.Code, rec.Header().Get("Location"), rec.Header().Get("Set-Cookie"))
		}
	}
	if rec := h.rec(t, rolesUser("platform:iam:user:view"), req{target: "/ui/roles/orders:admin/edit"}); rec.Code != http.StatusForbidden {
		t.Errorf("no role:view: %d", rec.Code)
	}
	if rec := h.rec(t, rolesManager(), req{target: "/ui/roles/orders:nope/edit"}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown role: %d", rec.Code)
	}
}

func TestRolesUpdate(t *testing.T) {
	h := newRolesHarness()
	form := url.Values{
		"displayName": {"Orders chief"}, "description": {""}, "clientManaged": {"true"},
		"permission": {"orders:sales:order:view", "orders:sales:order:approve", "orders:sales:order:view"},
	}
	rec := h.rec(t, rolesManager(), rolesPost("/ui/roles/orders:admin/edit", form))
	if len(h.ops.updated) != 1 {
		t.Fatalf("updated %v", h.ops.updated)
	}
	u := h.ops.updated[0]
	if u.ID != "rol_1" || *u.DisplayName != "Orders chief" || *u.Description != "" || !*u.ClientManaged || strings.Join(u.Permissions, ",") != "orders:sales:order:view,orders:sales:order:approve" {
		t.Errorf("command %+v", u)
	}
	if !strings.Contains(rec.Body.String(), "/ui/roles/orders:admin") || !strings.Contains(rec.Header().Get("Set-Cookie"), "fc_ui_flash=role-updated") {
		t.Errorf("redirect: %s %q", rec.Body.String(), rec.Header().Get("Set-Cookie"))
	}
	native := h.rec(t, rolesManager(), req{method: http.MethodPost, target: "/ui/roles/orders:admin/edit", form: form})
	if native.Code != http.StatusSeeOther || native.Header().Get("Location") != "/ui/roles/orders:admin" {
		t.Errorf("native: %d %q", native.Code, native.Header().Get("Location"))
	}

	// Clearing every permission sends an empty list, not "unchanged".
	h.ops.updated = nil
	h.do(t, rolesManager(), rolesPost("/ui/roles/orders:admin/edit", url.Values{"displayName": {"x"}}))
	if h.ops.updated[0].Permissions == nil || len(h.ops.updated[0].Permissions) != 0 {
		t.Errorf("permissions %#v", h.ops.updated[0].Permissions)
	}
	// An unchecked client-managed switch clears the flag.
	if *h.ops.updated[0].ClientManaged {
		t.Error("client managed stayed on")
	}
}

func TestRolesUpdateErrors(t *testing.T) {
	h := newRolesHarness()
	blank := h.rec(t, rolesManager(), rolesPost("/ui/roles/orders:admin/edit", url.Values{"displayName": {"  "}}))
	mustContain(t, blank.Body.String(), "Display name is required.", "role-edit-display-name-field")
	if len(h.ops.updated) != 0 {
		t.Fatal("updated with a blank display name")
	}
	// The ceiling and confinement refusals land on the permissions field.
	for _, code := range []string{role.PermissionCeilingCode, role.OutsideApplicationCode} {
		h.ops.err = usecase.Authorization(code, "Refused: "+code)
		body := h.do(t, rolesManager(), rolesPost("/ui/roles/platform:auditor/edit", url.Values{"displayName": {"Auditor"}, "permission": {"platform:iam:role:view"}}))
		mustContain(t, body, "Refused: "+code, rolesPickerField)
	}
	h.ops.err = usecase.Authorization(role.PermissionCeilingCode, "You may only add or remove permissions you hold yourself")
	native := h.rec(t, rolesManager(), req{method: http.MethodPost, target: "/ui/roles/platform:auditor/edit", form: url.Values{"displayName": {"Auditor"}}})
	if native.Code != http.StatusForbidden || !strings.Contains(native.Body.String(), "You may only add or remove permissions you hold yourself") {
		t.Errorf("native: %d", native.Code)
	}
	// A role that is not admin-created is not saved, whatever the page offered.
	h.ops.err = nil
	h.ops.updated = nil
	mustContain(t, h.do(t, rolesManager(), rolesPost("/ui/roles/orders:viewer/edit", url.Values{"displayName": {"x"}})), "Only admin-created roles can be edited")
	if len(h.ops.updated) != 0 {
		t.Error("updated an SDK role")
	}
}

func TestRolesAddPermission(t *testing.T) {
	h := newRolesHarness()
	add := func(ctx, agg, act string, picked ...string) string {
		return h.do(t, rolesManager(), rolesPost("/ui/roles/orders:admin/edit/add-permission", url.Values{
			"displayName": {"Orders admin"}, "permission": picked, "newContext": {ctx}, "newAggregate": {agg}, "newAction": {act},
		}))
	}
	body := add("billing", "invoice", "pay", "orders:sales:order:view")
	mustContain(t, body, rolesPickerField, `value="orders:billing:invoice:pay"`, `id="role-edit-newperm"`, "Auto-formatted to lowercase")
	if !strings.Contains(body, `value="orders:billing:invoice:pay" checked`) {
		t.Errorf("the new permission is not checked: %s", body)
	}
	mustContain(t, body, `value="orders:sales:order:view" checked`)
	if len(h.ops.updated) != 0 {
		t.Error("adding saved the role")
	}
	// It reaches the role only when saved: the posted list carries it.
	h.do(t, rolesManager(), rolesPost("/ui/roles/orders:admin/edit", url.Values{"displayName": {"x"}, "permission": {"orders:billing:invoice:pay"}}))
	if got := h.ops.updated[0].Permissions; len(got) != 1 || got[0] != "orders:billing:invoice:pay" {
		t.Errorf("saved %v", got)
	}

	mustContain(t, add("Bill ing", "invoice", "pay"), "Each part must be lowercase letters, numbers and hyphens.")
	mustContain(t, add("sales", "order", "view"), "orders:sales:order:view already exists.")
	mustContain(t, add("sales", "", "view"), "Each part must be lowercase")
	// Platform permissions are code-defined, and the gate is the editor's.
	mustContain(t, h.do(t, rolesManager(), rolesPost("/ui/roles/platform:auditor/edit/add-permission", url.Values{"newContext": {"a"}, "newAggregate": {"b"}, "newAction": {"c"}})), "Platform permissions are code-defined.")
	rec := h.rec(t, rolesUser(rolesViewPerm), rolesPost("/ui/roles/orders:admin/edit/add-permission", url.Values{"newContext": {"a"}, "newAggregate": {"b"}, "newAction": {"c"}}))
	mustNotContain(t, rec.Body.String(), "orders:a:b:c")
	// Without JavaScript the Add button posts the page and gets the page back.
	native := h.rec(t, rolesManager(), req{method: http.MethodPost, target: "/ui/roles/orders:admin/edit/add-permission", form: url.Values{"newContext": {"a"}, "newAggregate": {"b"}, "newAction": {"c"}}})
	if native.Code != http.StatusOK {
		t.Fatalf("native: %d", native.Code)
	}
	mustContain(t, native.Body.String(), "Edit Role", `value="orders:a:b:c" checked`)
}

func TestRolesPickerLocks(t *testing.T) {
	user := rolesUser(rolesViewPerm, "platform:admin:audit:view")
	p := newRolesPicker(user, []bff.PermissionEntry{rolesPerm("platform:iam:role:view", ""), rolesPerm("platform:admin:audit:view", ""), rolesPerm("orders:a:b:c", "")},
		[]string{"platform:iam:role:view", "platform:iam:user:delete"}, []string{"platform:admin:audit:view", "orders:a:b:c", "platform:iam:user:delete"})
	if len(p.Locked) != 1 || p.Locked[0].Code != "platform:iam:user:delete" || !p.Locked[0].Held {
		t.Errorf("locked %+v", p.Locked)
	}
	if len(p.Options) != 3 || strings.Join(p.Selected, ",") != "orders:a:b:c,platform:admin:audit:view" {
		t.Errorf("options %v selected %v", p.Options, p.Selected)
	}
}
