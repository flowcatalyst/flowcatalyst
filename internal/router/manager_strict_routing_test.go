package router

// Pins R-13/R-16: FC_ROUTER_STRICT_ROUTING (Manager.SetStrictRouting). When
// on, a message with an empty pool_code, an empty/absent dispatch_mode, or
// an ordered mode with no message_group_id is malformed — route() ACKs it
// (never delivers, never NACKs) and raises a CONFIGURATION warning naming
// the message id, queue and missing field, instead of applying the usual
// fallback (DEFAULT-POOL / DefaultDispatchMode / the shared "" group). Off
// by default, today's fallback behaviour is unchanged.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

func TestMalformedRoutingReason(t *testing.T) {
	group := "g"
	cases := []struct {
		name string
		msg  common.Message
		want string
	}{
		{
			name: "well-formed ordered",
			msg: common.Message{
				PoolCode: "P", DispatchMode: common.DispatchNextOnError, MessageGroupID: &group,
			},
			want: "",
		},
		{
			name: "well-formed immediate, no group",
			msg:  common.Message{PoolCode: "P", DispatchMode: common.DispatchImmediate},
			want: "",
		},
		{
			name: "empty pool_code",
			msg:  common.Message{DispatchMode: common.DispatchImmediate},
			want: "empty pool_code",
		},
		{
			name: "empty dispatch_mode",
			msg:  common.Message{PoolCode: "P"},
			want: "empty dispatch_mode",
		},
		{
			name: "ordered with no group",
			msg:  common.Message{PoolCode: "P", DispatchMode: common.DispatchBlockOnError},
			want: "ordered dispatch_mode with no message_group_id",
		},
		{
			name: "empty pool_code wins over empty dispatch_mode",
			msg:  common.Message{},
			want: "empty pool_code",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, malformedRoutingReason(&tc.msg))
		})
	}
}

// mkStrict builds a queued message on queue "q", broker id == id, for the
// strict-routing route() tests below.
func mkStrict(id, poolCode string, mode common.DispatchMode, group *string) common.QueuedMessage {
	return common.QueuedMessage{
		Message: common.Message{
			ID: id, PoolCode: poolCode, DispatchMode: mode, MessageGroupID: group,
			MediationType: common.MediationTypeHTTP, MediationTarget: "http://example.invalid",
		},
		BrokerMessageID: "b-" + id,
		ReceiptHandle:   "rh-" + id,
		QueueIdentifier: "q",
	}
}

// TestRoute_StrictRouting_Disabled_UnchangedFallback pins that strict mode
// off (the default) preserves today's behaviour exactly: an empty pool_code
// still routes to DEFAULT-POOL rather than being ACKed as malformed.
func TestRoute_StrictRouting_Disabled_UnchangedFallback(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 99, done: make(chan struct{})}
	med := &cascadeMediator{}
	m, _, _ := newRouteHarness(med, cons)
	// strictRouting defaults to false; not calling SetStrictRouting at all.

	m.route(context.Background(), []common.QueuedMessage{
		mkStrict("m1", "", common.DispatchImmediate, nil),
	}, cons)

	require.Eventually(t, func() bool {
		med.mu.Lock()
		defer med.mu.Unlock()
		return len(med.seen) == 1
	}, time.Second, 5*time.Millisecond, "an empty pool_code must still fall back to DEFAULT-POOL and deliver")
}

