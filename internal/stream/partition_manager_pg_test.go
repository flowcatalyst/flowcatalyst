//go:build integration

package stream

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The partition manager keeps msg_payloads like the other messaging tables:
// it creates the forward partitions and drops the expired ones, and the same
// pass cleans a file payload store.
func TestPartitionManager_KeepsPayloadPartitions(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "pm_payloads")

	exists := func(name string) bool {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_class WHERE relname = $1`, name).Scan(&n))
		return n == 1
	}

	now := time.Now().UTC()
	// Beyond what migrations 019 and 123 create (this month plus 3).
	far := "msg_payloads_" + monthStart(now, 6).Format("2006_01")
	require.False(t, exists(far))
	_, err := pool.Exec(ctx, `CREATE TABLE msg_payloads_2020_01 PARTITION OF msg_payloads
		FOR VALUES FROM ('2020-01-01') TO ('2020-02-01')`)
	require.NoError(t, err)

	m := NewPartitionManager(pool)
	m.Config = PartitionManagerConfig{MonthsForward: 6}
	rec := &recordingExpirer{}
	m.PayloadStore = rec
	_, _, err = m.pass(ctx)
	require.NoError(t, err)
	require.Len(t, rec.cutoffs, 1, "the pass cleans a file payload store")
	assert.WithinDuration(t, now.AddDate(0, 0, -(90+PayloadExpirySlackDays)), rec.cutoffs[0], time.Minute)

	for offset := 0; offset <= 6; offset++ {
		name := "msg_payloads_" + monthStart(now, offset).Format("2006_01")
		assert.True(t, exists(name), name)
	}
	assert.False(t, exists("msg_payloads_2020_01"), "dropped past the retention")

	// A row under its owner's created_at lands in that month's partition.
	_, err = pool.Exec(ctx, `INSERT INTO msg_payloads (id, created_at, data, size) VALUES ('0PMTEST000001', $1, '{}', 2)`,
		monthStart(now, 6).Add(time.Hour))
	require.NoError(t, err)
	var part string
	require.NoError(t, pool.QueryRow(ctx, `SELECT tableoid::regclass::text FROM msg_payloads WHERE id = '0PMTEST000001'`).Scan(&part))
	assert.Equal(t, far, part)
}
