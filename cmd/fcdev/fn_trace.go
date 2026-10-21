package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// Trace logs: opt-in, time-boxed capture per function context
// (docs/function-trace-design.md §7, §10).

// fnTracePollInterval is how often `trace log -f` and `trace run` poll. A var
// so tests can shorten it.
var fnTracePollInterval = 2 * time.Second

// fnTraceNow is the clock the "time left" columns use; a var for tests.
var fnTraceNow = time.Now

const (
	// fnTraceOverlap is how far before the newest record a follow poll looks
	// back, because the runner ships records in batches; the ids already
	// printed are skipped.
	fnTraceOverlap = 15 * time.Second
	// fnTracePageMax is the platform's page-size cap.
	fnTracePageMax = 500
	// fnTracePollCap bounds how many records one follow poll reads.
	fnTracePollCap = 5000
)

func newFnTraceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trace",
		Short: "Switch a function's trace contexts on and off and read its trace logs",
		Long: `Trace logs are opt-in and time-boxed: a function writes to named contexts and
nothing is stored unless a context is switched on, for a limited time. "http" is
built in: while it is on, every call also records its request, response and timing.

  fcdev fn trace on  <address> <context> [--for 5m] [--sample N]
  fcdev fn trace off <address> <context>
  fcdev fn trace status <address>
  fcdev fn trace log <address> [filters] [-f]
  fcdev fn trace run <address> <context> [--for 5m]   (on, then follow, then off)

Switching needs platform:function:trace:manage, reading records needs
platform:function:trace:view, and bodies (and switching http on) need
platform:function:trace:view-bodies.`,
	}
	cmd.AddCommand(newFnTraceOnCmd(), newFnTraceOffCmd(), newFnTraceStatusCmd(), newFnTraceLogCmd(), newFnTraceRunCmd())
	return cmd
}

// traceState is one context's state, as GET /trace and PUT /trace/{context}
// return it.
type traceState struct {
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	BuiltIn      bool    `json:"builtIn"`
	EnabledUntil *string `json:"enabledUntil"`
	SampleRate   int     `json:"sampleRate"`
	Indefinite   bool    `json:"indefinite"`
	Live         bool    `json:"live"`
	MaxMinutes   int     `json:"maxMinutes"`
}

func (s traceState) until() (time.Time, bool) {
	if s.EnabledUntil == nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, *s.EnabledUntil)
	return t, err == nil
}

// traceRecord is one stored trace record; Raw keeps the platform's JSON for
// --json.
type traceRecord struct {
	ID             string          `json:"id"`
	At             time.Time       `json:"at"`
	Version        int             `json:"version"`
	InvocationID   string          `json:"invocationId"`
	CorrelationID  *string         `json:"correlationId"`
	Reference      *string         `json:"reference"`
	Context        string          `json:"context"`
	Kind           string          `json:"kind"`
	Level          string          `json:"level"`
	Message        string          `json:"message"`
	Data           json.RawMessage `json:"data"`
	RunnerID       *string         `json:"runnerId"`
	BodiesRedacted bool            `json:"bodiesRedacted"`
	Raw            json.RawMessage `json:"-"`
}

// traceFilter are the trace-log filters shared by `log` and `run`.
type traceFilter struct {
	context, kind, level, reference, correlation, invocation string
	version                                                  int
	since, until                                             string
	limit                                                    int
}

func (f *traceFilter) add(cmd *cobra.Command, withContext bool) {
	fl := cmd.Flags()
	if withContext {
		fl.StringVar(&f.context, "context", "", "only this context")
	}
	fl.StringVar(&f.kind, "kind", "", "only this kind: trace, http or log")
	fl.StringVar(&f.level, "level", "", "only this level: debug, info, warn or error")
	fl.StringVar(&f.reference, "reference", "", "only records with this business reference (trip or order number)")
	fl.StringVar(&f.correlation, "correlation", "", "only records with this correlation id")
	fl.StringVar(&f.invocation, "invocation", "", "only records of this invocation")
	fl.IntVar(&f.version, "version", 0, "only records of this function version")
	fl.StringVar(&f.since, "since", "", "only records after this: a duration back from now (10m, 2h), RFC 3339 or unix ms")
	fl.StringVar(&f.until, "until", "", "only records before this: a duration back from now, RFC 3339 or unix ms")
	fl.IntVar(&f.limit, "limit", 100, "newest records to read (follow: the backlog shown first)")
}

