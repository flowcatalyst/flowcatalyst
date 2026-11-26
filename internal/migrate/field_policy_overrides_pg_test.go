//go:build integration

package migrate_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Migration 130: an override holds only the overridable members, within the
// client-wide ranges; a lock names only an overridable member, once.
func TestMigration130_FieldPolicyOverrides(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig130")
	ins := func(shift, idle any) error {
		_, err := pool.Exec(ctx, `INSERT INTO field_policy_overrides (client_id, application_id, shift_max_secs, shared_idle_secs, updated_at)
			VALUES ('clt_mig130', 'app_mig130', $1, $2, NOW()) ON CONFLICT (client_id, application_id) DO UPDATE
			SET shift_max_secs = EXCLUDED.shift_max_secs, shared_idle_secs = EXCLUDED.shared_idle_secs`, shift, idle)
		return err
	}
	require.NoError(t, ins(nil, nil), "every member may follow the client")
	require.NoError(t, ins(3600, 60))
	assert.Error(t, ins(3599, nil), "shift below the client-wide range")
	assert.Error(t, ins(nil, 86401), "idle above the client-wide range")
	_, err := pool.Exec(ctx, `INSERT INTO field_policy_overrides (client_id, application_id, pin_min_length, updated_at)
		VALUES ('clt_mig130', 'app_mig130b', 4, NOW())`)
	assert.Error(t, err, "a member that cannot be overridden has no column")

	lock := func(member string) error {
		_, err := pool.Exec(ctx, `INSERT INTO field_policy_locks (client_id, member, locked_at) VALUES ('clt_mig130', $1, NOW())`, member)
		return err
	}
	require.NoError(t, lock("unlockKeysEnabled"))
	assert.Error(t, lock("unlockKeysEnabled"), "once")
	assert.Error(t, lock("pinMinLength"), "only an overridable member")
}

// Migration 130 grants the field-policy lock to an existing deployment's
// Platform Admin role, once, and to no other role.
func TestMigration130_LockPermission(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig130p")
	const perm = "platform:iam:field-policy:lock"
	_, err := pool.Exec(ctx, `INSERT INTO iam_roles (id, name, display_name) VALUES
		('rol_mig130r00001', 'platform:admin', 'Platform Admin'),
		('rol_mig130r00002', 'platform:field-administrator', 'Field Administrator')
		ON CONFLICT (name) DO NOTHING`)
	require.NoError(t, err)
	up := upSection(t, "130_field_policy_overrides.sql")
	for range 2 {
		_, err = pool.Exec(ctx, up)
		require.NoError(t, err)
	}
	count := func(role string) int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM iam_role_permissions p JOIN iam_roles r ON r.id = p.role_id
			WHERE r.name = $1 AND p.permission = $2`, role, perm).Scan(&n))
		return n
	}
	assert.Equal(t, 1, count("platform:admin"))
	assert.Equal(t, 0, count("platform:field-administrator"))
}
