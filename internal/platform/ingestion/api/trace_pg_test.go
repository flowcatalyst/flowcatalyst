//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

const (
	permTraceManage = "platform:ingestion:trace:manage"
	permTraceView   = "platform:ingestion:trace:view"
	permTraceBodies = "platform:ingestion:trace:view-bodies"
)

// traceAdmin holds the ingestion administrator's trace permissions.
func traceAdmin() []string {
	return []string{permView, permTraceManage, permTraceView, permTraceBodies}
}

func (e *env) traceRecord(ingestionID, windowID, capture, runtime, kind string, at time.Time, message, data string) ingestion.TraceLogRecord {
	rec := ingestion.TraceLogRecord{
		ID: "itr_" + unique() + unique(), IngestionID: ingestionID, Version: 1, WindowID: windowID, CaptureID: capture,
		RuntimeID: runtime, Kind: kind, Level: "info", Message: message, At: at, CreatedAt: time.Now().UTC(),
	}
	if data != "" {
		rec.Data = []byte(data)
	}
	return rec
}

func (e *env) storeTrace(recs ...ingestion.TraceLogRecord) {
	e.t.Helper()
	require.NoError(e.t, e.state.Repo.InsertTraceRecords(context.Background(), recs))
}

// Each trace route needs its permission, and the ingestion's reach: switching
// on needs manage and view-bodies, because capture records bodies.
func TestTrace_PermissionsAndReach(t *testing.T) {
	e := newEnv(t)
	id := e.create("trace-perm-" + unique())
	path := "/api/ingestions/" + id

	routes := []struct {
		name, method, path string
		body               any
		need               []string
	}{
		{"read", http.MethodGet, path + "/trace", nil, []string{permTraceView}},
		{"log", http.MethodGet, path + "/trace-log", nil, []string{permTraceView}},
		{"enable", http.MethodPut, path + "/trace", map[string]any{"windowSecs": 60}, []string{permTraceManage, permTraceBodies}},
		{"disable", http.MethodDelete, path + "/trace", nil, []string{permTraceManage}},
	}
	for _, rt := range routes {
		e.as(anchor())
		assert.Equal(t, http.StatusForbidden, e.do(rt.method, rt.path, rt.body).code, "%s: no permission", rt.name)
		for _, drop := range rt.need {
			e.as(anchor(without(rt.need, drop)...))
			r := e.do(rt.method, rt.path, rt.body)
			assert.Equal(t, http.StatusForbidden, r.code, "%s without %s: %s", rt.name, drop, r.raw)
			assert.Equal(t, "PERMISSION_REQUIRED", r.errCode(), rt.name)
		}
		e.as(anchor(rt.need...))
		r := e.do(rt.method, rt.path, rt.body)
		assert.Less(t, r.code, 300, "%s with %v: %s", rt.name, rt.need, r.raw)

		// Another client's operator does not reach the ingestion.
		e.as(clientTier([]string{string(e.clientB.ID)}, rt.need...))
		assert.Equal(t, http.StatusForbidden, e.do(rt.method, rt.path, rt.body).code, "%s: another client", rt.name)
	}
	e.as(anchor(traceAdmin()...))
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, "/api/ingestions/ing_nothing0000/trace", nil).code)
}

