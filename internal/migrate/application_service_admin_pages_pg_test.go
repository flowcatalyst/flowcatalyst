//go:build integration

package migrate_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Migration 080 (F20): the built-in application-service role gains the six
// admin-page permissions (the seeder never rewrites an existing role); no
// other role gets them; running it twice is a no-op; the Down removes exactly
// those six and nothing else from the role.
func TestMigration080_GrantsApplicationServiceAdminPagePermissions(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig080")

	_, err := pool.Exec(ctx, `INSERT INTO iam_roles (id, name, display_name) VALUES
		('rol_mig080r00001', 'platform:application-service', 'App service'),
		('rol_mig080r00002', 'platform:other', 'Other')
		ON CONFLICT (name) DO NOTHING`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO iam_role_permissions (role_id, permission)
		SELECT id, 'platform:application-service:event-type:view' FROM iam_roles WHERE name = 'platform:application-service'
		ON CONFLICT DO NOTHING`)
	require.NoError(t, err)

	up := upSection(t, "080_application_service_admin_pages.sql")
	for range 2 {
		_, err = pool.Exec(ctx, up)
		require.NoError(t, err)
	}
	want := []string{
		"platform:application-service:event:view",
		"platform:application-service:dispatch-job:view",
		"platform:application-service:user:view",
		"platform:application-service:user:create",
		"platform:application-service:invitation:view",
		"platform:application-service:invitation:manage",
	}
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
	got := perms("platform:application-service")
	assert.ElementsMatch(t, append([]string{"platform:application-service:event-type:view"}, want...), got, "exactly the six, once each, beside what was there")
	assert.Empty(t, perms("platform:other"))

	// Down removes the six and leaves the rest.
	down := downSection(t, "080_application_service_admin_pages.sql")
	_, err = pool.Exec(ctx, down)
	require.NoError(t, err)
	assert.Equal(t, []string{"platform:application-service:event-type:view"}, perms("platform:application-service"))
}

// downSection returns the Down half of a migration file.
func downSection(t *testing.T, file string) string {
	t.Helper()
	b, err := os.ReadFile("sql/" + file)
	require.NoError(t, err)
	_, after, ok := strings.Cut(string(b), "-- +goose Down")
	require.True(t, ok)
	return after
}
