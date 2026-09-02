//go:build integration

package identityprovider_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestFindByID_CorruptTypeFailsLoudly is the X-06 read boundary: a row
// whose type column holds a value that isn't one of the known Type
// constants (junk written before write-boundary validation existed, or a
// hand-edited row) must fail the read with a distinct error, never
// round-trip as that literal string and never silently coerce to INTERNAL.
func TestFindByID_CorruptTypeFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := identityprovider.NewRepository(pool)

	const id = "idp_corrupt_test1"
	testpg.WithConstraintDropped(t, pool, "oauth_identity_providers", "chk_oauth_identity_providers_type", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO oauth_identity_providers (id, code, name, type)
		 VALUES ($1, 'corrupt-idp-test1', 'Corrupt', 'NOT_A_REAL_TYPE')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_identity_providers WHERE id = $1`, id)
		})
	})

	ip, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt type must fail the read, not round-trip it")
	assert.Nil(t, ip)
	assert.Contains(t, err.Error(), "CORRUPT_IDENTITY_PROVIDER_TYPE")
}

// TestFindAll_CorruptTypeFailsTheWholeList pins the ruling's list semantics
// explicitly: "a list containing the row fails too" — one bad row must not
// be silently skipped or coerced while the rest of the list is returned
// successfully.
func TestFindAll_CorruptTypeFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := identityprovider.NewRepository(pool)

	const goodID = "idp_cl_good012345"
	const badID = "idp_cl_bad0012345"
	_, err := pool.Exec(ctx,
		`INSERT INTO oauth_identity_providers (id, code, name, type)
		 VALUES ($1, 'corruptlist-good-idp', 'Good', 'INTERNAL')`, goodID)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "oauth_identity_providers", "chk_oauth_identity_providers_type", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO oauth_identity_providers (id, code, name, type)
		 VALUES ($1, 'corruptlist-bad-idp', 'Bad', 'NOT_A_REAL_TYPE')`, badID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM oauth_identity_providers WHERE id = $1`, badID)
		})
	})

	rows, err := repo.FindAll(ctx)
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_IDENTITY_PROVIDER_TYPE")
}
