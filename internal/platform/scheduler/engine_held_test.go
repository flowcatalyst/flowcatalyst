package scheduler

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// More than a batch of held rows at the head of the order must not starve the
// groups behind them: a group just found held is skipped by the next claims'
// walk, so it does not fill their batch.
func TestHeld_AHeldGroupDoesNotStarveTheGroupsBehindIt(t *testing.T) {
	held := groupJobs("a", 30) // group "a" sorts first and holds far more rows than a batch
	for _, j := range held {
		j.mode = "BLOCK_ON_ERROR"
	}
	s := newFakeStore(append(held, groupJobs("z", 2)...)...)
	p := newTestEngine(Config{PollInterval: 2 * time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 4}, s)
	p.holdBack = func(context.Context, map[string]jobKey) (map[string]jobKey, error) {
		return map[string]jobKey{"a": {sequence: -1, id: "holder"}}, nil // the head of "a" has failed
	}
	runEngine(t, p)
	require.Eventually(t, func() bool { return s.status("z-00") == "QUEUED" && s.status("z-01") == "QUEUED" }, 5*time.Second, time.Millisecond,
		"a job of an unheld group behind a batch-full of held rows must be published")
	assert.Equal(t, []string{"z-00", "z-01"}, s.published())
	assert.Equal(t, 1, p.held.size(), "group a is remembered as held")
	assert.Equal(t, 1.0, value(t, MetricsRegistry, "fc_scheduler_held_groups"))
}

// A held group is tried again after the memory's TTL.
func TestHeld_AHeldGroupIsTriedAgainAfterTheTTL(t *testing.T) {
	old := heldGroupTTL
	heldGroupTTL = 80 * time.Millisecond
	defer func() { heldGroupTTL = old }()

	jobs := groupJobs("a", 2)
	for _, j := range jobs {
		j.mode = "BLOCK_ON_ERROR"
	}
	s := newFakeStore(jobs...)
	p := newTestEngine(Config{PollInterval: 2 * time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 4}, s)
	var holding atomic.Bool
	holding.Store(true)
	p.holdBack = func(context.Context, map[string]jobKey) (map[string]jobKey, error) {
		if holding.Load() {
			return map[string]jobKey{"a": {sequence: -1, id: "holder"}}, nil
		}
		return map[string]jobKey{}, nil
	}
	runEngine(t, p)
	time.Sleep(40 * time.Millisecond)
	assert.Empty(t, s.published(), "held")
	claimsWhileHeld := s.claimCount()
	holding.Store(false) // the head resolves; the group is only reconsidered after the TTL
	require.Eventually(t, func() bool { return s.pending() == 0 }, 5*time.Second, time.Millisecond, "tried again after the TTL")
	assert.Equal(t, []string{"a-00", "a-01"}, s.published())
	assert.Greater(t, s.claimCount(), claimsWhileHeld)
}

// The memory is capped: past the cap the oldest entry is dropped.
func TestHeld_TheMemoryIsCappedAndForgetsTheOldest(t *testing.T) {
	h := newHeldGroups(time.Minute, 3)
	t0 := time.Now()
	for i := range 5 {
		h.add(fmt.Sprintf("g%d", i), t0.Add(time.Duration(i)*time.Second))
	}
	assert.Equal(t, 3, h.size())
	assert.ElementsMatch(t, []string{"g2", "g3", "g4"}, h.active(t0.Add(10*time.Second)), "the two oldest were dropped")
	assert.Empty(t, h.active(t0.Add(2*time.Minute)), "entries expire")
	assert.Zero(t, h.size())
	assert.NotNil(t, h.active(t0), "never nil: it goes into <> ALL($n)")
}
