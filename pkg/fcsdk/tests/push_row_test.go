package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/outbox"
	"github.com/flowcatalyst/go-sdk/outboxpgx"
	"github.com/flowcatalyst/go-sdk/tests/internal/pgtest"
)

// A PUSH row is written in the caller's transaction with the SDK's own outbox table, a
// notification on outbox.PushNotifyChannel goes out with the commit and not before, and a rollback
// leaves neither.
func TestWritePushStoresARowAndNotifiesOnCommit(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.Schema(t)
	sink := outboxpgx.NewSink(outboxpgx.Config{ClientID: "acme-id", ClientCode: "acme"})

	pooled, err := pool.Acquire(ctx)
	require.NoError(t, err)
	listener := pooled.Hijack()
	defer func() { _ = listener.Close(ctx) }()
	_, err = listener.Exec(ctx, "LISTEN "+pgx.Identifier{outbox.PushNotifyChannel}.Sanitize())
	require.NoError(t, err)
	notified := func(wait time.Duration) bool {
		c, cancel := context.WithTimeout(ctx, wait)
		defer cancel()
		_, err := listener.WaitForNotification(c)
		return err == nil
	}

	// rolled back: no row, no notification
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, sink.WritePush(ctx, tx, outbox.NewPush("shop:orders:1", "order.updated", map[string]any{"n": 0})))
	require.NoError(t, tx.Rollback(ctx))
	assert.False(t, notified(300*time.Millisecond))
	assert.Empty(t, pgtest.OutboxRows(t, pool))

	// committed: one row, one notification, only after the commit
	tx, err = pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, sink.WritePush(ctx, tx, outbox.NewPush("shop:orders:1", "order.updated", map[string]any{"n": 1}).WithID("push-0000000001")))
	assert.False(t, notified(200*time.Millisecond), "not before the commit")
	require.NoError(t, tx.Commit(ctx))
	assert.True(t, notified(2*time.Second), "at the commit")

	rows := pgtest.OutboxRows(t, pool)
	require.Len(t, rows, 1)
	r := rows[0]
	assert.Equal(t, "push-0000000001", r.ID)
	assert.Equal(t, "PUSH", r.Type)
	require.NotNil(t, r.MessageGroup)
	assert.Equal(t, "shop:orders:1", *r.MessageGroup)
	require.NotNil(t, r.ClientID)
	assert.Equal(t, "acme-id", *r.ClientID)
	assert.Zero(t, r.Status)
	assert.Zero(t, r.RetryCount)
	require.NotNil(t, r.PayloadSize)
	assert.Equal(t, len(r.Payload), *r.PayloadSize)
	var p map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.Payload), &p))
	assert.Equal(t, map[string]any{
		"topic": "shop:orders:1", "type": "order.updated", "data": map[string]any{"n": float64(1)},
		"clientCode": "acme", "pushId": "push-0000000001",
	}, p)

	// a push the platform would refuse is never written
	tx, err = pool.Begin(ctx)
	require.NoError(t, err)
	err = sink.WritePush(ctx, tx, outbox.NewPush("Shop:orders:1", "order.updated", map[string]any{}))
	require.Error(t, err)
	require.NoError(t, tx.Rollback(ctx))
	assert.Len(t, pgtest.OutboxRows(t, pool), 1)
}

// An ungrouped push is stored with a NULL message group.
func TestWritePushUngrouped(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.Schema(t)
	sink := outboxpgx.NewSink(outboxpgx.Config{})
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, sink.WritePush(ctx, tx, outbox.NewPush("shop:board", "refresh", map[string]any{}).WithUngrouped()))
	require.NoError(t, tx.Commit(ctx))
	rows := pgtest.OutboxRows(t, pool)
	require.Len(t, rows, 1)
	assert.Nil(t, rows[0].MessageGroup)
	assert.Nil(t, rows[0].ClientID)
}
