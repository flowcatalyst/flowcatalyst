package pages

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// TraceStore is what the trace pages read: the function, its trace contexts
// (the union of the declared ones and the built-in http, with their windows)
// and the trace log. TraceReader is the real one.
type TraceStore interface {
	FindByID(ctx context.Context, id string) (*function.Function, error)
	Contexts(ctx context.Context, functionID string) ([]operations.TraceContextState, error)
	QueryTraceLog(ctx context.Context, f function.TraceLogFilter) ([]function.TraceLogRecord, error)
}

// TraceReader reads through the function repository, with the same context
// listing GET /api/functions/{id}/trace runs (its scope check included).
type TraceReader struct {
	*function.Repository
	Limits function.TraceLimits
}

// Contexts lists the function's trace contexts.
func (t TraceReader) Contexts(ctx context.Context, functionID string) ([]operations.TraceContextState, error) {
	_, states, err := operations.ListTraceContexts(ctx, t.Repository, t.Limits, functionID)
	return states, err
}

// TraceCommands are the two operations the switches run.
type TraceCommands interface {
	Enable(ctx context.Context, cmd operations.EnableTraceCommand) error
	Disable(ctx context.Context, cmd operations.DisableTraceCommand) error
}

// TraceOps runs EnableTraceContext and DisableTraceContext through the unit
// of work, as PUT and DELETE /api/functions/{id}/trace/{context} do; the
// permission checks are the page's.
type TraceOps struct {
	Repo   *function.Repository
	UoW    *usecasepgx.UnitOfWork
	Limits function.TraceLimits
}

// Enable opens the window, then prunes the function's old records (the API's
// housekeeping after the commit).
func (o TraceOps) Enable(ctx context.Context, cmd operations.EnableTraceCommand) error {
	if _, err := usecaseop.RunTx(ctx, o.UoW, operations.EnableTraceContext(o.Repo, o.Limits), cmd, auth.NewExecutionContext(ctx)); err != nil {
		return err
	}
	operations.PruneTraceAfterEnable(ctx, o.Repo, o.Limits, cmd.FunctionID)
	return nil
}

// Disable closes the window.
func (o TraceOps) Disable(ctx context.Context, cmd operations.DisableTraceCommand) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, operations.DisableTraceContext(o.Repo), cmd, auth.NewExecutionContext(ctx))
	return err
}

// FunctionTrace is the diagnostics of a function (the SPA's
// FunctionTraceControls and FunctionTraceLogPage): the trace switches, which
// the function drawer embeds through Switches, and the trace log page
// /ui/functions/{id}/traces. The log refreshes over a Datastar stream while a
// window is live.
type FunctionTrace struct {
	Store TraceStore
	Ops   TraceCommands
	Shell *shell.Shell

	// refresh, maxStream and now are the clock of the tests; zero values are
	// the defaults (5 s, 30 min, the wall clock).
	refresh   time.Duration
	maxStream time.Duration
	now       func() time.Time
}

const (
	traceBase = "/ui/functions"

	// traceDefaultRefresh is the SPA's auto-refresh interval.
	traceDefaultRefresh = 5 * time.Second
	// traceDefaultMaxStream bounds one stream: a tab left open on an
	// unlimited window does not hold a request for ever. The stream ends as
	// paused, and Resume opens a new one.
	traceDefaultMaxStream = 30 * time.Minute

	// tracePageSize is the SPA's TRACE_PAGE_SIZE.
	tracePageSize = 100

	traceSwitchesID = "fn-trace-switches"
	traceDialogSlot = "trace-dialog-slot"
	traceFiltersID  = "trace-filters"
	traceRecordsID  = "trace-records"
)

func (h *FunctionTrace) clock() time.Time {
	if h.now != nil {
		return h.now()
	}
	return time.Now()
}

func (h *FunctionTrace) every() time.Duration {
	if h.refresh > 0 {
		return h.refresh
	}
	return traceDefaultRefresh
}

func (h *FunctionTrace) streamLimit() time.Duration {
	if h.maxStream > 0 {
		return h.maxStream
	}
	return traceDefaultMaxStream
}