// Switching on opens a window with a new id every time, whose hard stop is the
// enable time plus the window plus the pickup grace; switching off sets the hard
// stop to now, and is idempotent. Each is audited by an event; off only for a
// window that was live.
func TestTrace_EnableDisable(t *testing.T) {
	e := newEnv(t)
	id := e.create("trace-switch-" + unique())
	e.as(anchor(traceAdmin()...))
	path := "/api/ingestions/" + id + "/trace"

	r := e.do(http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, false, r.json["live"])
	assert.Nil(t, r.json["windowId"], "never switched on: the fields are null")
	assert.Nil(t, r.json["hardStop"])
	assert.Equal(t, 900, r.num("maxWindowSecs"))

	// Defaults: a minute, a capture every 5 seconds.
	r = e.do(http.MethodPut, path, map[string]any{})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, true, r.json["live"])
	assert.Equal(t, 60, r.num("windowSecs"))
	assert.Equal(t, 5, r.num("sampleEverySecs"))
	first := r.str("windowId")
	assert.Regexp(t, `^itw_`, first)
	assert.Equal(t, "prn_ingtester0001", r.str("enabledBy"))

	r = e.do(http.MethodPut, path, map[string]any{"windowSecs": 300, "sampleEverySecs": 10})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.NotEqual(t, first, r.str("windowId"), "a new window id every time, so runtimes start a fresh capture")
	second := r.str("windowId")
	enabledAt, err := time.Parse(time.RFC3339Nano, r.str("enabledAt"))
	require.NoError(t, err)
	hardStop, err := time.Parse(time.RFC3339Nano, r.str("hardStop"))
	require.NoError(t, err)
	assert.Equal(t, 300*time.Second+ingestion.DefaultTracePickupGrace, hardStop.Sub(enabledAt), "enable time + window + pickup grace")
	assert.WithinDuration(t, time.Now(), enabledAt, 10*time.Second)

	// What GET reads is what PUT answered.
	got := e.do(http.MethodGet, path, nil)
	assert.Equal(t, second, got.str("windowId"))
	assert.Equal(t, true, got.json["live"])

	for _, bad := range []map[string]any{{"windowSecs": 61}, {"windowSecs": 1800}, {"windowSecs": -60}} {
		r = e.do(http.MethodPut, path, bad)
		assert.Equal(t, http.StatusBadRequest, r.code, "%v: %s", bad, r.raw)
		assert.Equal(t, "TRACE_WINDOW_INVALID", r.errCode(), "%v", bad)
	}
	for _, bad := range []map[string]any{{"sampleEverySecs": 61}, {"sampleEverySecs": -1}} {
		r = e.do(http.MethodPut, path, bad)
		assert.Equal(t, http.StatusBadRequest, r.code, "%v: %s", bad, r.raw)
		assert.Equal(t, "TRACE_SAMPLE_INVALID", r.errCode(), "%v", bad)
	}
	assert.Equal(t, second, e.do(http.MethodGet, path, nil).str("windowId"), "a refused enable changed nothing")

	before := time.Now()
	assert.Equal(t, http.StatusNoContent, e.do(http.MethodDelete, path, nil).code)
	r = e.do(http.MethodGet, path, nil)
	assert.Equal(t, false, r.json["live"])
	assert.Equal(t, second, r.str("windowId"), "the window stays, ended")
	stopped, err := time.Parse(time.RFC3339Nano, r.str("hardStop"))
	require.NoError(t, err)
	assert.WithinDuration(t, before, stopped, 10*time.Second, "switching off sets the hard stop to now")
	assert.Equal(t, http.StatusNoContent, e.do(http.MethodDelete, path, nil).code, "idempotent")
	assert.Equal(t, stopped.UnixMicro(), mustTime(t, e.do(http.MethodGet, path, nil).str("hardStop")).UnixMicro(), "again changes nothing")

	// A never-switched-on ingestion switches off without a row or an event.
	other := e.create("trace-never-" + unique())
	assert.Equal(t, http.StatusNoContent, e.do(http.MethodDelete, "/api/ingestions/"+other+"/trace", nil).code)
	assert.Equal(t, 0, e.traceEvents(other, "disabled"))

	assert.Equal(t, 2, e.traceEvents(id, "enabled"))
	assert.Equal(t, 1, e.traceEvents(id, "disabled"), "off is audited only when a live window ended")
	var data map[string]any
	require.NoError(t, json.Unmarshal([]byte(e.eventData(id, "enabled")), &data))
	assert.Equal(t, second, data["windowId"])
	assert.EqualValues(t, 300, data["windowSecs"])
	assert.EqualValues(t, 10, data["sampleEverySecs"])
	assert.NotEmpty(t, data["hardStop"])
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339Nano, s)
	require.NoError(t, err)
	return v
}

func (e *env) traceEvents(id, which string) int {
	return e.count(`SELECT count(*) FROM msg_events WHERE subject = $1 AND type = $2`, "platform.ingestion."+id, "platform:ingestion:trace:"+which)
}

