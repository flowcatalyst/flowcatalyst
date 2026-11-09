//go:build integration

package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/stream"
)

// Owner ruling 2026-11-09: the message group carries the subscription, so a
// BLOCK_ON_ERROR failure blocks only its own subscriber. Two subscriptions
// receive the same three events of one event group (real fan-out); A's first
// job fails terminally. A's later jobs are held — at the claim, at the
// delivery-time check, and by the reaper — while B's three flow, in order,
// under B's own group. Before the change both shared the event's bare group
// and A's failure held B too.
func TestPerSubscriptionGroup_OneSubscribersFailureDoesNotBlockAnother(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	_, err := pool.Exec(ctx,
		`INSERT INTO msg_subscriptions (id, code, name, target, status, mode, source, sequence)
		 VALUES ('sub_psg_a', 'psg-a', 'A', 'https://a.psg.test/hook', 'ACTIVE', 'BLOCK_ON_ERROR', 'UI', 1),
		        ('sub_psg_b', 'psg-b', 'B', 'https://b.psg.test/hook', 'ACTIVE', 'BLOCK_ON_ERROR', 'UI', 2)`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO msg_subscription_event_types (subscription_id, event_type_code)
		 VALUES ('sub_psg_a', 'psg:orders:order:changed'), ('sub_psg_b', 'psg:orders:order:changed')`)
	require.NoError(t, err)
	base := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	for i, id := range []string{"evt_psg_1", "evt_psg_2", "evt_psg_3"} {
		at := base.Add(time.Duration(i) * time.Second)
		_, err = pool.Exec(ctx,
			`INSERT INTO msg_events (id, type, source, time, message_group, created_at)
			 VALUES ($1, 'psg:orders:order:changed', 'psg:orders', $2, 'order-9', $2)`, id, at)
		require.NoError(t, err)
	}
	_, err = stream.NewFanOutWithConfig(pool, stream.FanOutConfig{SubscriptionTTL: time.Nanosecond}).
		Projector(stream.ProjectorConfig{}).Step(ctx, 100)
	require.NoError(t, err)

	jobOf := func(eventID, subID string) string {
		var id string
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT id FROM msg_dispatch_jobs WHERE event_id = $1 AND subscription_id = $2`, eventID, subID).Scan(&id))
		return id
	}
	a1, a2, a3 := jobOf("evt_psg_1", "sub_psg_a"), jobOf("evt_psg_2", "sub_psg_a"), jobOf("evt_psg_3", "sub_psg_a")
	b1, b2, b3 := jobOf("evt_psg_1", "sub_psg_b"), jobOf("evt_psg_2", "sub_psg_b"), jobOf("evt_psg_3", "sub_psg_b")

	// A's head fails terminally.
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET status = 'FAILED' WHERE id = $1`, a1)
	require.NoError(t, err)

	msgs := pollAndCapture(t, pool)
	var published []string
	groups := map[string]string{}
	for _, m := range msgs {
		published = append(published, m.ID)
		if m.MessageGroupID != nil {
			groups[m.ID] = *m.MessageGroupID
		}
	}
	assert.Equal(t, []string{b1, b2, b3}, published,
		"B's jobs flow, in order; A's are held behind A's failure and nothing else")
	assert.Equal(t, "sub_psg_b:order-9", groups[b1], "published under B's own group")
	for _, id := range []string{a2, a3} {
		assert.Equal(t, "PENDING", jobStatus(t, pool, id), "A's later jobs are held")
	}

	// The delivery-time check agrees: A's later job is held, B's is not.
	repo := dispatchjob.NewRepository(pool)
	posOf := func(id string) (string, int32, time.Time) {
		var g string
		var seq int32
		var at time.Time
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT message_group, sequence, created_at FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&g, &seq, &at))
		return g, seq, at
	}
	g, seq, at := posOf(a2)
	held, err := repo.GroupHeldBefore(ctx, g, seq, at, a2)
	require.NoError(t, err)
	assert.True(t, held, "A's second job is held by A's failure")
	g, seq, at = posOf(b2)
	held, err = repo.GroupHeldBefore(ctx, g, seq, at, b2)
	require.NoError(t, err)
	assert.False(t, held, "B's second job is not held by A's failure")

	// The reaper resets only A's stranded sibling, never B's in-flight job.
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET status = 'QUEUED' WHERE id = ANY($1)`, []string{a3, b3})
	require.NoError(t, err)
	reset, err := repo.SweepStrandedGroupSiblings(ctx, time.Hour)
	require.NoError(t, err)
	assert.Contains(t, reset, a3)
	assert.NotContains(t, reset, b3)
	assert.Equal(t, "QUEUED", jobStatus(t, pool, b3))
}

// A SUSPENDED or ARCHIVED pool's jobs wait: the claim leaves them PENDING and
// publishes nothing for them; an ACTIVE pool's job (and a pool-less one) is
// published. Resuming the pool releases its job.
func TestPollOnce_HeldPoolJobsWait(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_dispatch_pools (id, code, name, status) VALUES
		   ('dpl_heldsusp01', 'held-susp', 'S', 'SUSPENDED'),
		   ('dpl_heldarch01', 'held-arch', 'A', 'ARCHIVED'),
		   ('dpl_heldactv01', 'held-actv', 'X', 'ACTIVE')
		 ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status`)
	require.NoError(t, err)
	seed := func(id string, poolID *string) {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_dispatch_jobs (id, code, target_url, status, dispatch_pool_id, created_at, updated_at)
			 VALUES ($1, 'held:evt', 'http://sub.example/hook', 'PENDING', $2, NOW(), NOW())`, id, poolID)
		require.NoError(t, err)
	}
	seed("djheldsusp01", new("dpl_heldsusp01"))
	seed("djheldarch01", new("dpl_heldarch01"))
	seed("djheldactv01", new("dpl_heldactv01"))
	seed("djheldnone01", nil)

	var published []string
	for _, m := range pollAndCapture(t, pool) {
		published = append(published, m.ID)
	}
	assert.ElementsMatch(t, []string{"djheldactv01", "djheldnone01"}, published)
	assert.Equal(t, "PENDING", jobStatus(t, pool, "djheldsusp01"), "a suspended pool's job waits")
	assert.Equal(t, "PENDING", jobStatus(t, pool, "djheldarch01"), "an archived pool's job waits")

	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_pools SET status = 'ACTIVE' WHERE id = 'dpl_heldsusp01'`)
	require.NoError(t, err)
	published = nil
	for _, m := range pollAndCapture(t, pool) {
		published = append(published, m.ID)
	}
	assert.Contains(t, published, "djheldsusp01", "resumed: released")
}
