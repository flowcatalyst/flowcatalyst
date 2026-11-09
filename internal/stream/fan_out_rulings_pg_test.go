//go:build integration

package stream

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Owner rulings 2026-11-09, at fan-out: the job's group carries its
// subscription; delaySeconds and maxAgeSeconds are copied onto the job as
// not_before / expires_at, absolute against the event's created_at.
func TestFanOut_GroupCarriesTheSubscriptionAndDelayAgeAreCopied(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()

	_, err := pool.Exec(ctx,
		`INSERT INTO msg_subscriptions (id, code, name, target, status, mode, source, delay_seconds, max_age_seconds)
		 VALUES ('sub_fo_rul_a', 'fo-rul-a', 'A', 'https://a.fanout.test/hook', 'ACTIVE', 'BLOCK_ON_ERROR', 'UI', 30, 3600),
		        ('sub_fo_rul_b', 'fo-rul-b', 'B', 'https://b.fanout.test/hook', 'ACTIVE', 'BLOCK_ON_ERROR', 'UI', 0, 0)`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO msg_subscription_event_types (subscription_id, event_type_code)
		 VALUES ('sub_fo_rul_a', 'forul:test:evt'), ('sub_fo_rul_b', 'forul:test:evt')`)
	require.NoError(t, err)

	long := strings.Repeat("g", 195)
	_, err = pool.Exec(ctx,
		`INSERT INTO msg_events (id, type, source, time, message_group) VALUES
		   ('evt_forul_1', 'forul:test:evt', 'test://fanout', NOW(), 'order-1'),
		   ('evt_forul_2', 'forul:test:evt', 'test://fanout', NOW(), NULL),
		   ('evt_forul_3', 'forul:test:evt', 'test://fanout', NOW(), $1)`, long)
	require.NoError(t, err)

	_, err = NewFanOutWithConfig(pool, FanOutConfig{SubscriptionTTL: time.Nanosecond}).step(ctx, 100)
	require.NoError(t, err)

	type job struct {
		group                *string
		notBefore, expiresAt *time.Time
		createdAt            time.Time
	}
	read := func(eventID, subID string) job {
		var j job
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT message_group, not_before, expires_at, created_at FROM msg_dispatch_jobs
			  WHERE event_id = $1 AND subscription_id = $2`, eventID, subID).
			Scan(&j.group, &j.notBefore, &j.expiresAt, &j.createdAt))
		return j
	}

	a, b := read("evt_forul_1", "sub_fo_rul_a"), read("evt_forul_1", "sub_fo_rul_b")
	require.NotNil(t, a.group)
	require.NotNil(t, b.group)
	assert.Equal(t, "sub_fo_rul_a:order-1", *a.group, "the subscription's view of the event's group")
	assert.Equal(t, "sub_fo_rul_b:order-1", *b.group)

	require.NotNil(t, a.notBefore)
	require.NotNil(t, a.expiresAt)
	assert.True(t, a.createdAt.Add(30*time.Second).Equal(*a.notBefore), "created_at + delaySeconds")
	assert.True(t, a.createdAt.Add(time.Hour).Equal(*a.expiresAt), "created_at + maxAgeSeconds")
	assert.Nil(t, b.notBefore, "no delay: none")
	assert.Nil(t, b.expiresAt, "no age limit: none")

	assert.Nil(t, read("evt_forul_2", "sub_fo_rul_a").group, "an ungrouped event raises unordered jobs")

	d := read("evt_forul_3", "sub_fo_rul_a")
	require.NotNil(t, d.group)
	assert.LessOrEqual(t, len(*d.group), dispatchjob.MaxMessageGroupLen, "fits the column")
	assert.Equal(t, *dispatchjob.SubscriptionMessageGroup("sub_fo_rul_a", &long), *d.group)
}
