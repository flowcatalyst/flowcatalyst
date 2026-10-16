//go:build integration

package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A job that names its own connection (a function's fn.dispatch: no
// subscription) is held while that connection is PAUSED and released on
// reactivation. Jobs without a connection are untouched, and a paused
// SUBSCRIPTION's jobs are still held (the long-standing rule).
func TestPollOnce_PausedConnectionHoldsItsOwnJobs(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	const (
		connID = "conn_pcown00001"
		subID  = "sub_pcown0000001"
		saID   = "sa_pcown0000001"
	)
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_connections (id, code, name, service_account_id, status)
		 VALUES ($1, 'scheduler-paused-own-conn', 'Paused own conn (poller IT)', $2, 'PAUSED')`, connID, saID)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM msg_connections WHERE id = $1`, connID) })
	_, err = pool.Exec(ctx,
		`INSERT INTO msg_subscriptions (id, code, name, target, connection_id)
		 VALUES ($1, 'scheduler-paused-own-sub', 'Paused own sub (poller IT)', 'http://example.invalid/hook', $2)`, subID, connID)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM msg_subscriptions WHERE id = $1`, subID) })

	const (
		ownID   = "djpcownfn001"  // names the connection itself, no subscription
		subJob  = "djpcownsub01"  // reaches the connection through its subscription
		plainID = "djpcownplain1" // no connection, no subscription
	)
	seedJob(t, pool, ownID, "PENDING", "00_pcown_a", "")
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET connection_id = $2 WHERE id = $1`, ownID, connID)
	require.NoError(t, err)
	seedJob(t, pool, subJob, "PENDING", "00_pcown_b", subID)
	seedJob(t, pool, plainID, "PENDING", "00_pcown_c", "")

	newPoller := func() *PendingJobPoller {
		cfg := DefaultConfig()
		cfg.BatchSize = 50
		return NewPendingJobPoller(cfg, pool,
			NewMessageGroupDispatcher(pool, &capturePublisher{}, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process"),
			NewPausedConnectionCache(pool, time.Minute))
	}

	mustPoll(t, newPoller(), ctx)
	assert.Equal(t, "PENDING", jobStatus(t, pool, ownID), "a job naming a PAUSED connection is held")
	assert.Equal(t, "PENDING", jobStatus(t, pool, subJob), "a paused subscription's job is still held")
	assert.Equal(t, "QUEUED", jobStatus(t, pool, plainID), "a job with no connection is unaffected")

	// Reactivation releases both (a fresh cache stands for the TTL elapsing).
	_, err = pool.Exec(ctx, `UPDATE msg_connections SET status = 'ACTIVE' WHERE id = $1`, connID)
	require.NoError(t, err)
	mustPoll(t, newPoller(), ctx)
	assert.Equal(t, "QUEUED", jobStatus(t, pool, ownID), "reactivation releases the held job")
	assert.Equal(t, "QUEUED", jobStatus(t, pool, subJob))
}
