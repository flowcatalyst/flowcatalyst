package pages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/bff"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// PermissionsPage is the permissions page (/ui/permissions), mirroring the
// SPA's PermissionListPage. Reading makes the check of GET
// /bff/roles/permissions (role:view); "Create Permission" the check of POST
// /bff/roles/permissions (role:create and anchor scope).
type PermissionsPage struct {
	Catalogue PermissionCatalogue
	Apps      RolesApps
	Shell     *shell.Shell
}

const (
	permsPath       = "/ui/permissions"
	permsForm       = "perms-list"
	permsDialogSlot = "perm-dialog-slot"
	permsFormDialog = "perm-form"
	permsFormSignal = "permForm"

	permsLiveGet = "@get('/ui/permissions', {contentType: 'form'})"
)

// Facet query parameters: the SPA's URL keys (PermissionListPage: app, ctx, action).
const (
	permsFacetApp    = "app"
	permsFacetCtx    = "ctx"
	permsFacetAction = "action"
)

var permsList = &listSpec{
	path:        permsPath,
	sortKeys:    []string{"permission"},
	statusOK:    func(string) bool { return false },
	facets:      []string{permsFacetApp, permsFacetCtx, permsFacetAction},
	rows:        []int{50, 100, 250, 500},
	defaultRows: 100,
}

// permsActions are the SPA's action filter options.
var permsActions = []kit.Option{
	{Value: "view", Label: "view"},
	{Value: "create", Label: "create"},
	{Value: "update", Label: "update"},
	{Value: "delete", Label: "delete"},
	{Value: "retry", Label: "retry"},
}

// Mount registers the page's routes (the POST behind parseForm).
func (h *PermissionsPage) Mount(r chi.Router) {
	r.Get("/permissions", h.list)
	r.Get("/permissions/new", h.createForm)
	r.With(parseForm).Post("/permissions/new", h.create)
}

// ── list ───────────────────────────────────────────────────────────────

