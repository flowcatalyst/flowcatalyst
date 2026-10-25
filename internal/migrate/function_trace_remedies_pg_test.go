//go:build integration

package migrate_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// statements splits a migration section into its statements (092 runs
// without a transaction, so CREATE INDEX CONCURRENTLY cannot share an Exec).
func statements(section string) []string {
	var out []string
	for s := range strings.SplitSeq(section, ";\n") {
		var code []string
		for l := range strings.SplitSeq(s, "\n") {
			if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "--") {
				code = append(code, l)
			}
		}
		if len(code) > 0 {
			out = append(out, strings.Join(code, "\n"))
		}
	}
	return out
}

// Migration 092 (function trace remedies): the indefinite column and the
// created_at indexes exist and the (at) index is gone; a window stored with the
// old far-future sentinel becomes an indefinite window with the 7-day ceiling
// (an http one a 15-minute window); trace:indefinite goes to
// platform:messaging-admin only; platform:viewer loses trace:view; running it
// twice is a no-op; the Down puts the grants and indexes back.
func TestMigration092_FunctionTraceRemedies(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig092")
	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		require.NoError(t, pool.QueryRow(ctx, q, args...).Scan(&n))
		return n
	}
	index := func(name string) int { return count(`SELECT count(*) FROM pg_indexes WHERE indexname = $1`, name) }

	assert.Equal(t, 1, count(`SELECT count(*) FROM information_schema.columns WHERE table_name = 'fng_trace_contexts' AND column_name = 'indefinite'`))
	assert.Equal(t, 1, index("idx_fng_trace_log_created_at"))
	assert.Equal(t, 1, index("idx_fng_trace_log_function_created_at"))
	assert.Zero(t, index("idx_fng_trace_log_at"))

	_, err := pool.Exec(ctx, `INSERT INTO iam_roles (id, name, display_name) VALUES
		('rol_mig092r00001', 'platform:messaging-admin', 'Messaging admin'),
		('rol_mig092r00002', 'platform:viewer', 'Viewer'),
		('rol_mig092r00003', 'platform:function-publisher', 'Publisher'),
		('rol_mig092r00004', 'custom:ops', 'Ops')
		ON CONFLICT (name) DO NOTHING`)
	require.NoError(t, err)
	const (
		view       = "platform:function:trace:view"
		manage     = "platform:function:trace:manage"
		indefinite = "platform:function:trace:indefinite"
	)
	grant := func(role string, perms ...string) {
		for _, p := range perms {
			_, err := pool.Exec(ctx, `INSERT INTO iam_role_permissions (role_id, permission)
				SELECT id, $2 FROM iam_roles WHERE name = $1 ON CONFLICT DO NOTHING`, role, p)
			require.NoError(t, err)
		}
	}
	has := func(role, perm string) bool {
		return count(`SELECT count(*) FROM iam_role_permissions p JOIN iam_roles r ON r.id = p.role_id WHERE r.name = $1 AND p.permission = $2`, role, perm) == 1
	}
	grant("platform:messaging-admin", view, manage)
	grant("platform:viewer", view)
	grant("platform:function-publisher", view)
	grant("custom:ops", view, manage)

	_, err = pool.Exec(ctx, `INSERT INTO fng_trace_contexts (function_id, context, enabled_until, sample_rate, updated_at) VALUES
		('fnc_mig092a', 'orders', '9999-12-31T23:59:59Z', 1, now()),
		('fnc_mig092a', 'http',   '9999-12-31T23:59:59Z', 1, now()),
		('fnc_mig092a', 'audit',  now() + interval '5 minutes', 1, now())`)
	require.NoError(t, err)

	up := statements(upSection(t, "092_function_trace_remedies.sql"))
	require.NotEmpty(t, up)
	for range 2 {
		for _, s := range up {
			_, err := pool.Exec(ctx, s)
			require.NoError(t, err, s)
		}
	}

	type window struct {
		indefinite bool
		days       float64
	}
	get := func(context_ string) window {
		var w window
		require.NoError(t, pool.QueryRow(ctx, `SELECT indefinite, extract(epoch FROM enabled_until - now()) / 86400
			FROM fng_trace_contexts WHERE function_id = 'fnc_mig092a' AND context = $1`, context_).Scan(&w.indefinite, &w.days))
		return w
	}
	orders, httpw, audit := get("orders"), get("http"), get("audit")
	assert.True(t, orders.indefinite)
	assert.InDelta(t, 7, orders.days, 0.01, "the sentinel becomes the 7-day ceiling")
	assert.False(t, httpw.indefinite, "http never stays on until switched off")
	assert.InDelta(t, 15.0/(24*60), httpw.days, 0.001, "an http sentinel is cut to 15 minutes")
	assert.False(t, audit.indefinite)
	assert.Less(t, audit.days, 0.01, "a timed window is untouched")

	assert.True(t, has("platform:messaging-admin", indefinite))
	assert.False(t, has("custom:ops", indefinite), "a custom role is given it by hand")
	assert.False(t, has("platform:function-publisher", indefinite))
	assert.False(t, has("platform:viewer", view), "the viewer no longer reads trace records")
	assert.True(t, has("platform:function-publisher", view), "roles that operate functions keep it")
	assert.True(t, has("custom:ops", view))

	for _, s := range statements(downSection(t, "092_function_trace_remedies.sql")) {
		_, err := pool.Exec(ctx, s)
		require.NoError(t, err, s)
	}
	assert.True(t, has("platform:viewer", view))
	assert.False(t, has("platform:messaging-admin", indefinite))
	assert.Equal(t, 1, index("idx_fng_trace_log_at"))
	assert.Zero(t, index("idx_fng_trace_log_created_at"))
	assert.Zero(t, count(`SELECT count(*) FROM information_schema.columns WHERE table_name = 'fng_trace_contexts' AND column_name = 'indefinite'`))
}
