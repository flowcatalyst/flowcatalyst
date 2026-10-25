//go:build integration

package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The dev ingestor principal is created if missing and its role is reconciled
// to exactly platform:ingestion-runtime on every start, like the other fixed
// dev principals.
func TestBootstrapIngestorCredentials_RoleReconciled(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("FLOWCATALYST_APP_KEY", key)
	ctx := context.Background()
	pool := testpg.Pool(t)

	first, err := bootstrapLocalCredentials(ctx, pool, ingestorLocalClientID, "fcdev ingestor runtime", "Ingestor", ingestorRole)
	require.NoError(t, err)
	pid, roles := bootstrapRoles(t, ctx, ingestorLocalClientID)
	assert.Equal(t, []string{"platform:ingestion-runtime"}, roles)

	_, err = pool.Exec(ctx, `INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source, assigned_at)
		VALUES ($1,'platform:router','BOOTSTRAP',NOW()) ON CONFLICT DO NOTHING`, pid)
	require.NoError(t, err)

	second, err := bootstrapLocalCredentials(ctx, pool, ingestorLocalClientID, "fcdev ingestor runtime", "Ingestor", ingestorRole)
	require.NoError(t, err)
	_, roles = bootstrapRoles(t, ctx, ingestorLocalClientID)
	assert.Equal(t, []string{"platform:ingestion-runtime"}, roles)
	assert.NotEqual(t, first.Secret, second.Secret)
}
