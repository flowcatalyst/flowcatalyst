package pages

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	djops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/schemacodegen"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// DispatchJobStore is the slice of the dispatch-job repository the page
// reads (the SPA's GET /api/dispatch-jobs, /{id}, /{id}/attempts and
// /filter-options).
type DispatchJobStore interface {
	FindWithFilters(ctx context.Context, p dispatchjob.FilterParams) ([]dispatchjob.DispatchJob, error)
	FindByID(ctx context.Context, id string) (*dispatchjob.DispatchJob, error)
	// FindByIDs reads the ticked rows' statuses (the requeue dialog counts
	// the COMPLETED ones).
	FindByIDs(ctx context.Context, ids []string) ([]dispatchjob.DispatchJob, error)
	AttemptsByJob(ctx context.Context, jobID string) ([]dispatchjob.Attempt, error)
	DistinctValues(ctx context.Context, column string, limit int, accessibleClientIDs *[]string, since, until *time.Time) ([]string, error)
	// ResolveRefs resolves the by-name subscription and connection filters to
	// ids (the API does the same before its list query; an unresolved name
	// selects nothing).
	ResolveRefs(ctx context.Context, p dispatchjob.FilterParams) (dispatchjob.FilterParams, error)
}

// DispatchJobAudit reads a job's audit rows (the SPA's
// GET /api/audit-logs/entity/Dispatchjob/{id}); *audit.Repository satisfies it.
type DispatchJobAudit interface {
	FindWithFilters(ctx context.Context, p audit.FilterParams) ([]audit.Log, error)
}

// DispatchJobs is the dispatch jobs page (/ui/dispatch-jobs), mirroring the
// SPA's DispatchJobListPage, DispatchJobDetailDrawer and
// RequeueByFilterDialog. Every route makes the API's checks: the
// dispatch-job view permission, SQL tenant scoping (operations.VisibleTo)
// and per-job client scope; "sign" needs the raw-view permission.
type DispatchJobs struct {
	Jobs    DispatchJobStore
	Ops     DispatchJobCommands
	Clients ClientStore
	// Subs and Connections name the drawer's subscription and connection, as
	// the SPA does best effort (a caller without their view permissions sees
	// the bare id); Audit fills the drawer's Audit section for callers with
	// the audit-log view permission. Any may be nil.
	Subs        SubscriptionStore
	Connections ConnectionStore
	Audit       DispatchJobAudit
	// ClientIdentifier resolves a job's client id to the client's
	// identifier for the grid (the API's cached resolver); nil shows ids.
	ClientIdentifier func(ctx context.Context, clientID string) (string, bool)
	Shell            *shell.Shell
	// MaxWindowDays is the widest created_at window a list or a requeue may
	// cover (FC_DISPATCH_JOB_MAX_WINDOW_DAYS; < 1: the default, 31). A view with
	// no dates is held to the last 24 hours, as the API's list is.
	MaxWindowDays int
	// Now is the clock the time window is cut from (nil: time.Now).
	Now func() time.Time

	// facetCache holds the distinct statuses and codes (firehose.go): the
	// SPA reads them once per visit, the live list on every request, and a
	// DISTINCT over the projection is the costliest query of a request.
	facetCache distinctCache
}

// window holds p to the created_at window every list-shaped read carries (the
// API's rule: the last 24 hours when the view names no dates, at most
// MaxWindowDays).
func (h *DispatchJobs) window(p dispatchjob.FilterParams) (dispatchjob.FilterParams, dispatchjob.Window, error) {
	return dispatchjob.EnforceWindow(p, h.now(), h.MaxWindowDays)
}

func (h *DispatchJobs) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// scoped is the filter a read runs on, in the API's order: the view's filter
// (a window preset becomes its since), scoped to the caller's tenants, held
// to the time window, and its by-name subscription and connection references
// resolved to ids. A view that cannot be run (too many event types) is a 400.
func (h *DispatchJobs) scoped(ctx context.Context, ac *auth.AuthContext, q djQuery) (dispatchjob.FilterParams, dispatchjob.Window, error) {
	if err := q.validate(); err != nil {
		return dispatchjob.FilterParams{}, dispatchjob.Window{}, err
	}
	p := q.filterParams()
	if preset, ok := djWindowPresets[q.Window]; ok && p.Since == nil && p.Until == nil {
		since := h.now().Add(-preset.d)
		p.Since = &since
	}
	if q.stuckLens() {
		// Nothing newer than five minutes; the cap only ever lowers the bound.
		if cap := h.now().Add(-djStuckAfter); p.Until == nil || cap.Before(*p.Until) {
			p.Until = &cap
		}
	}
	p, win, err := h.window(djops.VisibleTo(ac, p))
	if err != nil {
		return p, win, err
	}
	p, err = h.Jobs.ResolveRefs(ctx, p)
	return p, win, err
}

const (
	djPath   = "/ui/dispatch-jobs"
	djFormID = "dj-list"

	djDrawerID   = "dj-drawer"
	djDrawerSig  = "selDj"
	djDialogSlot = "dj-dialog-slot"
	djRBFDialog  = "dj-rbf"
	// The drawer's confirm dialogs; their form action comes from the button.
	djCancelDialog = "dj-cancel"
	// djCancelInFlightDialog is the Cancel confirmation of a job in flight;
	// djRequeueInFlightDialog the Requeue confirmation of one.
	djCancelInFlightDialog  = "dj-cancel-inflight"
	djRequeueInFlightDialog = "dj-requeue-inflight"
	djCompleteDialog        = "dj-complete"
	// djSelDialog is the requeue dialog of ticked rows that include COMPLETED
	// jobs (the SPA's RequeueSelectedDialog).
	djSelDialog = "dj-sel"
	// djIgnoreDialog is the confirmation of "Ignore selected".
	djIgnoreDialog = "dj-ignore"

	// djSelSig counts the ticked rows (the "Requeue selected" label).
	djSelSig = "djSel"

	djViewPerm        = "platform:messaging:dispatch-job:view"
	djRawPerm         = "platform:messaging:dispatch-job:view-raw"
	djRequeuePerm     = auth.DispatchJobRequeuePerm
	djEditPayloadPerm = auth.DispatchJobEditPayloadPerm
	djCancelPerm      = auth.DispatchJobCancelPerm
	djCompletePerm    = auth.DispatchJobCompletePerm
	djRedrivePerm     = auth.DispatchJobRedriveCompletedPerm
	djAuditPerm       = "platform:admin:audit-log:view"
	// djAuditEntity is the audit entity type of a dispatch job: its domain
	// events use the subject platform.dispatchjob.<id>, which the audit
	// writer capitalises.
	djAuditEntity = "Dispatchjob"

	djLiveGet = "@get('/ui/dispatch-jobs', {contentType: 'form'})"
	// djNavLink is a link that changes the filters (the group link, the
	// error preset): its answer re-renders the whole toolbar, which a typed
	// search must never be (it would drop keystrokes made in flight).
	djNavLink = "@get(el.href, {headers: {'" + djToolbarHeader + "': 'full'}})"

	djToolbarHeader = "X-FC-Toolbar"

	// Facets: the SPA's URL keys.
	djFacetClients  = "clients"
	djFacetApps     = "applications"
	djFacetSubs     = "subdomains"
	djFacetAggs     = "aggregates"
	djFacetCodes    = "codes"
	djFacetStatuses = "statuses"

	// djStuckAfter is how old a PENDING or QUEUED job must be to count as
	// stuck (jobFilters.ts STUCK_AFTER_MS).
	djStuckAfter = 5 * time.Minute
	// djLensStuck is the lens parameter's one value.
	djLensStuck = "stuck"

	djDefaultSize = 200
	// djMaxRequeue is ResendByFilterMaxLimit: one requeue-by-filter run's cap.
	djMaxRequeue = djops.ResendByFilterMaxLimit
)

var (
	// djWindowPresets are the window picker's presets (jobWindow.ts). The
	// default (24 hours) is the absence of a preset: the window then comes
	// from the server and is shown as defaulted.
	djWindowPresets = map[string]djWindowPreset{
		"1h":  {"Last hour", time.Hour},
		"3d":  {"Last 3 days", 3 * 24 * time.Hour},
		"7d":  {"Last 7 days", 7 * 24 * time.Hour},
		"31d": {"Last 31 days", 31 * 24 * time.Hour},
	}
	djWindowOrder = []string{"1h", "", "3d", "7d", "31d"}

	djFacets = []string{djFacetClients, djFacetApps, djFacetSubs, djFacetAggs, djFacetCodes, djFacetStatuses}
	djSizes  = []int{50, 100, 200, 500, 1000}

	djList = &listSpec{
		path: djPath, sortKeys: []string{"createdAt"}, defaultDir: kit.SortDesc,
		statusOK: func(string) bool { return false }, facets: djFacets,
	}

	// djErrorStatuses are the "in error" statuses: FAILED and its legacy
	// alias ERROR (requeueByFilter.ts ERROR_STATUSES).
	djErrorStatuses = []string{"FAILED", "ERROR"}
	// djStuckStatuses are the statuses of the Stuck quick view.
	djStuckStatuses = []string{"PENDING", "QUEUED"}
)

type djWindowPreset struct {
	label string
	d     time.Duration
}

// djRedriveWord is the literal a redrive's second step asks to be typed (and
// the API's confirmRedriveCompleted).
const djRedriveWord = dispatchjob.RedriveConfirmation

// djStep sets a dialog form's step and submits it (the footer's buttons sit
// outside the form; both ids are constants).
func djStep(formID, step string) string {
	return "document.getElementById('" + kit.ID(formID) + "').elements.step.value = '" + kit.ID(step) + "'; document.getElementById('" + kit.ID(formID) + "').requestSubmit()"
}

