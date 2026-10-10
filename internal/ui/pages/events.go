package pages

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/schemacodegen"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// EventStore is the slice of the event repository the page reads (the
// SPA's GET /api/events, /{id} and /filter-options).
type EventStore interface {
	FindWithFilters(ctx context.Context, p event.FilterParams) ([]event.Event, error)
	FindByID(ctx context.Context, id string) (*event.Event, error)
	DistinctValues(ctx context.Context, column string, limit int, accessibleClientIDs *[]string) ([]string, error)
	// ContextByID reads the context data from the write table (the
	// projection has none), as GET /api/events/{id} does.
	ContextByID(ctx context.Context, id string) ([]event.ContextEntry, error)
}

// Events is the events page (/ui/events), mirroring the SPA's EventListPage
// and EventDetailDrawer: a read-only firehose list with the API's checks
// (event:view, SQL tenant scoping, per-event client access on the drawer).
type Events struct {
	Events  EventStore
	Clients ClientStore
	Shell   *shell.Shell

	// facetCache holds the distinct event types (firehose.go).
	facetCache distinctCache
}

const (
	evPath   = "/ui/events"
	evFormID = "ev-list"

	evDrawerID  = "ev-drawer"
	evDrawerSig = "selEv"

	evViewPerm = "platform:messaging:event:view"

	evLiveGet = "@get('/ui/events', {contentType: 'form'})"

	// Facets: the SPA's URL keys.
	evFacetClients = "clients"
	evFacetApps    = "applications"
	evFacetSubs    = "subdomains"
	evFacetAggs    = "aggregates"
	evFacetTypes   = "types"

	evDefaultSize = 200
)

