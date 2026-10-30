package pages

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// EventTypeStore is the slice of the event-type repository the page reads.
type EventTypeStore interface {
	FindWithFilters(ctx context.Context, application, clientID, status, subdomain, aggregate *string) ([]eventtype.EventType, error)
	FindByID(ctx context.Context, id string) (*eventtype.EventType, error)
}

// EventTypeApps is the slice of the application repository the page needs:
// FindByCode confines an application service account to its own
// applications' event types (as GET /api/event-types does), FindActive lists
// the application codes the create form suggests (the SPA's
// applicationsApi.list({activeOnly: true})).
type EventTypeApps interface {
	FindByCode(ctx context.Context, code string) (*application.Application, error)
	FindActive(ctx context.Context) ([]application.Application, error)
}

// EventTypes is the event types page (/ui/event-types), mirroring the SPA's
// EventTypeListPage, EventTypeCreateDrawer, EventTypeDetailDrawer,
// SchemaViewerDialog and EventTypeAddSchemaPage. Reads make the API's checks
// (GET /api/event-types: the read permission, client scoping, application
// confinement); every action makes the check of the endpoint the SPA calls
// and runs its use case (EventTypeCommands).
type EventTypes struct {
	Store EventTypeStore
	Ops   EventTypeCommands
	Apps  EventTypeApps
	Shell *shell.Shell
}

const (
	etPath   = "/ui/event-types"
	etFormID = "et-list"

	etDrawerID   = "et-drawer"
	etDrawerSig  = "selEt"
	etDialogSlot = "et-dialog-slot"

	etArchiveDialog   = "et-archive"
	etDeleteDialog    = "et-delete"
	etFinaliseDialog  = "et-finalise"
	etDeprecateDialog = "et-deprecate"

	etLiveGet = "@get('/ui/event-types', {contentType: 'form'})"
)

// Facet query parameters: the SPA's URL keys (useEventTypes: app, sub, agg).
const (
	etFacetApp = "app"
	etFacetSub = "sub"
	etFacetAgg = "agg"
)

var etList = &listSpec{
	path:        etPath,
	sortKeys:    []string{"code"},
	statusOK:    func(s string) bool { _, ok := eventtype.ParseStatus(s); return ok },
	facets:      []string{etFacetApp, etFacetSub, etFacetAgg},
	rows:        []int{50, 100, 250, 500},
	defaultRows: 100,
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *EventTypes) Mount(r chi.Router) {
	r.Get("/event-types", h.list)
	r.Get("/event-types/new", h.createForm)
	r.Get("/event-types/export", h.export)
	r.Get("/event-types/{id}", h.detail)
	r.Get("/event-types/{id}/edit", h.editForm)
	r.Get("/event-types/{id}/add-schema", h.addSchemaPage)
	r.Get("/event-types/{id}/schemas/{version}", h.schemaViewer)
	post := r.With(parseForm)
	post.Post("/event-types/new", h.create)
	post.Post("/event-types/sync-platform", h.syncPlatform)
	post.Post("/event-types/{id}/edit", h.update)
	post.Post("/event-types/{id}/add-schema", h.addSchema)
	post.Post("/event-types/{id}/archive", h.archive)
	post.Post("/event-types/{id}/delete", h.delete)
	post.Post("/event-types/{id}/schemas/{version}/finalise", h.finalise)
	post.Post("/event-types/{id}/schemas/{version}/deprecate", h.deprecate)
}

// ── reading ────────────────────────────────────────────────────────────

// visible is every event type the caller may read (visibleEventTypes).
func (h *EventTypes) visible(ctx context.Context, ac *auth.AuthContext) ([]eventtype.EventType, error) {
	return visibleEventTypes(ctx, h.Store, h.Apps, ac)
}