// TestRoute_StrictRouting_EmptyPoolCode_Acked pins the malformed-empty-pool
// case: ACKed, never delivered, never NACKed, tracker entry released, and a
// CONFIGURATION warning naming the message id and queue is raised.
func TestRoute_StrictRouting_EmptyPoolCode_Acked(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 99, done: make(chan struct{})}
	med := &cascadeMediator{}
	m, tr, _ := newRouteHarness(med, cons)
	m.SetStrictRouting(true)
	ws := NewWarningService(DefaultWarningServiceConfig())
	m.SetWarnings(ws)

	m.route(context.Background(), []common.QueuedMessage{
		mkStrict("m1", "", common.DispatchImmediate, nil),
	}, cons)

	require.Eventually(t, func() bool {
		cons.mu.Lock()
		defer cons.mu.Unlock()
		return len(cons.acked) == 1
	}, time.Second, 5*time.Millisecond, "malformed message must be ACKed")

	cons.mu.Lock()
	acked, nacked := append([]string(nil), cons.acked...), append([]string(nil), cons.nacked...)
	cons.mu.Unlock()
	assert.Equal(t, []string{"rh-m1"}, acked)
	assert.Empty(t, nacked, "must never be NACKed")

	med.mu.Lock()
	seen := append([]string(nil), med.seen...)
	med.mu.Unlock()
	assert.Empty(t, seen, "must never be delivered")

	assert.Equal(t, 0, tr.Count(), "the route-time tracker entry must be released")

	warnings := ws.ByCategory(WarningCategoryConfiguration)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].Message, "m1")
	assert.Contains(t, warnings[0].Message, "q")
	assert.Contains(t, warnings[0].Message, "pool_code")
}

// TestRoute_StrictRouting_EmptyDispatchMode_Acked pins the malformed-empty-
// mode case.
func TestRoute_StrictRouting_EmptyDispatchMode_Acked(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 99, done: make(chan struct{})}
	med := &cascadeMediator{}
	m, tr, _ := newRouteHarness(med, cons)
	m.SetStrictRouting(true)

	m.route(context.Background(), []common.QueuedMessage{
		mkStrict("m2", defaultPoolCode, "", nil),
	}, cons)

	require.Eventually(t, func() bool {
		cons.mu.Lock()
		defer cons.mu.Unlock()
		return len(cons.acked) == 1
	}, time.Second, 5*time.Millisecond, "malformed message must be ACKed")
	med.mu.Lock()
	seen := len(med.seen)
	med.mu.Unlock()
	assert.Equal(t, 0, seen, "must never be delivered")
	assert.Equal(t, 0, tr.Count())
}

// TestRoute_StrictRouting_OrderedNoGroup_Acked pins the malformed
// ordered-mode-without-group case.
func TestRoute_StrictRouting_OrderedNoGroup_Acked(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 99, done: make(chan struct{})}
	med := &cascadeMediator{}
	m, tr, _ := newRouteHarness(med, cons)
	m.SetStrictRouting(true)

	m.route(context.Background(), []common.QueuedMessage{
		mkStrict("m3", defaultPoolCode, common.DispatchBlockOnError, nil),
	}, cons)

	require.Eventually(t, func() bool {
		cons.mu.Lock()
		defer cons.mu.Unlock()
		return len(cons.acked) == 1
	}, time.Second, 5*time.Millisecond, "malformed message must be ACKed")
	med.mu.Lock()
	seen := len(med.seen)
	med.mu.Unlock()
	assert.Equal(t, 0, seen, "must never be delivered")
	assert.Equal(t, 0, tr.Count())
}

// TestRoute_StrictRouting_WellFormed_StillDelivers verifies strict mode
// doesn't reject well-formed messages: pool_code + dispatch_mode set, and a
// group present whenever the mode requires ordering.
func TestRoute_StrictRouting_WellFormed_StillDelivers(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 99, done: make(chan struct{})}
	med := &cascadeMediator{}
	m, _, _ := newRouteHarness(med, cons)
	m.SetStrictRouting(true)
	group := "g"

	m.route(context.Background(), []common.QueuedMessage{
		mkStrict("m4", defaultPoolCode, common.DispatchNextOnError, &group),
	}, cons)

	require.Eventually(t, func() bool {
		med.mu.Lock()
		defer med.mu.Unlock()
		return len(med.seen) == 1
	}, time.Second, 5*time.Millisecond, "a well-formed ordered message must still deliver under strict routing")
}
