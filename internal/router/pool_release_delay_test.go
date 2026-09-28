package router

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// Owner decision #41 (2026-09-26): a message released because its target is
// unreachable, unavailable or behind an open breaker goes back to the broker
// with the outcome's own delay, never with none. A zero delay makes it
// visible at once, so an outage becomes a hot redelivery loop that spends an
// SQS receive count toward the DLQ on every lap.
func TestDispositionOfReleasesCarryTheOutcomeDelay(t *testing.T) {
	cases := []struct {
		name    string
		outcome common.MediationOutcome
		want    time.Duration
	}{
		{"unavailable 503", common.ErrorProcess(30, "HTTP 503"), 30 * time.Second},
		{"connection refused", common.ErrorConnection("refused"), 30 * time.Second},
		{"open breaker", common.CircuitOpen(5), 5 * time.Second},
		{"unavailable with no delay", common.ErrorProcess(0, "HTTP 503"), 0},
	}
	for _, tc := range cases {
		for _, mode := range []common.DispatchMode{common.DispatchImmediate, common.DispatchNextOnError, common.DispatchBlockOnError} {
			d := DispositionOf(tc.outcome, 0, mode, true)
			assert.Equal(t, BrokerRelease, d.Action, "%s / %s", tc.name, mode)
			assert.Equal(t, GroupRelease, d.Group, "%s / %s", tc.name, mode)
			assert.Equal(t, tc.want, d.RetryAfter, "%s / %s", tc.name, mode)
		}
	}
}

func TestSiblingNackDelayIsNeverShorterThanTheHead(t *testing.T) {
	u := func(v uint32) *uint32 { return &v }
	assert.Equal(t, uint32(10), *siblingNackDelay(nil))
	assert.Equal(t, uint32(10), *siblingNackDelay(u(3)))
	assert.Equal(t, uint32(30), *siblingNackDelay(u(30)))
	assert.Equal(t, uint32(3600), *siblingNackDelay(u(3600)))
}

// The head of a group whose target is down goes back with the outcome's
// delay, and every sibling behind it with no less, so none of them can
// surface before the head and overtake it (delivery run 5, platform-down).
func TestPoolReleasedGroupHoldsSiblingsBehindTheHead(t *testing.T) {
	cases := []struct {
		name      string
		failWith  common.MediationOutcome
		head, sib uint32
	}{
		{"unavailable", common.ErrorProcess(30, "HTTP 503"), 30, 30},
		{"connection refused", common.ErrorConnection("refused"), 30, 30},
		{"open breaker", common.CircuitOpen(5), 5, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
			out := tc.failWith
			med := &cascadeMediator{failID: "m1", failWith: &out}
			pool := newCascadePool(med, func(string) queue.Consumer { return cons })

			submitBatch(context.Background(), pool, []common.QueuedMessage{
				releaseMsg("m1", "g", common.DispatchNextOnError),
				releaseMsg("m2", "g", common.DispatchNextOnError),
				releaseMsg("m3", "g", common.DispatchNextOnError),
			})
			select {
			case <-cons.done:
			case <-time.After(3 * time.Second):
				t.Fatal("timed out waiting for the group to be released")
			}

			cons.mu.Lock()
			defer cons.mu.Unlock()
			require.ElementsMatch(t, []string{"m1", "m2", "m3"}, cons.nacked)
			require.NotNil(t, cons.nackDelays["m1"], "the head must never be released with no delay")
			assert.Equal(t, tc.head, *cons.nackDelays["m1"])
			for _, id := range []string{"m2", "m3"} {
				require.NotNil(t, cons.nackDelays[id], "sibling %s released with no delay", id)
				assert.Equal(t, tc.sib, *cons.nackDelays[id], "sibling %s", id)
				assert.GreaterOrEqual(t, *cons.nackDelays[id], *cons.nackDelays["m1"],
					"sibling %s must not come back before its head", id)
			}
		})
	}
}

// An IMMEDIATE message has no group behind it; it alone goes back, still
// with the outcome's delay.
func TestPoolReleasedImmediateMessageCarriesTheOutcomeDelay(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 1, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1", failWith: unreachable(http.StatusBadGateway)}
	med.failWith.DelaySeconds = 30
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })

	submitBatch(context.Background(), pool, []common.QueuedMessage{
		releaseMsg("m1", "", common.DispatchImmediate),
	})
	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the release")
	}
	cons.mu.Lock()
	defer cons.mu.Unlock()
	require.NotNil(t, cons.nackDelays["m1"])
	assert.Equal(t, uint32(30), *cons.nackDelays["m1"])
}

// A parked group is handed back as a whole with the sibling delay: never
// visible at once.
func TestReleaseParkedGroupsNacksWithTheSiblingDelay(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 2, done: make(chan struct{})}
	pool := newCascadePool(&cascadeMediator{}, func(string) queue.Consumer { return cons })
	pool.mu.Lock()
	pool.groupQs["g"] = &groupQueue{
		msgs: []common.QueuedMessage{
			releaseMsg("p1", "g", common.DispatchBlockOnError),
			releaseMsg("p2", "g", common.DispatchBlockOnError),
		},
		parkedAt: time.Now().Add(-time.Hour),
	}
	pool.mu.Unlock()
	pool.queueSize.Store(2)

	assert.Equal(t, 2, pool.ReleaseParkedGroups(context.Background(), time.Minute))
	cons.mu.Lock()
	defer cons.mu.Unlock()
	for _, id := range []string{"p1", "p2"} {
		require.NotNil(t, cons.nackDelays[id], "%s released with no delay", id)
		assert.Equal(t, siblingNackDelaySeconds, *cons.nackDelays[id])
	}
}
