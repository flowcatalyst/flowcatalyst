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

	reaped := tr.Reap(30*time.Minute, 0)
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

	assert.Equal(t, 0, tr.Reap(30*time.Minute, 0), "recently-redelivered entry must not be reaped")
	assert.Equal(t, 1, tr.Count())
}

// TestInFlightReapEndsTheRetryExemption pins the FIRST of the two bugs that
// made an orphaned entry immortal: the reaper's Attempts>0 exemption was
// unconditional. It was justified by "the retry budget always ends in a
// release that removes the entry", which a drainer cancelled mid-retry never
// reaches — it parks the message and returns. The exemption must therefore
// expire with the retry that earned it.
//
// The ceiling is set far out here so it cannot be what does the reaping;
// this test is only about the exemption.
func TestInFlightReapEndsTheRetryExemption(t *testing.T) {
	tr := router.NewInFlightTracker()

	msg := common.Message{ID: "msg_retrying"}
	im := common.NewInFlightMessage(&msg, "broker-r", "queue-a", "", "receipt-1")
	require.Equal(t, router.RegisterNew, tr.Register(im))
	tr.MarkRetrying(msg.ID, "broker-r")
	require.Equal(t, uint(1), im.Attempts)
	require.False(t, im.LastRetryAt.IsZero(), "MarkRetrying must stamp LastRetryAt")

	// A LIVE retry: idle bound long exceeded, but the attempt is recent, so
	// the entry is still owned and must survive.
	im.StartedAt = time.Now().Add(-time.Hour)
	im.LastSeenAt = time.Now().Add(-time.Hour)
	assert.Equal(t, 0, tr.Reap(30*time.Minute, 24*time.Hour),
		"a live in-pipeline retry keeps its entry")

	// The owner died between attempts: no new attempt for well past the
	// grace. The entry is now an orphan and must be reapable.
	im.LastRetryAt = time.Now().Add(-time.Hour)
	assert.Equal(t, 1, tr.Reap(30*time.Minute, 24*time.Hour),
		"a retry that stopped attempting is an orphan, not a live retry")
	assert.Equal(t, 0, tr.Count())
}

// TestInFlightReapCeilingBeatsRedeliveryRefresh pins the SECOND bug, and the
// nastier one: the idle bound is self-defeating. Every redelivery refreshes
// LastSeenAt — including the redeliveries Register drops as duplicates of
// this very entry — so an orphan resets its own idle clock forever while the
// message it keeps dropping is never delivered and never acked. On a FIFO
// queue that pins the head of a message group until broker retention.
//
// The absolute ceiling (measured on StartedAt, which nothing refreshes) is
// the only thing that can break the loop, so it must fire even when the
// retry exemption is live AND the idle clock was just refreshed.
func TestInFlightReapCeilingBeatsRedeliveryRefresh(t *testing.T) {
	tr := router.NewInFlightTracker()

	msg := common.Message{ID: "msg_orphan"}
	im := common.NewInFlightMessage(&msg, "broker-x", "queue-a", "", "receipt-1")
	require.Equal(t, router.RegisterNew, tr.Register(im))
	tr.MarkRetrying(msg.ID, "broker-x") // exemption live: LastRetryAt is now

	// The owning drainer is cancelled mid-retry and parks the message; from
	// here nothing will ever remove this entry. Time passes.
	im.StartedAt = time.Now().Add(-3 * time.Hour)

	// The broker keeps redelivering. Each copy is dropped as a duplicate and
	// refreshes the idle clock, so the idle bound can never fire.
	for range 3 {
		redelivery := common.NewInFlightMessage(&msg, "broker-x", "queue-a", "", "receipt-fresh")
		require.Equal(t, router.RegisterRedelivery, tr.Register(redelivery))
	}
	require.WithinDuration(t, time.Now(), im.LastSeenAt, time.Second,
		"the redeliveries must have refreshed the idle clock (the trap being tested)")

	assert.Equal(t, 1, tr.Reap(15*time.Minute, 2*time.Hour),
		"the absolute ceiling must reap an orphan the idle bound can never see")
	assert.Equal(t, 0, tr.Count())

	// The message is processable again: the next redelivery owns the pipeline
	// instead of being dropped into the void.
	assert.Equal(t, router.RegisterNew,
		tr.Register(common.NewInFlightMessage(&msg, "broker-x", "queue-a", "", "receipt-final")))
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