// parseTraceTime turns --since/--until into an RFC 3339 instant: a Go
// duration counts back from now; RFC 3339 and unix ms pass through.
func parseTraceTime(flag, v string, now time.Time) (string, error) {
	if v == "" {
		return "", nil
	}
	if d, err := time.ParseDuration(v); err == nil {
		if d < 0 {
			return "", fmt.Errorf("--%s: %q is negative", flag, v)
		}
		return now.Add(-d).UTC().Format(time.RFC3339Nano), nil
	}
	if _, err := time.Parse(time.RFC3339, v); err == nil {
		return v, nil
	}
	if ms, err := strconv.ParseInt(v, 10, 64); err == nil && ms > 0 {
		return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano), nil
	}
	return "", fmt.Errorf("--%s: %q is not a duration (10m), an RFC 3339 time or unix milliseconds", flag, v)
}

// resolve validates the filter and normalises since/until.
func (f *traceFilter) resolve(now time.Time) error {
	var err error
	if f.since, err = parseTraceTime("since", f.since, now); err != nil {
		return err
	}
	if f.until, err = parseTraceTime("until", f.until, now); err != nil {
		return err
	}
	if f.limit < 1 {
		return errors.New("--limit must be >= 1")
	}
	if f.kind != "" && !slices.Contains([]string{"trace", "http", "log"}, f.kind) {
		return fmt.Errorf("--kind %q: want trace, http or log", f.kind)
	}
	return nil
}

// query builds the trace-log query for one page.
func (f *traceFilter) query(since, cursor string, limit int) url.Values {
	q := url.Values{}
	set := func(k, v string) {
		if v != "" {
			q.Set(k, v)
		}
	}
	set("context", f.context)
	set("kind", f.kind)
	set("level", f.level)
	set("reference", f.reference)
	set("correlationId", f.correlation)
	set("invocationId", f.invocation)
	if f.version > 0 {
		q.Set("version", strconv.Itoa(f.version))
	}
	set("since", since)
	set("until", f.until)
	set("cursor", cursor)
	q.Set("limit", strconv.Itoa(limit))
	return q
}

// traceView is how records are rendered.
type traceView struct{ json, bodies bool }

func (v *traceView) add(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&v.json, "json", false, "print one raw JSON record per line")
	cmd.Flags().BoolVar(&v.bodies, "bodies", false, "also print the request and response bodies of http records")
}

// traceError turns the platform's trace-log errors into plain messages.
// notFound is what a 404 means for the call.
func traceError(err error, notFound string) error {
	ae, ok := errors.AsType[*apiError](err)
	if !ok {
		return err
	}
	switch ae.Status {
	case http.StatusForbidden:
		return errors.New("this credential may not use trace logs (needs platform:function:trace:view, :manage and :view-bodies as the call requires)")
	case http.StatusNotFound:
		return errors.New(notFound)
	case http.StatusBadRequest:
		if msg := firstNonEmpty(ae.Message, ae.Detail); msg != "" {
			return errors.New(msg)
		}
	}
	return err
}

// traceTarget resolves the function an address names for a trace command.
func traceTarget(ctx context.Context, api *fnAPI, address string) (*fnFunction, error) {
	fn, err := api.findFunction(ctx, address)
	if err != nil {
		return nil, err
	}
	if fn == nil {
		return nil, fmt.Errorf("no function %s", address)
	}
	return fn, nil
}