// Mount registers the routes (POSTs behind parseForm). They are full paths,
// so the functions list page can mount its own beside them.
func (h *FunctionTrace) Mount(r chi.Router) {
	r.Get("/functions/{id}/traces", h.log)
	r.Get("/functions/{id}/traces/more", h.more)
	r.Get("/functions/{id}/trace/switches", h.switches)
	r.Get("/functions/{id}/trace/{context}/enable", h.enableForm)
	post := r.With(parseForm)
	post.Post("/functions/{id}/trace/{context}/enable", h.enable)
	post.Post("/functions/{id}/trace/{context}/disable", h.disable)
}

// traceFunctionID reads and checks the {id} path parameter: an id that could
// not be an element id is not a function (and must never reach an
// expression).
func traceFunctionID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("Function", id)
	}
	return id, nil
}

// traceContextName reads and checks the {context} path parameter.
func traceContextName(r *http.Request) (string, error) {
	name := chi.URLParam(r, "context")
	if name != function.TraceHTTPContext && !function.TraceContextPattern.MatchString(name) {
		return "", httperror.NotFound("Trace context", name)
	}
	return name, nil
}

func traceLogPath(fnID string) string { return traceBase + "/" + fnID + "/traces" }

// traceBackPath is the function's own page (the drawer's deep link).
func traceBackPath(fnID string) string { return traceBase + "/" + fnID }

func traceContextPath(fnID, name string) string {
	return traceBase + "/" + fnID + "/trace/" + name
}

