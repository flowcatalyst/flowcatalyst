package processing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
)

func TestRetryDelayByStrategy(t *testing.T) {
	assert.Equal(t, time.Duration(0), retryDelay(dispatchjob.RetryImmediate, 3))
	assert.Equal(t, 30*time.Second, retryDelay(dispatchjob.RetryFixed, 1))
	assert.Equal(t, 30*time.Second, retryDelay(dispatchjob.RetryFixed, 5))
	assert.Equal(t, 5*time.Second, retryDelay(dispatchjob.RetryExponentialBackoff, 1))
	assert.Equal(t, 60*time.Second, retryDelay(dispatchjob.RetryExponentialBackoff, 4))
	assert.Equal(t, 15*time.Second, retryDelay("", 2), "unset: the default curve")
}

func TestParseDeferralNotBefore(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	d, ok := parseDeferralAt([]byte(`{"ack":false,"notBefore":"2026-11-09T10:00:40Z","delaySeconds":5}`), now)
	assert.True(t, ok)
	assert.Equal(t, 40*time.Second, d, "notBefore wins over delaySeconds")
	d, ok = parseDeferralAt([]byte(`{"ack":false,"notBefore":"2026-11-09T09:00:00Z"}`), now)
	assert.True(t, ok)
	assert.Equal(t, time.Duration(0), d, "past: no wait")
	d, _ = parseDeferralAt([]byte(`{"ack":false,"notBefore":"later","delaySeconds":9}`), now)
	assert.Equal(t, 9*time.Second, d, "unparseable notBefore: delaySeconds")
	d, _ = parseDeferralAt([]byte(`{"ack":false}`), now)
	assert.Equal(t, 30*time.Second, d)
	_, ok = parseDeferralAt([]byte(`{"ack":true,"notBefore":"2026-11-09T10:00:40Z"}`), now)
	assert.False(t, ok)
}

func TestBuildPayloadTellsTheEventsGroup(t *testing.T) {
	sub := "sub_a"
	g := "order-1"
	job := &dispatchjob.DispatchJob{
		ID: "j1", Code: "a:b:c:d", SubscriptionID: &sub,
		MessageGroup: dispatchjob.SubscriptionMessageGroup(sub, &g),
	}
	assert.Contains(t, string(buildPayload(job, "")), `"messageGroup":"order-1"`)
}