// djMaxEventTypes is the API's cap on eventTypes (TOO_MANY_EVENT_TYPES).
const djMaxEventTypes = 10

// Mount registers the page's routes (POSTs behind parseForm).
func (h *DispatchJobs) Mount(r chi.Router) {
	r.Get("/dispatch-jobs", h.list)
	r.Get("/dispatch-jobs/requeue-by-filter", h.requeueByFilterDialog)
	r.Get("/dispatch-jobs/requeue-errors", h.requeueErrors)
	r.Get("/dispatch-jobs/{id}", h.detail)
	r.Get("/dispatch-jobs/{id}/audit", h.auditSection)
	post := r.With(parseForm)
	post.Post("/dispatch-jobs/requeue", h.requeueSelected)
	post.Post("/dispatch-jobs/cancel", h.cancelSelected)
	post.Post("/dispatch-jobs/requeue-by-filter", h.requeueByFilter)
	post.Post("/dispatch-jobs/{id}/requeue", h.requeueOne)
	post.Post("/dispatch-jobs/{id}/redrive", h.redriveOne)
	post.Post("/dispatch-jobs/{id}/sign", h.sign)
	post.Post("/dispatch-jobs/{id}/cancel", h.cancel)
	post.Post("/dispatch-jobs/{id}/complete", h.complete)
	post.Post("/dispatch-jobs/{id}/payload", h.editPayload)
}

// ── URL state ──────────────────────────────────────────────────────────

// djQuery is one view of the list: the facets, the source search (q), the
// message group, the created-at day range, the sort, the result size and
// the keyset position. Everything but the size, sort and position is the
// filter a requeue-by-filter acts on.
type djQuery struct {
	listQuery
	Group    string
	From, To string // YYYY-MM-DD, UTC days
	// Window is a preset key (djWindowPresets); empty is the default window.
	// Dates, when chosen, win over it.
	Window string
	// Lens is "stuck" while the view is the Stuck quick view (PENDING and
	// QUEUED jobs older than five minutes); the statuses alone cannot say.
	Lens string
	// EventTypes are the event types asked for (a job's code is its event
	// type); Subscription is a subscription name and Connection a connection
	// code, each exact or ending in * for a prefix.
	EventTypes               []string
	Subscription, Connection string
	Size                     int
	// After is the page's keyset position: rows after this one in the
	// list's order ("Older"). Nil is the first page.
	After *dispatchjob.ListCursor
}

func parseDJQuery(v url.Values) djQuery {
	q := djQuery{listQuery: parseListQuery(v, djList), Size: djDefaultSize}
	q.Group = strings.TrimSpace(v.Get("group"))
	if len(q.Group) > maxQueryLen {
		q.Group = q.Group[:maxQueryLen]
	}
	q.From, q.To = validDay(v.Get("from")), validDay(v.Get("to"))
	if _, ok := djWindowPresets[v.Get("window")]; ok {
		q.Window = v.Get("window")
	}
	q.EventTypes = parseEventTypes(v["eventTypes"])
	// Picking other statuses by hand leaves the Stuck cap behind.
	if v.Get("lens") == djLensStuck && sameSet(q.facet(djFacetStatuses), djStuckStatuses) {
		q.Lens = djLensStuck
	}
	q.Subscription, q.Connection = clipQuery(v.Get("subscription")), clipQuery(v.Get("connection"))
	if n, err := strconv.Atoi(v.Get("size")); err == nil && slices.Contains(djSizes, n) {
		q.Size = n
	}
	if at, id, ok := parseCursor(v.Get("after")); ok {
		q.After = &dispatchjob.ListCursor{CreatedAt: at, ID: id}
	}
	return q
}

func clipQuery(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxQueryLen {
		s = s[:maxQueryLen]
	}
	return s
}

// parseEventTypes reads the event type filter: values, each possibly a
// comma-separated list, trimmed and de-duplicated. The count is not capped
// here (validate says so, as the API's 400 does).
func parseEventTypes(raw []string) []string {
	var out []string
	for _, r := range raw {
		for part := range strings.SplitSeq(r, ",") {
			part = strings.TrimSpace(part)
			if part != "" && len(part) <= maxQueryLen && !slices.Contains(out, part) {
				out = append(out, part)
			}
		}
	}
	return out
}

// validate refuses a view the API would: more than 10 event types.
func (q djQuery) validate() error {
	if len(q.EventTypes) > djMaxEventTypes {
		return usecase.Validation("TOO_MANY_EVENT_TYPES", fmt.Sprintf("at most %d event types can be filtered at once (got %d)", djMaxEventTypes, len(q.EventTypes)))
	}
	return nil
}

// filterValues is the filter alone: what "requeue all matching" acts on.
func (q djQuery) filterValues() url.Values {
	v := url.Values{}
	for _, name := range djFacets {
		for _, val := range q.Facets[name] {
			v.Add(name, val)
		}
	}
	if q.Q != "" {
		v.Set("q", q.Q)
	}
	if q.Group != "" {
		v.Set("group", q.Group)
	}
	if q.From != "" {
		v.Set("from", q.From)
	}
	if q.To != "" {
		v.Set("to", q.To)
	}
	if q.Window != "" {
		v.Set("window", q.Window)
	}
	if q.Lens != "" {
		v.Set("lens", q.Lens)
	}
	if len(q.EventTypes) > 0 {
		v.Set("eventTypes", strings.Join(q.EventTypes, ","))
	}
	if q.Subscription != "" {
		v.Set("subscription", q.Subscription)
	}
	if q.Connection != "" {
		v.Set("connection", q.Connection)
	}
	return v
}

func (q djQuery) values() url.Values {
	v := q.listQuery.values()
	maps.Copy(v, q.filterValues())
	if q.Size != djDefaultSize {
		v.Set("size", strconv.Itoa(q.Size))
	}
	if q.After != nil {
		v.Set("after", formatCursor(q.After.CreatedAt, q.After.ID))
	}
	return v
}

func encodeURL(path string, v url.Values) string {
	if enc := v.Encode(); enc != "" {
		return path + "?" + enc
	}
	return path
}

// sizeOnly keeps only a non-default result size and the time window (the
// error preset's URL).
func (q djQuery) sizeOnly() url.Values {
	v := url.Values{}
	if q.Size != djDefaultSize {
		v.Set("size", strconv.Itoa(q.Size))
	}
	// The error preset keeps the time window: its run is held to it.
	for k, val := range map[string]string{"window": q.Window, "from": q.From, "to": q.To} {
		if val != "" {
			v.Set(k, val)
		}
	}
	return v
}

// URL is the canonical URL of this view.
func (q djQuery) URL() string { return encodeURL(djPath, q.values()) }

// first is this view's first page.
func (q djQuery) first() djQuery {
	q.After = nil
	return q
}

// clearURL keeps the sort and the size (table-toolbar.md: Clear All clears
// what the user typed and picked).
func (q djQuery) clearURL() string {
	clear := djQuery{Size: q.Size}
	clear.spec, clear.Sort, clear.Dir = djList, q.Sort, q.Dir
	return clear.URL()
}

// sortURL flips the created-at order, from the first page.
func (q djQuery) sortURL() string {
	next := q.first()
	next.Sort = djList.sortKeys[0]
	next.Dir = kit.SortAsc
	if q.Dir == kit.SortAsc {
		next.Dir = kit.SortDesc
	}
	return next.URL()
}

// withGroup is the first page of this view narrowed to one message group.
func (q djQuery) withGroup(group string) djQuery {
	next := q.first()
	next.Group = group
	return next
}

func (q djQuery) filtered() bool {
	return q.listQuery.filtered() || q.Group != "" || q.From != "" || q.To != "" || q.Window != "" ||
		len(q.EventTypes) > 0 || q.Subscription != "" || q.Connection != ""
}

// failedLens reports whether the view is exactly the Failed quick view:
// FAILED and ERROR (their legacy alias) and nothing else in the status facet.
func (q djQuery) failedLens() bool {
	st := q.facet(djFacetStatuses)
	return len(st) == len(djErrorStatuses) && slices.ContainsFunc(st, func(s string) bool { return s == "FAILED" }) &&
		slices.ContainsFunc(st, func(s string) bool { return s == "ERROR" })
}

func sameSet(a, b []string) bool {
	return len(a) == len(b) && !slices.ContainsFunc(a, func(s string) bool { return !slices.Contains(b, s) })
}

// stuckLens reports whether the view is the Stuck quick view.
func (q djQuery) stuckLens() bool { return q.Lens == djLensStuck }

// failedLensURL toggles the Failed quick view on this view: it sets the status
// facet to FAILED and ERROR, or, when it is already on, clears it.
func (q djQuery) failedLensURL() string { return q.lensURL(q.failedLens(), djErrorStatuses, "") }

// stuckLensURL toggles the Stuck quick view: PENDING and QUEUED jobs created
// more than five minutes ago.
func (q djQuery) stuckLensURL() string { return q.lensURL(q.stuckLens(), djStuckStatuses, djLensStuck) }

func (q djQuery) lensURL(on bool, statuses []string, lens string) string {
	next := q.first()
	next.Lens = ""
	if on {
		next.Facets = q.withFacet(djFacetStatuses, nil).Facets
	} else {
		next.Facets = q.withFacet(djFacetStatuses, slices.Clone(statuses)).Facets
		next.Lens = lens
	}
	return next.URL()
}

// windowURL is this view with a window preset ("" is the default) and no
// chosen dates.
func (q djQuery) windowURL(key string) string {
	next := q.first()
	next.Window, next.From, next.To = key, "", ""
	return next.URL()
}

