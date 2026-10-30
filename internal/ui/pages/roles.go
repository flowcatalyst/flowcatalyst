package pages

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// RolesStore is the slice of the role repository the page reads.
type RolesStore interface {
	FindAll(ctx context.Context) ([]role.Role, error)
	FindByName(ctx context.Context, name string) (*role.Role, error)
}

// RolesApps is the slice of the application repository the page needs: the
// active applications the application filter and the create form offer (the
// SPA's GET /bff/roles/filters/applications).
type RolesApps interface {
	FindActive(ctx context.Context) ([]application.Application, error)
}

// Roles is the roles page (/ui/roles), mirroring the SPA's RoleListPage,
// RoleDetailDrawer and RoleEditPage. Every route makes the check of the BFF
// endpoint the SPA calls (GET /bff/roles: role:view; the writes: the role
// permission for the operation, with the anchor check on top) and runs its use
// case (RolesCommands), whose permission ceiling and application confinement
// refuse what the page leaves out.
type RolesPage struct {
	Store       RolesStore
	Ops         RolesCommands
	Permissions PermissionCatalogue
	Apps        RolesApps
	Shell       *shell.Shell
}

const (
	rolesPath    = "/ui/roles"
	rolesForm    = "roles-list"
	rolesTableID = "roles-table"

	rolesDrawerID   = "role-drawer"
	rolesDrawerSig  = "selRole"
	rolesDialogSlot = "role-dialog-slot"
	rolesDeleteDlg  = "role-delete"

	rolesLiveGet = "@get('/ui/roles', {contentType: 'form'})"
)

// Facet query parameters: the SPA's URL keys (RoleListPage: app, source).
const (
	rolesFacetApp    = "app"
	rolesFacetSource = "source"
)

var rolesList = &listSpec{
	path:        rolesPath,
	sortKeys:    []string{"name"},
	statusOK:    func(string) bool { return false },
	facets:      []string{rolesFacetApp, rolesFacetSource},
	rows:        []int{50, 100, 250, 500},
	defaultRows: 100,
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *RolesPage) Mount(r chi.Router) {
	r.Get("/roles", h.list)
	r.Get("/roles/new", h.createForm)
	r.Get("/roles/{name}", h.detail)
	r.Get("/roles/{name}/edit", h.editPage)
	post := r.With(parseForm)
	post.Post("/roles/new", h.create)
	post.Post("/roles/sync-platform", h.syncPlatform)
	post.Post("/roles/{name}/edit", h.update)
	post.Post("/roles/{name}/edit/add-permission", h.addPermission)
	post.Post("/roles/{name}/delete", h.delete)
}

// ── permissions ────────────────────────────────────────────────────────

// rolesPerms are the checks of the endpoints the SPA calls: create (POST
// /bff/roles) role:create, edit (PUT) role:update, delete role:delete, "Sync
// Platform Roles" all three; each with anchor scope on top.
type rolesPerms struct {
	Create, Update, Delete, Sync bool
}

func rolesPermsFor(ac *auth.AuthContext) rolesPerms {
	return rolesPerms{
		Create: rolesWrite(ac, auth.CanCreateRoles) == nil,
		Update: rolesWrite(ac, auth.CanUpdateRoles) == nil,
		Delete: rolesWrite(ac, auth.CanDeleteRoles) == nil,
		Sync:   rolesWrite(ac, auth.CanCreateRoles, auth.CanUpdateRoles, auth.CanDeleteRoles) == nil,
	}
}

// rolesWrite is the BFF's write gate: every permission check, then the anchor
// check of the handler.
func rolesWrite(ac *auth.AuthContext, checks ...func(*auth.AuthContext) error) error {
	for _, check := range checks {
		if err := check(ac); err != nil {
			return err
		}
	}
	return auth.RequireAnchor(ac)
}

// ── reading ────────────────────────────────────────────────────────────

// rolesName reads and checks the {name} path parameter. Names contain ":", so
// the segment may arrive percent-encoded; one that is not a canonical role
// name is not a role (and never reaches a query).
func rolesName(r *http.Request) (string, error) {
	name := chi.URLParam(r, "name")
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}
	if err := role.ValidateName(name); err != nil {
		return "", httperror.NotFound("Role", name)
	}
	return name, nil
}

func rolesURL(name string) string { return rolesPath + "/" + url.PathEscape(name) }

func (h *RolesPage) loadRole(ctx context.Context, name string) (*role.Role, error) {
	rl, err := h.Store.FindByName(ctx, name)
	if err != nil {
		return nil, usecase.Internal("REPO", "find role failed", err)
	}
	if rl == nil {
		return nil, httperror.NotFound("Role", name)
	}
	return rl, nil
}

func (h *RolesPage) applicationOptions(ctx context.Context) []kit.Option {
	return rolesApplicationOptions(ctx, h.Apps)
}

