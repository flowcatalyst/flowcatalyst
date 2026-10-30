package pages

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// FunctionStore is the slice of the function repository the page reads (the
// calls GET /api/functions and the by-id reads under it make).
type FunctionStore interface {
	FindByID(ctx context.Context, id string) (*function.Function, error)
	FindWithFilters(ctx context.Context, f function.ListFilters) ([]function.Function, error)
	CountWithFilters(ctx context.Context, f function.ListFilters) (int64, error)
	ListVersionsByFunction(ctx context.Context, functionID string) ([]function.Version, error)
	ListAliases(ctx context.Context, functionID string) ([]function.Alias, error)
	ListSettings(ctx context.Context, functionID string) ([]function.Setting, error)
	ListRoutesByFunction(ctx context.Context, functionID string) ([]function.Route, error)
	ServiceAccountCodes(ctx context.Context, accountIDs []string) (map[string]string, error)
}

// FunctionZones lists the claimed hostname zones (GET /api/function-domains).
type FunctionZones interface {
	FindWithFilters(ctx context.Context, f functiondomain.ListFilters) ([]functiondomain.FunctionDomain, error)
}

// FunctionSwitches renders a function's trace switches (the function trace
// page's Switches): the drawer's Diagnostics section.
type FunctionSwitches interface {
	Switches(ctx context.Context, fnID string, showLogLink bool) (templ.Component, error)
}

// Functions is the functions page (/ui/functions), mirroring the SPA's
// FunctionListPage, FunctionCreateDrawer and FunctionDetailDrawer: the list,
// the create and edit dialogs and the record drawer with its versions,
// aliases, declared surface, settings and public routes. Reads make the API's
// checks (CanReadFunctions, client scope); every write makes the permission
// check of the API endpoint it mirrors and runs its use case (FunctionCommands).
// A secret or database value is written and never read: no response of this
// page carries one.
type Functions struct {
	Functions FunctionStore
	Ops       FunctionCommands
	// Zones is optional: without it the routes tab does not list claimed zones.
	Zones FunctionZones
	// Clients backs the create form's client select; ClientNames the client
	// names and the list's client filter.
	Clients         ClientDirectory
	ClientNames     ClientStore
	Apps            ApplicationStore
	ServiceAccounts ServiceAccountDirectory
	// Trace is optional: without it the drawer has no Diagnostics section.
	Trace FunctionSwitches
	Shell *shell.Shell
}

const (
	fnsPath   = "/ui/functions"
	fnsFormID = "fns-list"

	fnsDrawerID   = "fns-drawer"
	fnsDrawerSig  = "selFn"
	fnsTabSig     = "fnTab"
	fnsDialogSlot = "fns-dialog-slot"

	fnsLiveGet = "@get('/ui/functions', {contentType: 'form'})"

	fnsFacetApp    = "application"
	fnsFacetClient = "client"

	fnsRetireDialog        = "fns-retire"
	fnsRemoveAliasDialog   = "fns-remove-alias"
	fnsRemoveSettingDialog = "fns-remove-setting"
	fnsRemoveRouteDialog   = "fns-remove-route"
	fnsDemoteDialog        = "fns-demote"

	// fnsTabOverview is the drawer's first tab.
	fnsTabOverview = "overview"
)

// fnsTabs are the drawer's tabs in order; the value is the ?tab= parameter.
var fnsTabs = []kit.TabItem{
	{Value: fnsTabOverview, Label: "Overview"},
	{Value: "versions", Label: "Versions"},
	{Value: "declares", Label: "What it declares"},
	{Value: "settings", Label: "Settings"},
	{Value: "routes", Label: "Routes"},
}

// fnsList is the functions list's URL state. The SPA pages 100 rows and does
// not sort (the API orders by address); the search is an address prefix.
var fnsList = &listSpec{
	path:        fnsPath,
	sortKeys:    []string{"address"},
	statusOK:    func(string) bool { return false },
	facets:      []string{fnsFacetApp, fnsFacetClient},
	rows:        []int{50, 100, 250, 500},
	defaultRows: 100,
}