// activeFilters is the Filters badge: the facets in use plus the group, the
// event type, subscription and connection filters. The window and its dates
// sit in their own bar above the table, so they are not counted here.
func (q djQuery) activeFilters() int {
	n := len(q.Facets)
	for _, s := range []string{q.Group, q.Subscription, q.Connection} {
		if s != "" {
			n++
		}
	}
	if len(q.EventTypes) > 0 {
		n++
	}
	return n
}

// filterParams converts the filter: since is the start of the "from" day,
// until the start of the day after "to" (the SPA's whole-day bounds, here
// in UTC).
func (q djQuery) filterParams() dispatchjob.FilterParams {
	p := dispatchjob.FilterParams{
		ClientIDs: q.facet(djFacetClients), Statuses: q.facet(djFacetStatuses),
		Applications: q.facet(djFacetApps), Subdomains: q.facet(djFacetSubs),
		Aggregates: q.facet(djFacetAggs), Codes: q.facet(djFacetCodes),
	}
	if q.Q != "" {
		source := q.Q
		p.Source = &source
	}
	if q.Group != "" {
		group := q.Group
		p.MessageGroup = &group
	}
	p.Since, p.Until = dayBounds(q.From, q.To)
	p.EventTypes = q.EventTypes
	if q.Subscription != "" {
		name := q.Subscription
		p.SubscriptionName = &name
	}
	if q.Connection != "" {
		code := q.Connection
		p.ConnectionCode = &code
	}
	return p
}

// djReferer is the list view the request came from (see refererList).
func djReferer(r *http.Request) djQuery {
	ref, err := url.Parse(r.Referer())
	if err != nil || ref.Path != djPath {
		return parseDJQuery(nil)
	}
	return parseDJQuery(ref.Query())
}

// ── list ───────────────────────────────────────────────────────────────

type djRow struct {
	ID, Href         string
	Descriptor, Code string
	Client           string
	ClientIsID       bool
	Platform         bool
	// CanRequeue and CanRedrive show the row's Requeue (a status the API
	// resets) or Redrive (a COMPLETED job, for holders of redrive-completed).
	CanRequeue, CanRedrive bool
	RedriveAction          string
	Group, GroupHref       string
	Status                 string
	Severity               kit.TagSeverity
	Meta, MetaFull         string
	Created                time.Time
	RequeueAction          string
	ValidID                bool
}

type djView struct {
	Query  djQuery
	Rows   []djRow
	Facets map[string][]kit.Option
	// ClientNames labels the client filter in the confirm summary.
	ClientNames map[string]string
	// OlderHref is the next page (empty on the last).
	OlderHref string
	CanSign   bool
	// CanRequeue shows the requeue actions (dispatch-job:requeue).
	CanRequeue bool
	// CanCancel shows "Ignore selected" (dispatch-job:cancel).
	CanCancel bool
	// CanRedrive adds the redrive of COMPLETED jobs (dispatch-job:redrive-completed
	// on top of the requeue permission).
	CanRedrive bool
	// Window is the created_at window the list covers (the one the query
	// applied, defaulted or not).
	Window dispatchjob.Window
	// WindowText is the line above the table ("Showing the last 24 hours").
	WindowText string
	// EventTypeOptions are the distinct codes seen in the widest window, the
	// event type filter's suggestions.
	EventTypeOptions []kit.Option
	Dialog           templ.Component
	Drawer           *djDetail
}

func (h *DispatchJobs) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, djViewPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseDJQuery(r.URL.Query()))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := view.patchList(sse, r.Header.Get(djToolbarHeader) == "full"); err != nil {
			slog.DebugContext(ctx, "ui: dispatch jobs patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *DispatchJobs) renderPage(w http.ResponseWriter, r *http.Request, status int, view djView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Dispatch Jobs", Path: djPath}, djPage(view))
}

// patchList patches the table, the pager, the requeue bar and the
// toolbar's live parts (or, with full, the whole toolbar), resets the
// selection and replaces the address.
func (v djView) patchList(sse *ds.SSE, full bool) error {
	errs := []error{
		patchDjTable(sse, v), sse.Patch(kit.CursorPager(v.pager())), sse.Patch(djBulkBar(v)), sse.Patch(djWindowBar(v)),
		sse.PatchSignals(map[string]any{djSelSig: 0}),
	}
	if full {
		errs = append(errs, sse.Patch(kit.TableToolbar(v.toolbar())))
	} else {
		errs = append(errs, sse.Patch(kit.TableToolbarState(v.toolbar().State())))
		for _, f := range djFacets {
			p := v.facetProps(f)
			errs = append(errs, sse.Patch(kit.MultiSelectSummary(p)))
			if f != djFacetClients && f != djFacetStatuses && f != djFacetApps {
				// The cascading facets' options follow the parents' choice.
				errs = append(errs, sse.Patch(kit.MultiSelectOptions(p)))
			}
		}
	}
	errs = append(errs, sse.ReplaceURL(v.Query.URL()))
	return errors.Join(errs...)
}

func (h *DispatchJobs) listView(ctx context.Context, ac *auth.AuthContext, q djQuery) (djView, error) {
	v := djView{
		Query: q, CanSign: auth.CanWritePermission(ac, djRawPerm) == nil,
		CanRequeue: auth.CanWritePermission(ac, djRequeuePerm) == nil,
		CanCancel:  auth.CanWritePermission(ac, djCancelPerm) == nil,
	}
	v.CanRedrive = v.CanRequeue && auth.CanWritePermission(ac, djRedrivePerm) == nil
	facets, names, err := h.filterOptions(ctx, ac, q)
	if err != nil {
		return djView{}, err
	}
	v.Facets, v.ClientNames = facets, names
	p, win, err := h.scoped(ctx, ac, q)
	if err != nil {
		return djView{}, err
	}
	v.Window, v.EventTypeOptions = win, facets["eventTypes"]
	v.WindowText = windowText(win, h.now())
	p.Limit, p.SortAscending, p.Cursor = q.Size, q.Dir == kit.SortAsc, q.After
	jobs, err := h.Jobs.FindWithFilters(ctx, p)
	if err != nil {
		return djView{}, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	for i := range jobs {
		row := h.toRow(ctx, q, &jobs[i])
		row.CanRequeue = v.CanRequeue && slices.Contains(dispatchjob.RequeueableStatuses, jobs[i].Status)
		row.CanRedrive = v.CanRedrive && jobs[i].Status == common.DispatchCompleted
		v.Rows = append(v.Rows, row)
	}
	// A full page may have an older one; a short page is the last.
	if len(jobs) == q.Size {
		last := jobs[len(jobs)-1]
		next := q
		next.After = &dispatchjob.ListCursor{CreatedAt: last.CreatedAt, ID: last.ID}
		v.OlderHref = next.URL()
	}
	return v, nil
}

// filterOptions are the SPA's: the clients the caller can see (active,
// sorted by name), the distinct statuses and codes the API's filter-options
// returns (scoped to the caller's tenants, as the list), and (new: the SPA leaves them empty, see docs/ui/README.md) the
// application, subdomain and aggregate segments of those codes, each
// narrowed by the levels chosen above it. A chosen value that is not among
// the options is kept as an option, so a URL never silently loses a filter.
func (h *DispatchJobs) filterOptions(ctx context.Context, ac *auth.AuthContext, q djQuery) (map[string][]kit.Option, map[string]string, error) {
	out := map[string][]kit.Option{}
	cs, err := allClients(ctx, h.Clients)
	if err != nil {
		return nil, nil, err
	}
	names := clientNames(cs)
	out[djFacetClients] = clientFilterOptions(cs, ac)
	// The values are read from the widest window a view may ask for, so every
	// date range the page allows finds its options; the cache keeps the read to
	// one per scope per TTL.
	since := time.Now().Add(-dispatchjob.MaxWindow(h.MaxWindowDays))
	recent := func(ctx context.Context, column string, limit int, scope *[]string) ([]string, error) {
		return h.Jobs.DistinctValues(ctx, column, limit, scope, &since, nil)
	}
	distinct, err := h.facetCache.get(ctx, recent, distinctScope(ac), "status", "code")
	if err != nil {
		return nil, nil, err
	}
	out[djFacetStatuses] = plainOptions(distinct["status"])
	levels := codeLevels(distinct["code"], q.facet(djFacetApps), q.facet(djFacetSubs), q.facet(djFacetAggs))
	for i, f := range []string{djFacetApps, djFacetSubs, djFacetAggs, djFacetCodes} {
		out[f] = plainOptions(levels[i])
	}
	for _, f := range djFacets {
		out[f] = withChosen(out[f], q.facet(f), names)
	}
	// Not a facet: the suggestions of the event type filter (every code seen).
	out["eventTypes"] = plainOptions(distinct["code"])
	return out, names, nil
}

func djURL(id string) string { return djPath + "/" + url.PathEscape(id) }

// djSeverity is the list's statusSeverity (requeueByFilter.ts).
func djSeverity(status string) kit.TagSeverity {
	switch status {
	case "COMPLETED":
		return kit.TagSuccess
	case "PENDING", "QUEUED":
		return kit.TagInfo
	case "PROCESSING":
		return kit.TagWarn
	case "FAILED", "ERROR":
		return kit.TagDanger
	}
	return kit.TagSecondary
}

// metadataPreview is the SPA's: key=value pairs, clipped at 20 characters.
func metadataPreview(meta []dispatchjob.Metadata) (preview, full string) {
	if len(meta) == 0 {
		return "", ""
	}
	pairs, lines := make([]string, 0, len(meta)), make([]string, 0, len(meta))
	for _, m := range meta {
		pairs = append(pairs, m.Key+"="+m.Value)
		lines = append(lines, m.Key+": "+m.Value)
	}
	preview = strings.Join(pairs, ", ")
	if r := []rune(preview); len(r) > 20 {
		preview = string(r[:20]) + "…"
	}
	return preview, strings.Join(lines, "\n")
}

func (h *DispatchJobs) toRow(ctx context.Context, q djQuery, j *dispatchjob.DispatchJob) djRow {
	row := djRow{
		ID: j.ID, Href: djURL(j.ID), Descriptor: deref(j.Descriptor), Code: j.Code,
		Status: string(j.Status), Severity: djSeverity(string(j.Status)), Created: j.CreatedAt,
		RequeueAction: djURL(j.ID) + "/requeue", RedriveAction: djURL(j.ID) + "/redrive", ValidID: kit.ValidRecordID(j.ID),
	}
	row.Meta, row.MetaFull = metadataPreview(j.Metadata)
	switch {
	case j.ClientID == nil || *j.ClientID == "":
		row.Platform = true
	case h.ClientIdentifier != nil:
		if ident, ok := h.ClientIdentifier(ctx, *j.ClientID); ok {
			row.Client = ident
		} else {
			row.Client, row.ClientIsID = *j.ClientID, true
		}
	default:
		row.Client, row.ClientIsID = *j.ClientID, true
	}
	if j.MessageGroup != nil && *j.MessageGroup != "" {
		row.Group = *j.MessageGroup
		row.GroupHref = q.withGroup(row.Group).URL()
	}
	return row
}

// ── drawer ─────────────────────────────────────────────────────────────

type djKV struct{ Key, Value string }

type djAttempt struct {
	Label           string
	Outcome         string
	OutcomeSeverity kit.TagSeverity
	At              time.Time
	Duration        string
	Sent, Timestamp string
	Headers, Error  string
	Response        string
	AttemptOfTheRun string
}

type djDetail struct {
	ID, Title                  string
	Status                     string
	StatusSeverity             kit.TagSeverity
	Descriptor, Code, ClientID string
	Group, Mode, Target        string
	Subscription, Event        string
	// SubName, SubCode and SubHref name the subscription, ConnCode and
	// ConnHref its connection (best effort: empty for a caller who may not
	// read them, then the bare id shows); ConnShown says the job has a
	// subscription, so the Connection line is drawn.
	SubName, SubCode, SubHref string
	ConnCode, ConnHref        string
	ConnShown                 bool
	Attempts                  string
	ScheduledFor, Created     time.Time
	Completed                 time.Time
	LastError                 string
	Metadata                  []djKV
	Payload                   string
	History                   []djAttempt
	ActionBase                string
	CanSign                   bool
	// CanRequeue shows the drawer's Requeue (dispatch-job:requeue, and a status
	// the API resets); CanRedrive its Redrive (a COMPLETED job, redrive-completed).
	CanRequeue, CanRedrive bool
	// CanRequeueInFlight shows the drawer's Requeue of a job in flight, behind
	// its own confirmation (a delivery under way may land as well); InFlight
	// picks the Cancel job confirmation that says so.
	CanRequeueInFlight, InFlight bool
	// CanAudit shows the Audit section (audit-log view); AuditHref loads it.
	CanAudit  bool
	AuditHref string
	// CanEditPayload is true while the job's payload may be edited (PENDING or
	// FAILED: the API's rule); EditText and ContentType fill the editor.
	CanEditPayload bool
	// CanCancel and CanComplete show the drawer's Cancel job / Mark completed:
	// the job is FAILED (the API's rule) and the caller holds dispatch-job:cancel
	// / dispatch-job:complete.
	CanCancel, CanComplete bool
	EditText               string
	ContentType            string
	// Plan is the sign action's answer, when the page renders it (without
	// Datastar); a live drawer patches it into its slot instead.
	Plan *djPlan
}

// editorLanguage is the payload editor's language: JSON for a JSON content type.
func (d djDetail) editorLanguage() string {
	if strings.Contains(strings.ToLower(d.ContentType), "json") {
		return "json"
	}
	return ""
}

func (d djDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: djDrawerID, Signal: djDrawerSig, Label: "Dispatch job", Size: kit.DrawerTwoThirds, Title: d.Title, Subtitle: d.ID}
}

