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

// Migration 150 prefixes a 0 seconds field onto stored 5-field crons (they
// never fired) and leaves 6-field and 7-field expressions alone. The UPDATE is
// lifted from the migration file so the test exercises the shipped SQL.
func TestMigration150_FiveFieldCronsGainSeconds(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	b, err := os.ReadFile("../../migrate/sql/150_scheduled_job_once_per_tick.sql")
	require.NoError(t, err)
	src := string(b)
	start := strings.Index(src, "UPDATE msg_scheduled_jobs")
	end := strings.Index(src, "-- +goose Down")
	require.True(t, start > 0 && end > start)
	update := src[start:end]

	const id = "sjb_mig150_test1"
	_, err = pool.Exec(ctx, `INSERT INTO msg_scheduled_jobs (id, code, name, crons)
		VALUES ($1, 'mig150-job', 'Mig150', ARRAY['*/5 * * * *','  0 0 * * *  ','0 0 * * * *','0 0 0 * * * 2030'])`, id)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM msg_scheduled_jobs WHERE id = $1`, id) })

	_, err = pool.Exec(ctx, update)
	require.NoError(t, err)

	var crons []string
	require.NoError(t, pool.QueryRow(ctx, `SELECT crons FROM msg_scheduled_jobs WHERE id = $1`, id).Scan(&crons))
	require.Equal(t, []string{"0 */5 * * * *", "0 0 0 * * *", "0 0 * * * *", "0 0 0 * * * 2030"}, crons)
}
