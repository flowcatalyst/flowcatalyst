//go:build integration

package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// stallPublisher blocks until its context ends, then reports everything
// unpublished (what the SQS publisher does when its calls time out).
type stallPublisher struct{}

func (stallPublisher) Publish(ctx context.Context, items []PublishItem) ([]string, error) {
	<-ctx.Done()
	return jobIDs(items), ctx.Err()
}

// A publish that never returns is cut off by the whole-poll deadline: the
// claim rolls back, the rows stay PENDING, and their locks are released (a
// following poll can claim them again).
func TestPollOnce_StalledPublishIsBoundedAndLeavesTheClaimPending(t *testing.T) {
	old := pollTimeout
	pollTimeout = 300 * time.Millisecond
	defer func() { pollTimeout = old }()

	ctx := context.Background()
	pool := testpg.Pool(t)
	const (
		id1 = "djbound00001"
		id2 = "djbound00002"
	)
	seedJob(t, pool, id1, "PENDING", "grp_bound_it01", "")
	seedJob(t, pool, id2, "PENDING", "grp_bound_it02", "")
	dispatcher := NewMessageGroupDispatcher(pool, stallPublisher{}, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process")
	poller := NewPendingJobPoller(DefaultConfig(), pool, dispatcher, NewPausedConnectionCache(pool, time.Minute))

	start := time.Now()
	_, published := mustPoll(t, poller, ctx)
	assert.Less(t, time.Since(start), 10*time.Second, "pollOnce returned instead of hanging")
	assert.Equal(t, 0, published)
	assert.Equal(t, "PENDING", jobStatus(t, pool, id1))
	assert.Equal(t, "PENDING", jobStatus(t, pool, id2))

	// Locks were released: a healthy poll claims and publishes them.
	capture := &capturePublisher{}
	healthy := NewPendingJobPoller(DefaultConfig(), pool,
		NewMessageGroupDispatcher(pool, capture, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process"),
		NewPausedConnectionCache(pool, time.Minute))
	mustPoll(t, healthy, ctx)
	assert.Equal(t, "QUEUED", jobStatus(t, pool, id1))
}

// cancelAfterPublish publishes everything, then cancels the poll's parent
// context, as a shutdown arriving between the publish and the commit would.
type cancelAfterPublish struct{ cancel context.CancelFunc }

func (c cancelAfterPublish) Publish(context.Context, []PublishItem) ([]string, error) {
	c.cancel()
	return nil, nil
}

// The mark-QUEUED UPDATE and COMMIT must survive that cancellation: abandoning
// them would roll the claim back and publish every job a second time.
func TestPollOnce_MarkAndCommitSurviveACancelledContext(t *testing.T) {
	pool := testpg.Pool(t)
	const (
		id1 = "djcancel0001"
		id2 = "djcancel0002"
	)
	seedJob(t, pool, id1, "PENDING", "grp_cancel_it01", "")
	seedJob(t, pool, id2, "PENDING", "grp_cancel_it02", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dispatcher := NewMessageGroupDispatcher(pool, cancelAfterPublish{cancel}, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process")
	poller := NewPendingJobPoller(DefaultConfig(), pool, dispatcher, NewPausedConnectionCache(pool, time.Minute))

	_, published, err := poller.pollOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, published)
	assert.Equal(t, "QUEUED", jobStatus(t, pool, id1))
	assert.Equal(t, "QUEUED", jobStatus(t, pool, id2))
}
