//go:build integration

package migrate_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Migration 129: one live unlock key per device, a key's thumbprint once, the
// closed sets of revocation reasons and attestation rules, policy defaults
// (off, NONE); re-running Up is harmless and Down removes what Up added.
func TestMigration129_FieldUnlockKeys(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig129")
	insert := func(id, device, jkt string) error {
		_, err := pool.Exec(ctx, `INSERT INTO field_unlock_keys
			(id, client_id, identity_id, device_id, jkt, public_key_jwk, pin_verified_at, created_at)
			VALUES ($1, 'clt_x', 'fwu_x', $2, $3, '{}', NOW(), NOW())`, id, device, jkt)
		return err
	}
	require.NoError(t, insert("fwk_mig129k00001", "fwd_mig129d00001", "jkt-1"))
	assert.Error(t, insert("fwk_mig129k00002", "fwd_mig129d00001", "jkt-2"), "a second live key on the device")
	assert.Error(t, insert("fwk_mig129k00003", "fwd_mig129d00002", "jkt-1"), "a thumbprint registered twice")

	_, err := pool.Exec(ctx, `UPDATE field_unlock_keys SET revoked_at = NOW(), revoked_reason = 'SOMETHING' WHERE id = 'fwk_mig129k00001'`)
	assert.Error(t, err, "an unknown revocation reason")
	_, err = pool.Exec(ctx, `UPDATE field_unlock_keys SET revoked_at = NOW(), revoked_reason = 'REPLACED' WHERE id = 'fwk_mig129k00001'`)
	require.NoError(t, err)
	require.NoError(t, insert("fwk_mig129k00002", "fwd_mig129d00001", "jkt-2"), "a revoked key frees the device")

	var enabled bool
	var rule string
	_, err = pool.Exec(ctx, `INSERT INTO field_policies (client_id) VALUES ('clt_mig129')`)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx, `SELECT unlock_keys_enabled, unlock_key_attestation FROM field_policies WHERE client_id = 'clt_mig129'`).Scan(&enabled, &rule))
	assert.False(t, enabled)
	assert.Equal(t, "NONE", rule)
	_, err = pool.Exec(ctx, `UPDATE field_policies SET unlock_key_attestation = 'MAYBE' WHERE client_id = 'clt_mig129'`)
	assert.Error(t, err)

	_, err = pool.Exec(ctx, upSection(t, "129_field_unlock_keys.sql"))
	require.NoError(t, err, "Up is idempotent")

	_, err = pool.Exec(ctx, downSection(t, "129_field_unlock_keys.sql"))
	require.NoError(t, err)
	var tables int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name IN ('field_unlock_keys', 'field_unlock_challenges')`).Scan(&tables))
	assert.Zero(t, tables)
	var cols int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'field_policies'
		  AND column_name IN ('unlock_keys_enabled', 'unlock_key_attestation')`).Scan(&cols))
	assert.Zero(t, cols)
}
