//go:build integration

package migrate_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Migration 121: the App Update Administrator role of an existing deployment
// gains the install-link permission; the application-service role does not;
// running it twice is a no-op.
func TestMigration121_GrantsInstallLinkPermissionToTheOperatorRole(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig121")
	_, err := pool.Exec(ctx, `INSERT INTO iam_roles (id, name, display_name) VALUES
		('rol_mig121r00001', 'platform:app-update-admin', 'App Update Administrator'),
		('rol_mig121r00002', 'platform:application-service', 'App service')
		ON CONFLICT (name) DO NOTHING`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DELETE FROM iam_role_permissions WHERE permission = 'platform:app-update:install-link:manage'`)
	require.NoError(t, err)
	up := upSection(t, "121_app_install_link_permission.sql")
	for range 2 {
		_, err = pool.Exec(ctx, up)
		require.NoError(t, err)
	}
	count := func(role string) int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM iam_role_permissions p JOIN iam_roles r ON r.id = p.role_id
			WHERE r.name = $1 AND p.permission = 'platform:app-update:install-link:manage'`, role).Scan(&n))
		return n
	}
	assert.Equal(t, 1, count("platform:app-update-admin"))
	assert.Equal(t, 0, count("platform:application-service"))
}