// visibleEventTypes is every event type the caller may read: GET
// /api/event-types's client scoping and, without the messaging view
// permission, only the event types of applications the principal is bound to.
func visibleEventTypes(ctx context.Context, store EventTypeStore, apps EventTypeApps, ac *auth.AuthContext) ([]eventtype.EventType, error) {
	all, err := store.FindWithFilters(ctx, nil, nil, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("find event types: %w", err)
	}
	vis := auth.FilterClientScoped(ac, all, func(et *eventtype.EventType) *string { return ids.StringPtr(et.ClientID) })
	if auth.CanReadAllEventTypes(ac) {
		return vis, nil
	}
	seen := map[string]bool{}
	kept := vis[:0:0]
	for _, et := range vis {
		ok, err := appAccess(ctx, apps, ac, et.Application, seen)
		if err != nil {
			return nil, err
		}
		if ok {
			kept = append(kept, et)
		}
	}
	return kept, nil
}

// appAccess reports whether the principal is bound to the application with
// this code (remembered in seen for one request).
func appAccess(ctx context.Context, apps EventTypeApps, ac *auth.AuthContext, code string, seen map[string]bool) (bool, error) {
	if ok, done := seen[code]; done {
		return ok, nil
	}
	app, err := apps.FindByCode(ctx, code)
	if err != nil {
		return false, usecase.Internal("REPO", "find application by code failed", err)
	}
	ok := app != nil && ac.CanAccessApplication(string(app.ID))
	seen[code] = ok
	return ok, nil
}

// loadEventType reads one event type with GET /api/event-types/{id}'s checks
// (the read permission is the caller's).
func (h *EventTypes) loadEventType(ctx context.Context, ac *auth.AuthContext, id string) (*eventtype.EventType, error) {
	et, err := h.Store.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if et == nil {
		return nil, httperror.NotFound("EventType", id)
	}
	if et.ClientID != nil && !ac.CanAccessClient(string(*et.ClientID)) {
		return nil, httperror.Forbidden("No access to this event type")
	}
	if !auth.CanReadAllEventTypes(ac) {
		ok, err := appAccess(ctx, h.Apps, ac, et.Application, map[string]bool{})
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, httperror.Forbidden("No access to this event type")
		}
	}
	return et, nil
}

// etID reads and checks the {id} path parameter.
func etID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("EventType", id)
	}
	return id, nil
}

var versionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._-]{0,31}$`)

// etVersion reads and checks the {version} path parameter.
func etVersion(r *http.Request) (string, error) {
	v := chi.URLParam(r, "version")
	if !versionPattern.MatchString(v) {
		return "", httperror.NotFound("SchemaVersion", v)
	}
	return v, nil
}

func etURL(id string) string { return etPath + "/" + url.PathEscape(id) }

func etVersionURL(id, version string) string {
	return etURL(id) + "/schemas/" + url.PathEscape(version)
}

// ── list ───────────────────────────────────────────────────────────────

func (h *EventTypes) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// GET /api/event-types's check.
	if err := auth.CanReadEventTypes(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(r.URL.Query(), etList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := view.patchList(sse); err != nil {
			slog.DebugContext(ctx, "ui: event types patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

// patchList answers a live list request: the table, the pager, the
// toolbar's state part, the facet summaries, the dependent facets' options
// (subdomains of the chosen applications, aggregates of both), the header
// actions (the export link carries the view) and the canonical URL.
func (v etView) patchList(sse *ds.SSE) error {
	return errors.Join(
		v.patchTable(sse),
		sse.Patch(kit.TableToolbarState(v.toolbar().State())),
		sse.Patch(kit.MultiSelectSummary(v.facetProps(etFacetApp))),
		sse.Patch(kit.MultiSelectSummary(v.facetProps(etFacetSub))),
		sse.Patch(kit.MultiSelectSummary(v.facetProps(etFacetAgg))),
		sse.Patch(kit.MultiSelectOptions(v.facetProps(etFacetSub))),
		sse.Patch(kit.MultiSelectOptions(v.facetProps(etFacetAgg))),
		sse.Patch(etActions(v)),
		sse.ReplaceURL(v.Query.URL()),
	)
}

func (v etView) patchTable(sse *ds.SSE) error {
	return errors.Join(sse.Patch(etTable(v)), sse.Patch(kit.Paginator(v.pager())))
}

func (h *EventTypes) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (etView, error) {
	all, err := h.visible(ctx, ac)
	if err != nil {
		return etView{}, err
	}
	return buildETView(ac, q, all), nil
}

func (h *EventTypes) renderPage(w http.ResponseWriter, r *http.Request, status int, view etView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Event Types", Path: etPath}, etPage(view))
}

// etPerms are the checks of the endpoints the SPA calls: create
// (POST /bff/event-types) CanCreateEventTypes; edit, archive, add schema,
// finalise and deprecate CanUpdateEventTypes; delete CanDeleteEventTypes;
// "Sync Platform Events" anchor scope and the event-type sync permission
// (POST /bff/event-types/sync-platform: CanSyncPlatformEventTypes).
type etPerms struct {
	Create, Update, Delete, Sync bool
}

func etPermsFor(ac *auth.AuthContext) etPerms {
	return etPerms{
		Create: auth.CanCreateEventTypes(ac) == nil,
		Update: auth.CanUpdateEventTypes(ac) == nil,
		Delete: auth.CanDeleteEventTypes(ac) == nil,
		Sync:   auth.CanSyncPlatformEventTypes(ac) == nil,
	}
}

type etVersionTag struct {
	Version, Status string
	Severity        kit.TagSeverity
}

type etRow struct {
	ID, Code, Name, Description string
	Versions                    []etVersionTag
	Status                      string
	StatusSeverity              kit.TagSeverity
	Archived                    bool
	DetailHref, EditHref        string
}

type etView struct {
	Query listQuery
	Rows  []etRow
	// Total is the number of event types the filters and search match.
	Total       int
	Page, Pages int
	Facets      map[string][]kit.Option
	Perms       etPerms
	// HasSchemas: some matched event type has a CURRENT schema (the SPA
	// shows "Export Schemas" only then).
	HasSchemas bool
	Dialog     templ.Component
	Drawer     *etDetail
}

func distinct(ets []eventtype.EventType, pick func(eventtype.EventType) string) []kit.Option {
	var vals []string
	for _, et := range ets {
		if v := pick(et); v != "" && !slices.Contains(vals, v) {
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

// keep is the event types whose pick value is in vals (all of them for none).
func keep(ets []eventtype.EventType, vals []string, pick func(eventtype.EventType) string) []eventtype.EventType {
	if len(vals) == 0 {
		return ets
	}
	out := make([]eventtype.EventType, 0, len(ets))
	for _, et := range ets {
		if slices.Contains(vals, pick(et)) {
			out = append(out, et)
		}
	}
	return out
}

// prune drops chosen values that are no longer options (the SPA prunes a
// facet when its options change).
func prune(vals []string, opts []kit.Option) []string {
	var out []string
	for _, v := range vals {
		if slices.ContainsFunc(opts, func(o kit.Option) bool { return o.Value == v }) {
			out = append(out, v)
		}
	}
	return out
}

func etApplication(et eventtype.EventType) string { return et.Application }
func etSubdomain(et eventtype.EventType) string   { return et.Subdomain }
func etAggregate(et eventtype.EventType) string   { return et.Aggregate }

// buildETView applies the SPA's filters to the visible event types:
// cascading facets (subdomains of the chosen applications, aggregates of the
// chosen applications and subdomains, options drawn from every status), the
// status, then the quick search over code, name and the code's parts; paged
// 100 rows by default, ordered by code.
func buildETView(ac *auth.AuthContext, q listQuery, all []eventtype.EventType) etView {
	v := etView{Perms: etPermsFor(ac), Facets: map[string][]kit.Option{}}

	v.Facets[etFacetApp] = distinct(all, etApplication)
	q = q.withFacet(etFacetApp, prune(q.facet(etFacetApp), v.Facets[etFacetApp]))
	byApp := keep(all, q.facet(etFacetApp), etApplication)

	v.Facets[etFacetSub] = distinct(byApp, etSubdomain)
	q = q.withFacet(etFacetSub, prune(q.facet(etFacetSub), v.Facets[etFacetSub]))
	bySub := keep(byApp, q.facet(etFacetSub), etSubdomain)

	v.Facets[etFacetAgg] = distinct(bySub, etAggregate)
	q = q.withFacet(etFacetAgg, prune(q.facet(etFacetAgg), v.Facets[etFacetAgg]))
	matched := keep(bySub, q.facet(etFacetAgg), etAggregate)

	needle := strings.ToLower(q.Q)
	shown := make([]eventtype.EventType, 0, len(matched))
	for _, et := range matched {
		if q.Status != "" && string(et.Status) != q.Status {
			continue
		}
		hay := strings.ToLower(strings.Join([]string{et.Code, et.Name, et.EventName, et.Application, et.Subdomain, et.Aggregate}, " "))
		if needle != "" && !strings.Contains(hay, needle) {
			continue
		}
		shown = append(shown, et)
	}
	slices.SortStableFunc(shown, func(a, b eventtype.EventType) int { return cmp.Compare(a.Code, b.Code) })

	v.Total = len(shown)
	v.HasSchemas = slices.ContainsFunc(shown, func(et eventtype.EventType) bool { return currentSchema(et) != "" })
	var lo, hi int
	q.Page, v.Pages, lo, hi = q.pageBounds(len(shown))
	v.Page = q.Page
	v.Query = q
	for _, et := range shown[lo:hi] {
		v.Rows = append(v.Rows, toETRow(et))
	}
	return v
}

// currentSchema is the event type's CURRENT schema text ("" when none).
func currentSchema(et eventtype.EventType) string {
	for _, sv := range et.SpecVersions {
		if sv.Status == eventtype.SpecCurrent {
			return string(sv.SchemaContent)
		}
	}
	return ""
}

func toETRow(et eventtype.EventType) etRow {
	row := etRow{
		ID: et.ID, Code: et.Code, Name: et.Name, Description: deref(et.Description),
		Status: string(et.Status), StatusSeverity: etStatusSeverity(et.Status), Archived: et.Status == eventtype.StatusArchived,
		DetailHref: etURL(et.ID), EditHref: etURL(et.ID) + "/edit",
	}
	for _, sv := range et.SpecVersions {
		row.Versions = append(row.Versions, etVersionTag{Version: sv.Version, Status: string(sv.Status), Severity: specSeverity(sv.Status)})
	}
	return row
}

// etStatusSeverity: the SPA tags CURRENT success, anything else secondary.
func etStatusSeverity(s eventtype.Status) kit.TagSeverity {
	if s == eventtype.StatusCurrent {
		return kit.TagSuccess
	}
	return kit.TagSecondary
}

// specSeverity is the SPA's getSchemaStatusSeverity.
func specSeverity(s eventtype.SpecVersionStatus) kit.TagSeverity {
	switch s {
	case eventtype.SpecCurrent:
		return kit.TagSuccess
	case eventtype.SpecFinalising:
		return kit.TagInfo
	case eventtype.SpecDeprecated:
		return kit.TagWarn
	}
	return kit.TagSecondary
}

// archivedNoEdit is why an archived event type's Edit is disabled (the SPA's
// drawer hides Edit for an archived type).
const archivedNoEdit = "Archived event types cannot be edited."

// rowActions: the SPA's row pencil (Edit) and its row click (View details).
func (v etView) rowActions(row etRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "et-" + row.ID + "-actions", RowLabel: row.Name}
	if v.Perms.Update {
		edit := kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}}
		if row.Archived {
			edit.DisabledReason = archivedNoEdit
		}
		p.Actions = append(p.Actions, edit)
	}
	p.Actions = append(p.Actions, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	return p
}
