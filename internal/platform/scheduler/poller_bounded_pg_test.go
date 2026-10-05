//go:build integration

package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stallPublisher blocks until its context ends, then reports everything
// unpublished (what the SQS publisher does when its calls time out).
type stallPublisher struct{}

func (stallPublisher) Publish(ctx context.Context, items []PublishItem) ([]string, error) {
	<-ctx.Done()
	return jobIDs(items), ctx.Err()
}

// A publish that never returns is cut off by the lane's publish deadline: the
// rows stay PENDING (no row is locked or marked while it stalls), and a
// following poll can claim them again.
func TestPollOnce_StalledPublishIsBoundedAndLeavesTheClaimPending(t *testing.T) {
	old := publishTimeout
	publishTimeout = 300 * time.Millisecond
	defer func() { publishTimeout = old }()

	ctx := context.Background()
	pool := testPool(t)
	const (
		id1 = "djbound00001"
		id2 = "djbound00002"
	)
	seedJob(t, pool, id1, "PENDING", "grp_bound_it01", "")
	seedJob(t, pool, id2, "PENDING", "grp_bound_it02", "")
	dispatcher := NewMessageGroupDispatcher(pool, stallPublisher{}, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process")
	poller := NewPendingJobPoller(DefaultConfig(), pool, dispatcher, NewPausedConnectionCache(pool, time.Minute))

	start := time.Now()
	claimed, _ := mustPoll(t, poller, ctx)
	assert.Less(t, time.Since(start), 10*time.Second, "the lane gave up instead of hanging")
	assert.GreaterOrEqual(t, claimed, 2)
	assert.Equal(t, "PENDING", jobStatus(t, pool, id1))
	assert.Equal(t, "PENDING", jobStatus(t, pool, id2))

	// Nothing was stranded: a healthy poll claims and publishes them.
	capture := &capturePublisher{}
	healthy := NewPendingJobPoller(DefaultConfig(), pool,
		NewMessageGroupDispatcher(pool, capture, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process"),
		NewPausedConnectionCache(pool, time.Minute))
	mustPoll(t, healthy, ctx)
	assert.Equal(t, "QUEUED", jobStatus(t, pool, id1))
}

// cancelAfterPublish publishes everything, then cancels the lanes' context, as a
// shutdown arriving between the publish and the status update would.
type cancelAfterPublish struct{ cancel context.CancelFunc }

func (c cancelAfterPublish) Publish(context.Context, []PublishItem) ([]string, error) {
	c.cancel()
	return nil, nil
}

// The mark-QUEUED UPDATE must survive that cancellation: abandoning it would
// leave the published rows PENDING and publish every job a second time.
func TestLane_MarkQueuedSurvivesACancelledContext(t *testing.T) {
	pool := testPool(t)
	const (
		id1 = "djcancel0001"
		id2 = "djcancel0002"
	)
	seedJob(t, pool, id1, "PENDING", "grp_cancel_it01", "")
	seedJob(t, pool, id2, "PENDING", "grp_cancel_it02", "")
	lctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dispatcher := NewMessageGroupDispatcher(pool, cancelAfterPublish{cancel}, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process")
	poller := NewPendingJobPoller(DefaultConfig(), pool, dispatcher, NewPausedConnectionCache(pool, time.Minute))

	wg := poller.startLanes(lctx)
	res := poller.claimOnce(context.Background())
	require.NoError(t, res.err)
	require.Equal(t, 2, res.submitted)
	wg.Wait() // the lanes exit once their batch is settled
	assert.Equal(t, "QUEUED", jobStatus(t, pool, id1))
	assert.Equal(t, "QUEUED", jobStatus(t, pool, id2))
}