// djDrawerSeverity is the drawer's statusSeverity (it differs from the
// list's: CANCELLED and EXPIRED are secondary, anything else not named is
// info).
func djDrawerSeverity(status string) kit.TagSeverity {
	switch status {
	case "COMPLETED":
		return kit.TagSuccess
	case "FAILED":
		return kit.TagDanger
	case "PROCESSING":
		return kit.TagWarn
	case "CANCELLED", "EXPIRED":
		return kit.TagSecondary
	}
	return kit.TagInfo
}

// attemptSeverity is the SPA's: success, a deferral (429 or a 2xx that was
// not acknowledged) is a warning, anything else danger.
func attemptSeverity(a *dispatchjob.Attempt) kit.TagSeverity {
	if a.Success {
		return kit.TagSuccess
	}
	code := 0
	if a.ResponseCode != nil {
		code = *a.ResponseCode
	}
	if code == http.StatusTooManyRequests || (code >= 200 && code < 300) {
		return kit.TagWarn
	}
	return kit.TagDanger
}

// requestLine is the SPA's summary of what was sent.
func requestLine(r *dispatchjob.RequestSummary) string {
	if r == nil {
		return "not recorded"
	}
	if r.UnsignedReason != "" {
		return "UNSIGNED — " + r.UnsignedReason
	}
	var parts []string
	if r.Signature {
		parts = append(parts, "signed by "+cmp.Or(r.SignedBy, "?"))
	}
	if r.Bearer {
		parts = append(parts, "bearer")
	}
	if len(parts) == 0 {
		return "no credentials"
	}
	return strings.Join(parts, " + ")
}

// prettyText indents a JSON document (the SPA's formatJson); anything else
// is shown as stored, and nothing as "-".
func prettyText(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	if p, ok := schemacodegen.PrettyJSON(*s); ok {
		return p
	}
	return *s
}

func toAttempts(rows []dispatchjob.Attempt) []djAttempt {
	out := make([]djAttempt, 0, len(rows))
	// Latest first; the position, not the stored number, is the label
	// (every requeue starts a new run numbered from 1).
	for i := len(rows) - 1; i >= 0; i-- {
		a := &rows[i]
		at := djAttempt{
			Label: "#" + strconv.Itoa(i+1), At: a.AttemptedAt, OutcomeSeverity: attemptSeverity(a),
			Sent:            requestLine(a.Request),
			AttemptOfTheRun: "Attempt " + strconv.Itoa(int(a.AttemptNumber)) + " of its run — a requeue starts a new run numbered from 1",
		}
		switch {
		case a.ResponseCode != nil && *a.ResponseCode != 0:
			at.Outcome = strconv.Itoa(*a.ResponseCode)
		case a.ErrorType != nil && *a.ErrorType != "":
			at.Outcome = string(*a.ErrorType)
		default:
			at.Outcome = "no response"
		}
		if a.DurationMillis != nil {
			at.Duration = strconv.FormatInt(*a.DurationMillis, 10) + "ms"
		}
		if a.Request != nil {
			at.Timestamp = a.Request.Timestamp
			at.Headers = strings.Join(a.Request.Headers, ", ")
		}
		at.Error = deref(a.ErrorMessage)
		if a.ResponseBody != nil && *a.ResponseBody != "" {
			at.Response = prettyText(a.ResponseBody)
		}
		out = append(out, at)
	}
	return out
}