// Mount registers the page's routes (POSTs behind parseForm). GET routes
// never change state: dialogs are fetched with GET, written with POST.
func (h *Functions) Mount(r chi.Router) {
	r.Get("/functions", h.list)
	r.Get("/functions/new", h.createForm)
	r.Get("/functions/{id}", h.detail)
	r.Get("/functions/{id}/edit", h.editForm)
	r.Get("/functions/{id}/delete", h.deleteForm)
	r.Get("/functions/{id}/aliases/new", h.aliasForm)
	r.Get("/functions/{id}/settings/{kind}/new", h.settingForm)
	r.Get("/functions/{id}/settings/{kind}/edit", h.settingForm)
	r.Get("/functions/{id}/routes/new", h.routeForm)
	r.Get("/functions/{id}/routes/{routeId}/edit", h.routeForm)
	post := r.With(parseForm)
	post.Post("/functions/new", h.create)
	post.Post("/functions/{id}/edit", h.update)
	post.Post("/functions/{id}/delete", h.delete)
	post.Post("/functions/{id}/aliases", h.putAlias)
	post.Post("/functions/{id}/aliases/{name}/delete", h.removeAlias)
	post.Post("/functions/{id}/versions/{number}/promote", h.promote)
	post.Post("/functions/{id}/versions/{number}/retire", h.retire)
	post.Post("/functions/{id}/settings/{kind}", h.saveSetting)
	post.Post("/functions/{id}/settings/{kind}/delete", h.removeSetting)
	post.Post("/functions/{id}/routes", h.saveRoute)
	post.Post("/functions/{id}/routes/{routeId}/delete", h.removeRoute)
}

// ── permissions ────────────────────────────────────────────────────────

// fnsPerms are what the viewer is offered; the server decides again on every
// post, with the API endpoint's check: create, edit, delete, retire and config
// values need CanWriteFunctions ("manage"), aliases CanPromoteFunctions,
// secrets and databases CanManageFunctionSecrets, routes
// CanManageFunctionRoutes. Zones are listed with CanManageFunctionDomains
// (the zones endpoint's gate).
type fnsPerms struct {
	Manage, Promote, Secrets, Routes, Zones bool
}

func fnsPermsFor(ac *auth.AuthContext) fnsPerms {
	return fnsPerms{
		Manage:  auth.CanWriteFunctions(ac) == nil,
		Promote: auth.CanPromoteFunctions(ac) == nil,
		Secrets: auth.CanManageFunctionSecrets(ac) == nil,
		Routes:  auth.CanManageFunctionRoutes(ac) == nil,
		Zones:   auth.CanManageFunctionDomains(ac) == nil,
	}
}

// ── list ───────────────────────────────────────────────────────────────

func (h *Functions) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadFunctions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(r.URL.Query(), fnsList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(view.patch(sse), sse.Patch(kit.TableToolbarState(view.toolbar().State())), sse.ReplaceURL(view.Query.URL())); err != nil {
			slog.DebugContext(ctx, "ui: functions patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *Functions) renderPage(w http.ResponseWriter, r *http.Request, status int, view fnsView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Functions", Path: fnsPath}, fnsPage(view))
}

type fnsRow struct {
	ID, Address, Application string
	Client                   string
	Pool                     string
	Warm                     bool
	Created, CreatedISO      string
	DetailHref, EditHref     string
}

type fnsView struct {
	Query       listQuery
	Rows        []fnsRow
	Total       int
	Page, Pages int
	// Apps and Clients are the filters' options.
	Apps, Clients []kit.Option
	Perms         fnsPerms
	// Dialog is a form dialog rendered with the page (a navigation to a
	// dialog's address, or a native post re-rendered with errors); Drawer the
	// record the drawer opens with.
	Dialog templ.Component
	Drawer *fnsDetail
}

// fnsNames are the client names by id (the list's and the drawer's label).
type fnsNames map[string]string

func (h *Functions) clientNames(ctx context.Context) (fnsNames, error) {
	n := fnsNames{}
	if h.ClientNames == nil {
		return n, nil
	}
	cs, err := h.ClientNames.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find clients failed", err)
	}
	for _, c := range cs {
		n[string(c.ID)] = c.Name
	}
	return n, nil
}

// label is the SPA's getClientLabel: the client's name, its id when unknown,
// "Platform" for none.
func (n fnsNames) label(clientID *string) string {
	if clientID == nil {
		return "Platform"
	}
	return cmp.Or(n[*clientID], *clientID)
}

// filterOptions are the SPA's: the applications (applicationsApi.list(), here
// narrowed to the caller's reach),
// and the active clients the caller can access.
func (h *Functions) filterOptions(ctx context.Context, ac *auth.AuthContext) (apps, clients []kit.Option, err error) {
	if h.Apps != nil && auth.CanReadApplications(ac) == nil {
		as, err := h.Apps.FindWithFilters(ctx, nil, nil)
		if err != nil {
			return nil, nil, usecase.Internal("REPO", "find applications failed", err)
		}
		apps = fnsApplicationOptions(ac, as)
	}
	if h.ClientNames != nil {
		cs, err := h.ClientNames.FindAll(ctx)
		if err != nil {
			return nil, nil, usecase.Internal("REPO", "find clients failed", err)
		}
		for _, c := range cs {
			if c.Status == client.StatusActive && (ac.IsAnchor() || ac.CanAccessClient(string(c.ID))) {
				clients = append(clients, kit.Option{Value: string(c.ID), Label: c.Name})
			}
		}
		slices.SortFunc(clients, func(a, b kit.Option) int { return strings.Compare(a.Label, b.Label) })
	}
	return apps, clients, nil
}

