package pages

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// The ingestion's Activity page, /ui/ingestions/{id}/activity
// (docs/ingestion-trace-design.md section 10): the counters, the last poll,
// exchange and failure, the runtimes, the trace switch and the trace records.
// The drawer's Activity tab asks the same address for a short summary
// (IngestionTabs); a navigation gets this page. The records refresh by a
// Datastar GET every 5 seconds while a window is live, so every refresh makes
// the caller's checks again: a session that ended or a permission withdrawn
// stops reading.

// IngestionTraceCommands are the two operations the switch runs.
type IngestionTraceCommands interface {
	Enable(ctx context.Context, cmd operations.EnableTraceCommand) error
	Disable(ctx context.Context, cmd operations.DisableTraceCommand) error
}

// IngestionTraceOps runs EnableTrace and DisableTrace through the unit of
// work, as PUT and DELETE /api/ingestions/{id}/trace do. Their permission
// checks are the use cases' own.
type IngestionTraceOps struct {
	Repo *ingestion.Repository
	UoW  *usecasepgx.UnitOfWork
	// Grace is the pickup grace added to a window's hard stop; zero reads
	// FC_INGESTION_TRACE_PICKUP_GRACE.
	Grace time.Duration
}

// Enable opens a window.
func (o IngestionTraceOps) Enable(ctx context.Context, cmd operations.EnableTraceCommand) error {
	grace := o.Grace
	if grace <= 0 {
		grace = ingestion.TracePickupGrace()
	}
	_, err := usecaseop.RunTx(ctx, o.UoW, operations.EnableTrace(o.Repo, grace), cmd, auth.NewExecutionContext(ctx))
	return err
}

// Disable ends the window.
func (o IngestionTraceOps) Disable(ctx context.Context, cmd operations.DisableTraceCommand) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, operations.DisableTrace(o.Repo), cmd, auth.NewExecutionContext(ctx))
	return err
}

// IngestionActivity serves the Activity page and its trace panel. IngestionTabs
// owns GET /ingestions/{id}/activity and hands a navigation to it.
type IngestionActivity struct {
	Store IngestionTabStore
	Ops   IngestionTraceCommands
	Shell *shell.Shell

	// now is the clock of the tests; nil is the wall clock.
	now func() time.Time
}

const (
	ingTraceBodyID    = "ing-trace"
	ingTraceSwitchID  = "ing-trace-switch"
	ingTraceRecordsID = "ing-trace-records"
	ingTraceFiltersID = "ing-trace-filters"
	ingTraceMoreID    = "ing-trace-more"

	// ingTracePageSize is the records one read of the trace panel takes: the
	// API's default page.
	ingTracePageSize = ingestion.DefaultTraceLogLimit
)

func (h *IngestionActivity) clock() time.Time {
	if h.now != nil {
		return h.now()
	}
	return time.Now().UTC()
}

// Mount registers the trace panel's routes (the posts behind parseForm).
func (h *IngestionActivity) Mount(r chi.Router) {
	r.Get("/ingestions/{id}/activity/trace", h.trace)
	post := r.With(parseForm)
	post.Post("/ingestions/{id}/trace/enable", h.enable)
	post.Post("/ingestions/{id}/trace/disable", h.disable)
}

func ingActivityURL(id string) string { return ingTabURL(id) + "/activity" }

// ── gates ──────────────────────────────────────────────────────────────

// ingTraceGates are what the viewer is offered; the server decides again on
// every request.
type ingTraceGates struct {
	View, Manage, Bodies bool
}

func ingTraceGatesOf(ac *auth.AuthContext) ingTraceGates {
	return ingTraceGates{
		View:   auth.CanViewIngestionTrace(ac) == nil,
		Manage: auth.CanManageIngestionTrace(ac) == nil,
		Bodies: auth.CanViewIngestionTraceBodies(ac),
	}
}

// ── the page ───────────────────────────────────────────────────────────

// ingActSummary is the page's summary cards.
type ingActSummary struct {
	Day, Week                           string
	LastPoll, LastExchange, LastFailure time.Time
}

// ingActView is everything the Activity page renders.
type ingActView struct {
	ID, Code, Name string
	Active         bool
	Back           string
	Summary        ingActSummary
	Routes         []ingRouteRow
	Trace          ingTraceView
}

