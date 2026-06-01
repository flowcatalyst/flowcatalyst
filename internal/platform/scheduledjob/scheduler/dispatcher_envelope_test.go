package scheduler

import (
	"encoding/json"
	"testing"
	"time"
)

// The delivered envelope MUST be camelCase: the Rust dispatcher's
// WebhookEnvelope and the SDK runner's ScheduledJobEnvelope are both
// #[serde(rename_all="camelCase")] and reject a missing required field with
// HTTP 400. snake_case keys would fail every Go→SDK firing.
func TestWebhookEnvelope_CamelCaseKeys(t *testing.T) {
	ts := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	corr := "corr-1"
	payload := json.RawMessage(`{"k":"v"}`)
	timeout := int32(30)
	env := webhookEnvelope{
		JobID:            "job-1",
		JobCode:          "code-1",
		InstanceID:       "inst-1",
		ScheduledFor:     &ts,
		FiredAt:          ts,
		TriggerKind:      "CRON",
		CorrelationID:    &corr,
		Payload:          &payload,
		TracksCompletion: true,
		TimeoutSeconds:   &timeout,
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Exact key set the SDK ScheduledJobEnvelope expects (camelCase).
	want := []string{
		"jobId", "jobCode", "instanceId", "scheduledFor", "firedAt",
		"triggerKind", "correlationId", "payload", "tracksCompletion", "timeoutSeconds",
	}
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("envelope missing camelCase key %q; got keys %v", k, keysOf(got))
		}
	}
	// No snake_case leakage.
	for _, bad := range []string{"job_id", "job_code", "instance_id", "trigger_kind", "tracks_completion"} {
		if _, ok := got[bad]; ok {
			t.Errorf("envelope has snake_case key %q — must be camelCase", bad)
		}
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
