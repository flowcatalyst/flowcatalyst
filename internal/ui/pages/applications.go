package pages

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appdocs"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// ApplicationStore is the slice of the application repository the page reads
// (GET /api/applications, GET /api/applications/{id}).
type ApplicationStore interface {
	FindWithFilters(ctx context.Context, appType, active *string) ([]application.Application, error)
	FindByID(ctx context.Context, id string) (*application.Application, error)
}

// LoginClientLookup answers GET /api/applications/{id}'s hasLoginClient.
type LoginClientLookup interface {
	HasLoginClientForApplication(ctx context.Context, applicationID string) (bool, error)
}

// AppDocsLister lists the documentation an application synced (the SPA reads
// it from GET /api/docs).
type AppDocsLister interface {
	ListByApplication(ctx context.Context, applicationID string) ([]appdocs.Summary, error)
}

// Applications is the applications page (/ui/applications), mirroring the
// SPA's ApplicationListPage, ApplicationCreateDrawer and
// ApplicationDetailDrawer (details, service account and login client
// provisioning with the shown-once credentials, synced documentation,
// activate/deactivate/delete).
type Applications struct {
	Apps   ApplicationStore
	Ops    ApplicationCommands
	Logins LoginClientLookup
	Docs   AppDocsLister
	Shell  *shell.Shell
}

const (
	appsPath   = "/ui/applications"
	appsFormID = "apps-list"

	appDrawerID   = "app-drawer"
	appDrawerSig  = "selApp"
	appDialogSlot = "app-dialog-slot"

	appActivateDialog   = "app-activate"
	appDeactivateDialog = "app-deactivate"
	appDeleteDialog     = "app-delete"

	appsLiveGet = "@get('/ui/applications', {contentType: 'form'})"
)

var appList = &listSpec{
	path:        appsPath,
	sortKeys:    []string{"code", "name", "type", "status", "created"},
	statusOK:    func(s string) bool { return s == "ACTIVE" || s == "INACTIVE" },
	facets:      []string{"type"},
	rows:        []int{50, 100, 250, 500},
	defaultRows: 100,
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *Applications) Mount(r chi.Router) {
	r.Get("/applications", h.list)
	r.Get("/applications/new", h.createForm)
	r.Get("/applications/{id}", h.detail)
	r.Get("/applications/{id}/edit", h.editForm)
	post := r.With(parseForm)
	post.Post("/applications/new", h.create)
	post.Post("/applications/{id}/edit", h.update)
	post.Post("/applications/{id}/activate", h.activate)
	post.Post("/applications/{id}/deactivate", h.deactivate)
	post.Post("/applications/{id}/delete", h.delete)
	post.Post("/applications/{id}/provision-service-account", h.provisionServiceAccount)
	post.Post("/applications/{id}/provision-login-client", h.provisionLoginClient)
}

// ── list ───────────────────────────────────────────────────────────────

