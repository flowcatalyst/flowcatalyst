package pages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/pushgw"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Push → Recent pushes (spec section 11, page 3): the newest pushes of an application read from
// the push log (nothing is copied). A row shows the id, time, topic, client and type. The data is
// shown only while a trace window is open for the application: an operator with the manage
// permission switches one on for 1, 5 or 15 minutes (audited), and it ends by itself.

const (
	pushRecentRegion = "push-recent-body"
	// pushDataMax cuts a push's data in the list; the log keeps it whole.
	pushDataMax = 2000
)

type pushRecentQuery struct {
	App, Topic, Type, Client string
	Before                   int64
}

func parsePushRecentQuery(v url.Values) pushRecentQuery {
	q := pushRecentQuery{
		App: strings.TrimSpace(v.Get("app")), Topic: strings.TrimSpace(v.Get("topic")),
		Type: strings.TrimSpace(v.Get("type")), Client: strings.TrimSpace(v.Get("client")),
	}
	if n, err := strconv.ParseInt(v.Get("before"), 10, 64); err == nil && n > 0 {
		q.Before = n
	}
	for _, s := range []*string{&q.App, &q.Topic, &q.Type, &q.Client} {
		if len(*s) > 255 {
			*s = (*s)[:255]
		}
	}
	return q
}

func (q pushRecentQuery) values() url.Values {
	v := url.Values{}
	set := func(k, s string) {
		if s != "" {
			v.Set(k, s)
		}
	}
	set("app", q.App)
	set("topic", q.Topic)
	set("type", q.Type)
	set("client", q.Client)
	if q.Before > 0 {
		v.Set("before", strconv.FormatInt(q.Before, 10))
	}
	return v
}

func (q pushRecentQuery) url() string {
	if v := q.values().Encode(); v != "" {
		return pushRecentPath + "?" + v
	}
	return pushRecentPath
}

func (q pushRecentQuery) filtered() bool { return q.Topic != "" || q.Type != "" || q.Client != "" }

type pushRecentRow struct {
	DomID  string
	ID     string
	At     time.Time
	Topic  string
	Type   string
	Client string
	// Data is the push's data; "" unless the trace window is open.
	Data string
}

type pushTraceState struct {
	Live     bool
	Ever     bool
	UntilMs  string
	UntilISO string
	Label    string // time left, mm:ss
	Window   string
	By       string
}

type pushRecentView struct {
	Gates   pushGates
	Q       pushRecentQuery
	Apps    []kit.Option
	Clients []kit.Option
	Trace   pushTraceState
	Rows    []pushRecentRow
	// Next is the URL of the next (older) page, "" at the end.
	Next string
	// Notice is why the list cannot be shown ("" when it can).
	Notice string
	// NoApp: no application has push configuration and none was named.
	NoApp bool
	Err   string
}

// refreshURL is the page's own URL, which the live timer asks for again.
func (v pushRecentView) refreshURL() string {
	q := v.Q
	q.Before = 0
	return q.url()
}

func pushWindowLabel(secs int) string {
	if secs%60 == 0 {
		return strconv.Itoa(secs/60) + " min"
	}
	return strconv.Itoa(secs) + " s"
}

