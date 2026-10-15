//go:build integration

package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func bootstrapRoles(t *testing.T, ctx context.Context, clientID string) (string, []string) {
	t.Helper()
	pool := testpg.Pool(t)
	c, err := auth.NewRepository(pool).OAuthClients.FindByClientID(ctx, clientID)
	require.NoError(t, err)
	require.NotNil(t, c)
	require.NotNil(t, c.PrincipalID)
	pid := string(*c.PrincipalID)
	rows, err := pool.Query(ctx, `SELECT role_name FROM iam_principal_roles WHERE principal_id=$1 ORDER BY role_name`, pid)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		out = append(out, n)
	}
	return pid, out
}

func TestBootstrapLocalCredentials_ReconcilesRole(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("FLOWCATALYST_APP_KEY", key)
	ctx := context.Background()
	pool := testpg.Pool(t)
	const cid = "fcdev-reconcile-test"

	first, err := bootstrapLocalCredentials(ctx, pool, cid, "reconcile test", "Reconcile", "platform:function-publisher")
	require.NoError(t, err)
	pid, roles := bootstrapRoles(t, ctx, cid)
	assert.Equal(t, []string{"platform:function-publisher"}, roles)

	// Older fcdev left a different role (plus an extra) behind.
	_, err = pool.Exec(ctx, `INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source, assigned_at)
		VALUES ($1,'platform:router','BOOTSTRAP',NOW()) ON CONFLICT DO NOTHING`, pid)
	require.NoError(t, err)

	second, err := bootstrapLocalCredentials(ctx, pool, cid, "reconcile test", "Reconcile", "platform:super-admin")
	require.NoError(t, err)
	_, roles = bootstrapRoles(t, ctx, cid)
	assert.Equal(t, []string{"platform:super-admin"}, roles)
	assert.NotEqual(t, first.Secret, second.Secret, "secret is re-minted each boot")
	assert.Equal(t, first.ClientID, second.ClientID)

	// Idempotent.
	_, err = bootstrapLocalCredentials(ctx, pool, cid, "reconcile test", "Reconcile", "platform:super-admin")
	require.NoError(t, err)
	pid2, roles := bootstrapRoles(t, ctx, cid)
	assert.Equal(t, pid, pid2)
	assert.Equal(t, []string{"platform:super-admin"}, roles)
}

func TestBootstrapLocalCredentials_RejectsMalformedRole(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("FLOWCATALYST_APP_KEY", key)
	_, err = bootstrapLocalCredentials(context.Background(), testpg.Pool(t), "fcdev-bad-role", "x", "x", "no-colon")
	require.Error(t, err)
}
