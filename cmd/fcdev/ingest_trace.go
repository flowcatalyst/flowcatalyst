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
	"time"

	"github.com/spf13/cobra"
)

// Ingestion traces: opt-in, time-boxed capture per ingestion
// (docs/ingestion-trace-design.md sections 7 and 10). The mechanics mirror
// `fcdev fn trace`; the names carry an ing prefix because both live in one
// package.

// ingTraceKinds are the record kinds a runtime ships.
var ingTraceKinds = []string{"poll", "exchange", "http", "error"}

func newIngestTraceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trace",
		Short: "Switch an ingestion's tracing on and off and read what it captured",
		Long: `Traces are opt-in and time-boxed: nothing is captured unless tracing is switched
on for the ingestion, and then each runtime that runs it records, for a minute or
so, what its polls and exchanges did, the HTTP calls an extension or an api poll
made included, with their requests and responses (credentials redacted).

  fcdev ingest trace on  <application>/<client>/<code> [--for 1m|5m|15m] [--every 5s]
  fcdev ingest trace off <application>/<client>/<code>
  fcdev ingest trace status <application>/<client>/<code>
  fcdev ingest trace log <application>/<client>/<code> [filters] [-f]

A runtime picks a window up at its next sync (a minute in production, 15 s under
fcdev) and captures for the chosen time from then. Switching on needs
platform:ingestion:trace:manage and :view-bodies, reading records needs
:view, and headers, query strings, bodies and error text need :view-bodies.`,
	}
	cmd.AddCommand(newIngestTraceOnCmd(), newIngestTraceOffCmd(), newIngestTraceStatusCmd(), newIngestTraceLogCmd())
	return cmd
}

// ingTraceState is an ingestion's trace window, as GET, PUT /trace return it.
type ingTraceState struct {
	Live            bool    `json:"live"`
	WindowID        *string `json:"windowId"`
	WindowSecs      *int    `json:"windowSecs"`
	SampleEverySecs *int    `json:"sampleEverySecs"`
	EnabledAt       *string `json:"enabledAt"`
	HardStop        *string `json:"hardStop"`
	EnabledBy       *string `json:"enabledBy"`
	MaxWindowSecs   int     `json:"maxWindowSecs"`
}

func (s ingTraceState) hardStop() (time.Time, bool) {
	if s.HardStop == nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, *s.HardStop)
	return t, err == nil
}

// ingTraceRecord is one stored trace record; Raw keeps the platform's JSON
// for --json.
type ingTraceRecord struct {
	ID        string          `json:"id"`
	At        time.Time       `json:"at"`
	Version   int             `json:"version"`
	WindowID  string          `json:"windowId"`
	CaptureID string          `json:"captureId"`
	RuntimeID string          `json:"runtimeId"`
	Kind      string          `json:"kind"`
	Level     string          `json:"level"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data"`
	Redacted  bool            `json:"redacted"`
	Raw       json.RawMessage `json:"-"`
}

// ingTraceError turns the platform's trace errors into plain messages.
func ingTraceError(err error, notFound string) error {
	ae, ok := errors.AsType[*apiError](err)
	if !ok {
		return err
	}
	switch ae.Status {
	case http.StatusForbidden:
		return errors.New("this credential may not use ingestion traces (needs platform:ingestion:trace:view, :manage and :view-bodies as the call requires)")
	case http.StatusNotFound:
		return errors.New(notFound)
	case http.StatusBadRequest:
		if msg := firstNonEmpty(ae.Message, ae.Detail); msg != "" {
			return errors.New(msg)
		}
	}
	return err
}

func ingTracePath(id, suffix string) string {
	return "/api/ingestions/" + url.PathEscape(id) + "/" + suffix
}

// ingTraceTarget resolves the ingestion a trace command names.
func ingTraceTarget(ctx context.Context, api *fnAPI, ref string) (*ingRow, error) {
	r, err := api.findIngestion(ctx, ref)
	if err != nil {
		return nil, ingTraceError(err, "no ingestion "+ref)
	}
	return r, nil
}

// ingTraceWindowSecs turns --for into the platform's window choice.
func ingTraceWindowSecs(d time.Duration) (int, error) {
	if slices.Contains([]time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute}, d) {
		return int(d / time.Second), nil
	}
	return 0, fmt.Errorf("--for %s: want 1m, 5m or 15m", d)
}

// ingTraceSampleSecs turns --every into whole seconds, 1 to 60.
func ingTraceSampleSecs(d time.Duration) (int, error) {
	if d < time.Second || d > time.Minute || d%time.Second != 0 {
		return 0, fmt.Errorf("--every %s: want a whole number of seconds from 1s to 60s", d)
	}
	return int(d / time.Second), nil
}