// enableTrace switches a context on for d and prints the state.
func enableTrace(ctx context.Context, api *fnAPI, fn *fnFunction, name string, d time.Duration, forever bool, sample int, out io.Writer) (traceState, error) {
	if !forever && d <= 0 {
		return traceState{}, errors.New("--for must be positive (for example 5m)")
	}
	if sample < 1 {
		return traceState{}, errors.New("--sample must be >= 1")
	}
	minutes := max(int((d+time.Minute-1)/time.Minute), 1)
	var st traceState
	body := map[string]any{"minutes": minutes, "sampleRate": sample}
	if forever {
		body = map[string]any{"indefinite": true, "sampleRate": sample}
	}
	err := api.json(ctx, http.MethodPut, "/api/functions/"+fn.ID+"/trace/"+url.PathEscape(name), body, &st)
	if err != nil {
		return st, traceError(err, fmt.Sprintf("%s has no trace context %q", fn.Address, name))
	}
	line := fmt.Sprintf("%s on %s", name, fn.Address)
	if forever {
		line += ": live until you switch it off (fcdev fn trace off)"
	} else if until, ok := st.until(); ok {
		line += fmt.Sprintf(": live until %s (%dm)", until.Local().Format("15:04:05"), minutes)
	} else {
		line += fmt.Sprintf(": on for %dm", minutes)
	}
	if st.SampleRate > 1 {
		line += fmt.Sprintf(", recording 1 call in %d", st.SampleRate)
	}
	fmt.Fprintln(out, line)
	if name == "http" {
		fmt.Fprintln(out, "note: http records request and response bodies (sensitive; authorization, cookie and secret-like headers are redacted)")
	}
	return st, nil
}

func disableTrace(ctx context.Context, api *fnAPI, fn *fnFunction, name string, out io.Writer) error {
	err := api.json(ctx, http.MethodDelete, "/api/functions/"+fn.ID+"/trace/"+url.PathEscape(name), nil, nil)
	if err != nil {
		return traceError(err, fmt.Sprintf("%s has no trace context %q", fn.Address, name))
	}
	fmt.Fprintf(out, "%s off on %s\n", name, fn.Address)
	return nil
}

func newFnTraceOnCmd() *cobra.Command {
	var pf fnPlatformFlags
	var forD time.Duration
	var sample int
	var noLimit bool
	var yes bool
	cmd := &cobra.Command{
		Use:   "on <address> <context>",
		Short: "Switch a trace context on, for a time or until you switch it off",
		Long: `Switches a function's trace context on. By default it turns itself off when the
time is up: --for takes a Go duration (5m, 1h), rounded up to whole minutes, and
the platform caps it (60 minutes, 15 for http). --no-limit keeps it on until
"fcdev fn trace off". --sample N records one call in N. Takes effect within
about a second.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			fn, err := traceTarget(ctx, api, args[0])
			if err != nil {
				return err
			}
			if err := api.confirm(fmt.Sprintf("switch trace context %s on for %s", args[1], fn.Address), yes); err != nil {
				return err
			}
			_, err = enableTrace(ctx, api, fn, args[1], forD, noLimit, sample, cmd.OutOrStdout())
			return err
		},
	}
	pf.add(cmd)
	cmd.Flags().DurationVar(&forD, "for", 5*time.Minute, "how long to keep it on (rounded up to whole minutes)")
	cmd.Flags().BoolVar(&noLimit, "no-limit", false, "no time limit: stay on until switched off (--for is ignored)")
	cmd.Flags().IntVar(&sample, "sample", 1, "record one call in N")
	addYesFlag(cmd, &yes)
	return cmd
}

func newFnTraceOffCmd() *cobra.Command {
	var pf fnPlatformFlags
	var yes bool
	cmd := &cobra.Command{
		Use:   "off <address> <context>",
		Short: "Switch a trace context off",
		Long:  `Switches a function's trace context off now. Records already stored are kept (30 days).`,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			fn, err := traceTarget(ctx, api, args[0])
			if err != nil {
				return err
			}
			if err := api.confirm(fmt.Sprintf("switch trace context %s off for %s", args[1], fn.Address), yes); err != nil {
				return err
			}
			return disableTrace(ctx, api, fn, args[1], cmd.OutOrStdout())
		},
	}
	pf.add(cmd)
	addYesFlag(cmd, &yes)
	return cmd
}

