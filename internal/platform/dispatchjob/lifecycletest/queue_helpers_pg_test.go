//go:build integration

package lifecycletest

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncQueueFixture makes msg_dispatch_queue agree with the job's current row
// after a TEST-ONLY direct write to msg_dispatch_jobs (which the lifecycle did
// not see): PENDING -> an exact mirror row; anything else -> no row.
func syncQueueFixture(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `DELETE FROM msg_dispatch_queue WHERE job_id = $1`, id)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO msg_dispatch_queue (job_id, job_created_at, message_group, sequence, scheduled_for,
		       subscription_id, dispatch_pool_id, client_id, mode, queue, version)
		SELECT id, created_at, message_group, sequence, scheduled_for, subscription_id,
		       dispatch_pool_id, client_id, mode, queue, updated_at
		  FROM msg_dispatch_jobs WHERE id = $1 AND status = 'PENDING'`, id)
	require.NoError(t, err)
}

// queueSnapshot is the queue row as JSON, nil when there is none.
func queueSnapshot(t *testing.T, pool *pgxpool.Pool, id string) map[string]any {
	t.Helper()
	var raw []byte
	err := pool.QueryRow(context.Background(),
		`SELECT to_jsonb(q) FROM msg_dispatch_queue q WHERE job_id = $1`, id).Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil
	}
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

// assertQueueInvariant: a queue row exists iff the job is PENDING, and then it
// mirrors the job (version = updated_at).
func assertQueueInvariant(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	ctx := context.Background()
	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&status))
	q := queueSnapshot(t, pool, id)
	if status != "PENDING" {
		assert.Nil(t, q, "job %s is %s: it must have no queue row", id, status)
		return
	}
	if !assert.NotNil(t, q, "job %s is PENDING: it must have a queue row", id) {
		return
	}
	var equal bool
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT q.version = j.updated_at
		   AND q.job_created_at = j.created_at
		   AND q.scheduled_for IS NOT DISTINCT FROM j.scheduled_for
		   AND q.message_group IS NOT DISTINCT FROM j.message_group
		   AND q.sequence = j.sequence
		   AND q.dispatch_pool_id IS NOT DISTINCT FROM j.dispatch_pool_id
		   AND q.client_id IS NOT DISTINCT FROM j.client_id
		   AND q.subscription_id IS NOT DISTINCT FROM j.subscription_id
		   AND q.mode = j.mode
		   AND q.queue IS NOT DISTINCT FROM j.queue
		  FROM msg_dispatch_jobs j JOIN msg_dispatch_queue q ON q.job_id = j.id
		 WHERE j.id = $1`, id).Scan(&equal))
	assert.True(t, equal, "queue row of %s does not mirror the job: %v", id, q)
}