// page renders the Activity page for an ingestion the caller was checked
// against (IngestionTabs.load).
func (h *IngestionActivity) page(w http.ResponseWriter, r *http.Request, i *ingestion.Ingestion) {
	w = noStore{w}
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	routes, err := h.Store.ListRouteStates(ctx, i.ID)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "list_route_states failed", err))
		return
	}
	a, err := h.Store.ActivityOf(ctx, i.ID, h.clock())
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "activity failed", err))
		return
	}
	q := parseIngTraceQuery(r.URL.Query())
	tv, err := h.traceView(ctx, ac, i, q, "", routes)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v := ingActView{
		ID: i.ID, Code: i.Code, Name: i.Definition.Name, Active: i.Active, Back: ingTabURL(i.ID),
		Summary: ingActSummary{Day: ingCounts(a.Day), Week: ingCounts(a.Week)}, Trace: tv,
	}
	for _, rs := range routes {
		v.Routes = append(v.Routes, ingRouteRowOf(i, rs))
	}
	v.Summary.LastPoll, v.Summary.LastExchange, v.Summary.LastFailure = derefTime(a.LastPoll), derefTime(a.LastExchange), derefTime(a.LastFailure)
	h.Shell.Render(w, r, shell.Page{Title: "Activity", Path: ingsPath}, ingActivityPage(v))
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// ingRouteRowOf is one runtime's report as a table row.
func ingRouteRowOf(i *ingestion.Ingestion, rs ingestion.RouteState) ingRouteRow {
	return ingRouteRow{
		Runtime: rs.RuntimeID, Status: rs.Status, Severity: ingRouteSeverity(rs.Status), Version: rs.Version,
		Stale: rs.Version != i.Version, Reported: rs.UpdatedAt, Error: deref(rs.Error),
	}
}

// ── the trace panel ────────────────────────────────────────────────────

// ingTraceQuery is the panel's filters, carried in the query string.
type ingTraceQuery struct {
	Kind, Runtime string
}

func parseIngTraceQuery(v url.Values) ingTraceQuery {
	q := ingTraceQuery{Runtime: traceText(v.Get("runtime"))}
	if k := v.Get("kind"); ingestion.ValidTraceKind(k) {
		q.Kind = k
	}
	return q
}

func (q ingTraceQuery) values() url.Values {
	v := url.Values{}
	if q.Kind != "" {
		v.Set("kind", q.Kind)
	}
	if q.Runtime != "" {
		v.Set("runtime", q.Runtime)
	}
	return v
}

func (q ingTraceQuery) filtered() bool { return q.Kind != "" || q.Runtime != "" }

// ingTraceWindowView is the switch's state.
type ingTraceWindowView struct {
	// Ever is false until tracing was first switched on.
	Ever bool
	Live bool
	// Label is the countdown at render time; UntilMs lets the browser keep it
	// ticking.
	Label     string
	UntilMs   int64
	UntilISO  string
	Window    string
	Sample    string
	EnabledBy string
	EnabledAt time.Time
}

// ingTraceView is everything the trace panel and its patches render.
type ingTraceView struct {
	ID     string
	Gates  ingTraceGates
	Window ingTraceWindowView
	Q      ingTraceQuery
	// Runtimes are the runtime filter's options: the runtimes that reported
	// the ingestion, and the chosen one when it is not among them.
	Runtimes []string
	Groups   []ingTraceGroup
	Next     string
	// Page numbers the groups of an appended page so that a capture split
	// over two pages keeps unique element ids.
	Page int
	Err  string
}

func (v ingTraceView) refreshURL() string { return ingActivityURL(v.ID) + "/trace" }

func (v ingTraceView) moreURL() string {
	q := v.Q.values()
	q.Set("cursor", v.Next)
	q.Set("page", strconv.Itoa(v.Page+1))
	return ingActivityURL(v.ID) + "/trace?" + q.Encode()
}

// switchView reads the gates and the window: what the switch renders. A
// caller without trace:view reads nothing (Gates.View is false).
func (h *IngestionActivity) switchView(ctx context.Context, ac *auth.AuthContext, i *ingestion.Ingestion, q ingTraceQuery) (ingTraceView, error) {
	v := ingTraceView{ID: i.ID, Gates: ingTraceGatesOf(ac), Q: q}
	if !v.Gates.View {
		return v, nil
	}
	w, err := h.Store.GetTraceWindow(ctx, i.ID)
	if err != nil {
		return v, usecase.Internal("REPO", "get_trace_window failed", err)
	}
	v.Window = ingTraceWindowOf(w, h.clock())
	return v, nil
}

