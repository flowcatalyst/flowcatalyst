package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// The delay an ack=false answer names (owner ruling 2026-11-09): notBefore —
// WHEN to come back — wins over delaySeconds, is rounded UP to whole seconds
// (never early), and an instant already past still means "not now" (1 s).
func TestDeferralDelaySeconds(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	str := func(s string) *string { return &s }
	u32 := func(n uint32) *uint32 { return &n }

	cases := []struct {
		name string
		r    mediationResponse
		want int
	}{
		{"notBefore wins over delaySeconds", mediationResponse{NotBefore: str("2026-11-09T10:00:20Z"), DelaySeconds: u32(999)}, 20},
		{"a fraction of a second rounds up", mediationResponse{NotBefore: str("2026-11-09T10:00:07.001Z")}, 8},
		{"an exact second is not padded", mediationResponse{NotBefore: str("2026-11-09T10:00:07.000Z")}, 7},
		{"a past notBefore is the minimum, not zero", mediationResponse{NotBefore: str("2026-11-09T09:00:00Z")}, 1},
		{"another zone is the same instant", mediationResponse{NotBefore: str("2026-11-09T12:00:30+02:00")}, 30},
		{"unparseable notBefore falls back to delaySeconds", mediationResponse{NotBefore: str("soon"), DelaySeconds: u32(12)}, 12},
		{"delaySeconds alone", mediationResponse{DelaySeconds: u32(45)}, 45},
		{"neither names a delay", mediationResponse{}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, deferralDelaySeconds(tc.r, now))
		})
	}
}

// A not-before deferral is honoured AT THE RIGHT TIME by the router: a real
// mediator against a target answering {"ack":false,"notBefore":now+20s,
// "delaySeconds":999} hands the message back to the broker once, on its first
// occurrence, with a 20 s delay — the notBefore, not the delaySeconds and not
// the deferred curve.
func TestRouterHonoursNotBeforeExactly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		at := time.Now().Add(20 * time.Second).UTC().Format(time.RFC3339Nano)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"ack":false,"notBefore":%q,"delaySeconds":999}`, at)
	}))
	defer srv.Close()

	med := NewHTTPMediator(DevMediatorConfig(), NewBreakerRegistry(DefaultBreakerConfig()))
	cons := &cascadeConsumer{wantTotal: 1, done: make(chan struct{})}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })
	group := "sub-1:order-7"
	pool.submit(context.Background(), common.QueuedMessage{
		Message: common.Message{
			ID: "job-1", MediationType: common.MediationTypeHTTP,
			MediationTarget: srv.URL, DispatchMode: common.DispatchBlockOnError, MessageGroupID: &group,
		},
		ReceiptHandle: "rh-job-1",
	})

	select {
	case <-cons.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the deferred message was never handed back to the broker")
	}
	cons.mu.Lock()
	nacked := append([]string(nil), cons.nacked...)
	acked := append([]string(nil), cons.acked...)
	cons.mu.Unlock()
	assert.Equal(t, []string{"rh-job-1"}, nacked)
	assert.Empty(t, acked)
	delay := cons.nackDelayFor("rh-job-1")
	require.NotNil(t, delay)
	assert.Equal(t, uint32(20), *delay, "redelivered at notBefore: not delaySeconds (999), not the curve")
}

// On a broker that cannot hold a message back (NATS), a named delay is still
// the wait — capped at the deferred ceiling — rather than the curve, which
// would come back late for a short remaining delay.
func TestDispositionOfDeferredNamedDelayOnNATSWaitsTheNamedDelay(t *testing.T) {
	short := DispositionOf(*deferredWithDelay(3), 4, common.DispatchBlockOnError, false)
	assert.Equal(t, BrokerRetry, short.Action)
	assert.Equal(t, 3*time.Second, short.RetryAfter, "the curve at attempt 4 would be 60s")

	long := DispositionOf(*deferredWithDelay(600), 0, common.DispatchImmediate, false)
	assert.Equal(t, BrokerRetry, long.Action)
	assert.Equal(t, deferredMaxDelay, long.RetryAfter, "a long wait is served a ceiling at a time")

	none := DispositionOf(*deferredWithDelay(0), 2, common.DispatchImmediate, false)
	assert.Equal(t, deferredDelay(2, 0), none.RetryAfter, "no named delay: the curve, unchanged")
}