func newIngestTraceOnCmd() *cobra.Command {
	var pf fnPlatformFlags
	var forD, every time.Duration
	var yes bool
	cmd := &cobra.Command{
		Use:   "on <application>/<client>/<code>",
		Short: "Switch tracing on for an ingestion, for 1, 5 or 15 minutes",
		Long: `Switches tracing on with a new window. Each runtime that runs the ingestion starts
capturing when it next syncs and captures for --for from then (so a one-minute
window is a minute of capture however long the sync took). A runtime starts a
capture at most once every --every, so at the default about 12 a minute.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			window, err := ingTraceWindowSecs(forD)
			if err != nil {
				return err
			}
			sample, err := ingTraceSampleSecs(every)
			if err != nil {
				return err
			}
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			r, err := ingTraceTarget(ctx, api, args[0])
			if err != nil {
				return err
			}
			if err := api.confirm("switch tracing on for "+r.ref(), yes); err != nil {
				return err
			}
			var st ingTraceState
			body := map[string]any{"windowSecs": window, "sampleEverySecs": sample}
			if err := api.json(ctx, http.MethodPut, ingTracePath(r.ID, "trace"), body, &st); err != nil {
				return ingTraceError(err, "no ingestion "+args[0])
			}
			out := cmd.OutOrStdout()
			line := fmt.Sprintf("tracing on for %s: %s per runtime, one capture every %ds", r.ref(), forD, sample)
			if until, ok := st.hardStop(); ok {
				line += fmt.Sprintf("; no runtime captures past %s", until.Local().Format("15:04:05"))
			}
			fmt.Fprintln(out, line)
			fmt.Fprintln(out, "note: each runtime starts when it next syncs; records hold request and response bodies (sensitive; credential-like values are redacted)")
			return nil
		},
	}
	pf.add(cmd)
	cmd.Flags().DurationVar(&forD, "for", time.Minute, "how long each runtime captures: 1m, 5m or 15m")
	cmd.Flags().DurationVar(&every, "every", 5*time.Second, "a runtime starts a capture at most this often: 1s to 60s")
	addYesFlag(cmd, &yes)
	return cmd
}

func newIngestTraceOffCmd() *cobra.Command {
	var pf fnPlatformFlags
	var yes bool
	cmd := &cobra.Command{
		Use:   "off <application>/<client>/<code>",
		Short: "Switch tracing off",
		Long:  `Switches tracing off now. Records already stored are kept (7 days).`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			r, err := ingTraceTarget(ctx, api, args[0])
			if err != nil {
				return err
			}
			if err := api.confirm("switch tracing off for "+r.ref(), yes); err != nil {
				return err
			}
			if err := api.json(ctx, http.MethodDelete, ingTracePath(r.ID, "trace"), nil, nil); err != nil {
				return ingTraceError(err, "no ingestion "+args[0])
			}
			fmt.Fprintf(cmd.OutOrStdout(), "tracing off for %s\n", r.ref())
			return nil
		},
	}
	pf.add(cmd)
	addYesFlag(cmd, &yes)
	return cmd
}

func newIngestTraceStatusCmd() *cobra.Command {
	var pf fnPlatformFlags
	cmd := &cobra.Command{
		Use:   "status <application>/<client>/<code>",
		Short: "Show whether tracing is on for an ingestion and until when",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			r, err := ingTraceTarget(ctx, api, args[0])
			if err != nil {
				return err
			}
			var st ingTraceState
			if err := api.json(ctx, http.MethodGet, ingTracePath(r.ID, "trace"), nil, &st); err != nil {
				return ingTraceError(err, "no ingestion "+args[0])
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s (%s)\n", r.ref(), r.ID)
			if st.WindowID == nil {
				fmt.Fprintln(out, "  tracing  off (never switched on)")
				return nil
			}
			state := "off"
			if st.Live {
				state = "live"
			}
			fmt.Fprintf(out, "  tracing  %s\n", state)
			fmt.Fprintf(out, "  window   %s", *st.WindowID)
			if st.WindowSecs != nil && st.SampleEverySecs != nil {
				fmt.Fprintf(out, ", %ds per runtime, one capture every %ds", *st.WindowSecs, *st.SampleEverySecs)
			}
			fmt.Fprintln(out)
			if until, ok := st.hardStop(); ok {
				line := "  stops    " + until.Local().Format("15:04:05")
				if left := until.Sub(fnTraceNow()); st.Live && left > 0 {
					line += " (" + left.Round(time.Second).String() + " left)"
				}
				fmt.Fprintln(out, line)
			}
			if st.EnabledBy != nil {
				fmt.Fprintf(out, "  by       %s\n", *st.EnabledBy)
			}
			return nil
		},
	}
	pf.add(cmd)
	return cmd
}

// ingTraceFilter are the trace-log filters.
type ingTraceFilter struct {
	kind, runtime, capture string
	since, until           string
	limit                  int
}

func (f *ingTraceFilter) add(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.StringVar(&f.kind, "kind", "", "only this kind: poll, exchange, http or error")
	fl.StringVar(&f.runtime, "runtime", "", "only this runtime's records")
	fl.StringVar(&f.capture, "capture", "", "only one capture's records")
	fl.StringVar(&f.since, "since", "", "only records after this: a duration back from now (10m, 2h), RFC 3339 or unix ms")
	fl.StringVar(&f.until, "until", "", "only records before this: a duration back from now, RFC 3339 or unix ms")
	fl.IntVar(&f.limit, "limit", 100, "newest records to read (follow: the backlog shown first)")
}

func (f *ingTraceFilter) resolve(now time.Time) error {
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
	if f.kind != "" && !slices.Contains(ingTraceKinds, f.kind) {
		return fmt.Errorf("--kind %q: want poll, exchange, http or error", f.kind)
	}
	return nil
}

func (f *ingTraceFilter) query(since, cursor string, limit int) url.Values {
	q := url.Values{}
	set := func(k, v string) {
		if v != "" {
			q.Set(k, v)
		}
	}
	set("kind", f.kind)
	set("runtimeId", f.runtime)
	set("captureId", f.capture)
	set("since", since)
	set("until", f.until)
	set("cursor", cursor)
	q.Set("limit", strconv.Itoa(limit))
	return q
}

// fetchIngTrace reads up to total records (newest first), paging with the
// keyset cursor.
func fetchIngTrace(ctx context.Context, api *fnAPI, r *ingRow, f *ingTraceFilter, since string, total int) ([]ingTraceRecord, error) {
	var all []ingTraceRecord
	cursor := ""
	for len(all) < total {
		page := min(total-len(all), fnTracePageMax)
		var res struct {
			Records    []json.RawMessage `json:"records"`
			NextCursor *string           `json:"nextCursor"`
		}
		if err := api.json(ctx, http.MethodGet, ingTracePath(r.ID, "trace-log?"+f.query(since, cursor, page).Encode()), nil, &res); err != nil {
			return nil, ingTraceError(err, "no trace log for "+r.ref())
		}
		for _, raw := range res.Records {
			var rec ingTraceRecord
			if err := json.Unmarshal(raw, &rec); err != nil {
				return nil, fmt.Errorf("decode trace record: %w", err)
			}
			rec.Raw = raw
			all = append(all, rec)
		}
		if res.NextCursor == nil || *res.NextCursor == "" || len(res.Records) == 0 {
			break
		}
		cursor = *res.NextCursor
	}
	return all, nil
}

// sortIngTraceOldestFirst orders records as a log reads: by time, then id.
func sortIngTraceOldestFirst(rs []ingTraceRecord) {
	slices.SortStableFunc(rs, func(a, b ingTraceRecord) int {
		if c := a.At.Compare(b.At); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

// ingTraceTailer prints records and, when following, only the new ones.
type ingTraceTailer struct {
	api    *fnAPI
	row    *ingRow
	f      *ingTraceFilter
	json   bool
	bodies bool
	out    io.Writer
	errW   io.Writer
	// seen are the ids printed within the overlap window; last is the newest
	// record time printed (or where reading started).
	seen map[string]time.Time
	last time.Time
	// floor drops records older than where the user asked to start, which
	// the overlap window would otherwise bring back.
	floor time.Time
}

func (t *ingTraceTailer) emit(rs []ingTraceRecord) {
	for _, r := range rs {
		t.seen[r.ID] = r.At
		if r.At.After(t.last) {
			t.last = r.At
		}
		printIngTraceRecord(t.out, r, t.json, t.bodies)
	}
}

// backlog prints the newest f.limit records, oldest first.
func (t *ingTraceTailer) backlog(ctx context.Context) error {
	rs, err := fetchIngTrace(ctx, t.api, t.row, t.f, t.f.since, t.f.limit)
	if err != nil {
		return err
	}
	if since, err := time.Parse(time.RFC3339Nano, t.f.since); err == nil {
		t.last, t.floor = since, since
	} else if len(rs) == 0 {
		t.last = fnTraceNow()
		t.floor = t.last
	}
	sortIngTraceOldestFirst(rs)
	t.emit(rs)
	return nil
}

// poll prints what arrived since the last poll. It asks from a little before
// the newest record (runtimes ship in batches) and skips ids already printed.
func (t *ingTraceTailer) poll(ctx context.Context) error {
	from := t.last.Add(-fnTraceOverlap)
	rs, err := fetchIngTrace(ctx, t.api, t.row, t.f, from.UTC().Format(time.RFC3339Nano), fnTracePollCap)
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
	sortIngTraceOldestFirst(fresh)
	t.emit(fresh)
	return nil
}

// follow polls until ctx ends. A transient failure is reported and retried; a
// refusal from the platform (4xx) ends it.
func (t *ingTraceTailer) follow(ctx context.Context) error {
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

func newIngestTraceLogCmd() *cobra.Command {
	var pf fnPlatformFlags
	var f ingTraceFilter
	var asJSON, bodies, follow bool
	cmd := &cobra.Command{
		Use:   "log <application>/<client>/<code>",
		Short: "Print an ingestion's trace records, optionally following new ones",
		Long: `Prints the newest trace records (--limit, default 100), oldest first so it reads
like a log: time, level, kind, runtime, capture and message. An http record is
one line, for example "GET https://api.example.com/trips -> 200 (85ms)"; --bodies
adds its request and response (without platform:ingestion:trace:view-bodies they
are hidden). -f polls every 2 seconds and prints only new records until Ctrl-C.
--json prints one raw JSON record per line.`,
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
			r, err := ingTraceTarget(ctx, api, args[0])
			if err != nil {
				return err
			}
			t := &ingTraceTailer{
				api: api, row: r, f: &f, json: asJSON, bodies: bodies, out: cmd.OutOrStdout(), errW: cmd.ErrOrStderr(),
				seen: map[string]time.Time{},
			}
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
	f.add(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "print one raw JSON record per line")
	cmd.Flags().BoolVar(&bodies, "bodies", false, "also print the request and response of http records")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "keep polling and print new records until Ctrl-C")
	return cmd
}

// ingTraceHTTP is the data of a kind "http" record.
type ingTraceHTTP struct {
	Client  string `json:"client"`
	Request struct {
		Method       string              `json:"method"`
		URL          string              `json:"url"`
		Headers      map[string][]string `json:"headers"`
		Body         any                 `json:"body"`
		BodyEncoding string              `json:"bodyEncoding"`
		BodyBytes    int                 `json:"bodyBytes"`
		Truncated    bool                `json:"bodyTruncated"`
	} `json:"request"`
	Response *struct {
		Status       int                 `json:"status"`
		Headers      map[string][]string `json:"headers"`
		Body         any                 `json:"body"`
		BodyEncoding string              `json:"bodyEncoding"`
		BodyBytes    int                 `json:"bodyBytes"`
		Truncated    bool                `json:"bodyTruncated"`
	} `json:"response"`
	DurationMs int64   `json:"durationMs"`
	Attempt    int     `json:"attempt"`
	Failure    *string `json:"failure"`
}

func printIngTraceRecord(w io.Writer, r ingTraceRecord, asJSON, bodies bool) {
	if asJSON {
		var b bytes.Buffer
		if json.Compact(&b, r.Raw) != nil {
			b.Reset()
			b.Write(r.Raw)
		}
		fmt.Fprintln(w, b.String())
		return
	}
	var h *ingTraceHTTP
	if r.Kind == "http" && len(r.Data) > 0 {
		h = &ingTraceHTTP{}
		if json.Unmarshal(r.Data, h) != nil {
			h = nil
		}
	}
	msg := r.Message
	if h != nil {
		status := "no response"
		if h.Response != nil {
			status = strconv.Itoa(h.Response.Status)
		} else if h.Failure != nil {
			status = "failed: " + *h.Failure
		}
		msg = fmt.Sprintf("%s %s -> %s (%dms", h.Request.Method, h.Request.URL, status, h.DurationMs)
		if h.Attempt > 1 {
			msg += fmt.Sprintf(", attempt %d", h.Attempt)
		}
		msg += ")"
	}
	fmt.Fprintf(w, "%s %-5s %-8s %s cap=%s  %s\n", r.At.Local().Format("15:04:05.000"), strings.ToUpper(r.Level), r.Kind, r.RuntimeID, r.CaptureID, msg)
	if h != nil && bodies {
		if r.Redacted {
			fmt.Fprintln(w, "    (headers, query string and bodies hidden: no view-bodies permission)")
			return
		}
		printTraceBody(w, "request", h.Request.Body, h.Request.BodyEncoding, h.Request.BodyBytes, h.Request.Truncated)
		if h.Response != nil {
			printTraceBody(w, "response", h.Response.Body, h.Response.BodyEncoding, h.Response.BodyBytes, h.Response.Truncated)
		}
	}
}