// eventData is the data of the newest event of a trace type.
func (e *env) eventData(id, which string) string {
	e.t.Helper()
	var data string
	require.NoError(e.t, e.pool.QueryRow(context.Background(),
		`SELECT data::text FROM msg_events WHERE subject = $1 AND type = $2 ORDER BY time DESC LIMIT 1`,
		"platform.ingestion."+id, "platform:ingestion:trace:"+which).Scan(&data))
	return data
}

// The trace log is read newest first with filters and a keyset cursor, and a
// reader without view-bodies gets each record with the sensitive parts withheld.
func TestTraceLog_FiltersCursorAndWithholding(t *testing.T) {
	e := newEnv(t)
	id := e.create("trace-log-" + unique())
	base := time.Now().UTC().Truncate(time.Millisecond)
	httpData := `{"client":"mix","request":{"method":"POST","url":"https://api.example.com/token?code=SECRET-QUERY","headers":{"Authorization":["[redacted]"]},"body":"grant_type=password&password=[redacted]","bodyEncoding":"utf8","bodyBytes":42,"bodyTruncated":false},` +
		`"response":{"status":200,"headers":{"X-Trace":["h"]},"body":"{\"access_token\":\"[redacted]\"}","bodyEncoding":"utf8","bodyBytes":30,"bodyTruncated":false},"durationMs":85,"attempt":1}`
	exchData := `{"source":{"fileName":"SECRET-FILE.csv"},"payloadBytes":120,"payload":"SECRET-PAYLOAD","records":3,"jobs":3,"outcome":"completed","error":"SECRET-ERR"}`
	pollData := `{"startedAt":"2026-10-30T10:00:00Z","durationMs":90,"httpCalls":1,"payloads":1,"records":3,"jobs":3,"outcome":"failed","error":"SECRET-POLL-ERR"}`
	e.storeTrace(
		e.traceRecord(id, "itw_1", "cap-a", "rt-1", "http", base.Add(-4*time.Second), "POST https://api.example.com/token?code=SECRET-QUERY -> 200", httpData),
		e.traceRecord(id, "itw_1", "cap-a", "rt-1", "exchange", base.Add(-3*time.Second), "file SECRET-FILE.csv", exchData),
		e.traceRecord(id, "itw_1", "cap-a", "rt-1", "poll", base.Add(-2*time.Second), "poll: 1 call, 1 payload", pollData),
		e.traceRecord(id, "itw_1", "cap-b", "rt-2", "error", base.Add(-time.Second), "boom SECRET-ERR-MSG", `{"error":"SECRET-ERR","kind":"x"}`),
	)

	e.as(anchor(traceAdmin()...))
	log := func(query string) response {
		r := e.do(http.MethodGet, "/api/ingestions/"+id+"/trace-log"+query, nil)
		require.Equal(t, http.StatusOK, r.code, r.raw)
		return r
	}
	kinds := func(r response) []string {
		var out []string
		for _, rec := range recordsOf(r) {
			out = append(out, rec["kind"].(string))
		}
		return out
	}
	assert.Equal(t, []string{"error", "poll", "exchange", "http"}, kinds(log("")), "newest first")
	assert.Equal(t, []string{"http"}, kinds(log("?kind=http")))
	assert.Equal(t, []string{"error"}, kinds(log("?runtimeId=rt-2")))
	assert.Equal(t, []string{"poll", "exchange", "http"}, kinds(log("?captureId=cap-a")))
	assert.Equal(t, []string{"poll", "exchange"}, kinds(log("?since="+url.QueryEscape(base.Add(-3500*time.Millisecond).Format(time.RFC3339Nano))+"&until="+url.QueryEscape(base.Add(-1500*time.Millisecond).Format(time.RFC3339Nano)))))
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodGet, "/api/ingestions/"+id+"/trace-log?since=yesterday", nil).code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodGet, "/api/ingestions/"+id+"/trace-log?cursor=junk", nil).code)

	// Keyset paging: two a page, no record twice or missed.
	page1 := log("?limit=2")
	assert.Equal(t, []string{"error", "poll"}, kinds(page1))
	next, _ := page1.json["nextCursor"].(string)
	require.NotEmpty(t, next)
	page2 := log("?limit=2&cursor=" + url.QueryEscape(next))
	assert.Equal(t, []string{"exchange", "http"}, kinds(page2))
	assert.Nil(t, page2.json["nextCursor"], "the last page has no cursor")

	// With bodies: everything the runtime sent.
	full := recordsOf(log("?kind=http"))[0]
	assert.Contains(t, string(mustJSON(t, full["data"])), "grant_type=password")
	assert.NotEqual(t, true, full["redacted"])

	// Without view-bodies: metadata only, marked redacted.
	e.as(anchor(permView, permTraceView))
	r := log("")
	raw := r.raw
	for _, secret := range []string{"SECRET-QUERY", "SECRET-FILE", "SECRET-PAYLOAD", "SECRET-ERR", "SECRET-POLL-ERR", "Authorization", "grant_type", "access_token", "X-Trace"} {
		assert.NotContains(t, raw, secret, "withheld from a reader without view-bodies")
	}
	byKind := map[string]map[string]any{}
	for _, rec := range recordsOf(r) {
		byKind[rec["kind"].(string)] = rec
		assert.Equal(t, true, rec["redacted"] == true || rec["kind"] == "poll", "%v", rec["kind"])
	}
	h := byKind["http"]["data"].(map[string]any)
	assert.Equal(t, "mix", h["client"])
	assert.Equal(t, "/token", h["request"].(map[string]any)["url"], "the URL path only")
	assert.Equal(t, "POST", h["request"].(map[string]any)["method"])
	assert.EqualValues(t, 200, h["response"].(map[string]any)["status"])
	assert.EqualValues(t, 85, h["durationMs"])
	assert.Equal(t, "POST /token -> 200", byKind["http"]["message"], "rebuilt from what stays, without the query")
	ex := byKind["exchange"]["data"].(map[string]any)
	assert.Equal(t, "completed", ex["outcome"])
	assert.EqualValues(t, 3, ex["records"])
	assert.NotContains(t, ex, "payload")
	poll := byKind["poll"]["data"].(map[string]any)
	assert.Equal(t, "failed", poll["outcome"])
	assert.EqualValues(t, 90, poll["durationMs"])
	assert.NotContains(t, poll, "error", "a poll keeps everything but its error")
	assert.Equal(t, true, byKind["poll"]["redacted"])
	assert.Nil(t, byKind["error"]["data"], "an error record keeps nothing in data")
}