func (h *Applications) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// GET /api/applications's check.
	if err := auth.CanReadApplications(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseAppQuery(r.URL.Query()))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(view.patchTable(sse), sse.Patch(kit.TableToolbarState(view.toolbar().State())), sse.ReplaceURL(view.Query.URL())); err != nil {
			slog.DebugContext(ctx, "ui: applications patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

// appTypeFacet is the SPA's ?type= filter (a single select, held as a facet).
const appTypeFacet = "type"

func parseAppQuery(v url.Values) listQuery { return parseListQuery(v, appList) }

// appTypeFilter is the chosen type ("" for all; an unknown value is ignored).
func appTypeFilter(q listQuery) string {
	if vals := q.facet(appTypeFacet); len(vals) > 0 {
		if t, ok := application.ParseType(vals[0]); ok {
			return string(t)
		}
	}
	return ""
}

func (h *Applications) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (appsView, error) {
	apps, err := h.Apps.FindWithFilters(ctx, nil, nil)
	if err != nil {
		return appsView{}, fmt.Errorf("find applications: %w", err)
	}
	if !auth.CanReadAllApplications(ac) {
		// An application service account sees only the applications it is bound to.
		apps = slices.DeleteFunc(apps, func(a application.Application) bool { return !ac.CanAccessApplication(string(a.ID)) })
	}
	return buildAppsView(ac, q, apps), nil
}

func (h *Applications) renderPage(w http.ResponseWriter, r *http.Request, status int, view appsView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Applications", Path: appsPath}, appsPage(view))
}

// appPerms are the API's checks, the permission for the operation: create
// CanCreateApplications; edit, activate and deactivate CanUpdateApplications;
// delete CanDeleteApplications. Provisioning is anchor-only, as the API.
type appPerms struct {
	Create, Update, Delete, Provision bool
}

func appPermsFor(ac *auth.AuthContext) appPerms {
	return appPerms{
		Create:    auth.CanCreateApplications(ac) == nil,
		Update:    auth.CanUpdateApplications(ac) == nil,
		Delete:    auth.CanDeleteApplications(ac) == nil,
		Provision: auth.RequireAnchor(ac) == nil,
	}
}

type appRow struct {
	ID, Code, Name, Description string
	Type                        string
	TypeSeverity                kit.TagSeverity
	Active                      bool
	Created, CreatedISO         string
	DetailHref, EditHref        string
}

type appsView struct {
	Query       listQuery
	Rows        []appRow
	Total       int
	Page, Pages int
	Perms       appPerms
	Dialog      templ.Component
	Drawer      *appDetail
}

func appStatus(a application.Application) string {
	if a.Active {
		return "ACTIVE"
	}
	return "INACTIVE"
}

// buildAppsView applies the SPA's table filters (type, status, the quick
// search over code, name and description), sort and paging.
func buildAppsView(ac *auth.AuthContext, q listQuery, apps []application.Application) appsView {
	v := appsView{Perms: appPermsFor(ac)}
	appType := appTypeFilter(q)
	q = q.withFacet(appTypeFacet, nil)
	if appType != "" {
		q = q.withFacet(appTypeFacet, []string{appType})
	}
	needle := strings.ToLower(q.Q)
	shown := make([]application.Application, 0, len(apps))
	for _, a := range apps {
		if appType != "" && string(a.Type) != appType {
			continue
		}
		if q.Status != "" && appStatus(a) != q.Status {
			continue
		}
		hay := strings.ToLower(a.Code + " " + a.Name + " " + deref(a.Description))
		if needle != "" && !strings.Contains(hay, needle) {
			continue
		}
		shown = append(shown, a)
	}
	slices.SortStableFunc(shown, func(a, b application.Application) int {
		var c int
		switch q.Sort {
		case "name":
			c = cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		case "type":
			c = cmp.Compare(a.Type, b.Type)
		case "status":
			c = cmp.Compare(appStatus(a), appStatus(b))
		case "created":
			c = a.CreatedAt.Compare(b.CreatedAt)
		default:
			c = cmp.Compare(a.Code, b.Code)
		}
		if q.Dir == kit.SortDesc {
			c = -c
		}
		return c
	})
	v.Total = len(shown)
	var lo, hi int
	q.Page, v.Pages, lo, hi = q.pageBounds(len(shown))
	v.Page, v.Query = q.Page, q
	for _, a := range shown[lo:hi] {
		v.Rows = append(v.Rows, toAppRow(a))
	}
	return v
}

func appURL(id string) string { return appsPath + "/" + url.PathEscape(id) }

// appTypeLabel and appTypeSeverity are the SPA's type tag.
func appTypeLabel(t application.Type) string {
	if t == application.TypeIntegration {
		return "Integration"
	}
	return "Application"
}

func appTypeSeverity(t application.Type) kit.TagSeverity {
	if t == application.TypeIntegration {
		return kit.TagInfo
	}
	return kit.TagPrimary
}

func toAppRow(a application.Application) appRow {
	return appRow{
		ID: string(a.ID), Code: a.Code, Name: a.Name, Description: deref(a.Description),
		Type: appTypeLabel(a.Type), TypeSeverity: appTypeSeverity(a.Type), Active: a.Active,
		Created: a.CreatedAt.UTC().Format("2006-01-02"), CreatedISO: a.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		DetailHref: appURL(string(a.ID)), EditHref: appURL(string(a.ID)) + "/edit",
	}
}

func (v appsView) rowActions(row appRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "app-" + row.ID + "-actions", RowLabel: row.Name}
	if v.Perms.Update {
		p.Actions = append(p.Actions, kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}})
	}
	p.Actions = append(p.Actions, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	return p
}

func (v appsView) patchTable(sse *ds.SSE) error {
	return errors.Join(sse.Patch(appsTable(v)), sse.Patch(kit.Paginator(v.pager())))
}

func appID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("Application", id)
	}
	return id, nil
}

// ── drawer ─────────────────────────────────────────────────────────────

type appDetail struct {
	ID, Code, Name, Description string
	Type                        string
	TypeSeverity                kit.TagSeverity
	BaseURL, IconURL, Website   string
	Logo                        string
	ServiceAccountID            string
	HasLoginClient              bool
	Active                      bool
	Created, Updated            string
	Docs                        []appdocs.Summary
	DocsVisible                 bool
	EditHref, ActionBase        string
	Perms                       appPerms
	// LoginForm is the login-client form's state (errors after a refused
	// provisioning).
	LoginForm appLoginForm
}

func (d appDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: appDrawerID, Signal: appDrawerSig, Label: "Application", Size: kit.DrawerWide, Title: d.Name, Subtitle: d.Code, Extras: appActiveTag(d.Active)}
}

// loadApplication reads an application with GET /api/applications/{id}'s
// checks, and what the drawer shows with it (login client, docs).
func (h *Applications) loadApplication(ctx context.Context, ac *auth.AuthContext, id string) (*application.Application, error) {
	if err := auth.CanReadApplication(ac, id); err != nil {
		return nil, err
	}
	a, err := h.Apps.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if a == nil {
		return nil, httperror.NotFound("Application", id)
	}
	return a, nil
}

