package router

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// R-04: GroupSnapshot is the data source for the dashboard's "blocked
// groups" panel — for every LIVE entry in groupQs it must report the group
// id, the pool it belongs to, how many messages are buffered, whether a
// drainer currently owns it, how long it's been parked (zero while a
// drainer owns it), and whether the GroupFlushRegistry is currently
// suppressing it.
func TestPoolGroupSnapshot(t *testing.T) {
	p := &Pool{
		cfg:     common.PoolConfig{Code: "TEST-POOL"},
		groupQs: map[string]*groupQueue{},
		flushes: NewGroupFlushRegistry(),
	}

	// g1: two messages buffered, no drainer running (freshly enqueued).
	require.True(t, p.enqueue("g1", common.QueuedMessage{Message: common.Message{ID: "m1"}}))
	require.True(t, p.enqueue("g1", common.QueuedMessage{Message: common.Message{ID: "m2"}}))

	// g2: a drainer owns it (working); parkedAt must read zero regardless
	// of what's stored, since Working takes precedence in the semantics.
	require.True(t, p.enqueue("g2", common.QueuedMessage{Message: common.Message{ID: "m3"}}))
	p.mu.Lock()
	p.groupQs["g2"].working = true
	p.mu.Unlock()

	// g3: parked — no drainer, and it's been sitting for a while.
	require.True(t, p.enqueue("g3", common.QueuedMessage{Message: common.Message{ID: "m4"}}))
	parkedSince := time.Now().Add(-90 * time.Second)
	p.clearWorking("g3")
	p.mu.Lock()
	p.groupQs["g3"].parkedAt = parkedSince
	p.mu.Unlock()

	// g4: currently suppressed by the flush registry.
	require.True(t, p.enqueue("g4", common.QueuedMessage{Message: common.Message{ID: "m5"}}))
	require.True(t, p.flushes.Flush("g4", time.Minute))

	snap := p.GroupSnapshot()
	require.Len(t, snap, 4, "one row per live group; a fully-drained group would be absent")

	byGroup := make(map[string]GroupInfo, len(snap))
	for _, gi := range snap {
		byGroup[gi.Group] = gi
		assert.Equal(t, "TEST-POOL", gi.PoolCode, "every row carries this pool's code")
	}

	g1 := byGroup["g1"]
	assert.Equal(t, 2, g1.Buffered)
	assert.False(t, g1.Working)
	assert.True(t, g1.ParkedAt.IsZero(), "never parked (never had a drainer to lose)")
	assert.False(t, g1.Suppressed)

	g2 := byGroup["g2"]
	assert.Equal(t, 1, g2.Buffered)
	assert.True(t, g2.Working, "a drainer owns g2")

	g3 := byGroup["g3"]
	assert.Equal(t, 1, g3.Buffered)
	assert.False(t, g3.Working)
	assert.True(t, g3.ParkedAt.Equal(parkedSince), "parkedAt reported verbatim")

	g4 := byGroup["g4"]
	assert.True(t, g4.Suppressed)
	assert.False(t, g4.SuppressedUntil.IsZero(), "a suppressed group carries its expiry")
	assert.True(t, g4.SuppressedUntil.After(time.Now()), "expiry is in the future")
}

// A pool with no live groups reports an empty (non-nil-required, but
// definitely zero-length) snapshot — draining removes the groupQs entry
// entirely (see drainGroup's empty-buffer exit), so nothing lingers.
func TestPoolGroupSnapshot_Empty(t *testing.T) {
	p := &Pool{
		cfg:     common.PoolConfig{Code: "TEST-POOL"},
		groupQs: map[string]*groupQueue{},
		flushes: NewGroupFlushRegistry(),
	}
	assert.Empty(t, p.GroupSnapshot())
}
