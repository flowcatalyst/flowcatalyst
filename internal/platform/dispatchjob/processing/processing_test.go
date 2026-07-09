package processing

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
)

func strp(s string) *string { return &s }

func TestBuildPayload_Envelope(t *testing.T) {
	job := &dispatchjob.DispatchJob{
		ID:            "dsj_1",
		Code:          "app:sub:agg:created",
		Source:        strp("svc"),
		Subject:       strp("order/42"),
		CorrelationID: strp("corr_1"),
		ClientID:      strp("clt_1"),
		MessageGroup:  strp("grp_1"),
		AttemptCount:  2,
		Payload:       strp(`{"amount":100}`),
	}
	var env map[string]any
	require.NoError(t, json.Unmarshal(buildPayload(job), &env))

	assert.Equal(t, "dsj_1", env["id"])
	assert.Equal(t, "app:sub:agg:created", env["type"])
	assert.Equal(t, "svc", env["source"])
	assert.Equal(t, "order/42", env["subject"])
	assert.Equal(t, "corr_1", env["correlationId"])
	assert.Equal(t, "clt_1", env["clientId"])
	assert.Equal(t, "grp_1", env["messageGroup"])
	assert.EqualValues(t, 3, env["attemptNumber"]) // attempt_count + 1

	data, ok := env["data"].(map[string]any)
	require.True(t, ok, "JSON payload should embed as an object")
	assert.EqualValues(t, 100, data["amount"])
}

func TestBuildPayload_DataOnly(t *testing.T) {
	job := &dispatchjob.DispatchJob{DataOnly: true, Payload: strp(`{"raw":true}`)}
	assert.JSONEq(t, `{"raw":true}`, string(buildPayload(job)))

	// Data-only with no payload still produces valid JSON.
	assert.JSONEq(t, `{}`, string(buildPayload(&dispatchjob.DispatchJob{DataOnly: true})))
}

func TestBuildPayload_NonJSONPayload(t *testing.T) {
	// A payload that isn't JSON passes through as a string rather than being
	// silently dropped.
	job := &dispatchjob.DispatchJob{Code: "x", Payload: strp("not-json")}
	var env map[string]any
	require.NoError(t, json.Unmarshal(buildPayload(job), &env))
	assert.Equal(t, "not-json", env["data"])
}

func TestParseDeferral(t *testing.T) {
	d, ok := parseDeferral([]byte(`{"ack":false,"delaySeconds":12}`))
	require.True(t, ok)
	assert.Equal(t, 12*time.Second, d)

	d, ok = parseDeferral([]byte(`{"ack":false}`))
	require.True(t, ok)
	assert.Equal(t, 30*time.Second, d, "default deferral when delaySeconds absent")

	_, ok = parseDeferral([]byte(`{"ack":true}`))
	assert.False(t, ok, "ack=true is a success, not a deferral")

	_, ok = parseDeferral([]byte(`{}`))
	assert.False(t, ok, "no ack field → not a deferral")

	_, ok = parseDeferral(nil)
	assert.False(t, ok)
}

func TestBackoffFor(t *testing.T) {
	assert.Equal(t, 5*time.Second, backoffFor(1))
	assert.Equal(t, 15*time.Second, backoffFor(2))
	assert.Equal(t, 120*time.Second, backoffFor(99), "clamps to the last backoff")
	assert.Equal(t, 5*time.Second, backoffFor(0), "guards a non-positive attempt number")
}