func timeOr(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// loadJob reads a job with the API's checks: not found, then the job's
// client scope.
func (h *DispatchJobs) loadJob(ctx context.Context, ac *auth.AuthContext, id string) (*dispatchjob.DispatchJob, error) {
	j, err := h.Jobs.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if j == nil {
		return nil, httperror.NotFound("DispatchJob", id)
	}
	if err := auth.CheckScopeAccess(ac, ids.StringPtr(j.ClientID)); err != nil {
		return nil, err
	}
	return j, nil
}

func (h *DispatchJobs) detailOf(ctx context.Context, ac *auth.AuthContext, j *dispatchjob.DispatchJob) (djDetail, error) {
	attempts, err := h.Jobs.AttemptsByJob(ctx, j.ID)
	if err != nil {
		return djDetail{}, usecase.Internal("REPO", "attempts failed", err)
	}
	d := djDetail{
		ID: j.ID, Title: cmp.Or(deref(j.Descriptor), j.Code, "Dispatch job"),
		Status: string(j.Status), StatusSeverity: djDrawerSeverity(string(j.Status)),
		Descriptor: cmp.Or(deref(j.Descriptor), "-"), Code: j.Code, ClientID: cmp.Or(deref(j.ClientID), "-"),
		Group: cmp.Or(deref(j.MessageGroup), "-"), Mode: string(j.Mode), Target: j.TargetURL,
		Subscription: cmp.Or(deref(j.SubscriptionID), "-"), Event: cmp.Or(deref(j.EventID), "-"),
		Attempts:     fmt.Sprintf("%d / %d", j.AttemptCount, j.MaxRetries),
		ScheduledFor: timeOr(j.ScheduledFor), Created: j.CreatedAt, Completed: timeOr(j.CompletedAt),
		LastError: deref(j.LastError), Payload: prettyText(j.Payload),
		History: toAttempts(attempts), ActionBase: djURL(j.ID),
		CanSign:            auth.CanWritePermission(ac, djRawPerm) == nil,
		CanRequeue:         auth.CanWritePermission(ac, djRequeuePerm) == nil && slices.Contains(dispatchjob.RequeueableStatuses, j.Status),
		CanRequeueInFlight: slices.Contains(dispatchjob.InFlightStatuses, j.Status) && auth.CanWritePermission(ac, djRequeuePerm) == nil,
		InFlight:           slices.Contains(dispatchjob.InFlightStatuses, j.Status),
		CanRedrive:         j.Status == common.DispatchCompleted && auth.CanWritePermission(ac, djRequeuePerm) == nil && auth.CanWritePermission(ac, djRedrivePerm) == nil,
		CanAudit:           h.Audit != nil && auth.CanWritePermission(ac, djAuditPerm) == nil, AuditHref: djURL(j.ID) + "/audit",
		// The editor opens on the stored text, pretty-printed for a JSON job.
		CanEditPayload: slices.Contains(dispatchjob.EditableStatuses, j.Status) && auth.CanWritePermission(ac, djEditPayloadPerm) == nil,
		ContentType:    j.PayloadContentType,
		CanCancel:      slices.Contains(dispatchjob.CancellableStatuses, j.Status) && auth.CanWritePermission(ac, djCancelPerm) == nil,
		CanComplete:    j.Status == common.DispatchFailed && auth.CanWritePermission(ac, djCompletePerm) == nil,
	}
	if j.Payload != nil {
		d.EditText = *j.Payload
		if pretty, ok := schemacodegen.PrettyJSON(*j.Payload); ok && strings.Contains(strings.ToLower(j.PayloadContentType), "json") {
			d.EditText = pretty
		}
	}
	for _, m := range j.Metadata {
		d.Metadata = append(d.Metadata, djKV{Key: m.Key, Value: m.Value})
	}
	h.nameRefs(ctx, ac, j, &d)
	return d, nil
}

// nameRefs fills the drawer's subscription and connection links, as the SPA
// does after the job loads: best effort, with the reads' own checks, so a
// caller who may not read subscriptions or connections (or a record that is
// gone) keeps the bare id.
func (h *DispatchJobs) nameRefs(ctx context.Context, ac *auth.AuthContext, j *dispatchjob.DispatchJob, d *djDetail) {
	if j.SubscriptionID == nil || *j.SubscriptionID == "" {
		return
	}
	d.ConnShown = true
	if h.Subs == nil || auth.CanReadSubscriptions(ac) != nil {
		return
	}
	sub, err := h.Subs.FindByID(ctx, *j.SubscriptionID)
	if err != nil || sub == nil || (sub.ClientID != nil && !ac.CanAccessClient(string(*sub.ClientID))) {
		return
	}
	d.SubName, d.SubCode, d.SubHref = sub.Name, sub.Code, "/ui/subscriptions/"+url.PathEscape(sub.ID)
	if sub.ConnectionID == nil || *sub.ConnectionID == "" || h.Connections == nil || auth.CanReadConnections(ac) != nil {
		return
	}
	conn, err := h.Connections.FindByID(ctx, *sub.ConnectionID)
	if err != nil || conn == nil || (conn.ClientID != nil && !ac.CanAccessClient(string(*conn.ClientID))) {
		return
	}
	d.ConnCode, d.ConnHref = conn.Code, "/ui/connections/"+url.PathEscape(conn.ID)
}

func djID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidRecordID(id) {
		return "", httperror.NotFound("DispatchJob", id)
	}
	return id, nil
}

