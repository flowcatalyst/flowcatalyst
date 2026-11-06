//go:build integration

package migrate_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Migration 102: the link column is nullable and re-runnable, a platform user
// is linked to at most one worker per client, may be linked in several
// clients, and workers without a link never collide.
func TestMigration102_FieldIdentityLinkedPrincipal(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig102")

	up := upSection(t, "102_field_identity_linked_principal.sql")
	for range 2 {
		_, err := pool.Exec(ctx, up)
		require.NoError(t, err)
	}

	insert := func(id, client, code string, principal any) error {
		_, err := pool.Exec(ctx, `INSERT INTO field_identities (id, client_id, worker_code, name, linked_principal_id)
			VALUES ($1, $2, $3, 'W', $4)`, id, client, code, principal)
		return err
	}
	require.NoError(t, insert("fwu_mig102w00001", "clt_a", "W1", nil))
	require.NoError(t, insert("fwu_mig102w00002", "clt_a", "W2", nil), "unlinked workers never collide")
	require.NoError(t, insert("fwu_mig102w00003", "clt_a", "W3", "prn_mig102p00001"))
	assert.Error(t, insert("fwu_mig102w00004", "clt_a", "W4", "prn_mig102p00001"), "one worker per client for a user")
	assert.NoError(t, insert("fwu_mig102w00005", "clt_b", "W5", "prn_mig102p00001"), "the same user in another client")
}
