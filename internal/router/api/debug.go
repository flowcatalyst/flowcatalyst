package api

import (
	"expvar"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/pprof"
	"runtime"
	runtimepprof "runtime/pprof"
	"sort"
	"sync/atomic"
	"text/tabwriter"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
)

// Debug endpoints: net/http/pprof, expvar, and a router dump that names the
// message or group every worker is on, followed by every goroutine's stack.
//
// They are mounted only when the caller asks (FC_DEBUG_ENDPOINTS_ENABLED) and
// only behind the gate it passes: a profile shows memory contents and the
// dump names messages, so they are never reachable anonymously. MountDebug
// refuses to mount without a gate.
//
// importing net/http/pprof and expvar registers their handlers on
// http.DefaultServeMux at init. Nothing in this codebase serves that mux, and
// the init below empties it, so nothing ever can by accident: the only route
// to these handlers is the gated one mounted here.
func init() {
	http.DefaultServeMux = http.NewServeMux()
}

// debugStart is when this package was loaded, for the dump's uptime line.
var debugStart = time.Now()

// MountDebug registers the debug surface on r (the router prefix, so these
// sit behind whatever that prefix requires too) under /debug:
//
//	/debug/                    index
//	/debug/dump                mediating workers, blocked groups, then every goroutine's stack
//	/debug/pprof/              pprof index (goroutine, heap, allocs, block, mutex, threadcreate)
//	/debug/pprof/{profile}     one profile; goroutine?debug=2 is the full stack dump,
//	                           goroutine?debug=1 groups them and shows each worker's
//	                           pprof labels (message_id, group, pool, queue)
//	/debug/pprof/profile       CPU profile (?seconds=N)
//	/debug/pprof/trace         execution trace (?seconds=N); deliveries show as router.dispatch tasks
//	/debug/pprof/cmdline, /debug/pprof/symbol
//	/debug/vars                expvar (memstats, cmdline, router counters)
//
// gate must authenticate and authorise every request; nil panics, so an
// ungated mount is a startup failure, not an open endpoint.
func MountDebug(r chi.Router, s *State, gate func(http.Handler) http.Handler) {
	if gate == nil {
		panic("routerapi.MountDebug: a gate is required; the debug endpoints are never anonymous")
	}
	publishRouterVars(s)
	r.Route("/debug", func(d chi.Router) {
		d.Use(gate)
		d.Use(noStore)
		d.Get("/", debugIndex)
		d.Get("/dump", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			WriteDump(w, s)
		})
		d.Get("/vars", expvar.Handler().ServeHTTP)
		d.Get("/pprof/", pprof.Index)
		d.Get("/pprof/cmdline", pprof.Cmdline)
		d.Get("/pprof/profile", pprof.Profile)
		d.Get("/pprof/symbol", pprof.Symbol)
		d.Post("/pprof/symbol", pprof.Symbol)
		d.Get("/pprof/trace", pprof.Trace)
		// Named profiles. pprof.Index serves these itself only when the path
		// starts at /debug/pprof/, which it does not under the router prefix.
		d.Get("/pprof/{profile}", func(w http.ResponseWriter, req *http.Request) {
			name := chi.URLParam(req, "profile")
			if runtimepprof.Lookup(name) == nil {
				http.NotFound(w, req)
				return
			}
			pprof.Handler(name).ServeHTTP(w, req)
		})
	})
}

// noStore keeps profiles and dumps out of every cache on the way back.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func debugIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	links := []struct{ href, what string }{
		{"dump", "router dump: which message or group each worker is on, blocked groups, then every goroutine's stack"},
		{"pprof/goroutine?debug=1", "goroutines grouped by stack, with each worker's pprof labels (message_id, group, pool, queue)"},
		{"pprof/goroutine?debug=2", "every goroutine's full stack"},
		{"pprof/", "pprof index (heap, allocs, block, mutex, threadcreate)"},
		{"pprof/profile?seconds=30", "30s CPU profile (go tool pprof)"},
		{"pprof/trace?seconds=5", "5s execution trace (go tool trace)"},
		{"vars", "expvar: memstats, cmdline, router counters"},
	}
	_, _ = io.WriteString(w, "<!doctype html><title>Router debug</title><h1>Router debug</h1><ul>")
	for _, l := range links {
		_, _ = fmt.Fprintf(w, `<li><a href="%s">%s</a> — %s</li>`, l.href, html.EscapeString(l.href), html.EscapeString(l.what))
	}
	_, _ = io.WriteString(w, "</ul>")
}

