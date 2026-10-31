package processing

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
)

// The envelope carries the job's creation time (UTC, RFC 3339, milliseconds)
// and, when the job has one, its payload content type.
func TestBuildPayload_TimeAndDataContentType(t *testing.T) {
	// A zone other than UTC and sub-millisecond digits: the envelope converts
	// and truncates to milliseconds.
	zone := time.FixedZone("SAST", 2*3600)
	job := &dispatchjob.DispatchJob{
		ID: "dsj_1", Code: "app:sub:agg:created", Payload: new(`{"a":1}`), PayloadContentType: "application/json",
		CreatedAt: time.Date(2026, 10, 31, 14, 5, 6, 789123456, zone),
	}
	var env map[string]any
	require.NoError(t, json.Unmarshal(buildPayload(job, ""), &env))
	assert.Equal(t, "2026-10-31T12:05:06.789Z", env["time"])
	assert.Equal(t, "application/json", env["dataContentType"])

	// Nothing else of the envelope moved.
	assert.Equal(t, "dsj_1", env["id"])
	assert.Equal(t, "app:sub:agg:created", env["type"])
	assert.EqualValues(t, 1, env["attemptNumber"])
	assert.Equal(t, map[string]any{"a": float64(1)}, env["data"])

	// A job with no content type writes no dataContentType.
	job.PayloadContentType = ""
	env = nil
	require.NoError(t, json.Unmarshal(buildPayload(job, ""), &env))
	_, has := env["dataContentType"]
	assert.False(t, has)
	assert.Equal(t, "2026-10-31T12:05:06.789Z", env["time"])
}

func TestBuildPayload_DataOnlyHasNoEnvelopeFields(t *testing.T) {
	job := &dispatchjob.DispatchJob{DataOnly: true, Payload: new("<a/>"), PayloadContentType: "application/xml", CreatedAt: time.Now()}
	assert.Equal(t, "<a/>", string(buildPayload(job, "")))
}

func TestDeliver_ContentType(t *testing.T) {
	deliver := func(job *dispatchjob.DispatchJob) captured {
		var out atomic.Value
		sub := fakeSubscriber(t, &out)
		t.Cleanup(sub.Close)
		job.TargetURL = sub.URL
		res := New(nil, nil).deliver(context.Background(), job)
		require.True(t, res.success)
		return out.Load().(captured)
	}

	// The envelope is JSON whatever the payload's type.
	c := deliver(&dispatchjob.DispatchJob{ID: "dsj_1", Code: "app:evt", Payload: new("<a/>"), PayloadContentType: "application/xml"})
	assert.Equal(t, "application/json", c.contentType)

	// A raw delivery sends the job's own type...
	c = deliver(&dispatchjob.DispatchJob{ID: "dsj_2", Code: "app:evt", DataOnly: true, Payload: new("<a/>"), PayloadContentType: "application/xml"})
	assert.Equal(t, "application/xml", c.contentType)
	assert.Equal(t, "<a/>", string(c.body))
	c = deliver(&dispatchjob.DispatchJob{ID: "dsj_3", Code: "app:evt", DataOnly: true, Payload: new("a,b"), PayloadContentType: "text/csv; charset=utf-8"})
	assert.Equal(t, "text/csv; charset=utf-8", c.contentType)

	// ... and application/json when it has none.
	c = deliver(&dispatchjob.DispatchJob{ID: "dsj_4", Code: "app:evt", DataOnly: true, Payload: new(`{"a":1}`)})
	assert.Equal(t, "application/json", c.contentType)
}
