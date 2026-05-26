package router_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
)

func TestInFlightDuplicateSwapsReceiptHandle(t *testing.T) {
	tr := router.NewInFlightTracker()

	msg := common.Message{ID: "msg_01", MediationType: common.MediationTypeHTTP, MediationTarget: "https://x"}
	im1 := common.NewInFlightMessage(&msg, "broker-1", "queue-a", "", "receipt-A")
	existing, isDup := tr.Insert(im1)
	require.False(t, isDup)
	assert.Same(t, im1, existing)

	// Broker redelivers with a new receipt handle (visibility expired).
	im2 := common.NewInFlightMessage(&msg, "broker-1", "queue-a", "", "receipt-B")
	existing, isDup = tr.Insert(im2)
	require.True(t, isDup)
	assert.Same(t, im1, existing, "should return the original tracker entry")
	assert.Equal(t, "receipt-B", existing.ReceiptHandle, "receipt handle should be swapped")

	assert.Equal(t, 1, tr.Count())
}

func TestInFlightRemoveAndReap(t *testing.T) {
	tr := router.NewInFlightTracker()

	msg := common.Message{ID: "msg_old"}
	im := common.NewInFlightMessage(&msg, "broker-old", "queue-a", "", "receipt-O")
	im.StartedAt = time.Now().Add(-time.Hour)
	_, _ = tr.Insert(im)
	assert.Equal(t, 1, tr.Count())

	reaped := tr.Reap(30 * time.Minute)
	assert.Equal(t, 1, reaped)
	assert.Equal(t, 0, tr.Count())
}