// rolesApplicationOptions are the active applications, by name (the SPA's
// application selects).
func rolesApplicationOptions(ctx context.Context, apps RolesApps) []kit.Option {
	active, err := apps.FindActive(ctx)
	if err != nil {
		slog.WarnContext(ctx, "ui: applications unavailable", "err", err)
		return nil
	}
	out := make([]kit.Option, 0, len(active))
	for _, a := range active {
		out = append(out, kit.Option{Value: a.Code, Label: a.Name})
	}
	slices.SortFunc(out, func(a, b kit.Option) int { return cmp.Compare(a.Label, b.Label) })
	return out
}

// ── list ───────────────────────────────────────────────────────────────

func (h *RolesPage) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// GET /bff/roles's check.
	if err := auth.CanReadRoles(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(r.URL.Query(), rolesList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		if err := view.patchList(ds.NewSSE(w, r)); err != nil {
			slog.DebugContext(ctx, "ui: roles patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

// patchList answers a live list request: the table, the pager, the toolbar's
// state part, the facet summaries and the canonical URL.
func (v rolesView) patchList(sse *ds.SSE) error {
	return errors.Join(
		v.patchTable(sse),
		sse.Patch(kit.TableToolbarState(v.toolbar().State())),
		sse.Patch(kit.MultiSelectSummary(v.facetProps(rolesFacetApp))),
		sse.Patch(kit.MultiSelectSummary(v.facetProps(rolesFacetSource))),
		sse.ReplaceURL(v.Query.URL()),
	)
}

func (v rolesView) patchTable(sse *ds.SSE) error {
	return errors.Join(sse.Patch(rolesTable(v)), sse.Patch(kit.Paginator(v.pager())))
}

func (h *RolesPage) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (rolesView, error) {
	all, err := h.Store.FindAll(ctx)
	if err != nil {
		return rolesView{}, usecase.Internal("REPO", "list roles failed", err)
	}
	return buildRoleView(ac, q, all, h.applicationOptions(ctx)), nil
}

func (h *RolesPage) renderPage(w http.ResponseWriter, r *http.Request, status int, view rolesView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Roles", Path: rolesPath}, rolesPage(view))
}

type rolesRow struct {
	ID, Name, DisplayName, Description string
	Permissions                        int
	Application                        string
	SourceLabel                        string
	SourceSeverity                     kit.TagSeverity
	DetailHref, EditHref, ActionBase   string
	// Editable is the SPA's pencil, Deletable its trash (admin-created roles
	// only); Locked replaces the trash with a lock for a role holding a
	// platform permission the caller lacks.
	Editable, Deletable, Locked bool
}

type rolesView struct {
	Query listQuery
	Rows  []rolesRow
	// Total is the number of roles the filters and search match.
	Total       int
	Page, Pages int
	Facets      map[string][]kit.Option
	Perms       rolesPerms
	Dialog      templ.Component
	Drawer      *rolesDetail
}

// rolesSources are the SPA's source filter options.
var rolesSources = []kit.Option{
	{Value: "CODE", Label: "Code-defined"},
	{Value: "DATABASE", Label: "Admin-created"},
	{Value: "SDK", Label: "SDK-registered"},
}

// buildRoleView applies the SPA's filters to the roles: the application and
// source facets, then the quick search over name, display name and
// description; ordered by name, paged 100 rows by default.
func buildRoleView(ac *auth.AuthContext, q listQuery, all []role.Role, apps []kit.Option) rolesView {
	v := rolesView{Perms: rolesPermsFor(ac), Query: q, Facets: map[string][]kit.Option{rolesFacetApp: apps, rolesFacetSource: rolesSources}}
	needle := strings.ToLower(q.Q)
	shown := make([]role.Role, 0, len(all))
	for _, rl := range all {
		if vals := q.facet(rolesFacetApp); len(vals) > 0 && !slices.Contains(vals, rl.ApplicationCode) {
			continue
		}
		if vals := q.facet(rolesFacetSource); len(vals) > 0 && !slices.ContainsFunc(vals, func(s string) bool { return strings.EqualFold(s, string(rl.Source)) }) {
			continue
		}
		hay := strings.ToLower(rl.Name + " " + rl.DisplayName + " " + deref(rl.Description))
		if needle != "" && !strings.Contains(hay, needle) {
			continue
		}
		shown = append(shown, rl)
	}
	slices.SortStableFunc(shown, func(a, b role.Role) int { return cmp.Compare(a.Name, b.Name) })

	v.Total = len(shown)
	var lo, hi int
	v.Query.Page, v.Pages, lo, hi = q.pageBounds(len(shown))
	v.Page = v.Query.Page
	for _, rl := range shown[lo:hi] {
		v.Rows = append(v.Rows, toRoleRow(ac, v.Perms, rl))
	}
	return v
}

func toRoleRow(ac *auth.AuthContext, perms rolesPerms, rl role.Role) rolesRow {
	admin := rl.Source == role.SourceDatabase
	label, severity := rolesSource(rl.Source, false)
	return rolesRow{
		ID: rl.ID, Name: rl.Name, DisplayName: cmp.Or(rl.DisplayName, rl.ShortName()), Description: deref(rl.Description),
		Permissions: len(rl.Permissions), Application: rl.ApplicationCode, SourceLabel: label, SourceSeverity: severity,
		DetailHref: rolesURL(rl.Name), EditHref: rolesURL(rl.Name) + "/edit", ActionBase: rolesURL(rl.Name),
		Editable: admin && perms.Update, Deletable: admin && perms.Delete, Locked: rolesAbove(ac, rl.Permissions),
	}
}

// rolesSource is the SPA's getSourceLabel and getSourceSeverity; the drawer
// spells the labels out.
func rolesSource(s role.Source, long bool) (string, kit.TagSeverity) {
	short, full, severity := string(s), string(s), kit.TagSecondary
	switch s {
	case role.SourceCode:
		short, full, severity = "Code", "Code-defined", kit.TagInfo
	case role.SourceDatabase:
		short, full, severity = "Admin", "Admin-created", kit.TagSuccess
	case role.SourceSDK:
		short, full, severity = "SDK", "SDK-registered", kit.TagWarn
	}
	if long {
		return full, severity
	}
	return short, severity
}

// rolesAbove is the SPA's isRoleAboveUser: the role holds a platform
// permission the caller does not (the permission ceiling, F10).
func rolesAbove(ac *auth.AuthContext, perms []string) bool {
	return len(role.PermissionsAboveCaller(ac, perms)) > 0
}

// rolesDeleteAboveHint is the SPA's ROLE_DELETE_ABOVE_USER_HINT.
const rolesDeleteAboveHint = "This role holds permissions you do not have, so you cannot delete it."

// rowActions: the SPA's row pencil (Edit, admin-created roles), its row click
// (View details) and its trash (Delete, or a lock for a role above the caller).
func (v rolesView) rowActions(row rolesRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "role-" + row.ID + "-actions", RowLabel: row.DisplayName}
	if row.Editable {
		p.Actions = append(p.Actions, kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref})
	}
	p.Actions = append(p.Actions, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	switch {
	case row.Deletable && row.Locked:
		p.Menu = append(p.Menu, kit.RowAction{Icon: "lock", Label: "Delete", DisabledReason: rolesDeleteAboveHint})
	case row.Deletable:
		p.Menu = append(p.Menu, kit.RowAction{Icon: "trash-2", Label: "Delete…", Destructive: true, Attrs: confirmTrigger(rolesDeleteDlg, row.ActionBase+"/delete")})
	}
	return p
}

// ── drawer ─────────────────────────────────────────────────────────────

type rolesPermission struct {
	Code, Application, Context, Aggregate, Action string
	Severity                                      kit.TagSeverity
}

type rolesDetail struct {
	ID, Name, DisplayName, ShortName, Description string
	Application                                   string
	SourceLabel                                   string
	SourceSeverity                                kit.TagSeverity
	Created, Updated                              templ.Component
	Permissions                                   []rolesPermission
	EditHref                                      string
	CanEdit                                       bool
}

func toRoleDetail(perms rolesPerms, rl *role.Role) rolesDetail {
	label, severity := rolesSource(rl.Source, true)
	d := rolesDetail{
		ID: rl.ID, Name: rl.Name, DisplayName: rl.DisplayName, ShortName: rl.ShortName(), Description: deref(rl.Description),
		Application: rl.ApplicationCode, SourceLabel: label, SourceSeverity: severity,
		Created:  kit.LocalTime(kit.LocalTimeProps{At: rl.CreatedAt, Empty: "—"}),
		Updated:  kit.LocalTime(kit.LocalTimeProps{At: rl.UpdatedAt, Empty: "—"}),
		EditHref: rolesURL(rl.Name) + "/edit",
		// The SPA offers Edit for admin-created roles to those who may update.
		CanEdit: rl.Source == role.SourceDatabase && perms.Update,
	}
	for _, code := range rl.Permissions {
		p := rolesPermission{Code: code}
		parts := strings.SplitN(code, ":", 4)
		for i, dst := range []*string{&p.Application, &p.Context, &p.Aggregate, &p.Action} {
			if i < len(parts) {
				*dst = parts[i]
			}
		}
		p.Severity = permissionActionSeverity(p.Action)
		d.Permissions = append(d.Permissions, p)
	}
	return d
}

// permissionActionSeverity is the SPA's getActionSeverity.
func permissionActionSeverity(action string) kit.TagSeverity {
	switch action {
	case "view":
		return kit.TagInfo
	case "create":
		return kit.TagSuccess
	case "update":
		return kit.TagWarn
	case "delete":
		return kit.TagDanger
	}
	return kit.TagSecondary
}

func (d rolesDetail) title() string { return cmp.Or(d.DisplayName, d.ShortName, "Role") }

func (d rolesDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: rolesDrawerID, Signal: rolesDrawerSig, Label: "Role", Size: kit.DrawerWide, Title: d.title(), Subtitle: d.Name, Extras: rolesTag(d.SourceLabel, d.SourceSeverity)}
}

func (h *RolesPage) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// GET /bff/roles/{rolesName}'s check.
	if err := auth.CanReadRoles(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	name, err := rolesName(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	rl, err := h.loadRole(ctx, name)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d := toRoleDetail(rolesPermsFor(ac), rl)
	if ds.IsDatastar(r) {
		if err := openRoleDrawer(ds.NewSSE(w, r), d); err != nil {
			slog.DebugContext(ctx, "ui: role drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(nil, rolesList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

func openRoleDrawer(sse *ds.SSE, d rolesDetail) error {
	return errors.Join(
		sse.Patch(rolesDrawerContent(d), ds.Inner("#"+rolesDrawerID)),
		sse.PatchSignals(map[string]any{rolesDrawerSig: d.ID}),
		sse.Focus(rolesDrawerID+"-title"),
	)
}

// ── actions ────────────────────────────────────────────────────────────

// errOnlyAdminRoles is the SPA's rule that only admin-created roles are
// edited or deleted (it offers neither for a code-defined or SDK-registered
// role); the use cases refuse a code-defined role only.
func errOnlyAdminRoles(verb string) error {
	return usecase.Conflict("ROLE_NOT_ADMIN_CREATED", "Only admin-created roles can be "+verb)
}

// delete is the SPA's trash (DELETE /bff/roles/{rolesName}): role:delete and
// anchor scope; the use case then applies the permission ceiling (a role
// holding a platform permission the caller lacks cannot be deleted).
func (h *RolesPage) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := rolesWrite(ac, auth.CanDeleteRoles); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	name, err := rolesName(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	rl, err := h.loadRole(ctx, name)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if rl.Source != role.SourceDatabase {
		h.Shell.Fail(w, r, errOnlyAdminRoles("deleted"))
		return
	}
	if err := h.Ops.Delete(ctx, rl.ID); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashRoleDeleted)
		http.Redirect(w, r, rolesPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac)
	// The drawer, when it shows the deleted role, closes.
	if r.PostFormValue("drawer") == rl.ID {
		if err := sse.PatchSignals(map[string]any{rolesDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: role drawer close not delivered", "err", err)
		}
	}
	if err := flashKey(sse, flashRoleDeleted); err != nil {
		slog.DebugContext(ctx, "ui: role flash not delivered", "err", err)
	}
}

// afterChange re-renders the list the user is on (the table and the header
// actions).
func (h *RolesPage) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext) {
	ctx := r.Context()
	view, err := h.listView(ctx, ac, refererList(r, rolesList))
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := view.patchTable(sse); err != nil {
		slog.DebugContext(ctx, "ui: roles table patch not delivered", "err", err)
	}
}

// syncPlatform is the SPA dashboard's "Sync Platform Roles" (POST
// /bff/roles/sync-platform: anchor scope and all three role permissions,
// because the sync creates, updates and removes roles): a flash with the tally.
func (h *RolesPage) syncPlatform(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := rolesWrite(ac, auth.CanCreateRoles, auth.CanUpdateRoles, auth.CanDeleteRoles); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	res, err := h.Ops.SyncPlatform(ctx)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashRolesSynced)
		http.Redirect(w, r, rolesPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac)
	msg := kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: "Platform Roles Synced", Detail: rolesSyncDetail(res)}
	if err := shell.Flash(sse, msg); err != nil {
		slog.DebugContext(ctx, "ui: sync flash not delivered", "err", err)
	}
}

// rolesSyncDetail is the SPA's toast text for a sync result.
func rolesSyncDetail(r RolesSyncResult) string {
	var parts []string
	for _, p := range []struct {
		n    int
		verb string
	}{{r.Created, "created"}, {r.Updated, "updated"}, {r.Removed, "removed"}} {
		if p.n > 0 {
			parts = append(parts, strconv.Itoa(p.n)+" "+p.verb)
		}
	}
	if len(parts) == 0 {
		return strconv.Itoa(r.Total) + " roles up to date"
	}
	return strings.Join(parts, ", ") + " (" + strconv.Itoa(r.Total) + " total)"
}

func rolesConfirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: rolesPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		Hidden:    []kit.HiddenField{drawerField(rolesDrawerSig)},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}
