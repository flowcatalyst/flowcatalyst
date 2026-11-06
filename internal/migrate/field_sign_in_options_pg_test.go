//go:build integration

package migrate_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Migration 101: the field-administrator role gains the impersonate permission
// once (running it twice is a no-op), other roles gain nothing, sessions get
// their device's key as jkt and keep working with a NULL device_id, and the
// Down removes only what Up added.
func TestMigration101_FieldSignInOptions(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig101")

	const perm = "platform:iam:field-worker:impersonate"
	_, err := pool.Exec(ctx, `INSERT INTO iam_roles (id, name, display_name) VALUES
		('rol_mig101r00001', 'platform:field-administrator', 'Field Administrator'),
		('rol_mig101r00002', 'platform:viewer', 'Viewer')
		ON CONFLICT (name) DO NOTHING`)
	require.NoError(t, err)
	count := func(role string) int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM iam_role_permissions p JOIN iam_roles r ON r.id = p.role_id
			WHERE r.name = $1 AND p.permission = $2`, role, perm).Scan(&n))
		return n
	}

	// The ScratchDB is fully migrated: Up already ran on empty roles. Re-run it
	// with the roles present, twice.
	up := upSection(t, "101_field_sign_in_options.sql")
	for range 2 {
		_, err = pool.Exec(ctx, up)
		require.NoError(t, err)
	}
	assert.Equal(t, 1, count("platform:field-administrator"), "granted once")
	assert.Equal(t, 0, count("platform:viewer"), "nobody else")

	// device_id is nullable; existing rows take their device's jkt on re-run.
	_, err = pool.Exec(ctx, `INSERT INTO field_devices (id, client_id, mode, jkt, public_key_jwk) VALUES ('fwd_mig101dev001', 'clt_x', 'SHARED', 'jkt-101', '{}')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO field_sessions (id, family_id, identity_id, device_id, application_id, refresh_hash, idle_expires_at, absolute_expires_at)
		VALUES ('fws_mig101s00001', 'fws_mig101s00001', 'fwu_x', 'fwd_mig101dev001', 'app_x', 'h1', NOW(), NOW()),
		       ('fws_mig101s00002', 'fws_mig101s00002', 'fwu_x', NULL, 'app_x', 'h2', NOW(), NOW())`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, up)
	require.NoError(t, err)
	var jkt *string
	require.NoError(t, pool.QueryRow(ctx, `SELECT jkt FROM field_sessions WHERE id = 'fws_mig101s00001'`).Scan(&jkt))
	require.NotNil(t, jkt)
	assert.Equal(t, "jkt-101", *jkt)

	// One ACTIVE badge per worker is enforced by the schema.
	_, err = pool.Exec(ctx, `INSERT INTO field_identities (id, client_id, worker_code, name) VALUES ('fwu_mig101w00001', 'clt_x', 'W1', 'W')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO field_badges (id, client_id, identity_id, token_hash) VALUES ('fwb_mig101b00001', 'clt_x', 'fwu_mig101w00001', 'a')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO field_badges (id, client_id, identity_id, token_hash) VALUES ('fwb_mig101b00002', 'clt_x', 'fwu_mig101w00001', 'b')`)
	assert.Error(t, err, "a second ACTIVE badge for the worker is refused")
}