func recordsOf(r response) []map[string]any {
	var out []map[string]any
	arr, _ := r.json["records"].([]any)
	for _, x := range arr {
		m, _ := x.(map[string]any)
		out = append(out, m)
	}
	return out
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// Deleting an ingestion deletes its trace window and its records with it.
func TestTrace_DeleteIngestionRemovesWindowAndRecords(t *testing.T) {
	e := newEnv(t)
	id := e.create("trace-del-" + unique())
	keep := e.create("trace-keep-" + unique())
	e.as(anchor(append(traceAdmin(), permDelete)...))
	for _, i := range []string{id, keep} {
		require.Equal(t, http.StatusOK, e.do(http.MethodPut, "/api/ingestions/"+i+"/trace", map[string]any{}).code)
		e.storeTrace(e.traceRecord(i, "itw_x", "cap", "rt", "poll", time.Now().UTC(), "poll", ""))
	}
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_trace_windows WHERE ingestion_id = $1`, id))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_trace_log WHERE ingestion_id = $1`, id))

	require.Equal(t, http.StatusNoContent, e.do(http.MethodDelete, "/api/ingestions/"+id, nil).code)
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_trace_windows WHERE ingestion_id = $1`, id))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_trace_log WHERE ingestion_id = $1`, id))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_trace_windows WHERE ingestion_id = $1`, keep), "another ingestion's trace stays")
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_trace_log WHERE ingestion_id = $1`, keep))
}
