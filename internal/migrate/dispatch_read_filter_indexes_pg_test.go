//go:build integration

package migrate_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func sectionOf(t *testing.T, file, marker string) string {
	t.Helper()
	b, err := os.ReadFile(file)
	require.NoError(t, err)
	s := string(b)
	up, down, _ := strings.Cut(s, "-- +goose Down")
	if marker == "down" {
		return down
	}
	return strings.TrimPrefix(up, "-- +goose Up")
}

// Migration 086 swaps two single-column indexes of the read projection for composites that
// end in created_at DESC and adds connection_id with a partial index. Down restores the
// singles. The online script (scripts/ops/086-dispatch-read-indexes-online.sql) builds the
// same indexes concurrently per partition and attaches them, so a populated database can be
// prepared first and the migration's CREATE ... IF NOT EXISTS then do nothing.
func TestMigration086_SwapsIndexesAndOnlineScriptPreparesThem(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig086")
	file := "sql/086_dispatch_job_read_filter_indexes.sql"

	has := func(name string) bool {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = $1`, name).Scan(&n))
		return n == 1
	}
	assertUp := func() {
		for _, n := range []string{
			"idx_msg_dispatch_jobs_read_code_created",
			"idx_msg_dispatch_jobs_read_subscription_created",
			"idx_msg_dispatch_jobs_read_connection_created",
		} {
			assert.True(t, has(n), n)
		}
		assert.False(t, has("idx_msg_dispatch_jobs_read_code"))
		assert.False(t, has("idx_msg_dispatch_jobs_read_subscription_id"))
		var col int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
			WHERE table_name = 'msg_dispatch_jobs_read' AND column_name = 'connection_id'`).Scan(&col))
		assert.Equal(t, 1, col)
		// Every partition's index is built, valid and attached.
		var bad int
		require.NoError(t, pool.QueryRow(ctx, `
			SELECT count(*) FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
			 WHERE i.indrelid IN (SELECT inhrelid FROM pg_inherits WHERE inhparent = 'msg_dispatch_jobs_read'::regclass)
			   AND (c.relname LIKE '%\_code\_created\_%idx' OR c.relname LIKE '%\_subscription\_created\_%idx' OR c.relname LIKE '%\_connection\_created\_%idx'
			        OR c.relname LIKE '%\_connection\_id\_created\_at\_idx' OR c.relname LIKE '%\_code\_created\_at\_idx' OR c.relname LIKE '%\_subscription\_id\_created\_at\_idx')
			   AND NOT i.indisvalid`).Scan(&bad))
		assert.Zero(t, bad)
	}
	assertUp()

	// Down: the singles come back, the composites and the column go.
	_, err := pool.Exec(ctx, sectionOf(t, file, "down"))
	require.NoError(t, err)
	assert.True(t, has("idx_msg_dispatch_jobs_read_code"))
	assert.True(t, has("idx_msg_dispatch_jobs_read_subscription_id"))
	assert.False(t, has("idx_msg_dispatch_jobs_read_code_created"))
	assert.False(t, has("idx_msg_dispatch_jobs_read_connection_created"))

	// The online script on the pre-086 state: parent indexes ON ONLY + per-partition
	// CONCURRENTLY + ATTACH.
	if _, err := exec.LookPath("psql"); err != nil {
		t.Skip("psql not installed: cannot run the online script")
	}
	cc := pool.Config().ConnConfig
	script := func() {
		cmd := exec.Command("psql", "-h", cc.Host, "-p", fmt.Sprint(cc.Port), "-U", cc.User, "-d", cc.Database,
			"-v", "ON_ERROR_STOP=1", "-q", "-f", "../../scripts/ops/086-dispatch-read-indexes-online.sql")
		cmd.Env = append(os.Environ(), "PGPASSWORD="+cc.Password)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	script()
	assert.True(t, has("idx_msg_dispatch_jobs_read_code_created"), "the script built the parent index")
	// Running the script again is a no-op.
	script()

	// Then the migration: its CREATEs find the indexes; its DROPs remove the singles.
	_, err = pool.Exec(ctx, sectionOf(t, file, "up"))
	require.NoError(t, err)
	assertUp()
	// ... and the up is idempotent.
	_, err = pool.Exec(ctx, sectionOf(t, file, "up"))
	require.NoError(t, err)
	assertUp()
}
