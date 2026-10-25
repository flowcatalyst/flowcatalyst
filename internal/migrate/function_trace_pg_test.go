//go:build integration

package migrate_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Migration 087 (function trace logs): the two tables and their indexes exist,
// and the three trace permissions go to the roles that hold the matching
// function permissions, once: trace:view to every holder of function:view,
// trace:manage and trace:view-bodies to every holder of function:manage. A
// role with neither (and the CI platform:function-deployer role) gains nothing; running it twice is a no-op; the Down
// removes only the new permissions and the tables.
func TestMigration087_FunctionTraceTablesAndGrants(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig087")

	for _, table := range []string{"fng_trace_contexts", "fng_trace_log"} {
		var n int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1`, table).Scan(&n))
		assert.Equal(t, 1, n, table)
	}
	for _, idx := range []string{
		"idx_fng_trace_log_function_at", "idx_fng_trace_log_function_reference", "idx_fng_trace_log_function_correlation",
		"idx_fng_trace_log_function_invocation", "idx_fng_trace_log_function_context",
		// 092 swapped the (at) retention index for created_at ones.
		"idx_fng_trace_log_created_at", "idx_fng_trace_log_function_created_at",
	} {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE indexname = $1`, idx).Scan(&n))
		assert.Equal(t, 1, n, idx)
	}

	const (
		view   = "platform:function:function:view"
		manage = "platform:function:function:manage"
	)
	_, err := pool.Exec(ctx, `INSERT INTO iam_roles (id, name, display_name) VALUES
		('rol_mig087r00001', 'platform:messaging-admin', 'Messaging admin'),
		('rol_mig087r00002', 'platform:viewer', 'Viewer'),
		('rol_mig087r00003', 'platform:function-publisher', 'Publisher'),
		('rol_mig087r00004', 'custom:ops', 'Ops'),
		('rol_mig087r00005', 'platform:iam-admin', 'IAM admin'),
		('rol_mig087r00006', 'platform:super-admin', 'Super'),
		('rol_mig087r00007', 'platform:function-deployer', 'Deployer')
		ON CONFLICT (name) DO NOTHING`)
	require.NoError(t, err)
	grant := func(role string, perms ...string) {
		for _, p := range perms {
			_, err := pool.Exec(ctx, `INSERT INTO iam_role_permissions (role_id, permission)
				SELECT id, $2 FROM iam_roles WHERE name = $1 ON CONFLICT DO NOTHING`, role, p)
			require.NoError(t, err)
		}
	}
	grant("platform:messaging-admin", view, manage, "platform:messaging:event:view")
	grant("platform:viewer", view)
	grant("platform:function-publisher", view, "platform:function:version:publish")
	grant("custom:ops", manage)
	grant("platform:iam-admin", "platform:iam:user:view")
	grant("platform:super-admin", "platform:*:*:*")
	grant("platform:function-deployer", view, manage, "platform:function:version:publish")

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
	roles := []string{"platform:messaging-admin", "platform:viewer", "platform:function-publisher", "custom:ops", "platform:iam-admin", "platform:super-admin", "platform:function-deployer"}
	before := map[string][]string{}
	for _, r := range roles {
		before[r] = perms(r)
	}

	up := upSection(t, "087_function_trace.sql")
	for range 2 {
		_, err = pool.Exec(ctx, up)
		require.NoError(t, err)
	}

	tv, tm, tb := "platform:function:trace:view", "platform:function:trace:manage", "platform:function:trace:view-bodies"
	with := func(role string, add ...string) []string { return append(append([]string{}, before[role]...), add...) }
	assert.ElementsMatch(t, with("platform:messaging-admin", tv, tm, tb), perms("platform:messaging-admin"), "view and manage holder gets all three, once each")
	assert.ElementsMatch(t, with("platform:viewer", tv), perms("platform:viewer"), "a viewer reads records, bodies withheld")
	assert.ElementsMatch(t, with("platform:function-publisher", tv), perms("platform:function-publisher"))
	assert.ElementsMatch(t, with("custom:ops", tm, tb), perms("custom:ops"), "manage alone gets manage and view-bodies")
	assert.ElementsMatch(t, before["platform:iam-admin"], perms("platform:iam-admin"), "a role with no function permission gains nothing")
	assert.ElementsMatch(t, before["platform:super-admin"], perms("platform:super-admin"))
	assert.ElementsMatch(t, before["platform:function-deployer"], perms("platform:function-deployer"), "the CI deployer role stays minimal")

	_, err = pool.Exec(ctx, downSection(t, "087_function_trace.sql"))
	require.NoError(t, err)
	for r, want := range before {
		assert.ElementsMatch(t, want, perms(r), "down restores %s", r)
	}
	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables WHERE table_name IN ('fng_trace_contexts','fng_trace_log')`).Scan(&n))
	assert.Zero(t, n)
}