func (h *DispatchJobs) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, djViewPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := djID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	j, err := h.loadJob(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d, err := h.detailOf(ctx, ac, j)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		if err := openDjDrawer(ds.NewSSE(w, r), d); err != nil {
			slog.DebugContext(ctx, "ui: dispatch job drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseDJQuery(nil))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

// patchDjTable replaces the table rather than morphing it: a morph keeps
// each row's <time> and resets its text to the UTC fallback without
// re-running the localisation (local-time.md, Behaviour), and replacing
// 1,000 rows is cheaper than diffing them.
func patchDjTable(sse *ds.SSE, v djView) error {
	return sse.Patch(djTable(v), ds.WithMode(ds.ModeReplace))
}

// patchDjDrawer fills the drawer: the content morphs in (header, focus
// target), then the body is replaced so its local times are localised again.
func patchDjDrawer(sse *ds.SSE, d djDetail) error {
	return errors.Join(
		sse.Patch(djDrawerContent(d), ds.Inner("#"+djDrawerID)),
		sse.Patch(djDrawerBody(d), ds.WithMode(ds.ModeReplace)),
	)
}

func openDjDrawer(sse *ds.SSE, d djDetail) error {
	return errors.Join(
		patchDjDrawer(sse, d),
		sse.PatchSignals(map[string]any{djDrawerSig: d.ID}),
		sse.Focus(djDrawerID+"-title"),
	)
}

// ── audit ──────────────────────────────────────────────────────────────

type djAuditEntry struct {
	Operation, Actor string
	At               time.Time
}

// djAudit is the drawer's Audit section: who changed the job by hand, newest
// first. Failed is set when the rows could not be read.
type djAudit struct {
	Entries []djAuditEntry
	Failed  bool
}

// auditOperationLabel turns an audit operation name into words:
// "CancelCommand" is "Cancel", "EditPayloadCommand" is "Edit payload".
func auditOperationLabel(op string) string {
	base := strings.TrimSuffix(op, "Command")
	var b strings.Builder
	for i, r := range base {
		if i > 0 && unicode.IsUpper(r) && !unicode.IsUpper(rune(base[i-1])) {
			b.WriteByte(' ')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	words := b.String()
	if words == "" {
		return op
	}
	return strings.ToUpper(words[:1]) + words[1:]
}

// auditSection fills the drawer's Audit section when it is opened (the SPA
// reads GET /api/audit-logs/entity/Dispatchjob/{id} then): the audit-log view
// permission, and the job's own checks.
func (h *DispatchJobs) auditSection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := djID(r)
	if err == nil && h.Audit == nil {
		err = httperror.NotFound("DispatchJob", id)
	}
	if err == nil {
		if err = auth.CanWritePermission(ac, djViewPerm); err == nil {
			if err = auth.CanWritePermission(ac, djAuditPerm); err == nil {
				_, err = h.loadJob(ctx, ac, id)
			}
		}
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		http.Redirect(w, r, djURL(id), http.StatusSeeOther) //nolint:gosec // G710: djPath plus an id kit.ValidRecordID accepted
		return
	}
	entity := djAuditEntity
	rows, err := h.Audit.FindWithFilters(ctx, audit.FilterParams{EntityType: &entity, EntityID: &id, Limit: 500})
	a := djAudit{Failed: err != nil}
	if err != nil {
		slog.WarnContext(ctx, "ui: dispatch job audit not read", "id", id, "err", err)
	}
	for _, l := range rows {
		a.Entries = append(a.Entries, djAuditEntry{Operation: auditOperationLabel(l.Operation), Actor: cmp.Or(deref(l.PrincipalName), deref(l.PrincipalID), "system"), At: l.PerformedAt})
	}
	if err := ds.NewSSE(w, r).Patch(djAuditBody(a), ds.WithMode(ds.ModeReplace)); err != nil {
		slog.DebugContext(ctx, "ui: dispatch job audit patch not delivered", "err", err)
	}
}

// ── sign ───────────────────────────────────────────────────────────────

type djPlan struct {
	Sent, Target string
	Headers      []djKV
	Verify, Body string
}

// verifyCommand is the subscriber-side check, as one paste (the SPA's).
func verifyCommand(headers map[string]string) string {
	return "php artisan flowcatalyst:verify-signature --timestamp='" + headers["X-FlowCatalyst-Timestamp"] +
		"' --signature='" + headers["X-FlowCatalyst-Signature"] + "' --body-file=body.json"
}

// sign is the dry run: the delivery as it would go out now, with a real
// signature, without sending it. Gated on the raw-view permission (the API's).
func (h *DispatchJobs) sign(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// The answer carries a live signature (and the body): no-store, not the
	// stream helper's no-cache.
	w = noStore{w}
	if err := auth.CanWritePermission(ac, djRawPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := djID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	j, err := h.loadJob(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	plan, err := h.Ops.Plan(ctx, j)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p := djPlan{Sent: requestLine(plan.Request), Verify: verifyCommand(plan.Headers), Body: plan.Body}
	if plan.Request != nil {
		p.Target = plan.Request.Target
	}
	for _, k := range slices.Sorted(maps.Keys(plan.Headers)) {
		p.Headers = append(p.Headers, djKV{Key: k, Value: plan.Headers[k]})
	}
	if !ds.IsDatastar(r) {
		// The plan exists only as an answer: without Datastar, show the
		// drawer page with it.
		d, err := h.detailOf(ctx, ac, j)
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		view, err := h.listView(ctx, ac, parseDJQuery(nil))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		d.Plan = &p
		view.Drawer = &d
		h.renderPage(w, r, http.StatusOK, view)
		return
	}
	if err := ds.NewSSE(w, r).Patch(djPlanSection(p), ds.Inner("#"+djDrawerID+"-plan")); err != nil {
		slog.DebugContext(ctx, "ui: delivery plan patch not delivered", "err", err)
	}
}

// ── requeue (ids) ──────────────────────────────────────────────────────

// requeueOne is the row's and the drawer's Requeue.
func (h *DispatchJobs) requeueOne(w http.ResponseWriter, r *http.Request) {
	id, err := djID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.requeue(w, r, []string{id})
}

// redriveOne is the row's and the drawer's Redrive of a COMPLETED job: the
// same two-step dialog as ticked rows that include COMPLETED ones.
func (h *DispatchJobs) redriveOne(w http.ResponseWriter, r *http.Request) {
	id, err := djID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := auth.CanWritePermission(auth.FromContext(r.Context()), djRedrivePerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.requeue(w, r, []string{id})
}

// requeueSelected is "Requeue selected": the ticked rows' ids.
func (h *DispatchJobs) requeueSelected(w http.ResponseWriter, r *http.Request) {
	var picked []string
	for _, id := range r.PostForm["ids"] {
		if id = strings.TrimSpace(id); id != "" && !slices.Contains(picked, id) {
			picked = append(picked, id)
		}
	}
	h.requeue(w, r, picked)
}

// djSelection is the dialog of a requeue of ticked rows that include COMPLETED
// jobs (RequeueSelectedDialog) or jobs in flight: a plain requeue leaves
// COMPLETED ones alone, redriving them is the separate, dangerous option behind
// a typed confirmation, and requeueing the ones in flight is a checkbox.
type djSelection struct {
	IDs    []string
	Drawer string
	// Completed counts the COMPLETED jobs the caller may redrive (0 without
	// dispatch-job:redrive-completed); InFlight the QUEUED or PROCESSING ones.
	Completed, InFlight int
	// Consent is the in-flight checkbox as the caller last left it.
	Consent bool
	// Confirm is the second step (the word and the count); Error is its
	// message after a wrong entry.
	Confirm bool
	Error   string
}

// others is how many of the ticked rows a plain requeue would reset.
func (s djSelection) others() int { return len(s.IDs) - s.Completed }

// requeue runs a requeue of ids. A caller who may redrive and has COMPLETED
// jobs among them, or any caller with jobs in flight among them who has not
// consented, goes through the dialog (step "" opens it, "confirm" shows the
// typed confirmation, "redrive" runs it); everyone else requeues at once and is
// told what the status guard left alone.
func (h *DispatchJobs) requeue(w http.ResponseWriter, r *http.Request, picked []string) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, djRequeuePerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	step := r.PostFormValue("step")
	redrivePerm := auth.CanWritePermission(ac, djRedrivePerm)
	if (step == "confirm" || step == "redrive") && redrivePerm != nil {
		h.Shell.Fail(w, r, redrivePerm)
		return
	}
	opts := RequeueOptions{InFlight: formFlag(r.PostFormValue("inflight"))}
	if len(picked) > 0 && step != "plain" {
		sel, err := h.selection(ctx, ac, picked, r.PostFormValue("drawer"))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		if redrivePerm != nil {
			sel.Completed = 0
		}
		sel.Consent = opts.InFlight
		switch {
		case step == "redrive":
			if msg := redriveEntryError(r.PostFormValue("word"), r.PostFormValue("count"), sel.Completed); msg != "" {
				sel.Confirm, sel.Error = true, msg
				h.showSelectionDialog(w, r, sel)
				return
			}
			opts.Redrive = true
			h.runRequeue(w, r, ac, picked, opts)
		case step == "confirm" && sel.Completed > 0:
			sel.Confirm = true
			h.showSelectionDialog(w, r, sel)
		case sel.Completed > 0, sel.InFlight > 0 && !opts.InFlight:
			h.showSelectionDialog(w, r, sel)
		case step != "" && sel.Completed == 0 && redrivePerm == nil:
			h.Shell.Fail(w, r, usecase.Validation("NOTHING_TO_REDRIVE", "None of the selected jobs is COMPLETED any more."))
		default:
			h.runRequeue(w, r, ac, picked, opts)
		}
		return
	}
	h.runRequeue(w, r, ac, picked, opts)
}

// formFlag reads a checkbox or hidden flag: "1", "on" or "true".
func formFlag(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "on", "true":
		return true
	}
	return false
}

// selection counts the COMPLETED and in-flight jobs among the ticked ones the
// caller can reach (the use case drops the rest).
func (h *DispatchJobs) selection(ctx context.Context, ac *auth.AuthContext, picked []string, drawer string) (djSelection, error) {
	jobs, err := h.Jobs.FindByIDs(ctx, picked)
	if err != nil {
		return djSelection{}, usecase.Internal("REPO", "find_by_ids failed", err)
	}
	sel := djSelection{IDs: picked, Drawer: drawer}
	for i := range jobs {
		if !auth.CanAccessScope(ac, ids.StringPtr(jobs[i].ClientID)) {
			continue
		}
		switch {
		case jobs[i].Status == common.DispatchCompleted:
			sel.Completed++
		case slices.Contains(dispatchjob.InFlightStatuses, jobs[i].Status):
			sel.InFlight++
		}
	}
	return sel, nil
}

// redriveEntryError checks the second step: the word typed exactly and the
// number of jobs the run resets retyped (thousands separators tolerated).
func redriveEntryError(word, count string, want int) string {
	n, err := strconv.Atoi(strings.NewReplacer(",", "", " ", "").Replace(strings.TrimSpace(count)))
	switch {
	case want <= 0:
		return "There is nothing to redrive."
	case word != djRedriveWord:
		return "Type " + djRedriveWord + " exactly as shown."
	case err != nil || n != want:
		return "Type the number of jobs shown: " + thousands(int64(want)) + "."
	}
	return ""
}

func (h *DispatchJobs) showSelectionDialog(w http.ResponseWriter, r *http.Request, sel djSelection) {
	if !ds.IsDatastar(r) {
		// The dialog exists only as an answer: show the list with it open.
		ac := auth.FromContext(r.Context())
		view, err := h.listView(r.Context(), ac, djReferer(r))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		view.Dialog = djSelectionDialog(sel)
		h.renderPage(w, r, http.StatusOK, view)
		return
	}
	if err := patchDialogSlot(ds.NewSSE(w, r), djDialogSlot, djSelectionDialog(sel)); err != nil {
		slog.DebugContext(r.Context(), "ui: requeue dialog not delivered", "err", err)
	}
}

func (h *DispatchJobs) runRequeue(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, picked []string, opts RequeueOptions) {
	ctx := r.Context()
	var res RequeueResult
	if len(picked) > 0 {
		var err error
		if res, err = h.Ops.Requeue(ctx, picked, opts); err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashDjRequeued)
		http.Redirect(w, r, djReferer(r).URL(), http.StatusSeeOther) //nolint:gosec // G710: djPath plus a query re-encoded from parsed list state
		return
	}
	sse := ds.NewSSE(w, r)
	h.refreshList(sse, r, ac)
	if drawer := r.PostFormValue("drawer"); drawer != "" && slices.Contains(picked, drawer) {
		h.refreshDrawer(sse, r, ac, drawer)
	}
	if err := errors.Join(sse.CloseDialog(djSelDialog), shell.Flash(sse, idsOutcome(res, len(picked) == 1))); err != nil {
		slog.DebugContext(ctx, "ui: requeue outcome not delivered", "err", err)
	}
}

// idsOutcome is the SPA's toast for a requeue by ids: what was reset, what the
// guard left alone. A single job that was not reset is an error (the drawer
// and row say why).
func idsOutcome(res RequeueResult, single bool) kit.FlashMessageProps {
	skipped := skipNote(int64(res.SkippedInFlight), int64(res.SkippedCompleted))
	if res.Requeued == 0 {
		if single {
			return kit.FlashMessageProps{Kind: kit.FlashError, Summary: "Not requeued", Detail: "No selected job could be requeued." + skipped}
		}
		return kit.FlashMessageProps{Kind: kit.FlashInfo, Summary: "Nothing requeued", Detail: "No selected job could be requeued." + skipped}
	}
	n := int64(res.Requeued)
	return kit.FlashMessageProps{
		Kind: kit.FlashSuccess, Summary: "Requeued " + counted(n, "job") + redrivenNote(int64(res.Redriven)),
		Detail: counted(n, "dispatch job") + " reset to PENDING" + skipped + inFlightNote(int64(res.InFlight)),
	}
}

// skipNote is what a requeue left alone, in words ("" when nothing): jobs in
// flight are never reset, COMPLETED ones only by a confirmed redrive.
func skipNote(inFlight, completed int64) string {
	var parts []string
	if inFlight > 0 {
		parts = append(parts, counted(inFlight, "job")+" in flight (QUEUED or PROCESSING) left alone")
	}
	if completed > 0 {
		parts = append(parts, counted(completed, "COMPLETED job")+" left alone")
	}
	if len(parts) == 0 {
		return ""
	}
	return " — " + strings.Join(parts, "; ")
}

// inFlightNote says how many of the reset jobs were in flight (" 1 of them was
// in flight."), "" when none.
func inFlightNote(n int64) string {
	switch n {
	case 0:
		return ""
	case 1:
		return " 1 of them was in flight."
	}
	return " " + thousands(n) + " of them were in flight."
}

func redrivenNote(n int64) string {
	if n == 0 {
		return ""
	}
	return " (" + counted(n, "COMPLETED job") + " redriven)"
}

// ── cancel / complete ──────────────────────────────────────────────────

// djIgnore is the dialog of "Ignore selected": how many of the ticked rows the
// caller can reach and how many of those the use case would mark CANCELLED.
type djIgnore struct {
	IDs    []string
	Drawer string
	// Cancellable is how many would be cancelled (InFlight of them QUEUED or
	// being delivered), Others how many reachable ones are left as they are
	// (already finished).
	Cancellable, InFlight, Others int
}

// cancelSelected is "Ignore selected": the ticked rows' ids, every unfinished
// one (in flight included) to CANCELLED (dispatch-job:cancel). Step ""
// opens the confirmation, "confirm" runs it.
func (h *DispatchJobs) cancelSelected(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, djCancelPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	var picked []string
	for _, id := range r.PostForm["ids"] {
		if id = strings.TrimSpace(id); id != "" && !slices.Contains(picked, id) {
			picked = append(picked, id)
		}
	}
	if r.PostFormValue("step") == "confirm" {
		h.runIgnore(w, r, ac, picked)
		return
	}
	jobs, err := h.Jobs.FindByIDs(ctx, picked)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "find_by_ids failed", err))
		return
	}
	ig := djIgnore{IDs: picked, Drawer: r.PostFormValue("drawer")}
	for i := range jobs {
		if !auth.CanAccessScope(ac, ids.StringPtr(jobs[i].ClientID)) {
			continue
		}
		if slices.Contains(dispatchjob.CancellableStatuses, jobs[i].Status) {
			ig.Cancellable++
			if slices.Contains(dispatchjob.InFlightStatuses, jobs[i].Status) {
				ig.InFlight++
			}
		} else {
			ig.Others++
		}
	}
	if ig.Cancellable == 0 {
		h.Shell.Fail(w, r, usecase.Validation("NOTHING_TO_IGNORE", "None of the selected jobs can be ignored: they are all finished."))
		return
	}
	if !ds.IsDatastar(r) {
		// The dialog exists only as an answer: show the list with it open.
		view, err := h.listView(ctx, ac, djReferer(r))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		view.Dialog = djIgnoreConfirm(ig)
		h.renderPage(w, r, http.StatusOK, view)
		return
	}
	if err := patchDialogSlot(ds.NewSSE(w, r), djDialogSlot, djIgnoreConfirm(ig)); err != nil {
		slog.DebugContext(ctx, "ui: ignore dialog not delivered", "err", err)
	}
}

func (h *DispatchJobs) runIgnore(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, picked []string) {
	ctx := r.Context()
	res, err := h.Ops.CancelMany(ctx, picked)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashDjIgnored)
		http.Redirect(w, r, djReferer(r).URL(), http.StatusSeeOther) //nolint:gosec // G710: djPath plus a query re-encoded from parsed list state
		return
	}
	sse := ds.NewSSE(w, r)
	h.refreshList(sse, r, ac)
	if drawer := r.PostFormValue("drawer"); drawer != "" && slices.Contains(picked, drawer) {
		h.refreshDrawer(sse, r, ac, drawer)
	}
	if err := errors.Join(sse.CloseDialog(djIgnoreDialog), shell.Flash(sse, ignoreOutcome(res))); err != nil {
		slog.DebugContext(ctx, "ui: ignore outcome not delivered", "err", err)
	}
}