func (h *Applications) detailOf(ctx context.Context, ac *auth.AuthContext, a *application.Application) (appDetail, error) {
	d := appDetail{
		ID: string(a.ID), Code: a.Code, Name: a.Name, Description: deref(a.Description),
		Type: appTypeLabel(a.Type), TypeSeverity: appTypeSeverity(a.Type),
		BaseURL: deref(a.DefaultBaseURL), IconURL: deref(a.IconURL), Website: deref(a.Website),
		ServiceAccountID: deref(a.ServiceAccountID), Active: a.Active,
		Created: a.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"), Updated: a.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		EditHref: appURL(string(a.ID)) + "/edit", ActionBase: appURL(string(a.ID)), Perms: appPermsFor(ac),
		LoginForm: appLoginForm{ClientType: "PUBLIC"},
	}
	if a.Logo != nil && *a.Logo != "" {
		d.Logo = cmp.Or(deref(a.LogoMimeType), "Configured")
	}
	if h.Logins != nil {
		has, err := h.Logins.HasLoginClientForApplication(ctx, string(a.ID))
		if err != nil {
			return appDetail{}, usecase.Internal("REPO", "has_login_client_for_application failed", err)
		}
		d.HasLoginClient = has
	}
	// The SPA reads the docs index (GET /api/docs: CanReadPlatformDocs) and
	// leaves the section empty when it cannot.
	if h.Docs != nil && auth.CanReadPlatformDocs(ac) == nil {
		docs, err := h.Docs.ListByApplication(ctx, string(a.ID))
		if err != nil {
			slog.WarnContext(ctx, "ui: application docs unavailable", "err", err)
		}
		d.Docs, d.DocsVisible = docs, true
	}
	return d, nil
}

func (h *Applications) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := appID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	a, err := h.loadApplication(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d, err := h.detailOf(ctx, ac, a)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		if err := openAppDrawer(ds.NewSSE(w, r), d); err != nil {
			slog.DebugContext(ctx, "ui: application drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseAppQuery(nil))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

func openAppDrawer(sse *ds.SSE, d appDetail) error {
	return errors.Join(
		sse.Patch(appDrawerContent(d), ds.Inner("#"+appDrawerID)),
		sse.PatchSignals(map[string]any{appDrawerSig: d.ID}),
		sse.Focus(appDrawerID+"-title"),
	)
}

// ── state actions ──────────────────────────────────────────────────────

type appAction struct {
	check   func(*auth.AuthContext) error
	run     func(ApplicationCommands, context.Context, string) error
	flash   string
	deleted bool
}

var (
	activateAppAction   = appAction{check: auth.CanUpdateApplications, run: ApplicationCommands.Activate, flash: flashAppActivated}
	deactivateAppAction = appAction{check: auth.CanUpdateApplications, run: ApplicationCommands.Deactivate, flash: flashAppDeactivated}
	deleteAppAction     = appAction{check: auth.CanDeleteApplications, run: ApplicationCommands.Delete, flash: flashAppDeleted, deleted: true}
)

func (h *Applications) activate(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, activateAppAction)
}

func (h *Applications) deactivate(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, deactivateAppAction)
}

func (h *Applications) delete(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, deleteAppAction)
}

func (h *Applications) runAction(w http.ResponseWriter, r *http.Request, a appAction) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := a.check(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := appID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := a.run(h.Ops, ctx, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, a.flash)
		http.Redirect(w, r, appsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac, id, a.deleted)
	if err := flashKey(sse, a.flash); err != nil {
		slog.DebugContext(ctx, "ui: application flash not delivered", "err", err)
	}
}

func (h *Applications) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string, deleted bool) {
	ctx := r.Context()
	if view, err := h.listView(ctx, ac, refererList(r, appList)); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := view.patchTable(sse); err != nil {
		slog.DebugContext(ctx, "ui: applications table patch not delivered", "err", err)
	}
	if r.PostFormValue("drawer") != id {
		return
	}
	if deleted {
		if err := sse.PatchSignals(map[string]any{appDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: application drawer close not delivered", "err", err)
		}
		return
	}
	h.refreshDrawer(sse, r, ac, id, nil)
}

// refreshDrawer re-renders the drawer for id; login, when set, keeps the
// login-client form's state (a refused provisioning).
func (h *Applications) refreshDrawer(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string, login *appLoginForm) {
	ctx := r.Context()
	a, err := h.loadApplication(ctx, ac, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	d, err := h.detailOf(ctx, ac, a)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if login != nil {
		d.LoginForm = *login
	}
	if err := sse.Patch(appDrawerContent(d), ds.Inner("#"+appDrawerID)); err != nil {
		slog.DebugContext(ctx, "ui: application drawer patch not delivered", "err", err)
	}
}

func appConfirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: appsPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		Hidden:    []kit.HiddenField{drawerField(appDrawerSig)},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}