// fnsApplicationOptions offers the applications the caller may see: every
// one to a holder of the admin application view (CanReadAllApplications),
// else only the applications it reaches (all-applications access or its
// granted ones), the platform config page's rule.
func fnsApplicationOptions(ac *auth.AuthContext, as []application.Application) []kit.Option {
	opts := make([]kit.Option, 0, len(as))
	for _, a := range as {
		if !auth.CanReadAllApplications(ac) && !ac.CanAccessApplication(string(a.ID)) {
			continue
		}
		opts = append(opts, kit.Option{Value: string(a.ID), Label: a.Name + " (" + a.Code + ")"})
	}
	slices.SortFunc(opts, func(a, b kit.Option) int { return strings.Compare(a.Label, b.Label) })
	return opts
}

func (h *Functions) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (fnsView, error) {
	v := fnsView{Perms: fnsPermsFor(ac)}
	var err error
	if v.Apps, v.Clients, err = h.filterOptions(ctx, ac); err != nil {
		return fnsView{}, err
	}
	q = q.withFacet(fnsFacetApp, prune(q.facet(fnsFacetApp), v.Apps)).withFacet(fnsFacetClient, prune(q.facet(fnsFacetClient), v.Clients))
	// GET /api/functions: the filters in SQL, a caller below anchor confined
	// to platform rows and its clients, so the count and the page agree.
	var filters function.ListFilters
	if q.Q != "" {
		filters.AddressPrefix = &q.Q
	}
	if vals := q.facet(fnsFacetApp); len(vals) > 0 {
		filters.ApplicationID = &vals[0]
	}
	if vals := q.facet(fnsFacetClient); len(vals) > 0 {
		filters.ClientID = &vals[0]
	}
	if !ac.IsAnchor() {
		clients := ac.Clients
		filters.AccessibleClientIDs = &clients
	}
	total, err := h.Functions.CountWithFilters(ctx, filters)
	if err != nil {
		return fnsView{}, usecase.Internal("REPO", "count_with_filters failed", err)
	}
	v.Total = int(total)
	var lo int
	q.Page, v.Pages, lo, _ = q.pageBounds(v.Total)
	limit, offset := int64(q.Rows), int64(lo)
	filters.Limit, filters.Offset = &limit, &offset
	rows, err := h.Functions.FindWithFilters(ctx, filters)
	if err != nil {
		return fnsView{}, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	names, err := h.clientNames(ctx)
	if err != nil {
		return fnsView{}, err
	}
	for i := range rows {
		v.Rows = append(v.Rows, toFnsRow(&rows[i], names))
	}
	v.Page, v.Query = q.Page, q
	return v, nil
}

func fnsURL(id string) string { return fnsPath + "/" + url.PathEscape(id) }

func toFnsRow(f *function.Function, n fnsNames) fnsRow {
	return fnsRow{
		ID: f.ID, Address: f.Address, Application: f.ApplicationCode, Client: n.label(f.ClientID),
		Pool: cmp.Or(deref(f.Pool), "default"), Warm: f.Warm,
		Created: f.CreatedAt.UTC().Format("2006-01-02"), CreatedISO: f.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		DetailHref: fnsURL(f.ID), EditHref: fnsURL(f.ID) + "/edit",
	}
}

// rowActions: Edit inline (the SPA's pencil, with manage), View details in
// the menu.
func (v fnsView) rowActions(row fnsRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "fns-" + row.ID + "-actions", RowLabel: row.Address}
	if v.Perms.Manage {
		p.Actions = append(p.Actions, kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}})
	}
	p.Menu = append(p.Menu, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	return p
}

func (v fnsView) patch(sse *ds.SSE) error {
	return errors.Join(sse.Patch(fnsTable(v)), sse.Patch(kit.Paginator(v.pager())))
}

// fnsID reads and checks the {id} path parameter: an id that could not be an
// element id is not a function (and must never reach an expression).
func fnsID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("Function", id)
	}
	return id, nil
}

// loadFunction reads a function with GET /api/functions/{id}'s checks: the
// read permission and access to its client.
func (h *Functions) loadFunction(ctx context.Context, ac *auth.AuthContext, id string) (*function.Function, error) {
	if err := auth.CanReadFunctions(ac); err != nil {
		return nil, err
	}
	return h.scopedFunction(ctx, ac, id)
}

// scopedFunction is the function with access to its client checked, the
// by-id rule every write use case repeats. Writes call it for the page that
// follows the action, not as their own gate (the use case is that).
func (h *Functions) scopedFunction(ctx context.Context, ac *auth.AuthContext, id string) (*function.Function, error) {
	f, err := h.Functions.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if f == nil {
		return nil, httperror.NotFound("Function", id)
	}
	if err := auth.CheckScopeAccess(ac, f.ClientID); err != nil {
		return nil, err
	}
	return f, nil
}
