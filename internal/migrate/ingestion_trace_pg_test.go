//go:build integration

package migrate_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Migration 094 (ingestion traces): the two tables, their indexes and the two
// new metrics columns exist (polls defaults to 0);
// trace:manage and trace:view-bodies go to the roles that hold
// ingestion:delete, trace:view to those and to the holders of ingestion:update,
// once; the pipeline's route author, the viewer and an unrelated role gain
// nothing; running it twice is a no-op; the Down removes only the new
// permissions, columns and tables.
func TestMigration094_IngestionTraceTablesAndGrants(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig094")
	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		require.NoError(t, pool.QueryRow(ctx, q, args...).Scan(&n))
		return n
	}

	for _, table := range []string{"ing_trace_windows", "ing_trace_log"} {
		assert.Equal(t, 1, count(`SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1`, table), table)
	}
	for _, idx := range []string{"idx_ing_trace_log_ingestion_at", "idx_ing_trace_log_ingestion_capture", "idx_ing_trace_log_created_at"} {
		assert.Equal(t, 1, count(`SELECT count(*) FROM pg_indexes WHERE indexname = $1`, idx), idx)
	}
	for _, col := range []string{"polls", "last_poll_at"} {
		assert.Equal(t, 1, count(`SELECT count(*) FROM information_schema.columns WHERE table_name = 'ing_exchange_metrics' AND column_name = $1`, col), col)
	}

	// A metrics row that does not name the new columns (an older platform's
	// insert, a row that predates the migration) reads polls = 0 and no last poll.
	_, err := pool.Exec(ctx, `INSERT INTO ing_exchange_metrics (ingestion_id, runtime_id, window_start, window_secs)
		VALUES ('ing_mig094', 'rt', now(), 60)`)
	require.NoError(t, err)
	assert.Equal(t, 1, count(`SELECT count(*) FROM ing_exchange_metrics WHERE ingestion_id = 'ing_mig094' AND polls = 0 AND last_poll_at IS NULL`))

	_, err = pool.Exec(ctx, `INSERT INTO iam_roles (id, name, display_name) VALUES
		('rol_mig094r00001', 'platform:ingestion-admin', 'Admin'),
		('rol_mig094r00002', 'platform:ingestion-operator', 'Operator'),
		('rol_mig094r00003', 'platform:ingestion-viewer', 'Viewer'),
		('rol_mig094r00004', 'platform:ingestion-route-author', 'Route author'),
		('rol_mig094r00005', 'custom:ops', 'Ops'),
		('rol_mig094r00006', 'platform:iam-admin', 'IAM admin'),
		('rol_mig094r00007', 'platform:super-admin', 'Super')
		ON CONFLICT (name) DO NOTHING`)
	require.NoError(t, err)
	grant := func(role string, perms ...string) {
		for _, p := range perms {
			_, err := pool.Exec(ctx, `INSERT INTO iam_role_permissions (role_id, permission)
				SELECT id, $2 FROM iam_roles WHERE name = $1 ON CONFLICT DO NOTHING`, role, p)
			require.NoError(t, err)
		}
	}
	const (
		view   = "platform:ingestion:ingestion:view"
		update = "platform:ingestion:ingestion:update"
		del    = "platform:ingestion:ingestion:delete"
	)
	grant("platform:ingestion-admin", view, update, del)
	grant("platform:ingestion-operator", view, update, "platform:ingestion:ingestion:test")
	grant("platform:ingestion-viewer", view)
	grant("platform:ingestion-route-author", view, update, "platform:ingestion:ingestion:route")
	grant("custom:ops", del)
	grant("platform:iam-admin", "platform:iam:user:view")
	grant("platform:super-admin", "platform:*:*:*")

	perms := func(role string) []string {
		rows, err := pool.Query(ctx, `SELECT p.permission FROM iam_role_permissions p JOIN iam_roles r ON r.id = p.role_id WHERE r.name = $1`, role)
		require.NoError(t, err)
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			require.NoError(t, rows.Scan(&s))
			out = append(out, s)
		}
		return out
	}
	roles := []string{
		"platform:ingestion-admin", "platform:ingestion-operator", "platform:ingestion-viewer",
		"platform:ingestion-route-author", "custom:ops", "platform:iam-admin", "platform:super-admin",
	}
	before := map[string][]string{}
	for _, r := range roles {
		before[r] = perms(r)
	}

	up := upSection(t, "094_ingestion_trace.sql")
	for range 2 {
		_, err = pool.Exec(ctx, up)
		require.NoError(t, err)
	}

	tv, tm, tb := "platform:ingestion:trace:view", "platform:ingestion:trace:manage", "platform:ingestion:trace:view-bodies"
	with := func(role string, add ...string) []string { return append(append([]string{}, before[role]...), add...) }
	assert.ElementsMatch(t, with("platform:ingestion-admin", tv, tm, tb), perms("platform:ingestion-admin"), "the administrator gets all three, once each")
	assert.ElementsMatch(t, with("platform:ingestion-operator", tv), perms("platform:ingestion-operator"), "an operator reads records, bodies withheld")
	assert.ElementsMatch(t, before["platform:ingestion-viewer"], perms("platform:ingestion-viewer"), "a viewer gains nothing")
	assert.ElementsMatch(t, before["platform:ingestion-route-author"], perms("platform:ingestion-route-author"), "the pipeline's role gains nothing")
	assert.ElementsMatch(t, with("custom:ops", tv, tm, tb), perms("custom:ops"), "delete alone gets all three")
	assert.ElementsMatch(t, before["platform:iam-admin"], perms("platform:iam-admin"))
	assert.ElementsMatch(t, before["platform:super-admin"], perms("platform:super-admin"))

	_, err = pool.Exec(ctx, downSection(t, "094_ingestion_trace.sql"))
	require.NoError(t, err)
	for r, want := range before {
		assert.ElementsMatch(t, want, perms(r), "down restores %s", r)
	}
	assert.Zero(t, count(`SELECT count(*) FROM information_schema.tables WHERE table_name IN ('ing_trace_windows','ing_trace_log')`))
	assert.Zero(t, count(`SELECT count(*) FROM information_schema.columns WHERE table_name = 'ing_exchange_metrics' AND column_name IN ('polls','last_poll_at')`))
}
