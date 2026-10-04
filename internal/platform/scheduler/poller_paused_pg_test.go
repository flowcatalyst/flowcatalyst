//go:build integration

package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Jobs of a paused subscription used to be claimed, dropped in Go and left
// PENDING, so with BatchSize or more of them sorting first (the claim orders by
// message_group) the same rows were re-claimed every tick and nothing behind
// them was ever published. The paused filter now lives in the claim query.
func TestPollOnce_PausedJobsDoNotStarveTheQueue(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	const (
		connID    = "conn_starve0001"
		subID     = "sub_starve000001"
		activeID  = "djstarveact01"
		batchSize = 5
	)
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_connections (id, code, name, service_account_id, status)
		 VALUES ($1, 'scheduler-poller-starve-conn', 'Starve conn (poller IT)', 'sa_starve0000001', 'PAUSED')`,
		connID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO msg_subscriptions (id, code, name, target, connection_id)
		 VALUES ($1, 'scheduler-poller-starve-sub', 'Starve sub (poller IT)', 'http://example.invalid/hook', $2)`,
		subID, connID)
	require.NoError(t, err)

	// More paused jobs than one claim holds, in groups that sort before the
	// active job's ("00" < "01"), ahead of every other test's "grp_*" rows.
	var pausedIDs []string
	for i := range batchSize + 2 {
		id := fmt.Sprintf("djstarvepa%02d", i)
		pausedIDs = append(pausedIDs, id)
		seedJob(t, pool, id, "PENDING", fmt.Sprintf("00_starve_paused_%02d", i), subID)
	}
	seedJob(t, pool, activeID, "PENDING", "01_starve_active", "")

	cfg := DefaultConfig()
	cfg.BatchSize = batchSize
	capture := &capturePublisher{}
	poller := NewPendingJobPoller(cfg, pool,
		NewMessageGroupDispatcher(pool, capture, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process"),
		NewPausedConnectionCache(pool, time.Minute))

	mustPoll(t, poller, ctx)
	assert.Equal(t, "QUEUED", jobStatus(t, pool, activeID), "an active job behind a wall of paused jobs is still published")
	for _, id := range pausedIDs {
		assert.Equal(t, "PENDING", jobStatus(t, pool, id), "paused jobs stay PENDING")
	}
}
