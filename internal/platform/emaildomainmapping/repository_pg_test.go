//go:build integration

package emaildomainmapping_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestFindByID_CorruptScopeTypeFailsLoudly is the X-06 read boundary: a row
// whose scope_type column holds a value that isn't one of the known
// ScopeType constants (junk written before write-boundary validation
// existed, or a hand-edited row) must fail the read with a distinct error,
// never round-trip as that literal string and never silently coerce to
// ANCHOR.
func TestFindByID_CorruptScopeTypeFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := emaildomainmapping.NewRepository(pool)

	const id = "edm_corrupt_test1"
	testpg.WithConstraintDropped(t, pool, "tnt_email_domain_mappings", "chk_tnt_email_domain_mappings_scope_type", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO tnt_email_domain_mappings (id, email_domain, identity_provider_id, scope_type)
		 VALUES ($1, 'corrupt-edm-test1.example.com', 'idp_corrupttest01', 'NOT_A_REAL_SCOPE')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM tnt_email_domain_mappings WHERE id = $1`, id)
		})
	})

	edm, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt scope type must fail the read, not round-trip it")
	assert.Nil(t, edm)
	assert.Contains(t, err.Error(), "CORRUPT_EDM_SCOPE_TYPE")
}

// TestFindAll_CorruptScopeTypeFailsTheWholeList pins the ruling's list
// semantics explicitly: "a list containing the row fails too" — one bad row
// must not be silently skipped or coerced while the rest of the list is
// returned successfully.
func TestFindAll_CorruptScopeTypeFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := emaildomainmapping.NewRepository(pool)

	const goodID = "edm_cl_good012345"
	const badID = "edm_cl_bad0012345"
	_, err := pool.Exec(ctx,
		`INSERT INTO tnt_email_domain_mappings (id, email_domain, identity_provider_id, scope_type)
		 VALUES ($1, 'corruptlist-good.example.com', 'idp_corrupttest01', 'ANCHOR')`, goodID)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "tnt_email_domain_mappings", "chk_tnt_email_domain_mappings_scope_type", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO tnt_email_domain_mappings (id, email_domain, identity_provider_id, scope_type)
		 VALUES ($1, 'corruptlist-bad.example.com', 'idp_corrupttest01', 'NOT_A_REAL_SCOPE')`, badID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM tnt_email_domain_mappings WHERE id = $1`, badID)
		})
	})

	rows, err := repo.FindAll(ctx)
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_EDM_SCOPE_TYPE")
}
