package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A lane forgets only the in-flight entries that are its own: an entry a newer claim
// re-added for the same id (it has a newer generation) must survive, or the copy
// waiting in a lane is invisible to the claims that follow.
func TestInflight_ASettledBatchDoesNotForgetANewerEntry(t *testing.T) {
	set := newInflightSet()
	set.add([]laneJob{lj("g-00", "g", 25)})
	set.removeSettled([]laneJob{lj("g-00", "g", 24)})
	assert.Equal(t, 1, set.size(), "the older claim's settle leaves the newer entry")
	set.removeSettled([]laneJob{lj("g-00", "g", 25), lj("not-there", "g", 25)})
	assert.Zero(t, set.size(), "a settle of its own entry removes it")
}

// A claim landing just before a failed batch is removed from the in-flight set still
// excludes the batch; it takes the jobs BEHIND it, which carry a generation the
// poison mark covers and are dropped. Nothing overtakes: the group is published
// once, in order.
func TestOrdering_AClaimJustBeforeTheSettleCannotOvertake(t *testing.T) {
	s := newFakeStore(groupJobs("g", 4)...)
	inPublish := make(chan struct{})
	gate := make(chan struct{})
	var calls atomic.Int32
	s.publishFn = func(_ context.Context, toks []DispatchJobToken) []string {
		if calls.Add(1) == 1 {
			close(inPublish)
			<-gate
			return tokenIDs(toks) // the broker rejects the first batch
		}
		for _, tk := range toks {
			s.logPublished(tk.JobID)
		}
		return nil
	}
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 2, LaneBatch: 100}, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := p.startLanes(ctx)
	defer func() { cancel(); wg.Wait() }()

	require.Equal(t, 2, p.claimOnce(ctx).submitted) // g-00, g-01 -> the lane, blocked in publish
	<-inPublish

	var mu sync.Mutex
	var claimed int
	var once sync.Once
	p.hookReleased = func() {
		once.Do(func() {
			res := p.claimOnce(ctx) // g-00, g-01 are still in flight: it takes g-02, g-03
			mu.Lock()
			claimed = res.submitted
			mu.Unlock()
		})
	}
	close(gate)
	waitIdle(t, p)
	mu.Lock()
	assert.Equal(t, 2, claimed, "the in-flight jobs are excluded; the jobs behind them are claimed")
	mu.Unlock()
	require.Empty(t, s.published(), "the jobs behind a failed batch are dropped, never published ahead of it")
	for range 10 {
		if s.pending() == 0 {
			break
		}
		require.NoError(t, p.claimOnce(ctx).err)
		waitIdle(t, p)
	}
	require.Equal(t, []string{"g-00", "g-01", "g-02", "g-03"}, s.published())
	require.Zero(t, p.inflight.size())
}

// The claim excludes this process's in-flight ids; a row that comes back anyway is
// dropped and counted, never published twice.
func TestClaim_ARowAlreadyInFlightIsDroppedAndCounted(t *testing.T) {
	s := newFakeStore(ungroupedJobs("u", 3)...)
	p := newTestEngine(Config{PollInterval: time.Millisecond, Dispatchers: 1, BufferCapacity: 10, BatchSize: 10}, s)
	p.claimRows = func(ctx context.Context, limit int, paused, held, _ []string) ([]dispatchClaim, error) {
		return s.claimRows(ctx, limit, paused, held, []string{}) // a claim that ignores the exclusion
	}
	p.inflight.add([]laneJob{lj("u00", "", 1)})
	before := value(t, MetricsRegistry, "fc_scheduler_claims_already_inflight_total")
	res := settleOnce(t, p, context.Background())
	assert.Equal(t, 2, res.submitted)
	assert.Equal(t, []string{"u01", "u02"}, s.published())
	assert.Equal(t, 1.0, value(t, MetricsRegistry, "fc_scheduler_claims_already_inflight_total")-before)
}
