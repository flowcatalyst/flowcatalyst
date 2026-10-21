package control

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

func TestTraceRateLimiter_BucketRefillsAndIsPerFunction(t *testing.T) {
	l := newTraceRateLimiter()
	t0 := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

	assert.Equal(t, 150, l.take("a", 150, t0))
	assert.Equal(t, 50, l.take("a", 100, t0), "only the 50 left of the second's 200")
	assert.Equal(t, 0, l.take("a", 10, t0))
	assert.Equal(t, 100, l.take("b", 100, t0), "another function has its own bucket")

	assert.Equal(t, 100, l.take("a", 500, t0.Add(500*time.Millisecond)), "half a second refills 100")
	assert.Equal(t, fncontrol.MaxTraceRecordsPerSec, l.take("a", 500, t0.Add(time.Hour)), "the bucket never holds more than a second's worth")
}

func TestCutRunesAndCleanText(t *testing.T) {
	assert.Equal(t, "abc", cutRunes("abc", 5))
	assert.Equal(t, "ab", cutRunes("abc", 2))
	assert.Equal(t, strings.Repeat("é", 3), cutRunes(strings.Repeat("é", 10), 3), "characters, not bytes")
	assert.Equal(t, "ab", cleanText("a\x00b"))
	assert.Equal(t, "a�b", cleanText("a\xffb"))
}

func TestTraceData(t *testing.T) {
	assert.Nil(t, traceData(nil, 100))
	assert.Nil(t, traceData([]byte("null"), 100))
	assert.Nil(t, traceData([]byte("{broken"), 100))
	assert.JSONEq(t, `{"a":1}`, string(traceData([]byte(`{"a":1}`), 100)))
	assert.JSONEq(t, `{"truncated":true,"bytes":18}`, string(traceData([]byte(`{"a":"0123456789"}`), 10)), "over the cap is replaced by the stub")
}