// traceView reads the window and one page of records, with the API's redaction
// for a viewer without view-bodies. A caller without trace:view reads nothing.
func (h *IngestionActivity) traceView(ctx context.Context, ac *auth.AuthContext, i *ingestion.Ingestion, q ingTraceQuery, cursor string, routes []ingestion.RouteState) (ingTraceView, error) {
	v, err := h.switchView(ctx, ac, i, q)
	if err != nil || !v.Gates.View {
		return v, err
	}
	if routes == nil {
		if routes, err = h.Store.ListRouteStates(ctx, i.ID); err != nil {
			return v, usecase.Internal("REPO", "list_route_states failed", err)
		}
	}
	for _, rs := range routes {
		v.Runtimes = append(v.Runtimes, rs.RuntimeID)
	}
	if q.Runtime != "" && !slices.Contains(v.Runtimes, q.Runtime) {
		v.Runtimes = append(v.Runtimes, q.Runtime)
	}
	page, err := h.recordsPage(ctx, i.ID, q, cursor)
	if err != nil {
		return v, err
	}
	v.Next = page.next
	v.Groups = ingTraceGroups(page.records, v.Gates.Bodies, 0)
	return v, nil
}

func ingTraceWindowOf(w *ingestion.TraceWindow, now time.Time) ingTraceWindowView {
	if w == nil {
		return ingTraceWindowView{}
	}
	v := ingTraceWindowView{
		Ever: true, Live: w.Live(now), UntilMs: w.HardStop.UnixMilli(), UntilISO: w.HardStop.UTC().Format(time.RFC3339),
		Window: ingWindowLabel(w.WindowSecs), Sample: "one capture every " + strconv.Itoa(w.SampleEverySecs) + " s",
		EnabledBy: deref(w.EnabledBy), EnabledAt: w.EnabledAt,
	}
	if v.Live {
		v.Label = traceCountdown(w.HardStop.Sub(now))
	}
	return v
}

// ingWindowLabel names a window length: "1 minute", "5 minutes".
func ingWindowLabel(secs int) string {
	if secs%60 == 0 {
		n := secs / 60
		if n == 1 {
			return "1 minute"
		}
		return strconv.Itoa(n) + " minutes"
	}
	return strconv.Itoa(secs) + " seconds"
}

// ingTraceWindowOptions are the window choices (the design's 1, 5 and 15
// minutes).
var ingTraceWindowOptions = []kit.Option{
	{Value: "60", Label: "1 minute"},
	{Value: "300", Label: "5 minutes"},
	{Value: "900", Label: "15 minutes"},
}

var ingTraceKindOptions = []kit.Option{
	{Value: "poll", Label: "Polls"},
	{Value: "exchange", Label: "Exchanges"},
	{Value: "http", Label: "HTTP calls"},
	{Value: "error", Label: "Errors"},
}

func (v ingTraceView) runtimeOptions() []kit.Option {
	return plainOptions(v.Runtimes)
}