// ignoreOutcome is the toast for an ignore by ids: what was cancelled, what the
// status guard left alone.
func ignoreOutcome(res CancelResult) kit.FlashMessageProps {
	var skipped string
	if res.Skipped > 0 {
		skipped = " " + thousands(int64(res.Skipped)) + " left as they were: already finished."
	}
	if res.Cancelled == 0 {
		return kit.FlashMessageProps{Kind: kit.FlashInfo, Summary: "Nothing ignored", Detail: "No selected job could be ignored." + skipped}
	}
	summary := "Job ignored"
	if res.Cancelled > 1 {
		summary = counted(int64(res.Cancelled), "job") + " ignored"
	}
	return kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: summary, Detail: counted(int64(res.Cancelled), "dispatch job") + " marked CANCELLED." + skipped}
}

// cancel is the drawer's "Cancel job": the API's POST /dispatch-jobs/{id}/cancel
// (PENDING or FAILED -> CANCELLED; dispatch-job:cancel). The use case enforces the status
// rule and the client scope.
func (h *DispatchJobs) cancel(w http.ResponseWriter, r *http.Request) {
	h.statusAction(w, r, djCancelPerm, h.Ops.Cancel, flashDjCancelled)
}

// complete is the drawer's "Mark completed": POST /dispatch-jobs/{id}/complete
// (FAILED -> COMPLETED, the delivery happened out of band; dispatch-job:complete).
func (h *DispatchJobs) complete(w http.ResponseWriter, r *http.Request) {
	h.statusAction(w, r, djCompletePerm, h.Ops.Complete, flashDjCompleted)
}

func (h *DispatchJobs) statusAction(w http.ResponseWriter, r *http.Request, perm string, run func(context.Context, string) error, flash string) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, perm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := djID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := run(ctx, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flash)
		http.Redirect(w, r, djURL(id), http.StatusSeeOther) //nolint:gosec // G710: djPath plus an id kit.ValidRecordID accepted
		return
	}
	sse := ds.NewSSE(w, r)
	h.refreshList(sse, r, ac)
	h.refreshDrawer(sse, r, ac, id)
	if err := shell.Flash(sse, FlashMessages[flash]); err != nil {
		slog.DebugContext(ctx, "ui: status action flash not delivered", "err", err)
	}
}

// ── edit payload ───────────────────────────────────────────────────────

// editPayload is the drawer's "Edit payload": the API's PUT
// /dispatch-jobs/{id}/payload. Same gate (dispatch-job:edit-payload) and the
// same use case, which enforces the status rule (PENDING / FAILED), the client
// scope and the payload validation. Status and attempts are not touched:
// Requeue stays a separate action.
func (h *DispatchJobs) editPayload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, djEditPayloadPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := djID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	var contentType *string
	if ct := strings.TrimSpace(r.PostFormValue("contentType")); ct != "" {
		contentType = &ct
	}
	if err := h.Ops.EditPayload(ctx, id, r.PostFormValue("payload"), contentType); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashDjPayloadSaved)
		http.Redirect(w, r, djURL(id), http.StatusSeeOther) //nolint:gosec // G710: djPath plus an id kit.ValidRecordID accepted
		return
	}
	sse := ds.NewSSE(w, r)
	h.refreshDrawer(sse, r, ac, id)
	if err := shell.Flash(sse, FlashMessages[flashDjPayloadSaved]); err != nil {
		slog.DebugContext(ctx, "ui: payload flash not delivered", "err", err)
	}
}

// refreshList re-renders the list view the request came from.
func (h *DispatchJobs) refreshList(sse *ds.SSE, r *http.Request, ac *auth.AuthContext) {
	ctx := r.Context()
	view, err := h.listView(ctx, ac, djReferer(r))
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := errors.Join(patchDjTable(sse, view), sse.Patch(kit.CursorPager(view.pager())), sse.Patch(djBulkBar(view)), sse.PatchSignals(map[string]any{djSelSig: 0})); err != nil {
		slog.DebugContext(ctx, "ui: dispatch jobs table patch not delivered", "err", err)
	}
}

func (h *DispatchJobs) refreshDrawer(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string) {
	ctx := r.Context()
	if !kit.ValidRecordID(id) {
		return
	}
	j, err := h.loadJob(ctx, ac, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	d, err := h.detailOf(ctx, ac, j)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := patchDjDrawer(sse, d); err != nil {
		slog.DebugContext(ctx, "ui: dispatch job drawer patch not delivered", "err", err)
	}
}

// ── requeue by filter ──────────────────────────────────────────────────

type djStatusCount struct {
	Status   string
	Count    int64
	Severity kit.TagSeverity
}

// djPreview is the RequeueByFilterDialog: the filter in words, what it
// matches, what this caller may requeue, and the form that acts on exactly
// that filter (its URL carries it).
type djPreview struct {
	Summary []djKV
	// Window is the created_at range the counts cover, in words.
	Window          string
	Count, Eligible int64
	OutOfScope      int64
	// Completed and InFlight are matches a requeue leaves alone: COMPLETED
	// ones (redriven only with the API's two redrive fields) and QUEUED or
	// PROCESSING ones (never).
	Completed, InFlight int64
	ToRequeue           int64
	OverCap, Changed    bool
	ByStatus            []djStatusCount
	Action              string
	// CanRedrive offers "Also redrive the COMPLETED jobs" (redrive-completed);
	// RedriveCount is the number a redrive confirms (every job it resets).
	CanRedrive   bool
	RedriveCount int64
	// RedriveHref opens the redrive step and Review returns to the first
	// (the filter's own URL).
	RedriveHref, Review string
	// Redrive is the second step (the word and the count); Error is its
	// message after a wrong entry.
	Redrive bool
	Error   string
}

func (p djPreview) title() string {
	if p.Redrive {
		return "Redrive COMPLETED jobs"
	}
	return "Requeue all matching jobs"
}

// confirmCount is the number the run is confirmed with: the eligible jobs, or
// with a redrive those plus the COMPLETED ones.
func (p djPreview) confirmCount() int64 {
	if p.Redrive {
		return p.RedriveCount
	}
	return p.Eligible
}

// djRBFOpts is how the requeue-by-filter dialog opens: after a changed
// match (nothing was reset), or at the redrive step.
type djRBFOpts struct {
	Changed, Redrive bool
	Error            string
}

// summary is summarizeFilters: the active filters as label/value lines.
func (q djQuery) summary(clientNames map[string]string) []djKV {
	var out []djKV
	add := func(label string, vals []string, labels map[string]string) {
		if len(vals) == 0 {
			return
		}
		shown := make([]string, 0, len(vals))
		for _, v := range vals {
			shown = append(shown, cmp.Or(labels[v], v))
		}
		out = append(out, djKV{Key: label, Value: strings.Join(shown, ", ")})
	}
	add("Client", q.facet(djFacetClients), clientNames)
	add("Application", q.facet(djFacetApps), nil)
	add("Subdomain", q.facet(djFacetSubs), nil)
	add("Aggregate", q.facet(djFacetAggs), nil)
	add("Code", q.facet(djFacetCodes), nil)
	add("Status", q.facet(djFacetStatuses), nil)
	add("Event type", q.EventTypes, nil)
	windowLabel := ""
	if p, ok := djWindowPresets[q.Window]; ok && q.From == "" && q.To == "" {
		windowLabel = p.label
	}
	for _, kv := range []djKV{{"Source", q.Q}, {"Message group", q.Group}, {"Subscription", q.Subscription}, {"Connection", q.Connection}, {"Window", windowLabel}, {"Created from", q.From}, {"Created to", q.To}} {
		if kv.Value != "" {
			out = append(out, kv)
		}
	}
	return out
}

func (h *DispatchJobs) preview(ctx context.Context, ac *auth.AuthContext, q djQuery, names map[string]string, opts djRBFOpts) (djPreview, error) {
	f, win, err := h.scoped(ctx, ac, q)
	if err != nil {
		return djPreview{}, err
	}
	sum, err := h.Ops.Preview(ctx, f)
	if err != nil {
		return djPreview{}, err
	}
	window := win.From.Format("2006-01-02 15:04") + " – " + win.To.Format("2006-01-02 15:04") + " UTC"
	if win.Defaulted {
		window += " (the last 24 hours: no dates chosen)"
	}
	p := djPreview{
		Summary: q.summary(names), Window: window, Count: sum.Count, Eligible: sum.Eligible, Completed: sum.Completed, InFlight: sum.InFlight,
		OutOfScope: sum.Count - sum.Eligible - sum.Completed - sum.InFlight,
		ToRequeue:  min(sum.Eligible, int64(djMaxRequeue)), OverCap: sum.Eligible > int64(djMaxRequeue), Changed: opts.Changed,
		Action:     encodeURL(djPath+"/requeue-by-filter", q.filterValues()),
		CanRedrive: sum.Completed > 0 && auth.CanWritePermission(ac, djRedrivePerm) == nil, RedriveCount: sum.RedriveEligible(),
		Error: opts.Error,
	}
	p.Redrive = opts.Redrive && p.CanRedrive
	p.Review = p.Action
	step := q.filterValues()
	step.Set("step", "redrive")
	p.RedriveHref = encodeURL(djPath+"/requeue-by-filter", step)
	for status, n := range sum.ByStatus {
		p.ByStatus = append(p.ByStatus, djStatusCount{Status: status, Count: n, Severity: djSeverity(status)})
	}
	slices.SortFunc(p.ByStatus, func(a, b djStatusCount) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), strings.Compare(a.Status, b.Status))
	})
	return p, nil
}

