package pages

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/pushgw"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Push → Live (spec section 11, page 2): open streams per application and per client, the relay
// lag and the pushes of the last minute, summed over the gateway tasks that wrote a stats row in
// the last 30 seconds. The page refreshes itself every few seconds.

const (
	pushLiveRegion = "push-live-body"
	// pushLiveTick asks for the body again; the handler answers a Datastar request with a patch.
	pushLiveTick = "@get('" + pushLivePath + "', {retryMaxCount: 0})"
)

type pushLiveAppRow struct {
	Code   string
	Href   string
	Open   int
	Limit  int
	Pushes string // ingest's last minute, "" when it saw none
}

type pushLiveClientRow struct {
	Name string
	Open int
}

type pushLiveView struct {
	Gates       pushGates
	Tasks       int
	Driver      string
	Newest      string
	Open        int
	LagP50      string
	LagP99      string
	LagSamples  int
	Published   int
	Apps        []pushLiveAppRow
	Clients     []pushLiveClientRow
	Unavailable string // why the stats cannot be read, "" when they can
}

func (h *PushConsole) liveView(r *http.Request) pushLiveView {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	v := pushLiveView{Gates: pushGatesFor(ac)}
	now := h.now()
	l, err := h.Stats.Live(ctx, now)
	if err != nil {
		slog.ErrorContext(ctx, "ui: push stats not readable", "err", err)
		v.Unavailable = "The gateway's stats could not be read."
		return v
	}
	v.Tasks, v.Driver = l.Tasks, l.Driver
	if l.Tasks == 0 {
		return v
	}
	newest := l.Newest
	v.Newest = pushAge(&newest, now)
	v.Open = l.Open
	p50, ok50 := l.Lag.Quantile(0.5)
	p99, ok99 := l.Lag.Quantile(0.99)
	v.LagP50, v.LagP99, v.LagSamples = pushSeconds(p50, ok50), pushSeconds(p99, ok99), int(l.Lag.Total())
	v.Published = int(l.Published)

	// limits per application come from the settings, so the page shows the headroom
	limits := map[string]int{}
	if apps, err := h.Config.ListConfigured(ctx); err == nil {
		for _, a := range apps {
			if resp, err := h.Settings.Read(ctx, a.ApplicationID, a.Code); err == nil {
				limits[a.Code] = resp.Effective.MaxStreams
			}
		}
	}
	codes := map[string]bool{}
	for code := range l.ByApplication {
		codes[code] = true
	}
	for code := range l.Ingested {
		codes[code] = true
	}
	counts := pushgw.SortedCounts(l.ByApplication)
	seen := map[string]bool{}
	appendRow := func(code string, open int) {
		seen[code] = true
		row := pushLiveAppRow{Code: code, Href: pushAppURL(code), Open: open, Limit: limits[code]}
		if res, ok := l.Ingested[code]; ok {
			row.Pushes = strconv.Itoa(int(res["accepted"])) + " accepted"
			if n := int(res["refused"]); n > 0 {
				row.Pushes += ", " + strconv.Itoa(n) + " refused"
			}
		}
		v.Apps = append(v.Apps, row)
	}
	for _, c := range counts {
		appendRow(c.Key, c.N)
	}
	for code := range codes {
		if !seen[code] {
			appendRow(code, 0)
		}
	}
	names := pushClientNames(ctx, h.Clients)
	for _, c := range pushgw.SortedCounts(l.ByClient) {
		v.Clients = append(v.Clients, pushLiveClientRow{Name: pushClientLabel(names, c.Key), Open: c.N})
	}
	return v
}

func (h *PushConsole) live(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.CanViewPush(auth.FromContext(ctx)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if h.Stats == nil {
		h.Shell.Fail(w, r, usecase.Internal("PUSH", "push stats are not wired", nil))
		return
	}
	v := h.liveView(r)
	if ds.IsDatastar(r) {
		if err := ds.NewSSE(w, r).Patch(pushLiveBody(v)); err != nil {
			slog.DebugContext(ctx, "ui: push live patch not delivered", "err", err)
		}
		return
	}
	h.Shell.Render(w, r, shell.Page{Title: "Push live", Path: pushLivePath}, pushLivePage(v))
}
