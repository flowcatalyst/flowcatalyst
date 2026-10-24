package pages

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/schemacodegen"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// AuditStore is the slice of the audit log repository the page reads (the
// SPA's GET /api/audit-logs, /{id} and the distinct-value facets).
type AuditStore interface {
	FindWithCursor(ctx context.Context, p audit.CursorFilterParams) ([]audit.Log, error)
	FindByID(ctx context.Context, id string) (*audit.Log, error)
	DistinctValues(ctx context.Context, column string, limit int) ([]string, error)
}

// AuditLogPage is the audit log page (/ui/audit-log), mirroring the SPA's
// AuditLogListPage: a read-only, newest-first list filtered by entity type,
// operation, application and client, with a detail drawer. Like the API it
// needs platform:admin:audit-log:view and nothing else (the log is not
// tenant-scoped). The SPA offers no export and no principal or date filter,
// so this page has none either.
type AuditLogPage struct {
	Store   AuditStore
	Apps    ApplicationStore
	Clients ClientStore
	Shell   *shell.Shell

	// facetCache holds the distinct entity types, operations and application
	// ids (the log is not tenant-scoped: one entry).
	facetCache distinctCache
}

const (
	auditPath   = "/ui/audit-log"
	auditFormID = "audit-list"

	auditDrawerID  = "audit-drawer"
	auditDrawerSig = "selAudit"

	auditViewPerm = "platform:admin:audit-log:view"

	auditLiveGet = "@get('/ui/audit-log', {contentType: 'form'})"

	// auditPageSize is the SPA's fixed page size.
	auditPageSize = 100

	auditFacetApps    = "applications"
	auditFacetClients = "clients"
)

var auditFacets = []string{auditFacetApps, auditFacetClients}

// auditRowClick opens a row's drawer from anywhere on the row, through its
// View details link.
const auditRowClick = "const r = evt.target.closest('tr'); r && !evt.target.closest('a') && r.querySelector('a')?.click()"

// Mount registers the page's routes (read-only: no POSTs).
func (h *AuditLogPage) Mount(r chi.Router) {
	r.Get("/audit-log", h.list)
	r.Get("/audit-log/{id}", h.detail)
}

// ── URL state ──────────────────────────────────────────────────────────

// auditQuery is one view of the list: the filters and the keyset position.
type auditQuery struct {
	EntityType, Operation string
	Facets                map[string][]string
	After                 *audit.Cursor
}

func parseAuditQuery(v url.Values) auditQuery {
	q := auditQuery{EntityType: queryValue(v.Get("entityType")), Operation: queryValue(v.Get("operation"))}
	for _, name := range auditFacets {
		if vals := facetValues(v[name]); len(vals) > 0 {
			if q.Facets == nil {
				q.Facets = map[string][]string{}
			}
			q.Facets[name] = vals
		}
	}
	if at, id, ok := parseCursor(v.Get("after")); ok {
		q.After = &audit.Cursor{PerformedAt: at, ID: id}
	}
	return q
}

// queryValue trims a single-valued filter and caps its length.
func queryValue(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxQueryLen {
		return s[:maxQueryLen]
	}
	return s
}

func (q auditQuery) facet(name string) []string { return q.Facets[name] }

func (q auditQuery) filtered() bool {
	return q.EntityType != "" || q.Operation != "" || len(q.Facets) > 0
}

func (q auditQuery) activeFilters() int {
	n := len(q.Facets)
	if q.EntityType != "" {
		n++
	}
	if q.Operation != "" {
		n++
	}
	return n
}

// URL is the canonical URL of this view.
func (q auditQuery) URL() string {
	v := url.Values{}
	if q.EntityType != "" {
		v.Set("entityType", q.EntityType)
	}
	if q.Operation != "" {
		v.Set("operation", q.Operation)
	}
	for _, name := range auditFacets {
		for _, val := range q.Facets[name] {
			v.Add(name, val)
		}
	}
	if q.After != nil {
		v.Set("after", formatCursor(q.After.PerformedAt, q.After.ID))
	}
	return encodeURL(auditPath, v)
}

func (q auditQuery) first() auditQuery {
	q.After = nil
	return q
}