var (
	evFacets = []string{evFacetClients, evFacetApps, evFacetSubs, evFacetAggs, evFacetTypes}
	evSizes  = []int{50, 100, 200, 500, 1000}

	evList = &listSpec{
		path: evPath, sortKeys: []string{"time"}, defaultDir: kit.SortDesc,
		statusOK: func(string) bool { return false }, facets: evFacets,
	}

	// eventIDPattern: event ids are untyped TSIDs (they may start with a
	// digit, so kit.ValidID does not fit); an id never enters an expression.
	eventIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// Mount registers the page's routes (read-only: no POSTs).
func (h *Events) Mount(r chi.Router) {
	r.Get("/events", h.list)
	r.Get("/events/{id}", h.detail)
}

// ── URL state ──────────────────────────────────────────────────────────

// evQuery is one view of the list: the facets, the source search (q), the
// result size and the keyset position.
type evQuery struct {
	listQuery
	Size  int
	After *event.ListCursor
}

func parseEvQuery(v url.Values) evQuery {
	q := evQuery{listQuery: parseListQuery(v, evList), Size: evDefaultSize}
	if n, err := strconv.Atoi(v.Get("size")); err == nil && slices.Contains(evSizes, n) {
		q.Size = n
	}
	if at, id, ok := parseCursor(v.Get("after")); ok {
		q.After = &event.ListCursor{CreatedAt: at, ID: id}
	}
	return q
}

func (q evQuery) values() url.Values {
	v := q.listQuery.values()
	if q.Size != evDefaultSize {
		v.Set("size", strconv.Itoa(q.Size))
	}
	if q.After != nil {
		v.Set("after", formatCursor(q.After.CreatedAt, q.After.ID))
	}
	return v
}

// URL is the canonical URL of this view.
func (q evQuery) URL() string { return encodeURL(evPath, q.values()) }

func (q evQuery) first() evQuery {
	q.After = nil
	return q
}

// clearURL keeps the size (table-toolbar.md: Clear All clears what the user
// typed and picked).
func (q evQuery) clearURL() string {
	clear := evQuery{Size: q.Size}
	clear.spec, clear.Sort, clear.Dir = evList, q.Sort, q.Dir
	return clear.URL()
}

func (q evQuery) filterParams() event.FilterParams {
	p := event.FilterParams{
		ClientIDs: q.facet(evFacetClients), Applications: q.facet(evFacetApps), Subdomains: q.facet(evFacetSubs),
		Aggregates: q.facet(evFacetAggs), Types: q.facet(evFacetTypes),
	}
	if q.Q != "" {
		source := q.Q
		p.Source = &source
	}
	return p
}

// evScope is the API's scopeFilters: a non-anchor sees platform-scoped
// events and its own clients' only.
func evScope(ac *auth.AuthContext, p event.FilterParams) event.FilterParams {
	if !ac.IsAnchor() {
		clients := ac.Clients
		p.AccessibleClientIDs = &clients
	}
	return p
}

// ── list ───────────────────────────────────────────────────────────────

type evRow struct {
	ID, ShortID, Href   string
	Type, Source        string
	Subject             string
	Client, ClientShort string
	Time                time.Time
}

type evView struct {
	Query       evQuery
	Rows        []evRow
	Facets      map[string][]kit.Option
	ClientNames map[string]string
	OlderHref   string
	Drawer      *evDetail
}

func (h *Events) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, evViewPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseEvQuery(r.URL.Query()))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		if err := view.patchList(ds.NewSSE(w, r)); err != nil {
			slog.DebugContext(ctx, "ui: events patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, view)
}

func (h *Events) renderPage(w http.ResponseWriter, r *http.Request, view evView) {
	h.Shell.Render(w, r, shell.Page{Title: "Events", Path: evPath}, evPage(view))
}

// patchList replaces the table (local times, see patchDjTable), patches
// the pager, the toolbar's live parts and the summaries, and the address.
func (v evView) patchList(sse *ds.SSE) error {
	errs := []error{
		sse.Patch(evTable(v), ds.WithMode(ds.ModeReplace)), sse.Patch(kit.CursorPager(v.pager())),
		sse.Patch(kit.TableToolbarState(v.toolbar().State())),
	}
	for _, f := range evFacets {
		p := v.facetProps(f)
		errs = append(errs, sse.Patch(kit.MultiSelectSummary(p)))
		if f == evFacetSubs || f == evFacetAggs || f == evFacetTypes {
			errs = append(errs, sse.Patch(kit.MultiSelectOptions(p)))
		}
	}
	errs = append(errs, sse.ReplaceURL(v.Query.URL()))
	return errors.Join(errs...)
}

func (h *Events) listView(ctx context.Context, ac *auth.AuthContext, q evQuery) (evView, error) {
	v := evView{Query: q}
	cs, err := allClients(ctx, h.Clients)
	if err != nil {
		return evView{}, err
	}
	v.ClientNames = clientNames(cs)
	// The SPA's options: applications, subdomains and event types from the
	// API's filter-options (aggregates it leaves empty); here every level
	// comes from the distinct event types, narrowed by the levels chosen
	// above it, as on the dispatch jobs page.
	distinct, err := h.facetCache.get(ctx, h.Events.DistinctValues, distinctScope(ac), "type")
	if err != nil {
		return evView{}, err
	}
	levels := codeLevels(distinct["type"], q.facet(evFacetApps), q.facet(evFacetSubs), q.facet(evFacetAggs))
	v.Facets = map[string][]kit.Option{evFacetClients: clientFilterOptions(cs, ac)}
	for i, f := range []string{evFacetApps, evFacetSubs, evFacetAggs, evFacetTypes} {
		v.Facets[f] = plainOptions(levels[i])
	}
	for _, f := range evFacets {
		v.Facets[f] = withChosen(v.Facets[f], q.facet(f), v.ClientNames)
	}
	p := evScope(ac, q.filterParams())
	p.Limit, p.Cursor = q.Size, q.After
	events, err := h.Events.FindWithFilters(ctx, p)
	if err != nil {
		return evView{}, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	for i := range events {
		v.Rows = append(v.Rows, toEvRow(&events[i]))
	}
	if len(events) == q.Size {
		last := events[len(events)-1]
		next := q
		next.After = &event.ListCursor{CreatedAt: last.CreatedAt, ID: last.ID}
		v.OlderHref = next.URL()
	}
	return v, nil
}

func evURL(id string) string { return evPath + "/" + url.PathEscape(id) }

// truncateID is the SPA's: ten characters and "...".
func truncateID(id string) string {
	if len(id) > 10 {
		return id[:10] + "..."
	}
	return id
}

func toEvRow(e *event.Event) evRow {
	row := evRow{
		ID: e.ID, ShortID: truncateID(e.ID), Href: evURL(e.ID), Type: e.Type, Source: e.Source,
		Subject: cmp.Or(e.Subject, "-"), Time: e.Time,
	}
	if c := ids.StringPtr(e.ClientID); c != nil && *c != "" {
		row.Client, row.ClientShort = *c, truncateID(*c)
	}
	return row
}

// ── drawer ─────────────────────────────────────────────────────────────

type evDetail struct {
	ID, Type                   string
	Application, Subdomain     string
	Aggregate, Source, Subject string
	Time, ProjectedAt          time.Time
	ClientID, MessageGroup     string
	Correlation, Causation     string
	Deduplication              string
	Data                       string
	Context                    []djKV
}

func (d evDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: evDrawerID, Signal: evDrawerSig, Label: "Event", Size: kit.DrawerWide, Title: cmp.Or(d.Type, "Event"), Subtitle: d.ID}
}

// formatData is the SPA's formatData: JSON pretty-printed (a JSON string
// holding JSON is parsed again), "-" for nothing.
func formatData(raw []byte) string {
	if len(raw) == 0 || string(raw) == "null" {
		return "-"
	}
	p, ok := schemacodegen.PrettyJSON(string(raw))
	if !ok {
		return string(raw)
	}
	// A JSON string: the SPA tries to parse its content as JSON.
	if strings.HasPrefix(strings.TrimSpace(string(raw)), `"`) {
		if s, err := strconv.Unquote(strings.TrimSpace(string(raw))); err == nil {
			if inner, ok := schemacodegen.PrettyJSON(s); ok {
				return inner
			}
			return s
		}
	}
	return p
}

func toEvDetail(e *event.Event) evDetail {
	d := evDetail{
		ID: e.ID, Type: e.Type, Application: cmp.Or(deref(e.Application), "-"), Subdomain: cmp.Or(deref(e.Subdomain), "-"),
		Aggregate: cmp.Or(deref(e.Aggregate), "-"), Source: e.Source, Subject: cmp.Or(e.Subject, "-"), Time: e.Time,
		MessageGroup: cmp.Or(deref(e.MessageGroup), "-"), Correlation: cmp.Or(deref(e.CorrelationID), "-"),
		Causation: cmp.Or(deref(e.CausationID), "-"), Deduplication: cmp.Or(e.DeduplicationID, "-"), Data: formatData(e.Data),
	}
	if c := ids.StringPtr(e.ClientID); c != nil {
		d.ClientID = *c
	}
	if e.ProjectedAt != nil {
		d.ProjectedAt = *e.ProjectedAt
	}
	for _, c := range e.Context {
		d.Context = append(d.Context, djKV{Key: c.Key, Value: c.Value})
	}
	return d
}

func (h *Events) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, evViewPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	if !eventIDPattern.MatchString(id) {
		h.Shell.Fail(w, r, httperror.NotFound("Event", id))
		return
	}
	e, err := h.Events.FindByID(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "find_by_id failed", err))
		return
	}
	if e == nil {
		h.Shell.Fail(w, r, httperror.NotFound("Event", id))
		return
	}
	// The API's per-event check: a client's event needs access to that
	// client; platform-scoped events are visible to any viewer.
	if c := ids.StringPtr(e.ClientID); c != nil && !ac.CanAccessClient(*c) {
		h.Shell.Fail(w, r, httperror.Forbidden("No access to this event"))
		return
	}
	if len(e.Context) == 0 {
		if e.Context, err = h.Events.ContextByID(ctx, e.ID); err != nil {
			h.Shell.Fail(w, r, usecase.Internal("REPO", "context_by_id failed", err))
			return
		}
	}
	d := toEvDetail(e)
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(
			sse.Patch(evDrawerContent(d), ds.Inner("#"+evDrawerID)),
			sse.Patch(evDrawerBody(d), ds.WithMode(ds.ModeReplace)),
			sse.PatchSignals(map[string]any{evDrawerSig: d.ID}),
			sse.Focus(evDrawerID+"-title"),
		); err != nil {
			slog.DebugContext(ctx, "ui: event drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseEvQuery(nil))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, view)
}