func traceState(w *pushgw.TraceWindow, now time.Time, names func(string) string) pushTraceState {
	st := pushTraceState{}
	if w == nil {
		return st
	}
	st.Ever = true
	st.Window = pushWindowLabel(w.WindowSecs)
	if w.EnabledBy != nil {
		st.By = names(*w.EnabledBy)
	}
	if w.Live(now) {
		st.Live = true
		left := int(w.HardStop.Sub(now).Seconds())
		st.UntilMs = strconv.FormatInt(w.HardStop.UnixMilli(), 10)
		st.UntilISO = w.HardStop.UTC().Format(time.RFC3339)
		st.Label = strconv.Itoa(left/60) + ":" + pad2(left%60)
	}
	return st
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func (h *PushConsole) recentView(ctx context.Context, ac *auth.AuthContext, q pushRecentQuery) (pushRecentView, *application.Application, error) {
	v := pushRecentView{Gates: pushGatesFor(ac), Q: q}
	configured, err := h.Config.ListConfigured(ctx)
	if err != nil {
		return v, nil, usecase.Internal("REPO", "list push applications failed", err)
	}
	have := map[string]bool{}
	for _, a := range configured {
		v.Apps = append(v.Apps, kit.Option{Value: a.Code, Label: a.Name + " (" + a.Code + ")"})
		have[a.Code] = true
	}
	if v.Q.App == "" && len(configured) > 0 {
		v.Q.App = configured[0].Code
	}
	if v.Q.App == "" {
		v.NoApp = true
		return v, nil, nil
	}
	app, err := h.Apps.FindByCode(ctx, v.Q.App)
	if err != nil {
		return v, nil, usecase.Internal("REPO", "find application failed", err)
	}
	if app == nil {
		v.Err = "No application has the code " + v.Q.App + "."
		return v, nil, nil
	}
	if !have[app.Code] {
		v.Apps = append(v.Apps, kit.Option{Value: app.Code, Label: app.Name + " (" + app.Code + ")"})
	}
	names := pushClientNames(ctx, h.Clients)
	v.Clients = append(v.Clients, kit.Option{Value: "_platform", Label: "Platform"})
	for id, n := range names {
		v.Clients = append(v.Clients, kit.Option{Value: id, Label: n})
	}
	sortOptions(v.Clients[1:])

	now := h.now()
	win, err := h.Traces.Get(ctx, string(app.ID))
	if err != nil {
		return v, app, usecase.Internal("REPO", "read the trace window failed", err)
	}
	v.Trace = traceState(win, now, func(id string) string { return id })
	if h.Log == nil {
		v.Notice = "The push log cannot be read from this process: the gateway uses the Redis log driver and this process has no Redis connection (FC_REDIS_URL)."
		return v, app, nil
	}
	rows, err := h.Log.Recent(ctx, pushgw.LogQuery{
		Application: app.Code, Topic: v.Q.Topic, Type: v.Q.Type, Client: v.Q.Client, BeforeID: v.Q.Before, Limit: pushgw.DefaultLogLimit + 1,
	})
	if err != nil {
		slog.ErrorContext(ctx, "ui: push log not readable", "err", err)
		v.Err = "The push log could not be read."
		return v, app, nil
	}
	if len(rows) > pushgw.DefaultLogLimit {
		rows = rows[:pushgw.DefaultLogLimit]
		next := v.Q
		next.Before = rows[len(rows)-1].ID
		v.Next = next.url()
	}
	for _, r := range rows {
		row := pushRecentRow{
			DomID: "push-" + strconv.FormatInt(r.ID, 10), ID: strconv.FormatInt(r.ID, 10), At: r.LoggedAt,
			Topic: r.Topic, Type: r.Type, Client: pushClientLabel(names, r.Client),
		}
		if v.Trace.Live { // the gate: data leaves the log only while the window is open
			row.Data = r.Data
			if len(row.Data) > pushDataMax {
				row.Data = row.Data[:pushDataMax] + "…"
			}
		}
		v.Rows = append(v.Rows, row)
	}
	return v, app, nil
}

func sortOptions(o []kit.Option) {
	for i := 1; i < len(o); i++ {
		for j := i; j > 0 && strings.ToLower(o[j].Label) < strings.ToLower(o[j-1].Label); j-- {
			o[j], o[j-1] = o[j-1], o[j]
		}
	}
}

func (h *PushConsole) recent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanViewPush(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v, _, err := h.recentView(ctx, ac, parsePushRecentQuery(r.URL.Query()))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(sse.Patch(pushRecentBody(v)), sse.ReplaceURL(v.Q.url())); err != nil {
			slog.DebugContext(ctx, "ui: push recent patch not delivered", "err", err)
		}
		return
	}
	h.Shell.Render(w, r, shell.Page{Title: "Recent pushes", Path: pushRecentPath}, pushRecentPage(v))
}

// ── the trace window ───────────────────────────────────────────────────

func (h *PushConsole) afterTrace(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, app *application.Application, flash string) {
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flash)
		http.Redirect(w, r, pushRecentPath+"?app="+url.QueryEscape(app.Code), http.StatusSeeOther)
		return
	}
	v, _, err := h.recentView(r.Context(), ac, pushRecentQuery{App: app.Code})
	sse := ds.NewSSE(w, r)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := errors.Join(sse.Patch(pushRecentBody(v)), sse.ReplaceURL(v.Q.url()), flashKey(sse, flash)); err != nil {
		slog.DebugContext(r.Context(), "ui: push trace patch not delivered", "err", err)
	}
}

func (h *PushConsole) traceApp(w http.ResponseWriter, r *http.Request) (*auth.AuthContext, *application.Application, bool) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanManagePush(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return nil, nil, false
	}
	app, err := h.Apps.FindByCode(ctx, strings.TrimSpace(r.PostFormValue("app")))
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "find application failed", err))
		return nil, nil, false
	}
	if app == nil {
		h.Shell.Deny(w, r, http.StatusNotFound, "There is no such application.")
		return nil, nil, false
	}
	return ac, app, true
}

func (h *PushConsole) traceEnable(w http.ResponseWriter, r *http.Request) {
	ac, app, ok := h.traceApp(w, r)
	if !ok {
		return
	}
	secs := pushgw.DefaultTraceWindowSecs
	if raw := strings.TrimSpace(r.PostFormValue("windowSecs")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			h.Shell.Fail(w, r, usecase.Validation("TRACE_WINDOW_INVALID", "windowSecs must be a whole number"))
			return
		}
		secs = n
	}
	if _, err := h.Traces.Enable(r.Context(), string(app.ID), secs, ac.PrincipalID, h.now()); err != nil {
		if ve, isVE := errors.AsType[*pushgw.ValidationError](err); isVE {
			err = usecase.Validation(ve.Code, ve.Message)
		}
		h.Shell.Fail(w, r, err)
		return
	}
	h.audit(r.Context(), ac, app, "PUSH_TRACE_ENABLED", map[string]any{"windowSecs": secs})
	h.afterTrace(w, r, ac, app, flashPushTraceOn)
}

func (h *PushConsole) traceDisable(w http.ResponseWriter, r *http.Request) {
	ac, app, ok := h.traceApp(w, r)
	if !ok {
		return
	}
	was, err := h.Traces.Disable(r.Context(), string(app.ID), h.now())
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("PUSH", "closing the trace window failed", err))
		return
	}
	if was {
		h.audit(r.Context(), ac, app, "PUSH_TRACE_DISABLED", map[string]any{})
	}
	h.afterTrace(w, r, ac, app, flashPushTraceOff)
}

func (h *PushConsole) audit(ctx context.Context, ac *auth.AuthContext, app *application.Application, op string, detail map[string]any) {
	if h.Audit != nil {
		h.Audit(ctx, ac, string(app.ID), op, detail)
	}
}