func newFnTraceStatusCmd() *cobra.Command {
	var pf fnPlatformFlags
	cmd := &cobra.Command{
		Use:   "status <address>",
		Short: "List a function's trace contexts and whether each is live",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			fn, err := traceTarget(ctx, api, args[0])
			if err != nil {
				return err
			}
			var res struct {
				Contexts []traceState `json:"contexts"`
			}
			if err := api.json(ctx, http.MethodGet, "/api/functions/"+fn.ID+"/trace", nil, &res); err != nil {
				return traceError(err, "no trace contexts for "+fn.Address)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s (%s)\n", fn.Address, fn.ID)
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "CONTEXT\tBUILT-IN\tSTATE\tUNTIL\tSAMPLE\tDESCRIPTION")
			now := fnTraceNow()
			for _, c := range res.Contexts {
				state, until := "off", "-"
				if c.Live {
					state = "live"
				}
				if c.Live && c.Indefinite {
					until = "until switched off"
				} else if t, ok := c.until(); ok && c.Live {
					until = t.Local().Format("15:04:05")
					if left := t.Sub(now); left > 0 {
						until += " (" + left.Round(time.Second).String() + " left)"
					}
				}
				builtIn := "no"
				if c.BuiltIn {
					builtIn = "yes"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n", c.Name, builtIn, state, until, c.SampleRate, c.Description)
			}
			return tw.Flush()
		},
	}
	pf.add(cmd)
	return cmd
}

// fetchTrace reads up to total records (newest first), paging with the
// keyset cursor.
func fetchTrace(ctx context.Context, api *fnAPI, fn *fnFunction, f *traceFilter, since string, total int) ([]traceRecord, error) {
	var all []traceRecord
	cursor := ""
	for len(all) < total {
		page := min(total-len(all), fnTracePageMax)
		var res struct {
			Records    []json.RawMessage `json:"records"`
			NextCursor *string           `json:"nextCursor"`
		}
		path := "/api/functions/" + fn.ID + "/trace-log?" + f.query(since, cursor, page).Encode()
		if err := api.json(ctx, http.MethodGet, path, nil, &res); err != nil {
			return nil, traceError(err, "no trace log for "+fn.Address)
		}
		for _, raw := range res.Records {
			var r traceRecord
			if err := json.Unmarshal(raw, &r); err != nil {
				return nil, fmt.Errorf("decode trace record: %w", err)
			}
			r.Raw = raw
			all = append(all, r)
		}
		if res.NextCursor == nil || *res.NextCursor == "" || len(res.Records) == 0 {
			break
		}
		cursor = *res.NextCursor
	}
	return all, nil
}

