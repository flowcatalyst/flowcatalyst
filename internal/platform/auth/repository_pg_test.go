//go:build integration

package auth_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestOAuthClientFindByID_CorruptClientTypeFailsLoudly is the X-06 read
// boundary: a row whose client_type column holds a value that isn't one of
// the known OAuthClientType constants (junk written before write-boundary
// validation existed, or a hand-edited row) must fail the read with a
// distinct error, never round-trip as that literal string and never
// silently coerce to PUBLIC.
func TestOAuthClientFindByID_CorruptClientTypeFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := auth.NewRepository(pool)

	const id = "ocl_corrupt_test1"
	testpg.WithConstraintDropped(t, pool, "oauth_clients", "chk_oauth_clients_client_type", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO oauth_clients (id, client_id, client_name, client_type)
		 VALUES ($1, 'corrupt-oauth-client-1', 'Corrupt', 'NOT_A_REAL_TYPE')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_clients WHERE id = $1`, id)
		})
	})

	c, err := repo.OAuthClients.FindByID(ctx, id)
	require.Error(t, err, "a corrupt client type must fail the read, not round-trip it")
	assert.Nil(t, c)
	assert.Contains(t, err.Error(), "CORRUPT_OAUTH_CLIENT_TYPE")
}

// TestOAuthClientFindAll_CorruptClientTypeFailsTheWholeList pins the
// ruling's list semantics explicitly: "a list containing the row fails
// too" — one bad row must not be silently skipped or coerced while the
// rest of the list is returned successfully.
func TestOAuthClientFindAll_CorruptClientTypeFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := auth.NewRepository(pool)

	const goodID = "ocl_cl_good012345"
	const badID = "ocl_cl_bad0012345"
	_, err := pool.Exec(ctx,
		`INSERT INTO oauth_clients (id, client_id, client_name, client_type)
		 VALUES ($1, 'corruptlist-good-oc', 'Good', 'PUBLIC')`, goodID)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "oauth_clients", "chk_oauth_clients_client_type", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO oauth_clients (id, client_id, client_name, client_type)
		 VALUES ($1, 'corruptlist-bad-oc', 'Bad', 'NOT_A_REAL_TYPE')`, badID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_clients WHERE id = $1`, badID)
		})
	})

	rows, err := repo.OAuthClients.FindAll(ctx)
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_OAUTH_CLIENT_TYPE")
}

// TestClientAuthConfigFindByID_CorruptConfigTypeFailsLoudly mirrors the
// above for tnt_client_auth_configs.config_type.
func TestClientAuthConfigFindByID_CorruptConfigTypeFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := auth.NewRepository(pool)

	const id = "cac_corrupt_test1"
	testpg.WithConstraintDropped(t, pool, "tnt_client_auth_configs", "chk_tnt_client_auth_configs_config_type", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO tnt_client_auth_configs (id, email_domain, config_type, auth_provider)
		 VALUES ($1, 'corrupt-configtype.example.com', 'NOT_A_REAL_TYPE', 'INTERNAL')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM tnt_client_auth_configs WHERE id = $1`, id)
		})
	})

	c, err := repo.ClientAuthConfigs.FindByID(ctx, id)
	require.Error(t, err, "a corrupt config type must fail the read, not round-trip it")
	assert.Nil(t, c)
	assert.Contains(t, err.Error(), "CORRUPT_AUTH_CONFIG_TYPE")
}

// TestClientAuthConfigFindByID_CorruptAuthProviderFailsLoudly mirrors the
// above for tnt_client_auth_configs.auth_provider.
func TestClientAuthConfigFindByID_CorruptAuthProviderFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := auth.NewRepository(pool)

	const id = "cac_corrupt_test2"
	testpg.WithConstraintDropped(t, pool, "tnt_client_auth_configs", "chk_tnt_client_auth_configs_auth_provider", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO tnt_client_auth_configs (id, email_domain, config_type, auth_provider)
		 VALUES ($1, 'corrupt-authprovider.example.com', 'ANCHOR', 'NOT_A_REAL_PROVIDER')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM tnt_client_auth_configs WHERE id = $1`, id)
		})
	})

	c, err := repo.ClientAuthConfigs.FindByID(ctx, id)
	require.Error(t, err, "a corrupt auth provider must fail the read, not round-trip it")
	assert.Nil(t, c)
	assert.Contains(t, err.Error(), "CORRUPT_AUTH_PROVIDER")
}

// TestClientAuthConfigFindAll_CorruptConfigTypeFailsTheWholeList pins the
// list semantics for tnt_client_auth_configs.
func TestClientAuthConfigFindAll_CorruptConfigTypeFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := auth.NewRepository(pool)

	const goodID = "cac_cl_good012345"
	const badID = "cac_cl_bad0012345"
	_, err := pool.Exec(ctx,
		`INSERT INTO tnt_client_auth_configs (id, email_domain, config_type, auth_provider)
		 VALUES ($1, 'corruptlist-good.example.com', 'ANCHOR', 'INTERNAL')`, goodID)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "tnt_client_auth_configs", "chk_tnt_client_auth_configs_config_type", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO tnt_client_auth_configs (id, email_domain, config_type, auth_provider)
		 VALUES ($1, 'corruptlist-bad.example.com', 'NOT_A_REAL_TYPE', 'INTERNAL')`, badID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM tnt_client_auth_configs WHERE id = $1`, badID)
		})
	})

	rows, err := repo.ClientAuthConfigs.FindAll(ctx)
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_AUTH_CONFIG_TYPE")
}
