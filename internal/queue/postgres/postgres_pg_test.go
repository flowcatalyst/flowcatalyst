//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

func newTestQueue(t *testing.T, name string) *Queue {
	t.Helper()
	q := &Queue{
		pool: testpg.Pool(t),
		cfg:  common.QueueConfig{Name: name, VisibilityTimeout: 30},
	}
	require.NoError(t, q.InitSchema(context.Background()))
	return q
}

// insertRaw writes a row straight into the queue table, bypassing Publish, so a
// payload that could never be produced by Publish can be planted.
func insertRaw(t *testing.T, q *Queue, id, payload string) {
	t.Helper()
	_, err := q.pool.Exec(context.Background(),
		`INSERT INTO queue_messages (id, queue_name, message_group_id, visible_at, payload, created_at, receive_count)
		 VALUES ($1, $2, NULL, 0, $3, 0, 0)`,
		id, q.cfg.Name, payload)
	require.NoError(t, err)
}

// TestPollQuarantinesMalformedRow is the poison-queue guard. The claim commits
// before the payload is parsed, so a row that can't be unmarshalled used to
// abort the whole poll: it stayed claimed, became visible again, was re-claimed,
// and failed identically forever — taking every other message claimed in the
// same batch down with it on every attempt. One bad row stopped the queue.
//
// It must now be moved to queue_messages_failed and the poll must succeed.
func TestPollQuarantinesMalformedRow(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, "poison-q")

	insertRaw(t, q, "bad-1", "{ this is not json")

	msgs, err := q.Poll(ctx, 10)

	require.NoError(t, err, "a malformed row must not fail the poll")
	require.Empty(t, msgs)

	// Gone from the queue…
	var remaining int
	require.NoError(t, q.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM queue_messages WHERE queue_name = $1`, q.cfg.Name).Scan(&remaining))
	require.Zero(t, remaining, "the poison row must not stay claimable")

	// …and recorded, with the payload and reason kept for diagnosis.
	var payload, reason string
	require.NoError(t, q.pool.QueryRow(ctx,
		`SELECT payload, error_message FROM queue_messages_failed WHERE queue_name = $1 AND id = $2`,
		q.cfg.Name, "bad-1").Scan(&payload, &reason))
	require.Equal(t, "{ this is not json", payload)
	require.NotEmpty(t, reason)

	// The queue keeps working rather than looping on the same row.
	msgs, err = q.Poll(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, msgs)
}

// A row that fails, is requeued and fails again is almost always being worked
// on — someone changed the payload, the schema, or the consumer. So the
// quarantine record keeps the LATEST failure: the one that describes what is
// wrong now. Keeping the first pinned the record to the original attempt and
// discarded every later diagnosis.
func TestRequarantineKeepsTheLatestFailure(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, "poison-requeue-q")

	insertRaw(t, q, "bad-repeat", "{ first attempt")
	_, err := q.Poll(ctx, 10)
	require.NoError(t, err)

	var payload, reason string
	require.NoError(t, q.pool.QueryRow(ctx,
		`SELECT payload, error_message FROM queue_messages_failed WHERE queue_name = $1 AND id = $2`,
		q.cfg.Name, "bad-repeat").Scan(&payload, &reason))
	require.Equal(t, "{ first attempt", payload)
	firstReason := reason

	// Same id requeued with a different bad payload — a second diagnosis.
	insertRaw(t, q, "bad-repeat", `{"still": broken}`)
	_, err = q.Poll(ctx, 10)
	require.NoError(t, err)

	require.NoError(t, q.pool.QueryRow(ctx,
		`SELECT payload, error_message FROM queue_messages_failed WHERE queue_name = $1 AND id = $2`,
		q.cfg.Name, "bad-repeat").Scan(&payload, &reason))
	assert.Equal(t, `{"still": broken}`, payload, "the record must describe the LATEST attempt")
	assert.NotEqual(t, firstReason, reason, "and carry the latest failure, not the first")

	var rows int
	require.NoError(t, q.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM queue_messages_failed WHERE queue_name = $1 AND id = $2`,
		q.cfg.Name, "bad-repeat").Scan(&rows))
	assert.Equal(t, 1, rows, "one row per quarantined message, updated in place")
}

// TestPollDeliversGoodMessagesAlongsideAPoisonRow: the batch must survive. The
// old behaviour returned an error for the whole poll, so healthy messages
// claimed in the same batch were stranded — invisible until their visibility
// lapsed, then stranded again by the same bad row.
func TestPollDeliversGoodMessagesAlongsideAPoisonRow(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, "mixed-q")

	_, err := q.Publish(ctx, common.Message{
		ID: "good-1", MediationType: common.MediationTypeHTTP, MediationTarget: "http://t/1",
	})
	require.NoError(t, err)
	insertRaw(t, q, "bad-1", "not json at all")
	_, err = q.Publish(ctx, common.Message{
		ID: "good-2", MediationType: common.MediationTypeHTTP, MediationTarget: "http://t/2",
	})
	require.NoError(t, err)

	msgs, err := q.Poll(ctx, 10)

	require.NoError(t, err)
	got := make([]string, 0, len(msgs))
	for _, m := range msgs {
		got = append(got, m.Message.ID)
	}
	require.ElementsMatch(t, []string{"good-1", "good-2"}, got,
		"healthy messages in the batch must still be delivered")

	var quarantined int
	require.NoError(t, q.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM queue_messages_failed WHERE queue_name = $1`, q.cfg.Name).Scan(&quarantined))
	require.Equal(t, 1, quarantined)
}

// TestQuarantineIsIdempotent: re-quarantining the same id must not error, so a
// row that somehow reappears can always be removed from the queue.
func TestQuarantineIsIdempotent(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, "idem-q")

	insertRaw(t, q, "bad-1", "{")
	_, err := q.Poll(ctx, 10)
	require.NoError(t, err)

	insertRaw(t, q, "bad-1", "{")
	_, err = q.Poll(ctx, 10)
	require.NoError(t, err, "a second quarantine of the same id must not fail")

	var remaining int
	require.NoError(t, q.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM queue_messages WHERE queue_name = $1`, q.cfg.Name).Scan(&remaining))
	require.Zero(t, remaining, "the row must be removed from the queue on the retry too")
}

// TestPollLeavesHealthyQueueUntouched: the quarantine path must not perturb the
// normal case — no error rows, ordinary delivery.
func TestPollLeavesHealthyQueueUntouched(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, "healthy-q")

	_, err := q.Publish(ctx, common.Message{
		ID: "ok-1", MediationType: common.MediationTypeHTTP, MediationTarget: "http://t/1",
	})
	require.NoError(t, err)

	msgs, err := q.Poll(ctx, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, "ok-1", msgs[0].Message.ID)

	var quarantined int
	require.NoError(t, q.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM queue_messages_failed WHERE queue_name = $1`, q.cfg.Name).Scan(&quarantined))
	require.Zero(t, quarantined)
}
