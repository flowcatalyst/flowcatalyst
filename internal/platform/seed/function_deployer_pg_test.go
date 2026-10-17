//go:build integration

package seed

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// F26: platform:function-deployer is created by the seed on a fresh database,
// re-seeding is idempotent, and on a database that predates the role (every
// other built-in already present) the next start adds it without touching the
// existing roles — no migration needed for a NEW role.
func TestSeedFunctionDeployerRole(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "seed_fn_deployer")
	s := NewSeeder(pool)
	require.NoError(t, s.Run(ctx))

	perms := func() []string {
		rows, err := pool.Query(ctx,
			`SELECT p.permission FROM iam_role_permissions p JOIN iam_roles r ON r.id = p.role_id
			  WHERE r.name = 'platform:function-deployer' ORDER BY 1`)
		require.NoError(t, err)
		defer rows.Close()
		var out []string
		for rows.Next() {
			var p string
			require.NoError(t, rows.Scan(&p))
			out = append(out, p)
		}
		require.NoError(t, rows.Err())
		return out
	}
	want := []string{
		"platform:admin:application:view",
		"platform:admin:client:view",
		"platform:function:alias:promote",
		"platform:function:function:manage",
		"platform:function:function:view",
		"platform:function:version:publish",
	}
	require.Equal(t, want, perms())

	// Idempotent re-seed.
	require.NoError(t, s.Run(ctx))
	require.Equal(t, want, perms())

	// A database that predates the role: delete it, keep the rest, re-seed.
	_, err := pool.Exec(ctx, `DELETE FROM iam_role_permissions WHERE role_id = (SELECT id FROM iam_roles WHERE name = 'platform:function-deployer')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DELETE FROM iam_roles WHERE name = 'platform:function-deployer'`)
	require.NoError(t, err)
	var before int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM iam_roles`).Scan(&before))
	require.NoError(t, s.Run(ctx))
	var after int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM iam_roles`).Scan(&after))
	require.Equal(t, before+1, after)
	require.Equal(t, want, perms())
}