// trace answers the trace panel's Datastar GET: a refresh tick, a filter
// change or "load more" (a cursor). A navigation goes to the page.
func (h *IngestionActivity) trace(w http.ResponseWriter, r *http.Request) {
	// Trace records can carry request and response bodies: never stored by a
	// cache.
	w = noStore{w}
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	i, ok := h.loadIngestion(w, r)
	if !ok {
		return
	}
	if err := auth.CanViewIngestionTrace(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	query := r.URL.Query()
	if !ds.IsDatastar(r) {
		http.Redirect(w, r, ingActivityURL(i.ID)+"?"+parseIngTraceQuery(query).values().Encode(), http.StatusSeeOther) //nolint:gosec // G710: id is a validated element id, the query is re-encoded
		return
	}
	q := parseIngTraceQuery(query)
	sse := ds.NewSSE(w, r)
	if cursor := query.Get("cursor"); cursor != "" {
		// Older records go below the ones shown; the button follows the new last page.
		page, err := h.recordsPage(ctx, i.ID, q, cursor)
		if err != nil {
			h.Shell.FailSSE(sse, r, err)
			return
		}
		n, _ := strconv.Atoi(query.Get("page"))
		v := ingTraceView{ID: i.ID, Q: q, Next: page.next, Page: max(n, 1)}
		v.Groups = ingTraceGroups(page.records, auth.CanViewIngestionTraceBodies(ac), v.Page)
		if err := errors.Join(sse.Patch(ingTraceGroupItems(v.Groups), ds.Append("#"+ingTraceRecordsID)), sse.Patch(ingTraceMore(v))); err != nil {
			slog.DebugContext(ctx, "ui: ingestion trace page not delivered", "err", err)
		}
		return
	}
	v, err := h.traceView(ctx, ac, i, q, "", nil)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := v.patch(sse); err != nil {
		slog.DebugContext(ctx, "ui: ingestion trace patch not delivered", "err", err)
	}
}

type ingRecordsPage struct {
	records []ingestion.TraceLogRecord
	next    string
}

// recordsPage reads the page after a cursor (the "load more" read).
func (h *IngestionActivity) recordsPage(ctx context.Context, id string, q ingTraceQuery, cursor string) (ingRecordsPage, error) {
	filter := ingestion.TraceLogFilter{IngestionID: id, Kind: q.Kind, RuntimeID: q.Runtime, Limit: ingTracePageSize + 1}
	if at, cid, ok := parseCursor(cursor); ok {
		filter.After = &ingestion.TraceCursor{At: at, ID: cid}
	}
	recs, err := h.Store.QueryTraceLog(ctx, filter)
	if err != nil {
		return ingRecordsPage{}, usecase.Internal("REPO", "query_trace_log failed", err)
	}
	out := ingRecordsPage{records: recs}
	if len(recs) > ingTracePageSize {
		out.records = recs[:ingTracePageSize]
		last := out.records[len(out.records)-1]
		out.next = formatCursor(last.At, last.ID)
	}
	return out, nil
}

// patch sends the regions a refresh touches: the switch (and with it the
// refresh timer, which is there only while a window is live), the records and
// the load-more button. The records are morphed so that rows the viewer has
// opened stay open.
func (v ingTraceView) patch(sse *ds.SSE) error {
	return errors.Join(
		sse.Patch(ingTraceSwitch(v), ds.WithMode(ds.ModeOuter)),
		sse.Patch(ingTraceRecordList(v), ds.WithMode(ds.ModeOuter)),
		sse.Patch(ingTraceMore(v)),
	)
}

// loadIngestion makes IngestionTabs.load's checks for the panel's own routes:
// the view permission, the id, the ingestion and its reach.
func (h *IngestionActivity) loadIngestion(w http.ResponseWriter, r *http.Request) (*ingestion.Ingestion, bool) {
	return findIngestion(w, r, h.Store, h.Shell)
}

// ── enable and disable ─────────────────────────────────────────────────

// afterSwitch answers a successful enable or disable: the switch and records
// again (their live state changed) and the flash. A native post goes back to
// the page.
func (h *IngestionActivity) afterSwitch(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, i *ingestion.Ingestion, flashKey string) {
	ctx := r.Context()
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashKey)
		http.Redirect(w, r, ingActivityURL(i.ID), http.StatusSeeOther) //nolint:gosec // G710: id is a validated element id
		return
	}
	sse := ds.NewSSE(w, r)
	v, err := h.switchView(ctx, ac, i, ingTraceQuery{})
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	// The switch again, and the filter bar resubmitted: whether the records
	// refresh by themselves depends on the window that just changed, and they
	// are read with the filters the viewer has set.
	if err := errors.Join(sse.Patch(ingTraceSwitch(v), ds.WithMode(ds.ModeOuter)), sse.PatchHTML(ingTraceKick, ds.Append("body"))); err != nil {
		slog.DebugContext(ctx, "ui: ingestion trace patch not delivered", "err", err)
	}
	if err := shell.Flash(sse, FlashMessages[flashKey]); err != nil {
		slog.DebugContext(ctx, "ui: ingestion trace flash not delivered", "err", err)
	}
}