// function loads the function with the API's scope check.
func (h *FunctionTrace) function(ctx context.Context, ac *auth.AuthContext, id string) (*function.Function, error) {
	f, err := h.Store.FindByID(ctx, id)
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

// ── gates ──────────────────────────────────────────────────────────────

// traceGates are what the viewer is offered; the server decides again on
// every request.
type traceGates struct {
	View, Manage, Bodies bool
}

func traceGatesOf(ac *auth.AuthContext) traceGates {
	return traceGates{
		View:   auth.CanViewFunctionTrace(ac) == nil,
		Manage: auth.CanManageFunctionTrace(ac) == nil,
		Bodies: auth.CanViewFunctionTraceBodies(ac),
	}
}

// enableBlocked is why a context's Enable control is unavailable, or "".
// Without manage the controls are not offered at all; http without
// view-bodies is shown disabled with this explanation (the SPA's
// enableBlockedReason).
func (g traceGates) enableBlocked(name string) string {
	if !g.Manage {
		return "You do not have permission to change tracing."
	}
	if name == function.TraceHTTPContext && !g.Bodies {
		return "Capturing http records request and response bodies, which needs the view-bodies permission (platform:function:trace:view-bodies)."
	}
	return ""
}

// ── switches ───────────────────────────────────────────────────────────

// TraceSwitchesView is the switches panel: one card per context.
type TraceSwitchesView struct {
	gates       traceGates
	cards       []traceCard
	hasDeclared bool
	// dialog is the enable dialog, when the page was asked for it.
	dialog templ.Component
	// showLogLink is the "View trace logs" button (off on the log page itself,
	// which is also what makes a post come back to the page and not the drawer).
	showLogLink bool
}

type traceCard struct {
	Name, Description string
	BuiltIn           bool
	Live, Indefinite  bool
	// Label is the countdown at render time (m:ss, h:mm:ss); UntilMs lets
	// the browser keep it ticking.
	Label     string
	UntilMs   int64
	UntilISO  string
	Sample    string
	Blocked   string
	MaxMinute int
}

func (h *FunctionTrace) cardsOf(states []operations.TraceContextState, gates traceGates, now time.Time) []traceCard {
	cards := make([]traceCard, 0, len(states))
	for _, st := range states {
		c := traceCard{
			Name: st.Name, Description: st.Description, BuiltIn: st.BuiltIn, Sample: traceSampleLabel(st.SampleRate),
			Blocked: gates.enableBlocked(st.Name), MaxMinute: st.MaxMinutes,
		}
		if st.EnabledUntil != nil {
			c.UntilMs, c.UntilISO = st.EnabledUntil.UnixMilli(), st.EnabledUntil.UTC().Format(time.RFC3339)
			c.Indefinite = st.Indefinite
			c.Live = st.Indefinite || st.EnabledUntil.After(now)
			if c.Live && !c.Indefinite {
				c.Label = traceCountdown(st.EnabledUntil.Sub(now))
			}
		}
		cards = append(cards, c)
	}
	return cards
}

// traceCountdown is "m:ss", or "h:mm:ss" from an hour up (the SPA's).
func traceCountdown(d time.Duration) string {
	total := int(math.Ceil(d.Seconds()))
	h, m, s := total/3600, total%3600/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// traceSampleLabel is the SPA's sampleLabel.
func traceSampleLabel(rate int) string {
	if rate > 1 {
		return "1 in " + strconv.Itoa(rate)
	}
	return "all invocations"
}

func (h *FunctionTrace) switchesOf(states []operations.TraceContextState, gates traceGates, showLogLink bool) TraceSwitchesView {
	v := TraceSwitchesView{gates: gates, showLogLink: showLogLink, cards: h.cardsOf(states, gates, h.clock())}
	for _, st := range states {
		if !st.BuiltIn {
			v.hasDeclared = true
		}
	}
	return v
}

// Switches is the switches component for the function drawer: the contexts as
// cards, the enable dialog's slot and, with showLogLink, the button to the
// trace log page. Without platform:function:trace:view it renders the notice
// only, and reads nothing. Its patch target is #fn-trace-switches; the
// drawer's own request is the page's, so its permission and scope rules are
// the caller's (the function was loaded with them).
func (h *FunctionTrace) Switches(ctx context.Context, fnID string, showLogLink bool) (templ.Component, error) {
	v, err := h.switchesView(ctx, auth.FromContext(ctx), fnID, showLogLink)
	if err != nil {
		return nil, err
	}
	return TraceSwitches(fnID, v), nil
}

func (h *FunctionTrace) switchesView(ctx context.Context, ac *auth.AuthContext, fnID string, showLogLink bool) (TraceSwitchesView, error) {
	gates := traceGatesOf(ac)
	if !gates.View {
		return TraceSwitchesView{gates: gates, showLogLink: showLogLink}, nil
	}
	states, err := h.Store.Contexts(ctx, fnID)
	if err != nil {
		return TraceSwitchesView{}, err
	}
	return h.switchesOf(states, gates, showLogLink), nil
}

// switches answers the countdown reaching zero (and the post-action refresh
// of a stale card): the switches body again, from the database.
func (h *FunctionTrace) switches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := traceFunctionID(r)
	if err == nil {
		err = auth.CanViewFunctionTrace(ac)
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	onPage := r.URL.Query().Get("page") == "traces"
	if !ds.IsDatastar(r) {
		http.Redirect(w, r, h.homePath(id, onPage), http.StatusSeeOther) //nolint:gosec // G710: id is a validated element id
		return
	}
	v, err := h.switchesView(ctx, ac, id, !onPage)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := ds.NewSSE(w, r).Patch(traceSwitchesBody(id, v)); err != nil {
		slog.DebugContext(ctx, "ui: trace switches patch not delivered", "err", err)
	}
}

// homePath is where a native post comes back to: the log page it was made on,
// else the function's own page.
func (h *FunctionTrace) homePath(fnID string, onPage bool) string {
	if onPage {
		return traceLogPath(fnID)
	}
	return traceBackPath(fnID)
}

// ── the log page ───────────────────────────────────────────────────────

// traceLogView is everything the log page and its patches render.
type traceLogView struct {
	FnID, Address string
	Q             traceQuery
	Records       []traceRec
	NextCursor    string
	Live          bool
	// Contexts are the context filter's options.
	Contexts []string
	Switches TraceSwitchesView
	Err      string
	Dialog   templ.Component
}

// logView reads the contexts and one page of records, with the API's
// redaction of bodies for a viewer without view-bodies.
func (h *FunctionTrace) logView(ctx context.Context, ac *auth.AuthContext, f *function.Function, q traceQuery, cursor string) (traceLogView, error) {
	states, err := h.Store.Contexts(ctx, f.ID)
	if err != nil {
		return traceLogView{}, err
	}
	gates := traceGatesOf(ac)
	now := h.clock()
	v := traceLogView{FnID: f.ID, Q: q, Switches: h.switchesOf(states, gates, false)}
	if auth.CanReadFunctions(ac) == nil {
		v.Address = f.Address
	}
	for _, c := range v.Switches.cards {
		v.Contexts = append(v.Contexts, c.Name)
		v.Live = v.Live || c.Live
	}
	recs, next, err := h.records(ctx, f.ID, q, cursor, now, gates.Bodies)
	if err != nil {
		return traceLogView{}, err
	}
	v.Records, v.NextCursor = recs, next
	return v, nil
}

// records reads one page, newest first, and the cursor of the next.
func (h *FunctionTrace) records(ctx context.Context, fnID string, q traceQuery, cursor string, now time.Time, bodies bool) ([]traceRec, string, error) {
	filter := q.filter(fnID, now)
	if cursor != "" {
		if at, id, ok := parseCursor(cursor); ok {
			filter.After = &function.TraceCursor{At: at, ID: id}
		}
	}
	filter.Limit = tracePageSize + 1 // one extra row says whether another page exists
	recs, err := h.Store.QueryTraceLog(ctx, filter)
	if err != nil {
		return nil, "", usecase.Internal("REPO", "query_trace_log failed", err)
	}
	next := ""
	if len(recs) > tracePageSize {
		recs = recs[:tracePageSize]
		last := recs[len(recs)-1]
		next = formatCursor(last.At, last.ID)
	}
	return traceRecords(recs, bodies), next, nil
}

// log is the trace log page. A navigation gets the page; a Datastar request
// (a filter change, Refresh, Pause and Resume, the first request of a live
// page) gets the log patched and, while a window is live and the page is not
// paused, the stream stays open and patches it again every few seconds.
func (h *FunctionTrace) log(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := traceFunctionID(r)
	if err == nil {
		err = auth.CanViewFunctionTrace(ac)
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f, err := h.function(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	q := parseTraceQuery(r.URL.Query())
	cursor := ""
	if !ds.IsDatastar(r) {
		cursor = r.URL.Query().Get("cursor")
	}
	v, err := h.logView(ctx, ac, f, q, cursor)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.renderPage(w, r, http.StatusOK, v)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := v.patch(sse, ds.ModeReplace); err != nil {
		slog.DebugContext(ctx, "ui: trace log patch not delivered", "err", err)
		return
	}
	if v.Live && !q.Paused {
		h.follow(r, sse, ac, f, q, v)
	}
}

func (h *FunctionTrace) renderPage(w http.ResponseWriter, r *http.Request, status int, v traceLogView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Trace logs", Path: traceBase}, traceLogPage(v))
}

// follow keeps the stream open: every interval it reads again and patches the
// log (morphing, so rows the viewer has expanded stay as they are), and it
// ends when the window closes, the viewer goes away or the stream has run for
// its limit (then the page reads as paused, and Resume opens a new one).
func (h *FunctionTrace) follow(r *http.Request, sse *ds.SSE, ac *auth.AuthContext, f *function.Function, q traceQuery, last traceLogView) {
	ctx := r.Context()
	tick := time.NewTicker(h.every())
	defer tick.Stop()
	limit := time.NewTimer(h.streamLimit())
	defer limit.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-limit.C:
			last.Q.Paused = true
			if err := sse.Patch(traceStatus(last)); err != nil {
				slog.DebugContext(ctx, "ui: trace stream end not delivered", "err", err)
			}
			return
		case <-tick.C:
		}
		v, err := h.logView(ctx, ac, f, q, "")
		if err != nil {
			// A failed read keeps the rows on screen and says so; the next tick tries again.
			_, msg := shell.Classify(r, err)
			last.Err = msg
			if perr := sse.Patch(traceError(last.Err)); perr != nil {
				return
			}
			continue
		}
		if err := v.patch(sse, ds.ModeOuter); err != nil {
			return
		}
		last = v
		if !v.Live {
			return
		}
	}
}

// patch sends the regions a log change touches: the status line, the context
// options, the error line, the records and the load-more button, then the
// address bar. mode is how the records apply: replace (new rows, collapsed)
// for a request of the viewer's, outer (morph; expanded rows stay) for a
// refresh tick.
func (v traceLogView) patch(sse *ds.SSE, mode ds.Mode) error {
	return errors.Join(
		sse.Patch(traceStatus(v)),
		sse.Patch(traceContextField(v)),
		sse.Patch(traceError(v.Err)),
		sse.Patch(traceRecordList(v), ds.WithMode(mode)),
		sse.Patch(traceMore(v)),
		sse.ReplaceURL(v.Q.URL(v.FnID)),
	)
}

// more appends the next (older) page to the list: the SPA's "Load more".
func (h *FunctionTrace) more(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := traceFunctionID(r)
	if err == nil {
		err = auth.CanViewFunctionTrace(ac)
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		http.Redirect(w, r, parseTraceQuery(r.URL.Query()).pageURL(id, r.URL.Query().Get("cursor")), http.StatusSeeOther) //nolint:gosec // G710: id is a validated element id, the query is re-encoded
		return
	}
	f, err := h.function(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	q := parseTraceQuery(r.URL.Query())
	recs, next, err := h.records(ctx, id, q, r.URL.Query().Get("cursor"), h.clock(), traceGatesOf(ac).Bodies)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v := traceLogView{FnID: f.ID, Q: q, Records: recs, NextCursor: next}
	sse := ds.NewSSE(w, r)
	if err := errors.Join(sse.Patch(traceRows(v), ds.Append("#"+traceRecordsID)), sse.Patch(traceMore(v))); err != nil {
		slog.DebugContext(ctx, "ui: trace log page not delivered", "err", err)
	}
}

// ── enable and disable ─────────────────────────────────────────────────

// afterSwitch answers a successful enable or disable: the switches again, the
// log page's stream restarted (its live state changed), and the flash. A
// native post goes back where it was made.
func (h *FunctionTrace) afterSwitch(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, fnID string, onPage bool, flashKey string, msg *kit.FlashMessageProps, closeDialog bool) {
	ctx := r.Context()
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashKey)
		http.Redirect(w, r, h.homePath(fnID, onPage), http.StatusSeeOther) //nolint:gosec // G710: fnID is a validated element id
		return
	}
	sse := ds.NewSSE(w, r)
	if closeDialog {
		if err := sse.CloseDialog(traceEnableDialog); err != nil {
			slog.DebugContext(ctx, "ui: trace dialog close not delivered", "err", err)
		}
	}
	if v, err := h.switchesView(ctx, ac, fnID, !onPage); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := sse.Patch(traceSwitchesBody(fnID, v)); err != nil {
		slog.DebugContext(ctx, "ui: trace switches patch not delivered", "err", err)
	}
	if onPage {
		// The log page's filter bar reopens the stream: whether it stays
		// open depends on the window that just changed.
		if err := sse.PatchHTML(traceKick, ds.Append("body")); err != nil {
			slog.DebugContext(ctx, "ui: trace log restart not delivered", "err", err)
		}
	}
	m := FlashMessages[flashKey]
	if msg != nil {
		m = *msg
	}
	if err := shell.Flash(sse, m); err != nil {
		slog.DebugContext(ctx, "ui: trace flash not delivered", "err", err)
	}
}

// traceKick resubmits the log page's filter bar (a self-removing element).
const traceKick = `<div hidden data-init="document.getElementById('` + traceFiltersID + `')?.requestSubmit(); el.remove()"></div>`

func (h *FunctionTrace) disable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := traceFunctionID(r)
	var name string
	if err == nil {
		name, err = traceContextName(r)
	}
	if err == nil {
		// DELETE /api/functions/{id}/trace/{context}'s rule.
		err = auth.CanManageFunctionTrace(ac)
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := h.Ops.Disable(ctx, operations.DisableTraceCommand{FunctionID: id, Context: name}); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	msg := kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: "Tracing off", Detail: name + " is off"}
	h.afterSwitch(w, r, ac, id, r.PostFormValue("page") == "traces", flashTraceOff, &msg, false)
}