// requeueByFilterDialog opens the confirm step for the view in the query.
func (h *DispatchJobs) requeueByFilterDialog(w http.ResponseWriter, r *http.Request) {
	h.openRequeueByFilter(w, r, parseDJQuery(r.URL.Query()), false, r.URL.Query().Get("step") == "redrive")
}

// requeueErrors is "Requeue everything in error": the list switches to every
// job in an error status (whatever else it was filtered by) and the same
// confirm step opens for that view.
func (h *DispatchJobs) requeueErrors(w http.ResponseWriter, r *http.Request) {
	q := parseDJQuery(url.Values{djFacetStatuses: djErrorStatuses})
	keep := parseDJQuery(r.URL.Query())
	q.Size, q.Window, q.From, q.To = keep.Size, keep.Window, keep.From, keep.To
	h.openRequeueByFilter(w, r, q, true, false)
}

func (h *DispatchJobs) openRequeueByFilter(w http.ResponseWriter, r *http.Request, q djQuery, switchList, redrive bool) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, djRequeuePerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showRequeueDialog(w, r, ac, q, switchList, djRBFOpts{Redrive: redrive})
}

// showRequeueDialog answers with the confirm step for q: patched into the
// dialog slot (with the list switched to q when switchList), or, without
// Datastar, the page at q with the dialog open.
func (h *DispatchJobs) showRequeueDialog(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, q djQuery, switchList bool, opts djRBFOpts) {
	ctx := r.Context()
	live := ds.IsDatastar(r)
	var view djView
	var names map[string]string
	if !live || switchList {
		v, err := h.listView(ctx, ac, q.first())
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		view, names = v, v.ClientNames
	} else {
		cs, err := allClients(ctx, h.Clients)
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		names = clientNames(cs)
	}
	p, err := h.preview(ctx, ac, q, names, opts)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !live {
		view.Dialog = djRequeueDialog(p)
		status := http.StatusOK
		if opts.Changed {
			status = http.StatusConflict
		}
		h.renderPage(w, r, status, view)
		return
	}
	sse := ds.NewSSE(w, r)
	var errs []error
	if switchList {
		errs = append(errs, view.patchList(sse, true))
	}
	errs = append(errs, patchDialogSlot(sse, djDialogSlot, djRequeueDialog(p)))
	if err := errors.Join(errs...); err != nil {
		slog.DebugContext(ctx, "ui: requeue-by-filter dialog not delivered", "err", err)
	}
}

// requeueByFilter runs the confirmed requeue on the filter in its URL. More
// eligible jobs than confirmed (FILTER_MATCH_CHANGED): nothing was reset, so
// the dialog shows the new counts and asks again.
func (h *DispatchJobs) requeueByFilter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, djRequeuePerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	q := parseDJQuery(r.URL.Query())
	confirm, err := strconv.ParseInt(r.PostFormValue("confirmCount"), 10, 64)
	if err != nil || confirm < 0 {
		h.Shell.Fail(w, r, usecase.Validation("INVALID_CONFIRM_COUNT", "confirmCount must be a whole number, not negative"))
		return
	}
	f, _, err := h.scoped(ctx, ac, q)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	redrive := r.PostFormValue("redrive") != ""
	if redrive {
		if err := auth.CanWritePermission(ac, djRedrivePerm); err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		// The typed word and count come first: a wrong entry runs nothing.
		if msg := redriveEntryError(r.PostFormValue("word"), r.PostFormValue("count"), int(confirm)); msg != "" {
			h.showRequeueDialog(w, r, ac, q, false, djRBFOpts{Redrive: true, Error: msg})
			return
		}
	}
	res, err := h.Ops.RequeueByFilter(ctx, f, confirm, redrive)
	if ue := usecase.AsError(err); ue != nil && ue.Code == "FILTER_MATCH_CHANGED" {
		h.showRequeueDialog(w, r, ac, q, false, djRBFOpts{Changed: true})
		return
	}
	if err != nil {
		if ds.IsDatastar(r) {
			// Batches before a failure stay committed: the list is stale
			// either way (the SPA reloads it too).
			sse := ds.NewSSE(w, r)
			h.Shell.FailSSE(sse, r, err)
			h.refreshList(sse, r, ac)
			return
		}
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashDjRequeuedByFilter)
		http.Redirect(w, r, q.URL(), http.StatusSeeOther) //nolint:gosec // G710: djPath plus a query re-encoded from parsed list state
		return
	}
	sse := ds.NewSSE(w, r)
	h.refreshList(sse, r, ac)
	if err := errors.Join(sse.CloseDialog(djRBFDialog), shell.Flash(sse, requeueOutcome(res))); err != nil {
		slog.DebugContext(ctx, "ui: requeue-by-filter outcome not delivered", "err", err)
	}
}

// requeueOutcome is the SPA's toast for a finished run.
func requeueOutcome(res djops.ResendByFilterResult) kit.FlashMessageProps {
	skipped := skipNote(res.SkippedInFlight, res.SkippedCompleted)
	summary := "Requeued " + counted(res.Requeued, "job") + redrivenNote(res.Redriven)
	switch {
	case res.Remaining > 0:
		return kit.FlashMessageProps{Kind: kit.FlashWarn, Summary: summary, Detail: thousands(res.Remaining) + " remain — run it again to continue." + skipped}
	case res.Requeued == 0:
		return kit.FlashMessageProps{Kind: kit.FlashInfo, Summary: "Nothing requeued", Detail: "No matching job could be requeued." + skipped}
	}
	return kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: summary, Detail: counted(res.Requeued, "dispatch job") + " reset to PENDING." + skipped}
}

// windowText is the line above the table: "Showing the last 24 hours" when
// the window ends about now, the dates otherwise (jobWindow.ts describeWindow).
func windowText(w dispatchjob.Window, now time.Time) string {
	span := w.To.Sub(w.From)
	if d := now.Sub(w.To); d < -5*time.Minute || d > 5*time.Minute {
		return "Showing " + w.From.UTC().Format("2006-01-02 15:04") + " to " + w.To.UTC().Format("2006-01-02 15:04") + " UTC"
	}
	switch hours := int(span.Round(time.Hour) / time.Hour); {
	case span < 90*time.Minute && span >= 30*time.Minute:
		return "Showing the last hour"
	case span < 90*time.Minute:
		return "Showing the last " + strconv.Itoa(max(1, int(span.Round(time.Minute)/time.Minute))) + " minutes"
	case hours >= 72 && hours%24 == 0:
		return "Showing the last " + strconv.Itoa(hours/24) + " days"
	default:
		return "Showing the last " + strconv.Itoa(hours) + " hours"
	}
}

// djWindowChoice is one preset button of the window bar.
type djWindowChoice struct {
	Label, Href string
	Active      bool
}

func (v djView) windowChoices() []djWindowChoice {
	custom := v.Query.From != "" || v.Query.To != ""
	out := make([]djWindowChoice, 0, len(djWindowOrder))
	for _, key := range djWindowOrder {
		label := "Last 24 hours"
		if p, ok := djWindowPresets[key]; ok {
			label = p.label
		}
		out = append(out, djWindowChoice{Label: label, Href: v.Query.windowURL(key), Active: !custom && v.Query.Window == key})
	}
	return out
}

// hidden is the toolbar form's hidden state: the sort, and the window preset
// and quick view (typing in the toolbar must not drop them).
func (v djView) hidden() []kit.HiddenField {
	h := v.Query.hidden()
	if v.Query.Window != "" {
		h = append(h, kit.HiddenField{Name: "window", Value: v.Query.Window})
	}
	if v.Query.Lens != "" {
		h = append(h, kit.HiddenField{Name: "lens", Value: v.Query.Lens})
	}
	return h
}

func djConfirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: djPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}
