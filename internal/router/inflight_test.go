package router_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
)

func TestInFlightRedeliverySwapsReceiptHandle(t *testing.T) {
	tr := router.NewInFlightTracker()

	msg := common.Message{ID: "msg_01", MediationType: common.MediationTypeHTTP, MediationTarget: "https://x"}
	im1 := common.NewInFlightMessage(&msg, "broker-1", "queue-a", "", "receipt-A")
	require.Equal(t, router.RegisterNew, tr.Register(im1))

	// Broker redelivers with a new receipt handle (visibility expired).
	im2 := common.NewInFlightMessage(&msg, "broker-1", "queue-a", "", "receipt-B")
	assert.Equal(t, router.RegisterRedelivery, tr.Register(im2))
	assert.Equal(t, "receipt-B", im1.ReceiptHandle, "receipt handle should be swapped onto the owner")

	assert.Equal(t, 1, tr.Count())
}

// TestInFlightExternalRequeueDoesNotContaminate pins the two bugs that made
// requeue storms self-sustaining: registering a requeued copy (same app id,
// different broker id) must NOT adopt its receipt handle onto the owner
// (handles are per-broker-message — the owner's ACK would delete the wrong
// SQS message) and must NOT leave a phantom byBroker entry that dedup-drops
// every future redelivery of the requeued message after the owner completes.
func TestInFlightExternalRequeueDoesNotContaminate(t *testing.T) {
	tr := router.NewInFlightTracker()

	msg := common.Message{ID: "app-1"}
	owner := common.NewInFlightMessage(&msg, "broker-1", "queue-a", "", "receipt-orig")
	require.Equal(t, router.RegisterNew, tr.Register(owner))

	requeue := common.NewInFlightMessage(&msg, "broker-2", "queue-a", "", "receipt-requeue")
	assert.Equal(t, router.RegisterExternalRequeue, tr.Register(requeue))
	assert.Equal(t, "receipt-orig", owner.ReceiptHandle,
		"a requeued copy's receipt handle must not replace the owner's")

	// Owner completes; a leftover redelivery of the requeued broker message
	// must now register as NEW (processable), not be dropped as a duplicate
	// of a phantom entry.
	tr.Remove(owner.MessageID, owner.BrokerMessageID)
	again := common.NewInFlightMessage(&msg, "broker-2", "queue-a", "", "receipt-requeue-2")
	assert.Equal(t, router.RegisterNew, tr.Register(again),
		"no phantom byBroker entry may survive the requeue registration")
}

func TestInFlightRemoveAndReap(t *testing.T) {
	tr := router.NewInFlightTracker()

	msg := common.Message{ID: "msg_old"}
	im := common.NewInFlightMessage(&msg, "broker-old", "queue-a", "", "receipt-O")
	im.StartedAt = time.Now().Add(-time.Hour)
	im.LastSeenAt = im.StartedAt
	require.Equal(t, router.RegisterNew, tr.Register(im))
	assert.Equal(t, 1, tr.Count())

	reaped := tr.Reap(30 * time.Minute)
	assert.Equal(t, 1, reaped)
	assert.Equal(t, 0, tr.Count())
}

// TestInFlightReapSkipsRecentlyRedelivered: while the broker still holds a
// message it keeps redelivering it, refreshing LastSeenAt via the handle swap
// — the reaper must age on that, not on StartedAt, or a long-buffered message
// (slow ordered group) loses its entry and redeliveries duplicate again.
func TestInFlightReapSkipsRecentlyRedelivered(t *testing.T) {
	tr := router.NewInFlightTracker()

	msg := common.Message{ID: "msg_buffered"}
	im := common.NewInFlightMessage(&msg, "broker-b", "queue-a", "", "receipt-1")
	im.StartedAt = time.Now().Add(-time.Hour)
	im.LastSeenAt = im.StartedAt
	require.Equal(t, router.RegisterNew, tr.Register(im))

	// A redelivery arrives (visibility lapsed) — swaps handle, refreshes age.
	redelivery := common.NewInFlightMessage(&msg, "broker-b", "queue-a", "", "receipt-2")
	require.Equal(t, router.RegisterRedelivery, tr.Register(redelivery))

	assert.Equal(t, 0, tr.Reap(30*time.Minute), "recently-redelivered entry must not be reaped")
	assert.Equal(t, 1, tr.Count())
}

// TestInFlightEnsureTrackedBackstop covers the pool's process-time backstop:
// the route-time entry is recognised (no swap — the entry's handle may be
// fresher), a reaped entry is restored, and a foreign copy (same app id,
// different broker id) is rejected without touching the owner.
func TestInFlightEnsureTrackedBackstop(t *testing.T) {
	tr := router.NewInFlightTracker()
	msg := common.Message{ID: "app-1"}

	owner := common.NewInFlightMessage(&msg, "broker-1", "queue-a", "", "receipt-stale")
	require.Equal(t, router.RegisterNew, tr.Register(owner))
	// A redelivery freshened the owner's handle while it sat buffered.
	tr.Register(common.NewInFlightMessage(&msg, "broker-1", "queue-a", "", "receipt-fresh"))

	// First dispatch re-asserts with the ROUTE-time (stale) handle; the fresh
	// one must survive.
	same := common.NewInFlightMessage(&msg, "broker-1", "queue-a", "", "receipt-stale")
	assert.True(t, tr.EnsureTracked(same))
	assert.Equal(t, "receipt-fresh", owner.ReceiptHandle, "EnsureTracked must never regress the handle")

	// A foreign copy is rejected.
	foreign := common.NewInFlightMessage(&msg, "broker-2", "queue-a", "", "receipt-requeue")
	assert.False(t, tr.EnsureTracked(foreign))

	// Reaped entry → restored.
	tr.Remove(msg.ID, "broker-1")
	assert.True(t, tr.EnsureTracked(same))
	assert.Equal(t, 1, tr.Count())
}