func (q auditQuery) params() audit.CursorFilterParams {
	p := audit.CursorFilterParams{ApplicationIDs: q.facet(auditFacetApps), ClientIDs: q.facet(auditFacetClients), After: q.After, Limit: auditPageSize + 1}
	if q.EntityType != "" {
		p.EntityType = &q.EntityType
	}
	if q.Operation != "" {
		p.Operation = &q.Operation
	}
	return p
}

// ── list ───────────────────────────────────────────────────────────────

type auditRow struct {
	ID, Href             string
	EntityType, EntityID string
	Operation, Principal string
	Application, Client  string
	Time                 time.Time
}

type auditView struct {
	Query       auditQuery
	Rows        []auditRow
	EntityTypes []kit.Option
	Operations  []kit.Option
	Facets      map[string][]kit.Option
	OlderHref   string
	Drawer      *auditDetail
}

func (h *AuditLogPage) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, auditViewPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseAuditQuery(r.URL.Query()))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		if err := view.patchList(ds.NewSSE(w, r)); err != nil {
			slog.DebugContext(ctx, "ui: audit log patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, view)
}

func (h *AuditLogPage) renderPage(w http.ResponseWriter, r *http.Request, view auditView) {
	h.Shell.Render(w, r, shell.Page{Title: "Audit Log", Path: auditPath}, auditPage(view))
}

// patchList replaces the table, the pager, the toolbar's live parts and the
// multi-select summaries, and the address.
func (v auditView) patchList(sse *ds.SSE) error {
	errs := []error{
		sse.Patch(auditTable(v), ds.WithMode(ds.ModeReplace)), sse.Patch(kit.CursorPager(v.pager())),
		sse.Patch(kit.TableToolbarState(v.toolbar().State())),
	}
	for _, f := range auditFacets {
		errs = append(errs, sse.Patch(kit.MultiSelectSummary(v.facetProps(f))))
	}
	errs = append(errs, sse.ReplaceURL(v.Query.URL()))
	return errors.Join(errs...)
}

// appNames reads the application names the filter and the rows show (the
// SPA's applications list); a caller who cannot read applications sees ids.
func (h *AuditLogPage) appNames(ctx context.Context, ac *auth.AuthContext) (map[string]string, error) {
	names := map[string]string{}
	if h.Apps == nil || auth.CanReadApplications(ac) != nil {
		return names, nil
	}
	apps, err := h.Apps.FindWithFilters(ctx, nil, nil)
	if err != nil {
		return nil, usecase.Internal("REPO", "list applications failed", err)
	}
	for _, a := range apps {
		names[string(a.ID)] = a.Name
	}
	return names, nil
}

func auditDistinct(store AuditStore) distinctFetch {
	return func(ctx context.Context, column string, limit int, _ *[]string) ([]string, error) {
		return store.DistinctValues(ctx, column, limit)
	}
}

func (h *AuditLogPage) listView(ctx context.Context, ac *auth.AuthContext, q auditQuery) (auditView, error) {
	v := auditView{Query: q}
	distinct, err := h.facetCache.get(ctx, auditDistinct(h.Store), nil, "entity_type", "operation", "application_id")
	if err != nil {
		return auditView{}, err
	}
	apps, err := h.appNames(ctx, ac)
	if err != nil {
		return auditView{}, err
	}
	cs, err := allClients(ctx, h.Clients)
	if err != nil {
		return auditView{}, err
	}
	clients := clientNames(cs)
	// Only applications that appear in the log are options (the SPA's rule).
	appOpts := make([]kit.Option, 0, len(distinct["application_id"]))
	for _, id := range distinct["application_id"] {
		appOpts = append(appOpts, kit.Option{Value: id, Label: cmp.Or(apps[id], id)})
	}
	v.EntityTypes = withChosen(plainOptions(distinct["entity_type"]), nonEmpty(q.EntityType), nil)
	v.Operations = withChosen(plainOptions(distinct["operation"]), nonEmpty(q.Operation), nil)
	v.Facets = map[string][]kit.Option{
		auditFacetApps:    withChosen(appOpts, q.facet(auditFacetApps), apps),
		auditFacetClients: withChosen(clientFilterOptions(cs, ac), q.facet(auditFacetClients), clients),
	}
	logs, err := h.Store.FindWithCursor(ctx, q.params())
	if err != nil {
		return auditView{}, usecase.Internal("REPO", "find_with_cursor failed", err)
	}
	more := len(logs) > auditPageSize
	if more {
		logs = logs[:auditPageSize]
	}
	for i := range logs {
		v.Rows = append(v.Rows, toAuditRow(&logs[i], apps, clients))
	}
	if more {
		last := logs[len(logs)-1]
		next := q
		next.After = &audit.Cursor{PerformedAt: last.PerformedAt, ID: last.ID}
		v.OlderHref = next.URL()
	}
	return v, nil
}

func auditURL(id string) string { return auditPath + "/" + url.PathEscape(id) }

func toAuditRow(l *audit.Log, apps, clients map[string]string) auditRow {
	return auditRow{
		ID: l.ID, Href: auditURL(l.ID), EntityType: l.EntityType, EntityID: l.EntityID,
		Operation: operationName(l.Operation), Principal: cmp.Or(deref(l.PrincipalName), "Unknown"),
		Application: contextLabel(l.ApplicationID, apps), Client: contextLabel(l.ClientID, clients), Time: l.PerformedAt,
	}
}

// contextLabel is the name of an application or client, its id when the name
// is unknown, empty when the entry has none.
func contextLabel(id *string, names map[string]string) string {
	if id == nil || *id == "" {
		return ""
	}
	return cmp.Or(names[*id], *id)
}

var capitals = regexp.MustCompile(`([A-Z])`)

// operationName is the SPA's formatOperationName: "CreateApplication" →
// "Create Application".
func operationName(op string) string {
	s := strings.TrimSpace(capitals.ReplaceAllString(op, " $1"))
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// entityTypeSeverity is the SPA's getEntityTypeSeverity.
func entityTypeSeverity(t string) kit.TagSeverity {
	switch t {
	case "ClientAuthConfig", "EventType":
		return kit.TagInfo
	case "Role":
		return kit.TagWarn
	case "Principal":
		return kit.TagSuccess
	}
	return kit.TagSecondary
}

// ── drawer ─────────────────────────────────────────────────────────────

type auditDetail struct {
	ID, EntityType, EntityID, Operation string
	Principal, PrincipalID              string
	Application, Client                 string
	Data                                string
	Time                                time.Time
}

func (d auditDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: auditDrawerID, Signal: auditDrawerSig, Label: "Audit log", Size: kit.DrawerWide, Title: "Audit Log Details", Subtitle: d.ID}
}

// operationData is the SPA's formatJson: indented when it is JSON, as stored
// when not.
func operationData(raw []byte) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if p, ok := schemacodegen.PrettyJSON(string(raw)); ok {
		return p
	}
	return string(raw)
}

