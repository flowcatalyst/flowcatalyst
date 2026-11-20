//go:build integration

package migrate_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// partitionBounds maps each monthly partition of parent (by its YYYY_MM
// suffix) to its bounds.
func partitionBounds(t *testing.T, pool *pgxpool.Pool, parent string) map[string]string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT right(c.relname, 7), pg_get_expr(c.relpartbound, c.oid)
		  FROM pg_inherits i
		  JOIN pg_class c ON c.oid = i.inhrelid
		  JOIN pg_class p ON p.oid = i.inhparent
		 WHERE p.relname = $1`, parent)
	require.NoError(t, err)
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var suffix, bound string
		require.NoError(t, rows.Scan(&suffix, &bound))
		out[suffix] = bound
	}
	require.NoError(t, rows.Err())
	return out
}

// Migration 122: msg_payloads is a partitioned table with a partition for
// the current month and every month msg_events has, with the same bounds;
// the three payload_id columns exist; Up runs twice; Down removes it all.
func TestMigration122_PayloadTable(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig122")
	file := "sql/122_payload_table.sql"

	assertUp := func() {
		t.Helper()
		var relkind string
		require.NoError(t, pool.QueryRow(ctx, `SELECT relkind::text FROM pg_class WHERE relname = 'msg_payloads'`).Scan(&relkind))
		assert.Equal(t, "p", relkind, "partitioned")

		payloads := partitionBounds(t, pool, "msg_payloads")
		current := time.Now().UTC().Format("2006_01")
		assert.Contains(t, payloads, current, "a partition for the current month")
		for suffix, bound := range partitionBounds(t, pool, "msg_events") {
			assert.Equal(t, bound, payloads[suffix], "msg_payloads_%s has the bounds of msg_events_%s", suffix, suffix)
		}

		for _, table := range []string{"msg_events", "msg_events_read", "msg_dispatch_jobs"} {
			var n int
			require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
				WHERE table_schema = current_schema() AND table_name = $1 AND column_name = 'payload_id'`, table).Scan(&n))
			assert.Equal(t, 1, n, "%s.payload_id", table)
		}
		// One index only: the primary key.
		var idx int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE tablename = 'msg_payloads'`).Scan(&idx))
		assert.Equal(t, 1, idx)
	}
	assertUp()

	_, err := pool.Exec(ctx, sectionOf(t, file, "up"))
	require.NoError(t, err, "Up runs again")
	assertUp()

	_, err = pool.Exec(ctx, sectionOf(t, file, "down"))
	require.NoError(t, err)
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_class WHERE relname LIKE 'msg_payloads%'`).Scan(&n))
	assert.Zero(t, n, "table and partitions gone")
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND column_name = 'payload_id'
		  AND table_name IN ('msg_events', 'msg_events_read', 'msg_dispatch_jobs')`).Scan(&n))
	assert.Zero(t, n, "payload_id columns gone")

	_, err = pool.Exec(ctx, sectionOf(t, file, "up"))
	require.NoError(t, err, "Up after Down")
	assertUp()
}
