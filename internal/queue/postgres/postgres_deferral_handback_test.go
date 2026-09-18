//go:build integration

package postgres

// A delayed group head blocks its successors (R4, owner ruling 2026-09-17,
// docs/spec/router-deferral-handback.md), ported from the Java reference
// (flowcatalyst-javalin 5420b516, PostgresQueueTest#delayedHeadBlocksOnlyItsOwnGroupsSuccessor
// / #claimedHeadStillDoesNotBlockItsGroup / #delayedHeadComesBackBeforeItsGroup).

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

func secs(n uint32) *uint32 { return &n }

func groupPtr(g string) *string {
	if g == "" {
		return nil
	}
	return &g
}

// insertQueued plants a row directly (bypassing Publish) with explicit
// created_at/visible_at, so a group's internal ordering can be controlled
// precisely — mirroring insertRaw's role in postgres_pg_test.go but with the
// timing/grouping columns this suite needs and insertRaw hard-codes away.
func insertQueued(t *testing.T, q *Queue, id, group string, createdAt, visibleAt int64) {
	t.Helper()
	payload, err := json.Marshal(common.Message{
		ID: id, MediationType: common.MediationTypeHTTP, MediationTarget: "http://t/" + id,
		MessageGroupID: groupPtr(group),
	})
	require.NoError(t, err)

	var groupCol any
	if group != "" {
		groupCol = group
	}
	_, err = q.pool.Exec(context.Background(),
		`INSERT INTO queue_messages (id, queue_name, message_group_id, visible_at, payload, created_at, receive_count)
		 VALUES ($1, $2, $3, $4, $5, $6, 0)`,
		id, q.cfg.Name, groupCol, visibleAt, string(payload), createdAt)
	require.NoError(t, err)
}

func setVisibleAt(t *testing.T, q *Queue, id string, visibleAt int64) {
	t.Helper()
	_, err := q.pool.Exec(context.Background(),
		`UPDATE queue_messages SET visible_at = $1 WHERE queue_name = $2 AND id = $3`,
		visibleAt, q.cfg.Name, id)
	require.NoError(t, err)
}

func idsOf(msgs []common.QueuedMessage) []string {
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.BrokerMessageID
	}
	return ids
}

// T9: a group head nacked with a delay blocks its own group's successor, but
// not another group's row or an ungrouped row. m2 is inserted only AFTER m1
// is nacked — before that instant it did not exist, so this also rules out
// "m2 merely arrived too late to be claimed alongside m1" as an alternative
// explanation for it being absent from the second poll.
func TestDelayedHeadBlocksOnlyItsOwnGroupsSuccessor(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, "r4-block-test")
	group := "grp-r4"
	now := time.Now().Unix()

	insertQueued(t, q, "m1", group, now, now)

	first, err := q.Poll(ctx, 10)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Equal(t, "m1", first[0].BrokerMessageID)

	require.NoError(t, q.Nack(ctx, first[0].ReceiptHandle, secs(600)))

	// Published only now — after the block was established.
	insertQueued(t, q, "m2", group, now, now)
	insertQueued(t, q, "other-group-row", "other-grp", now, now)
	insertQueued(t, q, "ungrouped-row", "", now, now)

	second, err := q.Poll(ctx, 10)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"other-group-row", "ungrouped-row"}, idsOf(second),
		"m2 is behind a head nacked with a delay; another group and an ungrouped row are unaffected")
}

// T10: a CLAIMED (not nacked) head still does not block its group — today's
// behaviour, unchanged by R4. Only the NEW clause (nacked-with-delay) may
// block; the pre-existing "claimed" state must not gain blocking power it
// never had.
func TestClaimedHeadStillDoesNotBlockItsGroup(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, "r4-claimed-test")
	group := "grp-r4-claimed"
	now := time.Now().Unix()

	insertQueued(t, q, "m1", group, now, now)
	insertQueued(t, q, "m2", group, now, now)

	first, err := q.Poll(ctx, 10)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Equal(t, "m1", first[0].BrokerMessageID)

	// m1 stays claimed — never nacked.
	second, err := q.Poll(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"m2"}, idsOf(second), "a claimed, in-flight head must not block its group on a later poll")
}

// T11: once a delayed head becomes visible again, it is claimed FIRST, not
// overtaken by its own group's successor — order is restored, not merely
// blocked.
func TestDelayedHeadComesBackBeforeItsGroup(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, "r4-order-test")
	group := "grp-r4-order"
	now := time.Now().Unix()

	insertQueued(t, q, "m1", group, now, now)

	first, err := q.Poll(ctx, 10)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Equal(t, "m1", first[0].BrokerMessageID)

	require.NoError(t, q.Nack(ctx, first[0].ReceiptHandle, secs(600)))
	insertQueued(t, q, "m2", group, now, now)

	// The delay has not elapsed: m2 stays blocked (T9's rule).
	stillBlocked, err := q.Poll(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, stillBlocked)

	// Move the delay into the past directly — this test is about ORDER, not
	// about actually waiting out 600s.
	setVisibleAt(t, q, "m1", now-10)

	afterVisible, err := q.Poll(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"m1"}, idsOf(afterVisible),
		"the returned head comes back first — not overtaken by its own successor")
}
