package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ingTracePlatform is a fake platform for `fcdev ingest trace`: one
// ingestion (ing_1), its window and its trace records.
type ingTracePlatform struct {
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []string
	// putBody is the last PUT /trace body; queries are the trace-log queries.
	putBody map[string]any
	queries []url.Values
	// state is the window GET and PUT answer with; status is the HTTP status
	// of the next trace call (zero is fine) and problem its body.
	state   string
	status  int
	problem string
	records []map[string]any
}

const ingTraceRow = `{"id":"ing_1","applicationCode":"ingest","clientId":"clt_1","clientIdentifier":"acme","code":"orders-sftp","name":"Orders","status":"ACTIVE","version":2,"latestVersion":3,"source":{"kind":"api-poll"},"split":{"kind":"json"},"target":{"code":"c","targetUrl":"https://h.example.com","pool":"orders","connection":"conn","mode":"IMMEDIATE","batchSize":1}}`

func newIngTracePlatform(t *testing.T) *ingTracePlatform {
	p := &ingTracePlatform{state: `{"live":false,"windowId":null,"windowSecs":null,"sampleEverySecs":null,"enabledAt":null,"hardStop":null,"enabledBy":null,"maxWindowSecs":900}`}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		key := r.Method + " " + r.URL.Path
		p.reqs = append(p.reqs, key)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/ingestions/ing_1/trace") && p.status != 0 {
			w.WriteHeader(p.status)
			_, _ = w.Write([]byte(p.problem))
			return
		}
		switch key {
		case "POST /oauth/token":
			_, _ = fmt.Fprintf(w, `{"access_token":%q,"expires_in":300,"token_type":"Bearer"}`, jwt(`{"sub":"prn_T","tier":"ANCHOR"}`))
		case "GET /api/ingestions":
			_, _ = fmt.Fprintf(w, `{"ingestions":[%s],"hasMore":false}`, ingTraceRow)
		case "GET /api/ingestions/ing_1/trace":
			_, _ = w.Write([]byte(p.state))
		case "PUT /api/ingestions/ing_1/trace":
			body, _ := io.ReadAll(r.Body)
			p.putBody = map[string]any{}
			_ = json.Unmarshal(body, &p.putBody)
			_, _ = w.Write([]byte(`{"live":true,"windowId":"itw_9","windowSecs":300,"sampleEverySecs":10,"enabledAt":"2026-10-30T10:00:00Z","hardStop":"2026-10-30T10:07:00Z","enabledBy":"prn_T","maxWindowSecs":900}`))
		case "DELETE /api/ingestions/ing_1/trace":
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/ingestions/ing_1/trace-log":
			p.queries = append(p.queries, r.URL.Query())
			since, _ := time.Parse(time.RFC3339Nano, r.URL.Query().Get("since"))
			var out []map[string]any
			for _, rec := range p.records { // newest first
				if at, _ := time.Parse(time.RFC3339Nano, rec["at"].(string)); !since.IsZero() && at.Before(since) {
					continue
				}
				if k := r.URL.Query().Get("kind"); k != "" && rec["kind"] != k {
					continue
				}
				out = append(out, rec)
			}
			b, _ := json.Marshal(map[string]any{"records": out, "nextCursor": nil})
			_, _ = w.Write(b)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"NOT_FOUND","message":"no such route"}`))
		}
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *ingTracePlatform) flags() []string {
	return []string{"--platform", p.srv.URL, "--client-id", "c", "--client-secret", "s"}
}

func (p *ingTracePlatform) count(prefix string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, r := range p.reqs {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}

// add puts a record at the front (newest first).
func (p *ingTracePlatform) add(id string, at time.Time, kind, level, message string, data any, extra map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	rec := map[string]any{
		"id": id, "at": at.UTC().Format(time.RFC3339Nano), "version": 2, "windowId": "itw_9", "captureId": "cap-" + id,
		"runtimeId": "rt-1", "kind": kind, "level": level, "message": message, "data": data,
	}
	maps.Copy(rec, extra)
	p.records = append([]map[string]any{rec}, p.records...)
}

const ingRef = "ingest/acme/orders-sftp"

func ingTraceHTTPData(redacted bool) map[string]any {
	d := map[string]any{
		"client": "mix",
		"request": map[string]any{
			"method": "POST", "url": "https://api.example.com/token", "bodyEncoding": "utf8", "bodyBytes": 40, "bodyTruncated": false,
			"body": `{"grant_type":"password","password":"[redacted]"}`,
		},
		"response": map[string]any{
			"status": 200, "bodyEncoding": "utf8", "bodyBytes": 30, "bodyTruncated": false, "body": `{"access_token":"[redacted]"}`,
		},
		"durationMs": 85, "attempt": 2,
	}
	if redacted {
		d["request"].(map[string]any)["body"] = nil
		d["response"].(map[string]any)["body"] = nil
	}
	return d
}

func TestIngestTraceOn(t *testing.T) {
	newFnTestEnv(t)
	useLocalUTC(t)
	p := newIngTracePlatform(t)

	out, _, err := runCobra(t, newIngestTraceCmd(), "", append([]string{"on", ingRef, "--for", "5m", "--every", "10s"}, p.flags()...)...)
	require.NoError(t, err, out)
	require.Equal(t, map[string]any{"windowSecs": float64(300), "sampleEverySecs": float64(10)}, p.putBody)
	require.Contains(t, out, "tracing on for ingest/acme/orders-sftp: 5m0s per runtime, one capture every 10s")
	require.Contains(t, out, "no runtime captures past 10:07:00")
	require.Contains(t, out, "each runtime starts when it next syncs")

	// Defaults: a minute, every five seconds.
	_, _, err = runCobra(t, newIngestTraceCmd(), "", append([]string{"on", ingRef}, p.flags()...)...)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"windowSecs": float64(60), "sampleEverySecs": float64(5)}, p.putBody)
}

func TestIngestTraceOnValidatesBeforeCalling(t *testing.T) {
	newFnTestEnv(t)
	p := newIngTracePlatform(t)
	for flag, want := range map[string][]string{
		"--for 2m":       {"--for", "2m"},
		"--for 30s":      {"--for", "30s"},
		"--for 1h":       {"--for", "1h"},
		"--every 0s":     {"--every", "0s"},
		"--every 61s":    {"--every", "61s"},
		"--every 1500ms": {"--every", "1500ms"},
	} {
		_, _, err := runCobra(t, newIngestTraceCmd(), "", append(append([]string{"on", ingRef}, want...), p.flags()...)...)
		require.Error(t, err, flag)
		require.ErrorContains(t, err, want[0], flag)
	}
	require.Zero(t, p.count("PUT"))
	require.Zero(t, p.count("GET"), "refused before it called the platform")
}

func TestIngestTraceErrorsAreReadable(t *testing.T) {
	newFnTestEnv(t)
	p := newIngTracePlatform(t)
	p.status, p.problem = http.StatusForbidden, `{"error":"PERMISSION_REQUIRED","message":"permission required: platform:ingestion:trace:manage"}`
	_, _, err := runCobra(t, newIngestTraceCmd(), "", append([]string{"on", ingRef}, p.flags()...)...)
	require.ErrorContains(t, err, "platform:ingestion:trace:view, :manage and :view-bodies")

	p.status, p.problem = http.StatusBadRequest, `{"error":"TRACE_WINDOW_INVALID","message":"windowSecs must be 60, 300 or 900"}`
	_, _, err = runCobra(t, newIngestTraceCmd(), "", append([]string{"on", ingRef}, p.flags()...)...)
	require.EqualError(t, err, "windowSecs must be 60, 300 or 900")

	_, _, err = runCobra(t, newIngestTraceCmd(), "", append([]string{"on", "ingest/acme/nothing"}, p.flags()...)...)
	require.ErrorContains(t, err, "no ingestion ingest/acme/nothing")
	_, _, err = runCobra(t, newIngestTraceCmd(), "", append([]string{"status", "orders-sftp"}, p.flags()...)...)
	require.ErrorContains(t, err, "<application>/<client>/<code>")
}

func TestIngestTraceOff(t *testing.T) {
	newFnTestEnv(t)
	p := newIngTracePlatform(t)
	out, _, err := runCobra(t, newIngestTraceCmd(), "", append([]string{"off", ingRef}, p.flags()...)...)
	require.NoError(t, err)
	require.Equal(t, "tracing off for ingest/acme/orders-sftp\n", out)
	require.Equal(t, 1, p.count("DELETE /api/ingestions/ing_1/trace"))
}

func TestIngestTraceOnAndOffAreGuardedOnProd(t *testing.T) {
	e := newFnTestEnv(t)
	p := newIngTracePlatform(t)
	profileWith(t, e, &profilePlatform{srv: p.srv}, "prod")
	for _, args := range [][]string{{"on", ingRef}, {"off", ingRef}} {
		_, _, err := runCobra(t, newIngestTraceCmd(), "", append(args, "--profile", "p")...)
		require.ErrorContains(t, err, "without a terminal", args[0])
	}
	require.Zero(t, p.count("PUT")+p.count("DELETE"), "nothing was switched")
	// Reading asks nothing.
	_, _, err := runCobra(t, newIngestTraceCmd(), "", "status", ingRef, "--profile", "p")
	require.NoError(t, err)
}

func TestIngestTraceStatus(t *testing.T) {
	newFnTestEnv(t)
	useLocalUTC(t)
	p := newIngTracePlatform(t)
	out, _, err := runCobra(t, newIngestTraceCmd(), "", append([]string{"status", ingRef}, p.flags()...)...)
	require.NoError(t, err)
	require.Contains(t, out, "ingest/acme/orders-sftp (ing_1)")
	require.Contains(t, out, "tracing  off (never switched on)")

	old := fnTraceNow
	fnTraceNow = func() time.Time { return time.Date(2026, 10, 30, 10, 5, 0, 0, time.UTC) }
	t.Cleanup(func() { fnTraceNow = old })
	p.state = `{"live":true,"windowId":"itw_9","windowSecs":300,"sampleEverySecs":10,"enabledAt":"2026-10-30T10:00:00Z","hardStop":"2026-10-30T10:07:00Z","enabledBy":"prn_T","maxWindowSecs":900}`
	out, _, err = runCobra(t, newIngestTraceCmd(), "", append([]string{"status", ingRef}, p.flags()...)...)
	require.NoError(t, err)
	require.Contains(t, out, "tracing  live")
	require.Contains(t, out, "itw_9, 300s per runtime, one capture every 10s")
	require.Contains(t, out, "stops    10:07:00 (2m0s left)")
	require.Contains(t, out, "by       prn_T")

	p.state = strings.Replace(p.state, `"live":true`, `"live":false`, 1)
	out, _, err = runCobra(t, newIngestTraceCmd(), "", append([]string{"status", ingRef}, p.flags()...)...)
	require.NoError(t, err)
	require.Contains(t, out, "tracing  off")
	require.NotContains(t, out, "left")
}

func TestIngestTraceLog(t *testing.T) {
	newFnTestEnv(t)
	useLocalUTC(t)
	p := newIngTracePlatform(t)
	base := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	p.add("a", base, "http", "info", "POST https://api.example.com/token -> 200", ingTraceHTTPData(false), nil)
	p.add("b", base.Add(time.Second), "poll", "info", "poll: 1 call, 0 payloads", map[string]any{"outcome": "ok"}, nil)
	p.add("c", base.Add(2*time.Second), "error", "error", "the source refused", nil, nil)

	out, _, err := runCobra(t, newIngestTraceCmd(), "", append([]string{"log", ingRef}, p.flags()...)...)
	require.NoError(t, err, out)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 3, out)
	require.Equal(t, "10:00:00.000 INFO  http     rt-1 cap=cap-a  POST https://api.example.com/token -> 200 (85ms, attempt 2)", lines[0], "oldest first, an http call in one line")
	require.Equal(t, "10:00:01.000 INFO  poll     rt-1 cap=cap-b  poll: 1 call, 0 payloads", lines[1])
	require.Equal(t, "10:00:02.000 ERROR error    rt-1 cap=cap-c  the source refused", lines[2])
	require.NotContains(t, out, "grant_type", "no bodies unless asked")

	// --bodies prints the request and response.
	out, _, err = runCobra(t, newIngestTraceCmd(), "", append([]string{"log", ingRef, "--bodies", "--kind", "http"}, p.flags()...)...)
	require.NoError(t, err)
	require.Contains(t, out, "request body (40 bytes):")
	require.Contains(t, out, `"grant_type": "password"`)
	require.Contains(t, out, `"password": "[redacted]"`)
	require.Contains(t, out, "response body (30 bytes):")
	require.NotContains(t, out, "the source refused")

	// A reader without view-bodies sees the call's metadata and a notice.
	p.mu.Lock()
	p.records = nil
	p.mu.Unlock()
	p.add("d", time.Now(), "http", "info", "POST /token -> 200", ingTraceHTTPData(true), map[string]any{"redacted": true})
	out, _, err = runCobra(t, newIngestTraceCmd(), "", append([]string{"log", ingRef, "--bodies"}, p.flags()...)...)
	require.NoError(t, err)
	require.Contains(t, out, "(headers, query string and bodies hidden: no view-bodies permission)")
	require.NotContains(t, out, "request body")

	// --json prints the raw records, one a line; the filters ride in the query.
	out, _, err = runCobra(t, newIngestTraceCmd(), "", append([]string{"log", ingRef, "--json", "--runtime", "rt-1", "--capture", "cap-d", "--since", "1h", "--limit", "7"}, p.flags()...)...)
	require.NoError(t, err)
	var rec map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &rec))
	require.Equal(t, "d", rec["id"])
	p.mu.Lock()
	q := p.queries[len(p.queries)-1]
	p.mu.Unlock()
	require.Equal(t, "rt-1", q.Get("runtimeId"))
	require.Equal(t, "cap-d", q.Get("captureId"))
	require.Equal(t, "7", q.Get("limit"))
	since, err := time.Parse(time.RFC3339Nano, q.Get("since"))
	require.NoError(t, err)
	require.WithinDuration(t, time.Now().Add(-time.Hour), since, 5*time.Second)

	_, _, err = runCobra(t, newIngestTraceCmd(), "", append([]string{"log", ingRef, "--kind", "weird"}, p.flags()...)...)
	require.ErrorContains(t, err, "--kind")
	_, _, err = runCobra(t, newIngestTraceCmd(), "", append([]string{"log", ingRef, "--limit", "0"}, p.flags()...)...)
	require.ErrorContains(t, err, "--limit")
}

// -f prints the backlog once, then only the records that arrive, each once,
// including a late one with an older timestamp (runtimes ship in batches).
func TestIngestTraceLogFollow(t *testing.T) {
	newFnTestEnv(t)
	useLocalUTC(t)
	shortPoll(t)
	p := newIngTracePlatform(t)
	now := time.Now()
	p.add("a", now.Add(-3*time.Second), "poll", "info", "first", nil, nil)
	p.add("b", now.Add(-2*time.Second), "poll", "info", "second", nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out lockedBuffer
	cmd := newIngestTraceCmd()
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"log", ingRef, "-f"}, p.flags()...))
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx) }()

	require.Eventually(t, func() bool { return strings.Contains(out.String(), "second") }, 5*time.Second, 5*time.Millisecond)
	p.add("c", now.Add(-time.Second), "poll", "info", "third", nil, nil)
	require.Eventually(t, func() bool { return strings.Contains(out.String(), "third") }, 5*time.Second, 5*time.Millisecond)
	p.add("d", now.Add(-2500*time.Millisecond), "poll", "info", "late", nil, nil)
	require.Eventually(t, func() bool { return strings.Contains(out.String(), "late") }, 5*time.Second, 5*time.Millisecond)
	time.Sleep(60 * time.Millisecond) // several more polls
	cancel()
	require.NoError(t, <-done)

	s := out.String()
	for _, m := range []string{"first", "second", "third", "late"} {
		require.Equal(t, 1, strings.Count(s, m), m)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	require.Empty(t, p.queries[0].Get("since"), "the backlog is not bounded by time")
	require.Greater(t, len(p.queries), 3)
	since, err := time.Parse(time.RFC3339Nano, p.queries[len(p.queries)-1].Get("since"))
	require.NoError(t, err)
	require.WithinDuration(t, now.Add(-time.Second-fnTraceOverlap), since, time.Second)
}
