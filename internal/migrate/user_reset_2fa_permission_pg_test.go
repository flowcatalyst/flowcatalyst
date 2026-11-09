//go:build integration

package migrate_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Migration 113: platform:iam:user:reset-2fa is granted to every role that
// holds platform:iam:user:update (the permission that gated the 2FA reset
// actions before), so nobody loses an ability on upgrade; roles without update
// gain nothing; running it twice is a no-op; the Down removes only reset-2fa.
func TestMigration113_GrantsReset2FAToUserUpdateHolders(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig113")

	const (
		update   = "platform:iam:user:update"
		reset2FA = "platform:iam:user:reset-2fa"
	)
	_, err := pool.Exec(ctx, `INSERT INTO iam_roles (id, name, display_name) VALUES
		('rol_mig113r00001', 'platform:iam-admin', 'IAM admin'),
		('rol_mig113r00002', 'platform:client-admin', 'Client admin'),
		('rol_mig113r00003', 'custom:helpdesk', 'Helpdesk'),
		('rol_mig113r00004', 'platform:iam-readonly', 'IAM read-only'),
		('rol_mig113r00005', 'platform:super-admin', 'Super')
		ON CONFLICT (name) DO NOTHING`)
	require.NoError(t, err)
	grant := func(role string, perms ...string) {
		for _, p := range perms {
			_, err := pool.Exec(ctx, `INSERT INTO iam_role_permissions (role_id, permission)
				SELECT id, $2 FROM iam_roles WHERE name = $1 ON CONFLICT DO NOTHING`, role, p)
			require.NoError(t, err)
		}
	}
	grant("platform:iam-admin", "platform:iam:user:view", update)
	grant("platform:client-admin", "platform:iam:user:view", update)
	grant("custom:helpdesk", update)
	grant("platform:iam-readonly", "platform:iam:user:view")
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
	roles := []string{"platform:iam-admin", "platform:client-admin", "custom:helpdesk", "platform:iam-readonly", "platform:super-admin"}
	before := map[string][]string{}
	for _, r := range roles {
		before[r] = perms(r)
	}

	up := upSection(t, "113_user_reset_2fa_permission.sql")
	for range 2 {
		_, err = pool.Exec(ctx, up)
		require.NoError(t, err)
	}

	for _, r := range []string{"platform:iam-admin", "platform:client-admin", "custom:helpdesk"} {
		assert.ElementsMatch(t, append(append([]string{}, before[r]...), reset2FA), perms(r), "%s keeps what it had and gains reset-2fa, once", r)
	}
	assert.ElementsMatch(t, before["platform:iam-readonly"], perms("platform:iam-readonly"), "a role without user:update gains nothing")
	assert.ElementsMatch(t, before["platform:super-admin"], perms("platform:super-admin"), "the wildcard already covers it")

	_, err = pool.Exec(ctx, downSection(t, "113_user_reset_2fa_permission.sql"))
	require.NoError(t, err)
	for r, want := range before {
		assert.ElementsMatch(t, want, perms(r), "down restores %s", r)
	}
}
