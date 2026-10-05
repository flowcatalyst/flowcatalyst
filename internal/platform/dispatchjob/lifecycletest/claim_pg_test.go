//go:build integration

package lifecycletest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

func ids(cs []dispatchjob.ClaimedJob) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

// The claim is a plain read of the PENDING jobs: due ones, in delivery order — group
// (ungrouped last), sequence, created_at, id — outside the paused subscriptions, the held
// groups and the caller's in-flight ids, up to the limit. It writes nothing: claiming twice
// returns the same rows.
func TestClaimPending_OrderAndExclusions(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	_, err := pool.Exec(ctx, `DELETE FROM msg_dispatch_jobs`)
	require.NoError(t, err)

	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	mk := func(group string, seq int32, age time.Duration) dispatchjob.DispatchJob {
		j := newJob(group, common.DispatchBlockOnError, seq)
		j.CreatedAt = base.Add(age)
		if group == "" {
			j.MessageGroup = nil
		}
		return j
	}
	b2, a2 := mk("b", 2, 3*time.Second), mk("a", 2, 2*time.Second)
	b1, a1late, a1early := mk("b", 1, 5*time.Second), mk("a", 1, 9*time.Second), mk("a", 1, time.Second)
	u1, u2 := mk("", 1, time.Second), mk("", 1, 2*time.Second)
	future := mk("a", 3, 0)
	soon := time.Now().Add(time.Hour)
	future.ScheduledFor = &soon
	paused := mk("a", 4, 0)
	pausedSub := "sub-paused-" + tsid.GenerateUntyped()[:6]
	paused.SubscriptionID = &pausedSub
	rows, err := lc.CreateBatch(ctx, []dispatchjob.DispatchJob{b2, a2, b1, a1late, a1early, u1, u2, future, paused})
	require.NoError(t, err)
	require.Len(t, rows, 9)

	got, err := lc.ClaimPending(ctx, 100, []string{pausedSub}, []string{"b"}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{a1early.ID, a1late.ID, a2.ID, u1.ID, u2.ID}, ids(got),
		"group, sequence, created_at order; ungrouped last; the future, paused and held rows are left")

	again, err := lc.ClaimPending(ctx, 100, []string{pausedSub}, []string{"b"}, nil)
	require.NoError(t, err)
	assert.Equal(t, ids(got), ids(again), "the claim writes nothing: claiming again returns the same rows")

	// the in-flight exclusion keeps a process's own claimed jobs out of its next claim
	next, err := lc.ClaimPending(ctx, 100, []string{pausedSub}, []string{"b"}, []string{a1early.ID, a2.ID})
	require.NoError(t, err)
	assert.Equal(t, []string{a1late.ID, u1.ID, u2.ID}, ids(next))

	// the limit bounds the claim to the first rows in order; nil arrays are empty
	two, err := lc.ClaimPending(ctx, 2, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{a1early.ID, a1late.ID}, ids(two))

	// the row carries what the scheduler publishes from
	assert.Equal(t, "BLOCK_ON_ERROR", got[0].Mode)
	assert.EqualValues(t, 1, got[0].Sequence)
	var updatedAt time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT updated_at FROM msg_dispatch_jobs WHERE id = $1`, a1early.ID).Scan(&updatedAt))
	assert.True(t, updatedAt.Equal(got[0].UpdatedAt), "UpdatedAt is the version mark-QUEUED checks")

	// the queue table is gone (migration 068)
	var reg *string
	require.NoError(t, pool.QueryRow(ctx, `SELECT to_regclass('msg_dispatch_queue')::text`).Scan(&reg))
	assert.Nil(t, reg, "msg_dispatch_queue must not exist")
}

// The backlog sample: PENDING count (capped) and the age of the first due job in claim order.
func TestPendingBacklog(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	_, err := pool.Exec(ctx, `DELETE FROM msg_dispatch_jobs`)
	require.NoError(t, err)
	n, oldest, err := lc.PendingBacklog(ctx)
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Zero(t, oldest)

	j1 := newJob("g-"+tsid.GenerateUntyped(), common.DispatchBlockOnError, 1)
	j1.CreatedAt = time.Now().Add(-90 * time.Second).UTC().Truncate(time.Microsecond)
	j2 := newJob("h-"+tsid.GenerateUntyped(), common.DispatchBlockOnError, 1)
	soon := time.Now().Add(time.Hour)
	j2.ScheduledFor = &soon
	_, err = lc.CreateBatch(ctx, []dispatchjob.DispatchJob{j1, j2})
	require.NoError(t, err)
	n, oldest, err = lc.PendingBacklog(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 2, n, "both are PENDING")
	assert.InDelta(t, 90, oldest.Seconds(), 5, "the first DUE job in claim order")
}