func (h *PermissionsPage) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadRoles(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(r.URL.Query(), permsList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		if err := view.patchList(ds.NewSSE(w, r)); err != nil {
			slog.DebugContext(ctx, "ui: permissions patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

// patchList answers a live list request: the table, the pager, the toolbar's
// state part, the facet summaries, the context options (they follow the chosen
// applications) and the canonical URL.
func (v permsView) patchList(sse *ds.SSE) error {
	return errors.Join(
		v.patchTable(sse),
		sse.Patch(kit.TableToolbarState(v.toolbar().State())),
		sse.Patch(kit.MultiSelectSummary(v.facetProps(permsFacetApp))),
		sse.Patch(kit.MultiSelectSummary(v.facetProps(permsFacetCtx))),
		sse.Patch(kit.MultiSelectOptions(v.facetProps(permsFacetCtx))),
		sse.Patch(kit.MultiSelectSummary(v.facetProps(permsFacetAction))),
		sse.ReplaceURL(v.Query.URL()),
	)
}

func (v permsView) patchTable(sse *ds.SSE) error {
	return errors.Join(sse.Patch(permsTable(v)), sse.Patch(kit.Paginator(v.pager())))
}

func (h *PermissionsPage) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (permsView, error) {
	// GET /bff/roles/permissions with no application: the whole catalogue.
	all, err := h.Catalogue.Catalogue(ctx, "")
	if err != nil {
		return permsView{}, usecase.Internal("REPO", "list permissions failed", err)
	}
	return buildPermsView(ac, q, all), nil
}

func (h *PermissionsPage) renderPage(w http.ResponseWriter, r *http.Request, status int, view permsView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Permissions", Path: permsPath}, permsPage(view))
}

type permsRow struct {
	Code, Application, Context, Aggregate, Action, Description string
	Severity                                                   kit.TagSeverity
}

type permsView struct {
	Query listQuery
	Rows  []permsRow
	// Total is the number of permissions the filters and search match.
	Total       int
	Page, Pages int
	Facets      map[string][]kit.Option
	// Create is the "Create Permission" button: role:create and anchor scope.
	Create bool
	Dialog templ.Component
}

func permsDistinct(all []bff.PermissionEntry, pick func(bff.PermissionEntry) string) []kit.Option {
	var vals []string
	for _, p := range all {
		if v := pick(p); v != "" && !slices.Contains(vals, v) {
			vals = append(vals, v)
		}
	}
	slices.Sort(vals)
	out := make([]kit.Option, 0, len(vals))
	for _, v := range vals {
		out = append(out, kit.Option{Value: v, Label: v})
	}
	return out
}

// buildPermsView applies the SPA's filters to the catalogue: the application,
// context (its options are the chosen applications' contexts) and action
// facets, then the quick search over the code and description, in the order
// the catalogue answers, paged 100 rows by default.
func buildPermsView(ac *auth.AuthContext, q listQuery, all []bff.PermissionEntry) permsView {
	v := permsView{Create: rolesWrite(ac, auth.CanCreateRoles) == nil, Facets: map[string][]kit.Option{permsFacetAction: permsActions}}
	v.Facets[permsFacetApp] = permsDistinct(all, func(p bff.PermissionEntry) string { return p.Application })

	inApps := all
	if apps := q.facet(permsFacetApp); len(apps) > 0 {
		inApps = slices.DeleteFunc(slices.Clone(all), func(p bff.PermissionEntry) bool { return !slices.Contains(apps, p.Application) })
	}
	v.Facets[permsFacetCtx] = permsDistinct(inApps, func(p bff.PermissionEntry) string { return p.Context })
	q = q.withFacet(permsFacetCtx, prune(q.facet(permsFacetCtx), v.Facets[permsFacetCtx]))

	needle := strings.ToLower(q.Q)
	var shown []permsRow
	for _, p := range all {
		if vals := q.facet(permsFacetApp); len(vals) > 0 && !slices.Contains(vals, p.Application) {
			continue
		}
		if vals := q.facet(permsFacetCtx); len(vals) > 0 && !slices.Contains(vals, p.Context) {
			continue
		}
		if vals := q.facet(permsFacetAction); len(vals) > 0 && !slices.Contains(vals, p.Action) {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(p.Permission+" "+p.Description), needle) {
			continue
		}
		shown = append(shown, permsRow{
			Code: p.Permission, Application: p.Application, Context: p.Context, Aggregate: p.Aggregate, Action: p.Action,
			Description: p.Description, Severity: permissionActionSeverity(p.Action),
		})
	}

	v.Total = len(shown)
	var lo, hi int
	q.Page, v.Pages, lo, hi = q.pageBounds(len(shown))
	v.Page, v.Query = q.Page, q
	v.Rows = shown[lo:hi]
	return v
}

// ── create ─────────────────────────────────────────────────────────────

// permsCreateKeys are the dialog's fields, in order.
var permsCreateKeys = []string{"application", "context", "aggregate", "action", "description"}

type permsCreateForm struct {
	Application, Context, Aggregate, Action, Description string
	Apps                                                 []kit.Option
	Errs                                                 formErrors
	Open                                                 bool
}

func (f permsCreateForm) id(key string) string         { return fieldID(permsFormDialog, key) }
func (f permsCreateForm) fieldError(key string) string { return f.Errs.Fields[key] }

func (f permsCreateForm) firstInvalid() string {
	for _, k := range permsCreateKeys {
		if f.fieldError(k) != "" {
			return f.id(k)
		}
	}
	return ""
}

func (f permsCreateForm) fields() []templ.Component {
	out := make([]templ.Component, 0, len(permsCreateKeys))
	for _, k := range permsCreateKeys {
		out = append(out, permsCreateField(f, k))
	}
	return out
}

func (f permsCreateForm) dialog() kit.FormDialogProps {
	return kit.FormDialogProps{
		ID: permsFormDialog, Signal: permsFormSignal, Width: kit.WidthWide, Action: permsPath + "/new", Open: f.Open, Error: f.Errs.Form,
		Title: "Create Permission", Description: "Define a permission code: application, context, aggregate and action", SubmitLabel: "Create Permission",
	}
}

func (f permsCreateForm) fieldProps(key, label string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.id(key), Required: required, Error: f.fieldError(key), Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f permsCreateForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func (f permsCreateForm) code() string {
	return f.Application + ":" + f.Context + ":" + f.Aggregate + ":" + f.Action
}

func (f permsCreateForm) signals() string {
	q := func(s string) string {
		return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", " ", "\r", " ").Replace(s) + "'"
	}
	return "{permApp: " + q(f.Application) + ", permCtx: " + q(f.Context) + ", permAgg: " + q(f.Aggregate) + ", permAct: " + q(f.Action) + "}"
}

func submittedPermsCreate(r *http.Request) permsCreateForm {
	return permsCreateForm{
		Application: strings.TrimSpace(r.PostFormValue("application")),
		Context:     strings.TrimSpace(r.PostFormValue("context")),
		Aggregate:   strings.TrimSpace(r.PostFormValue("aggregate")),
		Action:      strings.TrimSpace(r.PostFormValue("action")),
		Description: strings.TrimSpace(r.PostFormValue("description")),
	}
}

func (h *PermissionsPage) applicationOptions(ctx context.Context) []kit.Option {
	return rolesApplicationOptions(ctx, h.Apps)
}

func (h *PermissionsPage) createForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := rolesWrite(ac, auth.CanCreateRoles); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := permsCreateForm{Open: true, Apps: h.applicationOptions(ctx)}
	if len(f.Apps) > 0 {
		f.Application = f.Apps[0].Value
	}
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), permsDialogSlot, permsCreateDialog(f)); err != nil {
			slog.DebugContext(ctx, "ui: permission form patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(nil, permsList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = permsCreateDialog(f)
	h.renderPage(w, r, http.StatusOK, view)
}

// create defines a permission in the catalogue (POST /bff/roles/permissions:
// role:create and anchor scope). Each part must be a lowercase token, and the
// permission must be new (the SPA's Create button stays disabled otherwise).
func (h *PermissionsPage) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedPermsCreate(r)
	if err := rolesWrite(ac, auth.CanCreateRoles); err != nil {
		f.Errs.apply(r, err, nil)
		h.createFailed(w, r, ac, f)
		return
	}
	for _, p := range []struct{ key, value string }{{"application", f.Application}, {"context", f.Context}, {"aggregate", f.Aggregate}, {"action", f.Action}} {
		if !bff.ValidPermissionSegment(p.value) {
			f.Errs.field(p.key, "Use lowercase letters, numbers and hyphens.")
		}
	}
	if f.Errs.any() {
		h.createFailed(w, r, ac, f)
		return
	}
	catalogue, err := h.Catalogue.Catalogue(ctx, "")
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	code := f.code()
	if slices.ContainsFunc(catalogue, func(p bff.PermissionEntry) bool { return p.Permission == code }) {
		f.Errs.field("action", "This permission already exists.")
		h.createFailed(w, r, ac, f)
		return
	}
	if err := h.Catalogue.Define(ctx, code, optionalText(f.Description)); err != nil {
		f.Errs.apply(r, err, nil)
		h.createFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashPermCreated)
		http.Redirect(w, r, permsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(permsFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: permission form close not delivered", "err", err)
	}
	if view, err := h.listView(ctx, ac, refererList(r, permsList)); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := view.patchTable(sse); err != nil {
		slog.DebugContext(ctx, "ui: permissions table patch not delivered", "err", err)
	}
	if err := shell.Flash(sse, kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: "Created", Detail: "Permission " + code + " created"}); err != nil {
		slog.DebugContext(ctx, "ui: permission flash not delivered", "err", err)
	}
}

func (h *PermissionsPage) createFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f permsCreateForm) {
	f.Apps = h.applicationOptions(r.Context())
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, permsFormDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, permsList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	view.Dialog = permsCreateDialog(f)
	h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), view)
}
