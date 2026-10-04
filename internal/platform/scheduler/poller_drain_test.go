package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type pollResult struct {
	claimed, published int
	err                error
}

// scriptedPoller returns the scripted results in order, then a short empty
// batch. It counts calls.
func scriptedPoller(results []pollResult, calls *int, onCall func(n int)) func(context.Context) (int, int, error) {
	return func(context.Context) (int, int, error) {
		n := *calls
		*calls++
		if onCall != nil {
			onCall(*calls)
		}
		if n < len(results) {
			r := results[n]
			return r.claimed, r.published, r.err
		}
		return 0, 0, nil
	}
}

func drainTestPoller(poll func(context.Context) (int, int, error)) *PendingJobPoller {
	// PollInterval is irrelevant to drain; a huge one proves nothing sleeps.
	return &PendingJobPoller{cfg: Config{BatchSize: 100, PollInterval: time.Hour}, poll: poll}
}

func TestDrain_FullBatchesRepollWithoutSleeping(t *testing.T) {
	calls := 0
	p := drainTestPoller(scriptedPoller([]pollResult{{100, 100, nil}, {100, 100, nil}, {100, 100, nil}, {40, 40, nil}}, &calls, nil))
	start := time.Now()
	p.drain(context.Background())
	assert.Equal(t, 4, calls, "three full batches then the short one, all in a single tick")
	assert.Less(t, time.Since(start), time.Second)
}

func TestDrain_FullClaimThatPublishesNothingDoesNotRepoll(t *testing.T) {
	calls := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// A spinning loop would be stopped by the cancel after a few calls so the
	// failure is an assertion, not a hang.
	always := make([]pollResult, 50)
	for i := range always {
		always[i] = pollResult{100, 0, nil}
	}
	p := drainTestPoller(scriptedPoller(always, &calls, func(n int) {
		if n >= 5 {
			cancel()
		}
	}))
	p.drain(ctx)
	assert.Equal(t, 1, calls)
}

func TestDrain_ShortBatchAndErrorStop(t *testing.T) {
	calls := 0
	drainTestPoller(scriptedPoller([]pollResult{{99, 99, nil}, {100, 100, nil}}, &calls, nil)).drain(context.Background())
	assert.Equal(t, 1, calls, "a short batch ends the drain")

	calls = 0
	drainTestPoller(scriptedPoller([]pollResult{{100, 100, nil}, {0, 0, errors.New("boom")}, {100, 100, nil}}, &calls, nil)).drain(context.Background())
	assert.Equal(t, 2, calls, "an error ends the drain")
}

func TestDrain_RechecksLeaderAndContextBeforeEveryPass(t *testing.T) {
	calls := 0
	leader := true
	p := drainTestPoller(scriptedPoller([]pollResult{{100, 100, nil}, {100, 100, nil}, {100, 100, nil}}, &calls, func(n int) {
		if n == 2 {
			leader = false
		}
	}))
	p.IsLeader = func() bool { return leader }
	p.drain(context.Background())
	assert.Equal(t, 2, calls, "leadership lost mid-drain stops the next pass")

	calls = 0
	ctx, cancel := context.WithCancel(context.Background())
	p = drainTestPoller(scriptedPoller([]pollResult{{100, 100, nil}, {100, 100, nil}}, &calls, func(n int) { cancel() }))
	p.drain(ctx)
	assert.Equal(t, 1, calls, "a cancelled ctx stops the next pass")
}

func TestStarveWarn_RateLimitedToOncePerMinute(t *testing.T) {
	p := drainTestPoller(nil)
	t0 := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	assert.True(t, p.starveWarnDue(t0), "first warning is due")
	assert.False(t, p.starveWarnDue(t0.Add(59*time.Second)), "suppressed inside the minute")
	assert.True(t, p.starveWarnDue(t0.Add(61*time.Second)), "due again after a minute")
	assert.False(t, p.starveWarnDue(t0.Add(62*time.Second)))
}
