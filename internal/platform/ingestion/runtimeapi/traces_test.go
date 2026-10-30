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
