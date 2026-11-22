package runtimeapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

func TestTraceRecordAt(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	at, ok := traceRecordAt(now.Add(-time.Minute).UnixMilli(), now)
	assert.True(t, ok)
	assert.True(t, at.Equal(now.Add(-time.Minute)))

	at, ok = traceRecordAt(now.Add(59*time.Second).UnixMilli(), now)
	assert.True(t, ok)
	assert.True(t, at.Equal(now.Add(59*time.Second)), "within the minute it stays")

	at, ok = traceRecordAt(now.Add(5*time.Minute).UnixMilli(), now)
	assert.True(t, ok)
	assert.True(t, at.Equal(now), "more than a minute ahead is clamped to arrival")

	_, ok = traceRecordAt(now.Add(-59*time.Minute).UnixMilli(), now)
	assert.True(t, ok)
	_, ok = traceRecordAt(now.Add(-61*time.Minute).UnixMilli(), now)
	assert.False(t, ok, "more than an hour behind is dropped")
}

func TestTraceRecordID(t *testing.T) {
	assert.Equal(t, "itr_8d3c1f2e-aaaa-bbbb-cccc-0123456789ab", traceRecordID("8d3c1f2e-aaaa-bbbb-cccc-0123456789ab"), "stored as itr_<uuid>")
	for _, bad := range []string{"", "short", strings.Repeat("a", 65), "has space in it 123456", "semi;colon;colon;1234"} {
		got := traceRecordID(bad)
		assert.Regexp(t, `^itr_`, got, bad)
		assert.NotContains(t, got, bad+"x", bad)
		assert.NotEqual(t, got, traceRecordID(bad), "a fresh platform id each time")
	}
}

func TestTraceRateLimiter_FiftyASecondPerIngestion(t *testing.T) {
	var l traceRateLimiter
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, 50, l.take("ing_a", 120, now), "the burst is a second's worth")
	assert.Equal(t, 0, l.take("ing_a", 10, now))
	assert.Equal(t, 50, l.take("ing_b", 50, now), "another ingestion has its own bucket")
	assert.Equal(t, 25, l.take("ing_a", 100, now.Add(500*time.Millisecond)), "refills at 50 a second")
	assert.Equal(t, 50, l.take("ing_a", 100, now.Add(time.Hour)), "never more than a second's worth")
}

func TestNewTraceRecord_Truncations(t *testing.T) {
	now := time.Now().UTC()
	rec := newTraceRecord("rt\x00-1", IngestionTraceRecord{
		ID: "8d3c1f2e-aaaa-bbbb-cccc-0123456789ab", IngestionID: "ing_1", Version: -4, WindowID: "itw_1", CaptureID: strings.Repeat("c", 300),
		Kind: "poll", Level: "loud", Message: strings.Repeat("é", 1200) + "\x00", Data: json.RawMessage(`{"a":"b\u0000"}`),
	}, now, now)
	assert.Equal(t, "rt-1", rec.RuntimeID, "NUL stripped")
	assert.EqualValues(t, 0, rec.Version)
	assert.Len(t, rec.CaptureID, ingestion.MaxTraceIDLength)
	assert.Equal(t, "info", rec.Level, "an unknown level reads as info")
	assert.Len(t, []rune(rec.Message), ingestion.MaxTraceMessage)
	assert.NotContains(t, string(rec.Data), `\u0000`, "Postgres refuses it in jsonb")

	big := traceData(json.RawMessage(`{"x":"`+strings.Repeat("y", ingestion.MaxTraceData)+`"}`), ingestion.MaxTraceData)
	var stub map[string]any
	require.NoError(t, json.Unmarshal(big, &stub))
	assert.Equal(t, true, stub["truncated"])
	assert.Greater(t, stub["bytes"], float64(ingestion.MaxTraceData))
	assert.Nil(t, traceData(json.RawMessage(`not json`), 100))
	assert.Nil(t, traceData(json.RawMessage(`null`), 100))
	assert.Nil(t, traceData(nil, 100))
}

// Only a real NUL escape is replaced (owner's decision D11): an escaped
// backslash followed by the text u0000 is not one.
func TestReplaceNULEscapes(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`{"a":"\u0000"}`, "{\"a\":\"\uFFFD\"}"},
		{`{"a":"\\u0000"}`, `{"a":"\\u0000"}`},
		{`{"a":"\\\u0000"}`, "{\"a\":\"\\\\\uFFFD\"}"},
		{`{"a":"\\\\u0000"}`, `{"a":"\\\\u0000"}`},
		{`{"a":"x\u0000y\u0000"}`, "{\"a\":\"x\uFFFDy\uFFFD\"}"},
		{`{"\u0000":["\\u0000","\u0000",1]}`, "{\"\uFFFD\":[\"\\\\u0000\",\"\uFFFD\",1]}"},
		{`{"a":"\u00001"}`, "{\"a\":\"\uFFFD1\"}"},
		{`{"a":"\u0001","b":"\"\u0000"}`, "{\"a\":\"\\u0001\",\"b\":\"\\\"\uFFFD\"}"},
		{`{"a":"no escapes"}`, `{"a":"no escapes"}`},
	} {
		require.True(t, json.Valid([]byte(c.in)), c.in)
		got := string(replaceNULEscapes([]byte(c.in)))
		assert.Equal(t, c.want, got, c.in)
		assert.True(t, json.Valid([]byte(got)), "%s stays valid JSON: %s", c.in, got)
	}
}

// For any text, its JSON with the NUL escapes replaced is still valid JSON and
// decodes to the text with each NUL, and nothing else, replaced by U+FFFD.
func FuzzReplaceNULEscapes(f *testing.F) {
	for _, seed := range []string{"", "\x00", `\u0000`, "\\\x00", `\\u0000`, "a\x00b\\u0000c\\\\\x00", "\"\x00\\"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		in, err := json.Marshal(map[string]string{text: text})
		require.NoError(t, err)
		out := replaceNULEscapes(in)
		require.True(t, json.Valid(out), "%q became %q", in, out)
		var got, sent map[string]string
		require.NoError(t, json.Unmarshal(out, &got))
		require.NoError(t, json.Unmarshal(in, &sent))
		want := map[string]string{}
		for k, v := range sent {
			want[strings.ReplaceAll(k, "\x00", "\uFFFD")] = strings.ReplaceAll(v, "\x00", "\uFFFD")
		}
		assert.Equal(t, want, got)
	})
}