func toAuditDetail(l *audit.Log, apps, clients map[string]string) auditDetail {
	return auditDetail{
		ID: l.ID, EntityType: l.EntityType, EntityID: l.EntityID, Operation: operationName(l.Operation),
		Principal: cmp.Or(deref(l.PrincipalName), "Unknown"), PrincipalID: deref(l.PrincipalID),
		Application: contextLabel(l.ApplicationID, apps), Client: contextLabel(l.ClientID, clients),
		Data: operationData(l.OperationJSON), Time: l.PerformedAt,
	}
}

func (h *AuditLogPage) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, auditViewPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	if !eventIDPattern.MatchString(id) {
		h.Shell.Fail(w, r, httperror.NotFound("AuditLog", id))
		return
	}
	l, err := h.Store.FindByID(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "find_by_id failed", err))
		return
	}
	if l == nil {
		h.Shell.Fail(w, r, httperror.NotFound("AuditLog", id))
		return
	}
	apps, err := h.appNames(ctx, ac)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	cs, err := allClients(ctx, h.Clients)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d := toAuditDetail(l, apps, clientNames(cs))
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(
			sse.Patch(auditDrawerContent(d), ds.Inner("#"+auditDrawerID)),
			sse.PatchSignals(map[string]any{auditDrawerSig: d.ID}),
			sse.Focus(auditDrawerID+"-title"),
		); err != nil {
			slog.DebugContext(ctx, "ui: audit log drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseAuditQuery(nil))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, view)
}
