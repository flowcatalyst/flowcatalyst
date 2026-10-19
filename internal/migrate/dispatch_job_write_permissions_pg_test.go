//go:build integration

package migrate_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Migration 085: every role that holds platform:messaging:dispatch-job:view is
// granted the four ordinary write permissions (requeue, cancel, complete,
// edit-payload) so nobody loses an ability when the single view permission is
// split; platform:viewer (read-only by intent) is not; roles without view get
// nothing; redrive-completed is granted to no one; running it twice is a no-op;
// the Down removes only the new permissions.
func TestMigration085_GrantsDispatchJobWritePermissionsToViewHolders(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig085")

	const view = "platform:messaging:dispatch-job:view"
	_, err := pool.Exec(ctx, `INSERT INTO iam_roles (id, name, display_name) VALUES
		('rol_mig085r00001', 'platform:messaging-admin', 'Messaging admin'),
		('rol_mig085r00002', 'platform:viewer', 'Viewer'),
		('rol_mig085r00003', 'custom:ops', 'Ops'),
		('rol_mig085r00004', 'shop:support', 'App role'),
		('rol_mig085r00005', 'platform:iam-admin', 'IAM admin'),
		('rol_mig085r00006', 'platform:super-admin', 'Super')
		ON CONFLICT (name) DO NOTHING`)
	require.NoError(t, err)
	grant := func(role string, perms ...string) {
		for _, p := range perms {
			_, err := pool.Exec(ctx, `INSERT INTO iam_role_permissions (role_id, permission)
				SELECT id, $2 FROM iam_roles WHERE name = $1 ON CONFLICT DO NOTHING`, role, p)
			require.NoError(t, err)
		}
	}
	grant("platform:messaging-admin", view, "platform:messaging:dispatch-job:view-raw", "platform:messaging:event:view")
	grant("platform:viewer", view, "platform:messaging:event:view")
	grant("custom:ops", view)
	grant("shop:support", view, "shop:orders:order:view")
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
	writes := []string{
		"platform:messaging:dispatch-job:requeue",
		"platform:messaging:dispatch-job:cancel",
		"platform:messaging:dispatch-job:complete",
		"platform:messaging:dispatch-job:edit-payload",
	}
	before := map[string][]string{}
	for _, r := range []string{"platform:messaging-admin", "platform:viewer", "custom:ops", "shop:support", "platform:iam-admin", "platform:super-admin"} {
		before[r] = perms(r)
	}

	up := upSection(t, "085_dispatch_job_write_permissions.sql")
	for range 2 {
		_, err = pool.Exec(ctx, up)
		require.NoError(t, err)
	}

	for _, r := range []string{"platform:messaging-admin", "custom:ops", "shop:support"} {
		assert.ElementsMatch(t, append(append([]string{}, before[r]...), writes...), perms(r), "%s keeps what it had and gains the four writes, once each", r)
	}
	assert.ElementsMatch(t, before["platform:viewer"], perms("platform:viewer"), "the read-only built-in role gains nothing")
	assert.ElementsMatch(t, before["platform:iam-admin"], perms("platform:iam-admin"), "a role without view gains nothing")
	assert.ElementsMatch(t, before["platform:super-admin"], perms("platform:super-admin"))

	var redrive int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM iam_role_permissions WHERE permission = 'platform:messaging:dispatch-job:redrive-completed'`).Scan(&redrive))
	assert.Zero(t, redrive, "redrive-completed is granted to no one")

	_, err = pool.Exec(ctx, downSection(t, "085_dispatch_job_write_permissions.sql"))
	require.NoError(t, err)
	for r, want := range before {
		assert.ElementsMatch(t, want, perms(r), "down restores %s", r)
	}
}
