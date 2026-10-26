package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeTracePlatform is a platform with one function (demo.logger, fn_1) and
// its trace routes. Records are kept newest first, as the real API returns
// them, and paged two at a time to exercise the cursor.
type fakeTracePlatform struct {
	srv *httptest.Server
	mu  sync.Mutex
	// requests are "METHOD path?query" lines.
	requests []string
	// putBody is the last PUT body.
	putBody map[string]any
	// status, when non-zero, is what every trace route answers, with putError
	// as the body.
	status   int
	putError string
	records  []map[string]any
	queries  []url.Values
	// state is each context's state as GET /trace reports it; PUT and DELETE
	// change it.
	state map[string]map[string]any
	// onPut, when set, runs as a PUT is applied, before it is answered.
	onPut func()
}

func newFakeTracePlatform(t *testing.T) *fakeTracePlatform {
	p := &fakeTracePlatform{state: map[string]map[string]any{
		"http":   {"name": "http", "description": "Request and response of every call.", "builtIn": true, "enabledUntil": "2026-10-21T12:20:45Z", "sampleRate": 1, "live": true, "maxMinutes": 15, "indefiniteMaxDays": 0},
		"orders": {"name": "orders", "description": "Order mapping.", "builtIn": false, "enabledUntil": nil, "sampleRate": 1, "live": false, "maxMinutes": 60, "indefiniteMaxDays": 7},
	}}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.requests = append(p.requests, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"t","expires_in":300,"token_type":"Bearer"}`))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/functions" {
			if r.URL.Query().Get("addressPrefix") != "demo.logger" {
				_, _ = w.Write([]byte(`{"data":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"fn_1","address":"demo.logger","applicationCode":"demo"}]}`))
			return
		}
		if p.status != 0 && strings.HasPrefix(r.URL.Path, "/api/functions/fn_1/trace") {
			w.WriteHeader(p.status)
			_, _ = w.Write([]byte(p.putError))
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/functions/fn_1/trace":
			_ = json.NewEncoder(w).Encode(map[string]any{"contexts": []any{p.state["http"], p.state["orders"]}})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/functions/fn_1/trace/"):
			name := strings.TrimPrefix(r.URL.Path, "/api/functions/fn_1/trace/")
			p.putBody = nil
			_ = json.NewDecoder(r.Body).Decode(&p.putBody)
			minutes, _ := p.putBody["minutes"].(float64)
			until := time.Now().Add(time.Duration(max(minutes, 5)) * time.Minute).UTC().Format(time.RFC3339)
			st := map[string]any{
				"name": name, "builtIn": name == "http", "enabledUntil": until, "sampleRate": p.putBody["sampleRate"],
				"live": true, "maxMinutes": 60, "indefinite": p.putBody["indefinite"] == true,
			}
			p.state[name] = st
			if p.onPut != nil {
				p.onPut()
			}
			_ = json.NewEncoder(w).Encode(st)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/functions/fn_1/trace/"):
			name := strings.TrimPrefix(r.URL.Path, "/api/functions/fn_1/trace/")
			if st := p.state[name]; st != nil {
				st["live"], st["indefinite"], st["enabledUntil"] = false, false, time.Now().UTC().Format(time.RFC3339)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/api/functions/fn_1/trace-log":
			p.queries = append(p.queries, r.URL.Query())
			p.serveLog(w, r.URL.Query())
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(p.srv.Close)
	return p
}

// serveLog answers a trace-log page: since filter, two records per page,
// cursor = offset.
func (p *fakeTracePlatform) serveLog(w http.ResponseWriter, q url.Values) {
	var match []map[string]any
	for _, rec := range p.records {
		if s := q.Get("since"); s != "" {
			since, err := time.Parse(time.RFC3339Nano, s)
			if err == nil {
				at, _ := time.Parse(time.RFC3339Nano, rec["at"].(string))
				if at.Before(since) {
					continue
				}
			}
		}
		match = append(match, rec)
	}
	off, _ := strconv.Atoi(q.Get("cursor"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	page := min(limit, 2)
	end := min(off+page, len(match))
	var next any
	if end < len(match) && end-off < limit {
		next = strconv.Itoa(end)
	}
	if off > len(match) {
		off = len(match)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"records": match[off:end], "nextCursor": next})
}

func (p *fakeTracePlatform) count(prefix string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, r := range p.requests {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}

func (p *fakeTracePlatform) addRecord(id string, at time.Time, kind, level, msg string, data any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	rec := map[string]any{
		"id": id, "at": at.UTC().Format(time.RFC3339Nano), "version": 1, "invocationId": "inv-" + id,
		"correlationId": nil, "reference": "T-1", "context": "orders", "kind": kind, "level": level,
		"message": msg, "data": data, "runnerId": nil,
	}
	p.records = append([]map[string]any{rec}, p.records...) // newest first
}

// lockedBuffer is a buffer the command writes to while a test reads it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func traceCmd(p *fakeTracePlatform, out *lockedBuffer, args ...string) *cobraRun {
	cmd := newFnTraceCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs(append(args, "--platform", p.srv.URL, "--client-id", "c", "--client-secret", "s", "--runner", p.srv.URL))
	return &cobraRun{run: cmd.ExecuteContext}
}

type cobraRun struct{ run func(context.Context) error }

func runTrace(t *testing.T, p *fakeTracePlatform, args ...string) (string, error) {
	t.Helper()
	var out lockedBuffer
	err := traceCmd(p, &out, args...).run(context.Background())
	return out.String(), err
}

func useLocalUTC(t *testing.T) {
	old := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = old })
}

func shortPoll(t *testing.T) {
	old := fnTracePollInterval
	fnTracePollInterval = 10 * time.Millisecond
	t.Cleanup(func() { fnTracePollInterval = old })
}

func TestFnTraceOn(t *testing.T) {
	p := newFakeTracePlatform(t)
	out, err := runTrace(t, p, "on", "demo.logger", "orders")
	require.NoError(t, err, out)
	require.Contains(t, out, "orders on demo.logger: live until ")
	require.Contains(t, out, "(5m)")
	require.EqualValues(t, 5, p.putBody["minutes"])
	require.EqualValues(t, 1, p.putBody["sampleRate"])
	require.NotContains(t, out, "bodies")

	// 90s rounds up to 2 minutes; a sample is reported.
	out, err = runTrace(t, p, "on", "demo.logger", "http", "--for", "90s", "--sample", "4")
	require.NoError(t, err, out)
	require.EqualValues(t, 2, p.putBody["minutes"])
	require.EqualValues(t, 4, p.putBody["sampleRate"])
	require.Contains(t, out, "(2m), recording 1 call in 4")
	require.Contains(t, out, "note: http records request and response bodies")

	// Under a minute is one minute.
	_, err = runTrace(t, p, "on", "demo.logger", "orders", "--for", "1s")
	require.NoError(t, err)
	require.EqualValues(t, 1, p.putBody["minutes"])
}

func TestFnTraceOnRejectsBadFlags(t *testing.T) {
	p := newFakeTracePlatform(t)
	_, err := runTrace(t, p, "on", "demo.logger", "orders", "--for", "0s")
	require.ErrorContains(t, err, "--for must be positive")
	_, err = runTrace(t, p, "on", "demo.logger", "orders", "--sample", "0")
	require.ErrorContains(t, err, "--sample must be >= 1")
	require.Zero(t, p.count("PUT"))
}

func TestFnTraceOff(t *testing.T) {
	p := newFakeTracePlatform(t)
	out, err := runTrace(t, p, "off", "demo.logger", "orders")
	require.NoError(t, err, out)
	require.Contains(t, out, "orders off on demo.logger")
	require.Equal(t, 1, p.count("DELETE /api/functions/fn_1/trace/orders"))
}

func TestFnTraceErrorMapping(t *testing.T) {
	p := newFakeTracePlatform(t)
	for code, msg := range map[string]string{
		"TRACE_WINDOW_TOO_LONG":     "http can be on for at most 15 minutes",
		"TRACE_CONTEXT_UNKNOWN":     "demo.logger has no context nope",
		"TRACE_SAMPLE_RATE_INVALID": "sampleRate must be 1 to 1000",
		"TRACE_MINUTES_INVALID":     "minutes must be at least 1",
	} {
		p.status, p.putError = http.StatusBadRequest, fmt.Sprintf(`{"error":%q,"message":%q}`, code, msg)
		_, err := runTrace(t, p, "on", "demo.logger", "http")
		require.EqualError(t, err, msg, code)
	}

	p.status, p.putError = http.StatusForbidden, `{"error":"FORBIDDEN","message":"missing permission"}`
	for _, args := range [][]string{{"on", "demo.logger", "orders"}, {"off", "demo.logger", "orders"}, {"status", "demo.logger"}, {"log", "demo.logger"}} {
		_, err := runTrace(t, p, args...)
		require.ErrorContains(t, err, "this credential may not use trace logs", args[0])
	}

	p.status, p.putError = http.StatusNotFound, `{"error":"NOT_FOUND"}`
	_, err := runTrace(t, p, "off", "demo.logger", "nope")
	require.EqualError(t, err, `demo.logger has no trace context "nope"`)

	_, err = runTrace(t, p, "status", "demo.missing")
	require.EqualError(t, err, "no function demo.missing")
	_, err = runTrace(t, p, "on", "demo.missing", "orders")
	require.EqualError(t, err, "no function demo.missing")
}

func TestFnTraceStatus(t *testing.T) {
	useLocalUTC(t)
	old := fnTraceNow
	fnTraceNow = func() time.Time { return time.Date(2026, 10, 21, 12, 16, 35, 0, time.UTC) }
	t.Cleanup(func() { fnTraceNow = old })
	p := newFakeTracePlatform(t)
	out, err := runTrace(t, p, "status", "demo.logger")
	require.NoError(t, err, out)
	t.Log("\n" + out)
	require.Contains(t, out, "demo.logger (fn_1)")
	require.Regexp(t, `CONTEXT\s+BUILT-IN\s+STATE\s+UNTIL\s+SAMPLE\s+DESCRIPTION`, out)
	require.Regexp(t, `http\s+yes\s+live\s+12:20:45 \(4m10s left\)\s+1\s+Request and response`, out)
	require.Regexp(t, `orders\s+no\s+off\s+-\s+1\s+Order mapping\.`, out)
}

func TestFnTraceLogQueryAndOrder(t *testing.T) {
	useLocalUTC(t)
	p := newFakeTracePlatform(t)
	base := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		p.addRecord(fmt.Sprintf("r%d", i), base.Add(time.Duration(i)*time.Second), "trace", "info", fmt.Sprintf("step %d", i), map[string]any{"n": i})
	}
	out, err := runTrace(t, p, "log", "demo.logger", "--context", "orders", "--kind", "trace", "--level", "info",
		"--reference", "T-1", "--correlation", "c-9", "--invocation", "inv-r1", "--version", "3",
		"--since", "2026-10-21T11:00:00Z", "--until", "1792584000000", "--limit", "5")
	require.NoError(t, err, out)
	t.Log("\n" + out)

	// Three pages of two, newest first on the wire, oldest first on screen.
	require.Equal(t, 3, len(p.queries))
	q := p.queries[0]
	for k, want := range map[string]string{
		"context": "orders", "kind": "trace", "level": "info", "reference": "T-1", "correlationId": "c-9",
		"invocationId": "inv-r1", "version": "3", "since": "2026-10-21T11:00:00Z", "limit": "5",
	} {
		require.Equal(t, want, q.Get(k), k)
	}
	require.Equal(t, "2026-10-21T12:00:00Z", q.Get("until"))
	require.Equal(t, "", q.Get("cursor"))
	require.Equal(t, "2", p.queries[1].Get("cursor"))
	require.Equal(t, "3", p.queries[1].Get("limit"))
	i1, i5 := strings.Index(out, "step 1"), strings.Index(out, "step 5")
	require.True(t, i1 >= 0 && i5 > i1, "oldest first:\n"+out)
	require.Contains(t, out, `12:00:01.000 INFO  orders     inv=inv-r1 ref=T-1  step 1 {"n":1}`)

	// Durations count back from now; bad values are refused before any call.
	p.queries = nil
	_, err = runTrace(t, p, "log", "demo.logger", "--since", "10m")
	require.NoError(t, err)
	since, perr := time.Parse(time.RFC3339Nano, p.queries[0].Get("since"))
	require.NoError(t, perr)
	require.WithinDuration(t, time.Now().Add(-10*time.Minute), since, 5*time.Second)
	_, err = runTrace(t, p, "log", "demo.logger", "--since", "yesterday")
	require.ErrorContains(t, err, "--since")
	_, err = runTrace(t, p, "log", "demo.logger", "--kind", "nope")
	require.ErrorContains(t, err, "--kind")
}

func TestFnTraceLogHTTPRecords(t *testing.T) {
	useLocalUTC(t)
	p := newFakeTracePlatform(t)
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	p.addRecord("h1", at, "http", "info", "POST /orders", map[string]any{
		"request": map[string]any{
			"method": "POST", "path": "/orders", "rawQuery": "dry=1", "body": `{"trip":"T-1","lines":2}`,
			"bodyEncoding": "utf8", "bodyBytes": 25, "bodyTruncated": false,
		},
		"response": map[string]any{
			"status": 200, "body": "AAEC", "bodyEncoding": "base64", "bodyBytes": 70000, "bodyTruncated": true,
		},
		"durationMs": 12, "outcome": "ok",
	})

	out, err := runTrace(t, p, "log", "demo.logger")
	require.NoError(t, err, out)
	require.Contains(t, out, "POST /orders?dry=1 -> 200 (12ms, ok)")
	require.NotContains(t, out, "request body")

	out, err = runTrace(t, p, "log", "demo.logger", "--bodies")
	require.NoError(t, err, out)
	t.Log("\n" + out)
	require.Contains(t, out, "request body (25 bytes):")
	require.Contains(t, out, "\"trip\": \"T-1\"") // pretty JSON
	require.Contains(t, out, "response body (70000 bytes, base64, truncated):")
	require.Contains(t, out, "AAEC")

	out, err = runTrace(t, p, "log", "demo.logger", "--json")
	require.NoError(t, err, out)
	var lines []string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(l, "{") {
			lines = append(lines, l)
		}
	}
	require.Len(t, lines, 1)
	var rec map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &rec))
	require.Equal(t, "h1", rec["id"])

	// Redacted: no bodies, flagged.
	p.records[0]["bodiesRedacted"] = true
	data := p.records[0]["data"].(map[string]any)
	data["request"].(map[string]any)["body"] = nil
	data["response"].(map[string]any)["body"] = nil
	out, err = runTrace(t, p, "log", "demo.logger", "--bodies")
	require.NoError(t, err, out)
	require.Contains(t, out, "(bodies, headers and query string hidden: no view-bodies permission)")
	require.NotContains(t, out, "request body")
}

// TestFnTraceLogFollow: the backlog prints once, then only new records,
// each exactly once, with since advancing past the newest.
func TestFnTraceLogFollow(t *testing.T) {
	useLocalUTC(t)
	shortPoll(t)
	p := newFakeTracePlatform(t)
	now := time.Now()
	p.addRecord("a", now.Add(-3*time.Second), "trace", "info", "first", nil)
	p.addRecord("b", now.Add(-2*time.Second), "trace", "info", "second", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out lockedBuffer
	done := make(chan error, 1)
	go func() { done <- traceCmd(p, &out, "log", "demo.logger", "-f").run(ctx) }()

	require.Eventually(t, func() bool { return strings.Contains(out.String(), "second") }, 5*time.Second, 5*time.Millisecond)
	p.addRecord("c", now.Add(-time.Second), "trace", "info", "third", nil)
	require.Eventually(t, func() bool { return strings.Contains(out.String(), "third") }, 5*time.Second, 5*time.Millisecond)
	// A late record with an older timestamp (batched shipping) still appears.
	p.addRecord("d", now.Add(-2500*time.Millisecond), "trace", "info", "late", nil)
	require.Eventually(t, func() bool { return strings.Contains(out.String(), "late") }, 5*time.Second, 5*time.Millisecond)
	time.Sleep(60 * time.Millisecond) // several more polls
	cancel()
	require.NoError(t, <-done)

	s := out.String()
	t.Log("\n" + s)
	for _, m := range []string{"first", "second", "third", "late"} {
		require.Equal(t, 1, strings.Count(s, m), m)
	}
	// Each poll starts over without a cursor and asks from the newest record
	// minus the overlap; the first (backlog) query has no since at all.
	p.mu.Lock()
	defer p.mu.Unlock()
	require.Empty(t, p.queries[0].Get("since"))
	var firstPages []url.Values
	for _, q := range p.queries {
		if q.Get("cursor") == "" {
			firstPages = append(firstPages, q)
		}
	}
	require.Greater(t, len(firstPages), 2)
	since, err := time.Parse(time.RFC3339Nano, firstPages[len(firstPages)-1].Get("since"))
	require.NoError(t, err)
	require.WithinDuration(t, now.Add(-time.Second-fnTraceOverlap), since, time.Second)
	require.Equal(t, "500", firstPages[len(firstPages)-1].Get("limit"))
}

func TestFnTraceRunTurnsContextOffOnCancel(t *testing.T) {
	useLocalUTC(t)
	shortPoll(t)
	p := newFakeTracePlatform(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out lockedBuffer
	done := make(chan error, 1)
	go func() { done <- traceCmd(p, &out, "run", "demo.logger", "orders", "--sample", "2").run(ctx) }()

	require.Eventually(t, func() bool { return p.count("PUT") == 1 }, 5*time.Second, 5*time.Millisecond)
	// A record written after the run started is shown; an older one is not.
	p.addRecord("old", time.Now().Add(-time.Hour), "trace", "info", "ancient", nil)
	p.addRecord("new", time.Now().Add(time.Second), "trace", "info", "fresh", nil)
	require.Eventually(t, func() bool { return strings.Contains(out.String(), "fresh") }, 5*time.Second, 5*time.Millisecond)
	require.Zero(t, p.count("DELETE"), "still on while following")
	cancel()
	require.NoError(t, <-done)

	s := out.String()
	t.Log("\n" + s)
	require.NotContains(t, s, "ancient")
	require.EqualValues(t, 2, p.putBody["sampleRate"])
	require.Equal(t, 1, p.count("DELETE /api/functions/fn_1/trace/orders"))
	require.Contains(t, s, "orders off on demo.logger")
	p.mu.Lock()
	require.Equal(t, "orders", p.queries[0].Get("context"))
	p.mu.Unlock()
}

// A window that already covers the run (someone else's, or indefinite) is
// followed and never touched.
func TestFnTraceRunLeavesALongerWindowAlone(t *testing.T) {
	shortPoll(t)
	for name, st := range map[string]map[string]any{
		"indefinite": {"enabledUntil": time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339), "indefinite": true},
		"longer":     {"enabledUntil": time.Now().Add(50 * time.Minute).UTC().Format(time.RFC3339)},
	} {
		p := newFakeTracePlatform(t)
		maps.Copy(p.state["orders"], st)
		p.state["orders"]["live"] = true
		ctx, cancel := context.WithCancel(context.Background())
		var out lockedBuffer
		done := make(chan error, 1)
		go func() { done <- traceCmd(p, &out, "run", "demo.logger", "orders", "--for", "5m").run(ctx) }()
		require.Eventually(t, func() bool { return p.count("GET /api/functions/fn_1/trace-log") >= 2 }, 5*time.Second, 5*time.Millisecond)
		cancel()
		require.NoError(t, <-done)
		require.Zero(t, p.count("PUT"), name)
		require.Zero(t, p.count("DELETE"), name)
		require.Contains(t, out.String(), "orders is already on for demo.logger", name)
	}
}

// A shorter window that was on before the run is restored, not deleted.
func TestFnTraceRunRestoresAShorterWindow(t *testing.T) {
	shortPoll(t)
	p := newFakeTracePlatform(t)
	p.state["orders"]["live"] = true
	p.state["orders"]["sampleRate"] = 4
	p.state["orders"]["enabledUntil"] = time.Now().Add(3*time.Minute + 30*time.Second).UTC().Format(time.RFC3339)
	ctx, cancel := context.WithCancel(context.Background())
	var out lockedBuffer
	done := make(chan error, 1)
	go func() { done <- traceCmd(p, &out, "run", "demo.logger", "orders", "--for", "10m").run(ctx) }()
	require.Eventually(t, func() bool { return p.count("PUT") == 1 }, 5*time.Second, 5*time.Millisecond)
	cancel()
	require.NoError(t, <-done)
	require.Equal(t, 2, p.count("PUT"), "the run's window, then the restore")
	require.Zero(t, p.count("DELETE"))
	p.mu.Lock()
	require.EqualValues(t, 3, p.putBody["minutes"], "the time it had left")
	require.EqualValues(t, 4, p.putBody["sampleRate"])
	p.mu.Unlock()
	require.Contains(t, out.String(), "restored")

	// A window someone changed during the run is theirs.
	p = newFakeTracePlatform(t)
	ctx, cancel = context.WithCancel(context.Background())
	var out2 lockedBuffer
	done = make(chan error, 1)
	go func() { done <- traceCmd(p, &out2, "run", "demo.logger", "orders").run(ctx) }()
	require.Eventually(t, func() bool { return p.count("PUT") == 1 }, 5*time.Second, 5*time.Millisecond)
	p.mu.Lock()
	p.state["orders"]["enabledUntil"] = time.Now().Add(40 * time.Minute).UTC().Format(time.RFC3339)
	p.mu.Unlock()
	cancel()
	require.NoError(t, <-done)
	require.Zero(t, p.count("DELETE"))
	require.Contains(t, out2.String(), "was changed during the run")
}

// Ctrl-C while the switch is being made: the switch is on at the platform, so
// the run still ends cleanly and switches it back off.
func TestFnTraceRunStoppedDuringTheSwitchRestores(t *testing.T) {
	shortPoll(t)
	p := newFakeTracePlatform(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.onPut = cancel
	var out lockedBuffer
	done := make(chan error, 1)
	go func() { done <- traceCmd(p, &out, "run", "demo.logger", "orders").run(ctx) }()
	require.NoError(t, <-done)
	require.Equal(t, 1, p.count("PUT"))
	require.Equal(t, 1, p.count("DELETE"), "the context was off before the run")
}

func TestFnTraceRunKeepLeavesContextOn(t *testing.T) {
	shortPoll(t)
	p := newFakeTracePlatform(t)
	ctx, cancel := context.WithCancel(context.Background())
	var out lockedBuffer
	done := make(chan error, 1)
	go func() { done <- traceCmd(p, &out, "run", "demo.logger", "orders", "--keep").run(ctx) }()
	require.Eventually(t, func() bool { return p.count("GET /api/functions/fn_1/trace-log") >= 2 }, 5*time.Second, 5*time.Millisecond)
	cancel()
	require.NoError(t, <-done)
	require.Zero(t, p.count("DELETE"))
}