// WriteDump writes the router dump: what every worker is doing, which groups
// are held, then the stack of every goroutine. One request answers "what is
// this router stuck on" without correlating two endpoints by hand. Also what
// a SIGQUIT writes to stderr. s may be empty (no router in this process).
func WriteDump(w io.Writer, s *State) {
	if s == nil {
		s = &State{}
	}
	now := time.Now()
	_, _ = fmt.Fprintf(w, "FlowCatalyst router dump — %s (up %s), %d goroutines, %d recovered panics\n\n",
		now.UTC().Format(time.RFC3339), now.Sub(debugStart).Round(time.Second),
		runtime.NumGoroutine(), router.PanicsRecovered())

	writeMediating(w, s, now)
	writeBlockedGroups(w, s, now)

	_, _ = io.WriteString(w, "== Goroutines (the workers above carry pprof labels: see pprof/goroutine?debug=1) ==\n\n")
	if p := runtimepprof.Lookup("goroutine"); p != nil {
		_ = p.WriteTo(w, 2)
	}
}

func writeMediating(w io.Writer, s *State, now time.Time) {
	var entries []router.MediatingEntry
	if s.Mediating != nil {
		entries = s.Mediating.MediatingSnapshot()
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].MediatedAt.Before(entries[j].MediatedAt) })
	_, _ = fmt.Fprintf(w, "== Mediating: %d worker(s), oldest first ==\n", len(entries))
	if len(entries) == 0 {
		_, _ = io.WriteString(w, "(none)\n\n")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "AGE\tMESSAGE\tGROUP\tPOOL\tQUEUE\tATTEMPT\tTARGET")
	for _, e := range entries {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
			now.Sub(e.MediatedAt).Round(time.Millisecond), e.MessageID, orDash(e.Group),
			e.PoolCode, e.Queue, e.Attempts+1, e.Target)
	}
	_ = tw.Flush()
	_, _ = io.WriteString(w, "\n")
}

func writeBlockedGroups(w io.Writer, s *State, now time.Time) {
	var groups []router.GroupInfo
	if s.BlockedGroups != nil {
		groups = s.BlockedGroups.BlockedGroups()
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].PoolCode != groups[j].PoolCode {
			return groups[i].PoolCode < groups[j].PoolCode
		}
		return groups[i].Group < groups[j].Group
	})
	_, _ = fmt.Fprintf(w, "== Message groups holding buffered work: %d ==\n", len(groups))
	if len(groups) == 0 {
		_, _ = io.WriteString(w, "(none)\n\n")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "POOL\tGROUP\tBUFFERED\tDRAINER\tPARKED FOR\tSUPPRESSED UNTIL")
	for _, g := range groups {
		drainer, parked, suppressed := "running", "-", "-"
		if !g.Working {
			drainer = "none"
			if !g.ParkedAt.IsZero() {
				parked = now.Sub(g.ParkedAt).Round(time.Second).String()
			}
		}
		if g.Suppressed {
			suppressed = g.SuppressedUntil.UTC().Format(time.RFC3339)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\n", g.PoolCode, g.Group, g.Buffered, drainer, parked, suppressed)
	}
	_ = tw.Flush()
	_, _ = io.WriteString(w, "\n")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// debugState is the State the expvar "router" var reports on: the last one
// mounted (expvar names are process-global and can be published only once).
var debugState atomic.Pointer[State]

// publishRouterVars publishes the router's counters under expvar's "router"
// key.
func publishRouterVars(s *State) {
	debugState.Store(s)
	if expvar.Get("router") != nil {
		return
	}
	expvar.Publish("router", expvar.Func(func() any {
		out := map[string]any{
			"goroutines":      runtime.NumGoroutine(),
			"panicsRecovered": router.PanicsRecovered(),
		}
		s := debugState.Load()
		if s == nil {
			return out
		}
		if s.EventCounters != nil {
			out["events"] = s.EventCounters.EventCounters()
		}
		if s.Mediating != nil {
			out["mediating"] = len(s.Mediating.MediatingSnapshot())
		}
		return out
	}))
}
