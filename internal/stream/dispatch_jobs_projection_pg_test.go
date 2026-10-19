//go:build integration

package stream

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The list filters on the read projection's `code` (the event type) and
// `connection_id` (migration 086). Pinned end to end: a job raised by a subscription
// carries its event's type as its code and projects with no connection of its own; a
// job that names a connection (fn.dispatch) projects it, and a later projection of the
// same job keeps it (the upsert's DO UPDATE sets it).
func TestDispatchJobProjection_EventTypeAndConnection(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()

	insertActiveSubscription(t, pool, "sub_pj_evt", "pj-evt", "projection:evt:type", nil)
	insertUnfannedEvent(t, pool, "evt_pj_01", "projection:evt:type")
	n, err := NewFanOut(pool).step(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// A job that names its own connection, as a function's fn.dispatch does.
	_, err = pool.Exec(ctx, `
		INSERT INTO msg_dispatch_jobs (id, code, target_url, status, connection_id, created_at, updated_at)
		VALUES ('pjfn000000001', 'projection:function:f:dispatch', 'http://x', 'PENDING', 'con_pjfn0000001', now(), now())`)
	require.NoError(t, err)

	proj := NewDispatchJobProjection(pool)
	for {
		moved, err := proj.step(ctx, 100)
		require.NoError(t, err)
		if moved == 0 {
			break
		}
	}

	var code string
	var conn *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT code, connection_id FROM msg_dispatch_jobs_read WHERE event_id = 'evt_pj_01'`).Scan(&code, &conn))
	assert.Equal(t, "projection:evt:type", code, "a subscription's job carries the event type as its code")
	assert.Nil(t, conn, "a subscription's job names no connection of its own (its subscription's is looked up)")

	require.NoError(t, pool.QueryRow(ctx,
		`SELECT code, connection_id FROM msg_dispatch_jobs_read WHERE id = 'pjfn000000001'`).Scan(&code, &conn))
	assert.Equal(t, "projection:function:f:dispatch", code)
	require.NotNil(t, conn)
	assert.Equal(t, "con_pjfn0000001", *conn)

	// A status change re-projects the row; the connection stays.
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET status = 'QUEUED', updated_at = now() WHERE id = 'pjfn000000001'`)
	require.NoError(t, err)
	for {
		moved, err := proj.step(ctx, 100)
		require.NoError(t, err)
		if moved == 0 {
			break
		}
	}
	var status string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status, connection_id FROM msg_dispatch_jobs_read WHERE id = 'pjfn000000001'`).Scan(&status, &conn))
	assert.Equal(t, "QUEUED", status)
	require.NotNil(t, conn)
	assert.Equal(t, "con_pjfn0000001", *conn)
}
