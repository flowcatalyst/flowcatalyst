package ingestion

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The retention and the pickup grace are configurable and never non-positive.
func TestTraceRetentionAndGraceSettings(t *testing.T) {
	t.Setenv("FC_INGESTION_TRACE_RETENTION_DAYS", "")
	assert.Equal(t, 7*24*time.Hour, TraceRetention())
	t.Setenv("FC_INGESTION_TRACE_RETENTION_DAYS", "2")
	assert.Equal(t, 48*time.Hour, TraceRetention())
	t.Setenv("FC_INGESTION_TRACE_RETENTION_DAYS", "0")
	assert.Equal(t, 7*24*time.Hour, TraceRetention())

	t.Setenv("FC_INGESTION_TRACE_PICKUP_GRACE", "")
	assert.Equal(t, 2*time.Minute, TracePickupGrace())
	t.Setenv("FC_INGESTION_TRACE_PICKUP_GRACE", "90s")
	assert.Equal(t, 90*time.Second, TracePickupGrace())
	t.Setenv("FC_INGESTION_TRACE_PICKUP_GRACE", "soon")
	assert.Equal(t, 2*time.Minute, TracePickupGrace())
	t.Setenv("FC_INGESTION_TRACE_PICKUP_GRACE", "-1m")
	assert.Equal(t, 2*time.Minute, TracePickupGrace())
}

func TestValidTraceWindowSecs(t *testing.T) {
	for _, n := range []int{60, 300, 900} {
		assert.True(t, ValidTraceWindowSecs(n), n)
	}
	for _, n := range []int{0, 1, 59, 61, 120, 600, 901, 1800, -60} {
		assert.False(t, ValidTraceWindowSecs(n), n)
	}
}

func TestTraceWindowLiveIsComparedWhenRead(t *testing.T) {
	now := time.Now()
	assert.True(t, TraceWindow{HardStop: now.Add(time.Second)}.Live(now))
	assert.False(t, TraceWindow{HardStop: now}.Live(now), "the hard stop itself is no longer live")
	assert.False(t, TraceWindow{HardStop: now.Add(-time.Hour)}.Live(now))
}

func data(t *testing.T, r TraceLogRecord) map[string]any {
	t.Helper()
	if r.Data == nil {
		return nil
	}
	var m map[string]any
	require.NoError(t, json.Unmarshal(r.Data, &m))
	return m
}

func TestWithholdTraceDetail_Http(t *testing.T) {
	r := TraceLogRecord{Kind: TraceKindHTTP, Message: "GET https://u:p@api.example.com/trips?token=abc -> 200", Data: []byte(`{
		"client":"mix",
		"request":{"method":"GET","url":"https://u:p@api.example.com/trips?token=abc","headers":{"X":["y"]},"body":"SECRET","bodyEncoding":"utf8","bodyBytes":6,"bodyTruncated":true},
		"response":{"status":200,"headers":{"Set-Cookie":["s"]},"body":"SECRET-RESPONSE","bodyEncoding":"utf8","bodyBytes":15,"bodyTruncated":false},
		"durationMs":85,"attempt":2,"failure":null}`)}
	require.True(t, WithholdTraceDetail(&r))
	assert.Equal(t, "GET /trips -> 200", r.Message, "rebuilt: the runtime's line can carry the query")
	d := data(t, r)
	assert.Equal(t, "mix", d["client"])
	req := d["request"].(map[string]any)
	assert.Equal(t, map[string]any{"method": "GET", "url": "/trips", "bodyBytes": float64(6), "bodyTruncated": true}, req)
	assert.Equal(t, map[string]any{"status": float64(200), "bodyBytes": float64(15), "bodyTruncated": false}, d["response"])
	assert.EqualValues(t, 85, d["durationMs"])
	assert.EqualValues(t, 2, d["attempt"])
	assert.NotContains(t, string(r.Data), "SECRET")
	assert.NotContains(t, string(r.Data), "token=abc")
	assert.NotContains(t, string(r.Data), "api.example.com")

	// A call that got no response keeps its failure.
	r = TraceLogRecord{Kind: TraceKindHTTP, Message: "x", Data: []byte(`{"request":{"method":"POST","url":"/p?q=1"},"response":null,"durationMs":3,"attempt":1,"failure":"TIMEOUT"}`)}
	require.True(t, WithholdTraceDetail(&r))
	d = data(t, r)
	assert.Nil(t, d["response"])
	assert.Equal(t, "TIMEOUT", d["failure"])
	assert.Equal(t, "POST /p", r.Message)

	// Data that is not the expected shape is dropped, not passed through.
	r = TraceLogRecord{Kind: TraceKindHTTP, Message: "m", Data: []byte(`"SECRET"`)}
	require.True(t, WithholdTraceDetail(&r))
	assert.Nil(t, r.Data)
}

func TestWithholdTraceDetail_ExchangePollError(t *testing.T) {
	r := TraceLogRecord{Kind: TraceKindExchange, Message: "file SECRET.csv", Data: []byte(`{"source":{"file":"SECRET.csv"},"identity":"i","payloadBytes":120,"payload":"SECRET","records":3,"jobs":2,"outcome":"failed","error":"SECRET"}`)}
	require.True(t, WithholdTraceDetail(&r))
	assert.Equal(t, map[string]any{"payloadBytes": float64(120), "records": float64(3), "jobs": float64(2), "outcome": "failed"}, data(t, r))
	assert.Equal(t, "exchange: failed", r.Message)

	r = TraceLogRecord{Kind: TraceKindPoll, Message: "poll: 1 call, 0 payloads", Data: []byte(`{"durationMs":9,"httpCalls":1,"payloads":0,"outcome":"failed","error":"SECRET"}`)}
	require.True(t, WithholdTraceDetail(&r), "an error was withheld")
	assert.Equal(t, map[string]any{"durationMs": float64(9), "httpCalls": float64(1), "payloads": float64(0), "outcome": "failed"}, data(t, r))
	assert.Equal(t, "poll: 1 call, 0 payloads", r.Message, "a poll keeps everything but its error")

	r = TraceLogRecord{Kind: TraceKindPoll, Message: "poll", Data: []byte(`{"outcome":"ok"}`)}
	assert.False(t, WithholdTraceDetail(&r), "nothing to withhold from a poll without an error")

	r = TraceLogRecord{Kind: TraceKindError, Message: "boom SECRET", Data: []byte(`{"error":"SECRET"}`)}
	require.True(t, WithholdTraceDetail(&r))
	assert.Nil(t, r.Data, "an error record keeps nothing in data")
	assert.Empty(t, r.Message)
}
