package router

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// A deferred copy keeps its entry: a second broker copy of the same message
// id is a duplicate (deleted by the caller), the deferred copy itself
// re-enters as new when it returns, and the parked entry neither counts as
// in flight nor gets reaped before it is due (2026-09-22: "500 in flight but
// only 200 pending — use the messageId, not the SQS id").
func TestInFlightDeferredCopyDedupsRepublishedDuplicates(t *testing.T) {
	tr := NewInFlightTracker()
	first := common.NewInFlightMessage(&common.Message{ID: "job-1"}, "sqs-a", "q", "b1", "rh-a1")
	require.Equal(t, RegisterNew, tr.Register(first))

	tr.MarkDeferred("job-1", time.Now().Add(time.Hour))
	assert.Equal(t, 0, tr.Count(), "a parked copy is the broker's, not this process's")
	assert.Equal(t, 1, tr.DeferredCount())

	// The platform's stale recovery republished the job: new broker id.
	dup := common.NewInFlightMessage(&common.Message{ID: "job-1"}, "sqs-b", "q", "b2", "rh-b1")
	assert.Equal(t, RegisterExternalRequeue, tr.Register(dup),
		"a different broker copy of a deferred message is a duplicate to delete, not a new message to defer")

	// Not reaped while parked, even though it is idle past the bound.
	first.LastSeenAt = time.Now().Add(-time.Hour)
	assert.Equal(t, 0, tr.Reap(time.Minute, 4*time.Hour), "a parked entry is kept until overdue")

	// The deferred copy returns: it is itself again, and submitted.
	back := common.NewInFlightMessage(&common.Message{ID: "job-1"}, "sqs-a", "q", "b3", "rh-a2")
	assert.Equal(t, RegisterNew, tr.Register(back))
	assert.Equal(t, 1, tr.Count())
	assert.Equal(t, 0, tr.DeferredCount())
	rh, _ := tr.CurrentReceipt("job-1", "sqs-a")
	assert.Equal(t, "rh-a2", rh, "the returning copy's receipt is adopted")

	// Overdue parked entries do age out, so a copy the broker never returns
	// does not block a later republish for ever.
	tr.MarkDeferred("job-1", time.Now().Add(-deferredReapGrace-time.Second))
	first.LastSeenAt = time.Now().Add(-time.Hour)
	assert.Equal(t, 1, tr.Reap(time.Minute, 4*time.Hour))
}