// enable is PUT /api/ingestions/{id}/trace: the page checks the permissions
// the use case does, so a forged post is refused before anything is read.
func (h *IngestionActivity) enable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := firstError(auth.CanManageIngestionTrace(ac), auth.RequireIngestionTraceBodies(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, ok := h.loadIngestion(w, r)
	if !ok {
		return
	}
	cmd := operations.EnableTraceCommand{IngestionID: i.ID}
	var err error
	if cmd.WindowSecs, err = formInt(r, "windowSecs"); err == nil {
		cmd.SampleEverySecs, err = formInt(r, "sampleEverySecs")
	}
	if err == nil {
		err = h.Ops.Enable(ctx, cmd)
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.afterSwitch(w, r, ac, i, flashIngTraceOn)
}

// disable is DELETE /api/ingestions/{id}/trace's rule.
func (h *IngestionActivity) disable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanManageIngestionTrace(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, ok := h.loadIngestion(w, r)
	if !ok {
		return
	}
	if err := h.Ops.Disable(ctx, operations.DisableTraceCommand{IngestionID: i.ID}); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.afterSwitch(w, r, ac, i, flashIngTraceOff)
}

// formInt reads an optional whole-number field of a posted form; blank is 0
// (the use case's default). Anything else that is not a number is a 400, the
// same TRACE_* family a bad API body is.
func formInt(r *http.Request, name string) (int, error) {
	v := strings.TrimSpace(r.PostFormValue(name))
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, usecase.Validation("TRACE_FIELD_INVALID", name+" must be a whole number")
	}
	return n, nil
}

// ingTraceKick resubmits the filter bar (a self-removing element).
const ingTraceKick = `<div hidden data-init="document.getElementById('` + ingTraceFiltersID + `')?.requestSubmit(); el.remove()"></div>`

// ── records ────────────────────────────────────────────────────────────

// ingTraceGroup is one capture: its head (the poll, else the exchange, else the
// oldest record) and the records nested under it, oldest first.
type ingTraceGroup struct {
	DomID string
	Head  ingTraceRec
	Items []ingTraceRec
}

// ingTraceRec is one record, ready to render.
type ingTraceRec struct {
	DomID, ID           string
	At                  time.Time
	Kind, Level         string
	Message             string
	Version             int32
	Runtime             string
	Headline            string
	Outcome             string
	DurationText        string
	HTTP                *ingTraceHTTP
	Data                string
	DataID              string
	Redacted            bool
	atText, versionText string
}

type ingTraceHTTP struct {
	Summary string
	Panels  []tracePanel
}

// ingHTTPPart is one side of a captured call.
type ingHTTPPart struct {
	Method        string                      `json:"method"`
	URL           string                      `json:"url"`
	Status        *int                        `json:"status"`
	Headers       map[string]traceHeaderValue `json:"headers"`
	Body          *string                     `json:"body"`
	BodyEncoding  string                      `json:"bodyEncoding"`
	BodyBytes     *int64                      `json:"bodyBytes"`
	BodyTruncated bool                        `json:"bodyTruncated"`
}

// as is the part in the shape the function trace's body rendering reads.
func (p *ingHTTPPart) as() *traceHTTPPart {
	return &traceHTTPPart{
		Body: p.Body, BodyEncoding: p.BodyEncoding, BodyBytes: p.BodyBytes, BodyTruncated: p.BodyTruncated,
	}
}

// ingTraceGroups groups a page of records (newest first) by capture, newest
// capture first, each capture's records oldest first. page numbers the page
// for the groups' element ids.
func ingTraceGroups(recs []ingestion.TraceLogRecord, bodies bool, page int) []ingTraceGroup {
	byCapture := map[string][]ingestion.TraceLogRecord{}
	var order []string
	for _, r := range recs {
		if _, seen := byCapture[r.CaptureID]; !seen {
			order = append(order, r.CaptureID)
		}
		byCapture[r.CaptureID] = append(byCapture[r.CaptureID], r)
	}
	out := make([]ingTraceGroup, 0, len(order))
	for _, id := range order {
		rs := byCapture[id]
		slices.Reverse(rs) // oldest first
		head := 0
		for _, kind := range []string{ingestion.TraceKindPoll, ingestion.TraceKindExchange} {
			if k := slices.IndexFunc(rs, func(r ingestion.TraceLogRecord) bool { return r.Kind == kind }); k >= 0 {
				head = k
				break
			}
		}
		g := ingTraceGroup{DomID: "ing-cap-" + traceSafeID(id)}
		if page > 0 {
			g.DomID += "-p" + strconv.Itoa(page)
		}
		for k, r := range rs {
			rec := ingTraceRecOf(r, bodies)
			if k == head {
				g.Head = rec
			} else {
				g.Items = append(g.Items, rec)
			}
		}
		out = append(out, g)
	}
	return out
}

// ingBodiesHidden is shown on a redacted record.
const ingBodiesHidden = "Headers, query strings, bodies and error text hidden: you do not have permission to view them (platform:ingestion:trace:view-bodies)"

func ingTraceRecOf(r ingestion.TraceLogRecord, bodies bool) ingTraceRec {
	redacted := false
	if !bodies {
		// The API's rule: without view-bodies a record keeps only what
		// ingestion.WithholdTraceDetail leaves.
		redacted = ingestion.WithholdTraceDetail(&r)
	}
	rec := ingTraceRec{
		DomID: "ing-rec-" + traceSafeID(r.ID), ID: r.ID, At: r.At, Kind: r.Kind, Level: r.Level, Message: r.Message,
		Version: r.Version, Runtime: r.RuntimeID, Headline: r.Message, Redacted: redacted,
		DataID: "ing-data-" + traceSafeID(r.ID), atText: r.At.UTC().Format(time.RFC3339Nano), versionText: "v" + strconv.Itoa(int(r.Version)),
	}
	if r.Kind == ingestion.TraceKindHTTP {
		// An http record is never printed raw: its data holds the bodies.
		if h := parseIngTraceHTTP(r.Data, r.ID, bodies); h != nil {
			rec.HTTP = h
			rec.Headline = h.Summary
		}
	} else {
		var d struct {
			Outcome    string `json:"outcome"`
			DurationMs *int64 `json:"durationMs"`
		}
		if json.Unmarshal(r.Data, &d) == nil {
			rec.Outcome = d.Outcome
			if d.DurationMs != nil {
				rec.DurationText = strconv.FormatInt(*d.DurationMs, 10) + " ms"
			}
		}
		rec.Data = traceDataText(r.Data)
	}
	if rec.Headline == "" {
		rec.Headline = r.Kind
	}
	return rec
}

// parseIngTraceHTTP reads the data of an http record into its summary and
// panels. It is nil when the data is not that shape. Without bodies the body
// fields are not even kept.
func parseIngTraceHTTP(data []byte, recID string, bodies bool) *ingTraceHTTP {
	var d struct {
		Client     string       `json:"client"`
		Request    *ingHTTPPart `json:"request"`
		Response   *ingHTTPPart `json:"response"`
		DurationMs *float64     `json:"durationMs"`
		Attempt    int          `json:"attempt"`
		Failure    string       `json:"failure"`
	}
	if json.Unmarshal(data, &d) != nil || d.Request == nil {
		return nil
	}
	if !bodies {
		d.Request.Body = nil
		if d.Response != nil {
			d.Response.Body = nil
		}
	}
	req := d.Request
	head := strings.TrimSpace(cmp.Or(req.Method, "?") + " " + req.URL)
	parts := []string{head}
	if d.Response != nil && d.Response.Status != nil {
		parts[0] += " → " + strconv.Itoa(*d.Response.Status)
	} else if d.Failure != "" {
		parts[0] += " → failed: " + d.Failure
	}
	if d.DurationMs != nil {
		parts = append(parts, strconv.FormatFloat(*d.DurationMs, 'f', -1, 64)+" ms")
	}
	if d.Attempt > 1 {
		parts = append(parts, "attempt "+strconv.Itoa(d.Attempt))
	}
	if d.Client != "" {
		parts = append(parts, d.Client)
	}
	h := &ingTraceHTTP{Summary: strings.Join(parts, " · ")}
	base := "ing-body-" + traceSafeID(recID)
	reqPanel := tracePanel{Title: "Request", Line: head, Headers: traceHeaderRows(req.Headers), Body: traceBodyOf(req.as(), !bodies), BodyID: base + "-req"}
	h.Panels = append(h.Panels, reqPanel)
	if d.Response != nil {
		line := ""
		if d.Response.Status != nil {
			line = "Status " + strconv.Itoa(*d.Response.Status)
		}
		h.Panels = append(h.Panels, tracePanel{Title: "Response", Line: line, Headers: traceHeaderRows(d.Response.Headers), Body: traceBodyOf(d.Response.as(), !bodies), BodyID: base + "-res"})
	} else {
		// A call that got no response: why.
		h.Panels = append(h.Panels, tracePanel{Title: "Response", Line: "No response" + cmp.Or(": "+d.Failure, ""), Body: traceBody{State: "empty"}, BodyID: base + "-res"})
	}
	return h
}

// ingTraceEnableHint is the note beside the switch.
const ingTraceEnableHint = "Capture starts when each runtime picks the window up, within one sync (a minute in production), and lasts the chosen time from then."

// ingOutcomeSeverity colours a poll's or exchange's outcome.
func ingOutcomeSeverity(outcome string) kit.TagSeverity {
	switch outcome {
	case "ok", "completed":
		return kit.TagSuccess
	case "rejected":
		return kit.TagWarn
	case "failed":
		return kit.TagDanger
	}
	return kit.TagSecondary
}
