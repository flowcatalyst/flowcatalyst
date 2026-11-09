//go:build integration

package scheduledjob_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Migration 111 prefixes a 0 seconds field onto stored 5-field crons (they
// never fired) and leaves 6-field and 7-field expressions alone. The UPDATE is
// lifted from the migration file so the test exercises the shipped SQL.
func TestMigration111_FiveFieldCronsGainSeconds(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	b, err := os.ReadFile("../../migrate/sql/111_scheduled_job_once_per_tick.sql")
	require.NoError(t, err)
	src := string(b)
	start := strings.Index(src, "UPDATE msg_scheduled_jobs")
	end := strings.Index(src, "-- +goose Down")
	require.True(t, start > 0 && end > start)
	update := src[start:end]

	const id, okID = "sjb_mig111_test1", "sjb_mig111_test2"
	_, err = pool.Exec(ctx, `INSERT INTO msg_scheduled_jobs (id, code, name, crons, status)
		VALUES ($1, 'mig111-job', 'Mig111', ARRAY['*/5 * * * *','  0 0 * * *  ','0 0 * * * *','0 0 0 * * * 2030'], 'ACTIVE'),
		       ($2, 'mig111-ok', 'Mig111 ok', ARRAY['0 0 * * * *'], 'ACTIVE')`, id, okID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM msg_scheduled_jobs WHERE id = ANY($1)`, []string{id, okID})
	})

	_, err = pool.Exec(ctx, update)
	require.NoError(t, err)

	var crons []string
	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT crons, status FROM msg_scheduled_jobs WHERE id = $1`, id).Scan(&crons, &status))
	require.Equal(t, []string{"0 */5 * * * *", "0 0 0 * * *", "0 0 * * * *", "0 0 0 * * * 2030"}, crons)
	require.Equal(t, "PAUSED", status, "a job that never fired is paused, not started (owner ruling 2026-11-09)")

	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM msg_scheduled_jobs WHERE id = $1`, okID).Scan(&status))
	require.Equal(t, "ACTIVE", status, "a 6-field job is untouched")
}
