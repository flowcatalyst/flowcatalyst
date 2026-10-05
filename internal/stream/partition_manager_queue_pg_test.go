//go:build integration

package stream

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

// Dropping an expired msg_dispatch_jobs partition removes the queue rows of the
// jobs in that partition and no others.
func TestPartitionDrop_RemovesThatPartitionsQueueRows(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()

	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS msg_dispatch_jobs_2019_01 PARTITION OF msg_dispatch_jobs FOR VALUES FROM ('2019-01-01') TO ('2019-02-01')`,
		`CREATE TABLE IF NOT EXISTS msg_dispatch_jobs_2019_02 PARTITION OF msg_dispatch_jobs FOR VALUES FROM ('2019-02-01') TO ('2019-03-01')`,
	} {
		_, err := pool.Exec(ctx, ddl)
		require.NoError(t, err)
	}

	mk := func(created time.Time) dispatchjob.DispatchJob {
		g := "g-" + tsid.GenerateUntyped()
		return dispatchjob.DispatchJob{
			ID: tsid.GenerateUntyped(), Kind: dispatchjob.KindEvent, Code: "pm:test",
			TargetURL: "http://example.invalid", Protocol: dispatchjob.ProtocolHTTPWebhook,
			Mode: common.DispatchNextOnError, MessageGroup: &g, Sequence: 1, TimeoutSeconds: 30,
			MaxRetries: 3, RetryStrategy: dispatchjob.RetryExponentialBackoff, CreatedAt: created,
		}
	}
	old1 := []dispatchjob.DispatchJob{mk(time.Date(2019, 1, 10, 0, 0, 0, 0, time.UTC)), mk(time.Date(2019, 1, 31, 23, 59, 59, 0, time.UTC))}
	old2 := mk(time.Date(2019, 2, 1, 0, 0, 0, 0, time.UTC)) // first instant of the next partition
	cur := mk(time.Now().UTC().Truncate(time.Microsecond))

	lc := dispatchjob.NewLifecycle(pool)
	all := append(append([]dispatchjob.DispatchJob{}, old1...), old2, cur)
	rows, err := lc.CreateBatch(ctx, all)
	require.NoError(t, err)
	require.Len(t, rows, 4)

	inQueue := func(id string) bool {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM msg_dispatch_queue WHERE job_id = $1`, id).Scan(&n))
		return n == 1
	}
	for _, j := range all {
		require.True(t, inQueue(j.ID))
	}

	// "now" 2019-03-15 with 30 days retention: cutoff 2019-02-13 -> 2019_01 expired, 2019_02 not.
	m := NewPartitionManager(pool)
	dropped, err := m.dropOld(ctx, "msg_dispatch_jobs", time.Date(2019, 3, 15, 0, 0, 0, 0, time.UTC), 30)
	require.NoError(t, err)
	assert.Equal(t, 1, dropped)

	for _, j := range old1 {
		assert.False(t, inQueue(j.ID), "queue row of a job in the dropped partition")
	}
	assert.True(t, inQueue(old2.ID), "the next partition's queue row stays")
	assert.True(t, inQueue(cur.ID), "a current job's queue row stays")

	// leave the shared database clean for other tests
	_, err = pool.Exec(ctx, `DROP TABLE IF EXISTS msg_dispatch_jobs_2019_02`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DELETE FROM msg_dispatch_queue WHERE job_id = ANY($1)`, []string{old2.ID, cur.ID})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DELETE FROM msg_dispatch_jobs WHERE id = $1`, cur.ID)
	require.NoError(t, err)
}