// sortTraceOldestFirst orders records as a log reads: by time, then id.
func sortTraceOldestFirst(rs []traceRecord) {
	slices.SortStableFunc(rs, func(a, b traceRecord) int {
		if c := a.At.Compare(b.At); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

// traceTailer prints records and, when following, only the new ones.
type traceTailer struct {
	api  *fnAPI
	fn   *fnFunction
	f    *traceFilter
	view traceView
	out  io.Writer
	errW io.Writer
	// seen are the ids printed within the overlap window; last is the newest
	// record time printed (or where reading started).
	seen map[string]time.Time
	last time.Time
	// floor drops records older than where the user asked to start, which
	// the overlap window would otherwise bring back.
	floor time.Time
}

func newTraceTailer(api *fnAPI, fn *fnFunction, f *traceFilter, v traceView, out, errW io.Writer) *traceTailer {
	return &traceTailer{api: api, fn: fn, f: f, view: v, out: out, errW: errW, seen: map[string]time.Time{}}
}

func (t *traceTailer) emit(rs []traceRecord) {
	for _, r := range rs {
		t.seen[r.ID] = r.At
		if r.At.After(t.last) {
			t.last = r.At
		}
		printTraceRecord(t.out, r, t.view)
	}
}

// backlog prints the newest f.limit records, oldest first.
func (t *traceTailer) backlog(ctx context.Context) error {
	rs, err := fetchTrace(ctx, t.api, t.fn, t.f, t.f.since, t.f.limit)
	if err != nil {
		return err
	}
	if since, err := time.Parse(time.RFC3339Nano, t.f.since); err == nil {
		t.last, t.floor = since, since
	} else if len(rs) == 0 {
		t.last = fnTraceNow()
		t.floor = t.last
	}
	sortTraceOldestFirst(rs)
	t.emit(rs)
	return nil
}

// poll prints what arrived since the last poll. It asks from a little before
// the newest record (runners ship in batches) and skips ids already printed.
func (t *traceTailer) poll(ctx context.Context) error {
	from := t.last.Add(-fnTraceOverlap)
	rs, err := fetchTrace(ctx, t.api, t.fn, t.f, from.UTC().Format(time.RFC3339Nano), fnTracePollCap)
	if err != nil {
		return err
	}
	fresh := rs[:0]
	for _, r := range rs {
		if _, dup := t.seen[r.ID]; !dup && !r.At.Before(t.floor) {
			fresh = append(fresh, r)
		}
	}
	for id, at := range t.seen {
		if at.Before(from) {
			delete(t.seen, id)
		}
	}
	sortTraceOldestFirst(fresh)
	t.emit(fresh)
	return nil
}

// follow polls until ctx ends. A transient failure is reported and retried;
// a refusal from the platform (4xx) ends it.
func (t *traceTailer) follow(ctx context.Context) error {
	tick := time.NewTicker(fnTracePollInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		if err := t.poll(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if ae, ok := errors.AsType[*apiError](err); ok && ae.Status/100 == 4 {
				return err
			}
			fmt.Fprintf(t.errW, "fcdev: trace poll failed (will retry): %v\n", err)
		}
	}
}

func newFnTraceLogCmd() *cobra.Command {
	var pf fnPlatformFlags
	var f traceFilter
	var v traceView
	var follow bool
	cmd := &cobra.Command{
		Use:   "log <address>",
		Short: "Print a function's trace records, optionally following new ones",
		Long: `Prints the newest trace records (--limit, default 100), oldest first so it reads
like a log: time, level, context, invocation, reference and message. An http
record is one summary line, for example "POST /orders -> 200 (12ms, ok)"; --bodies
adds its request and response bodies (without platform:function:trace:view-bodies
they are hidden). -f polls every 2 seconds and prints only new records until
Ctrl-C. --json prints one raw JSON record per line.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := f.resolve(fnTraceNow()); err != nil {
				return err
			}
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			fn, err := traceTarget(ctx, api, args[0])
			if err != nil {
				return err
			}
			t := newTraceTailer(api, fn, &f, v, cmd.OutOrStdout(), cmd.ErrOrStderr())
			if err := t.backlog(ctx); err != nil {
				return err
			}
			if !follow {
				return nil
			}
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			return t.follow(ctx)
		},
	}
	pf.add(cmd)
	f.add(cmd, true)
	v.add(cmd)
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "keep polling and print new records until Ctrl-C")
	return cmd
}

func newFnTraceRunCmd() *cobra.Command {
	var pf fnPlatformFlags
	var f traceFilter
	var v traceView
	var forD time.Duration
	var sample int
	var keep, yes bool
	cmd := &cobra.Command{
		Use:   "run <address> <context>",
		Short: "Switch a context on, follow its records, and switch it off again on exit",
		Long: `The dev loop in one command: switches the context on (see "trace on"), follows
the function's records for it, and switches the context off again when you press
Ctrl-C or the window ends, unless --keep. Only records from now on are shown
unless --since says otherwise. Takes the same filters as "trace log".`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			now := fnTraceNow()
			f.context = args[1]
			if f.since == "" {
				f.since = "0s"
			}
			if err := f.resolve(now); err != nil {
				return err
			}
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			fn, err := traceTarget(ctx, api, args[0])
			if err != nil {
				return err
			}
			if err := api.confirm(fmt.Sprintf("switch trace context %s on for %s", args[1], fn.Address), yes); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			st, err := enableTrace(ctx, api, fn, args[1], forD, false, sample, out)
			if err != nil {
				return err
			}
			// Leave the context off however the follow ends.
			if !keep {
				defer func() {
					offCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
					defer cancel()
					if err := disableTrace(offCtx, api, fn, args[1], out); err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "fcdev: could not switch %s off (it expires on its own): %v\n", args[1], err)
					}
				}()
			}
			// Follow until Ctrl-C, or shortly after the window closes.
			followCtx := ctx
			if until, ok := st.until(); ok {
				var cancel context.CancelFunc
				followCtx, cancel = context.WithDeadline(ctx, until.Add(fnTracePollInterval*2))
				defer cancel()
			}
			fmt.Fprintln(cmd.ErrOrStderr(), "following; Ctrl-C to stop")
			t := newTraceTailer(api, fn, &f, v, out, cmd.ErrOrStderr())
			if err := t.backlog(followCtx); err != nil {
				return err
			}
			return t.follow(followCtx)
		},
	}
	pf.add(cmd)
	f.add(cmd, false)
	v.add(cmd)
	cmd.Flags().DurationVar(&forD, "for", 5*time.Minute, "how long to keep it on (rounded up to whole minutes)")
	cmd.Flags().IntVar(&sample, "sample", 1, "record one call in N")
	cmd.Flags().BoolVar(&keep, "keep", false, "leave the context on when this command exits")
	addYesFlag(cmd, &yes)
	return cmd
}

// --- rendering ---

// traceHTTP is the data of a kind "http" record.
type traceHTTP struct {
	Request struct {
		Method       string `json:"method"`
		Path         string `json:"path"`
		RawQuery     string `json:"rawQuery"`
		Body         any    `json:"body"`
		BodyEncoding string `json:"bodyEncoding"`
		BodyBytes    int    `json:"bodyBytes"`
		Truncated    bool   `json:"bodyTruncated"`
	} `json:"request"`
	Response struct {
		Status       int    `json:"status"`
		Body         any    `json:"body"`
		BodyEncoding string `json:"bodyEncoding"`
		BodyBytes    int    `json:"bodyBytes"`
		Truncated    bool   `json:"bodyTruncated"`
	} `json:"response"`
	DurationMs int64  `json:"durationMs"`
	Outcome    string `json:"outcome"`
}

func printTraceRecord(w io.Writer, r traceRecord, v traceView) {
	if v.json {
		var b bytes.Buffer
		if json.Compact(&b, r.Raw) != nil {
			b.Reset()
			b.Write(r.Raw)
		}
		fmt.Fprintln(w, b.String())
		return
	}
	var msg string
	var h *traceHTTP
	if r.Kind == "http" && len(r.Data) > 0 {
		h = &traceHTTP{}
		if json.Unmarshal(r.Data, h) != nil {
			h = nil
		}
	}
	switch {
	case h != nil:
		path := h.Request.Path
		if h.Request.RawQuery != "" {
			path += "?" + h.Request.RawQuery
		}
		msg = fmt.Sprintf("%s %s -> %d (%dms, %s)", h.Request.Method, path, h.Response.Status, h.DurationMs, firstNonEmpty(h.Outcome, "?"))
	default:
		msg = r.Message
		if len(r.Data) > 0 && string(r.Data) != "null" {
			var b bytes.Buffer
			if json.Compact(&b, r.Data) == nil {
				msg += " " + b.String()
			}
		}
	}
	line := fmt.Sprintf("%s %-5s %-10s inv=%s", r.At.Local().Format("15:04:05.000"), strings.ToUpper(r.Level), r.Context, r.InvocationID)
	if r.Reference != nil && *r.Reference != "" {
		line += " ref=" + *r.Reference
	}
	fmt.Fprintf(w, "%s  %s\n", line, msg)
	if h != nil && v.bodies {
		if r.BodiesRedacted {
			fmt.Fprintln(w, "    (bodies hidden: no view-bodies permission)")
			return
		}
		printTraceBody(w, "request", h.Request.Body, h.Request.BodyEncoding, h.Request.BodyBytes, h.Request.Truncated)
		printTraceBody(w, "response", h.Response.Body, h.Response.BodyEncoding, h.Response.BodyBytes, h.Response.Truncated)
	}
}

func printTraceBody(w io.Writer, which string, body any, encoding string, size int, truncated bool) {
	s, _ := body.(string)
	if s == "" {
		fmt.Fprintf(w, "    %s body: (empty)\n", which)
		return
	}
	notes := []string{fmt.Sprintf("%d bytes", size)}
	if encoding == "base64" {
		notes = append(notes, "base64")
	}
	if truncated {
		notes = append(notes, "truncated")
	}
	fmt.Fprintf(w, "    %s body (%s):\n", which, strings.Join(notes, ", "))
	if encoding != "base64" {
		var pretty bytes.Buffer
		if json.Valid([]byte(s)) && json.Indent(&pretty, []byte(s), "", "  ") == nil {
			s = pretty.String()
		}
	}
	for l := range strings.SplitSeq(strings.TrimRight(s, "\n"), "\n") {
		fmt.Fprintf(w, "      %s\n", l)
	}
}
