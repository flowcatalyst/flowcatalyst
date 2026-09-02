//go:build integration

package platformconfig_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestFindByID_CorruptScopeFailsLoudly is the X-06 read boundary: a row
// whose scope column holds a value that isn't one of the known Scope
// constants (junk written before write-boundary validation existed, or a
// hand-edited row) must fail the read with a distinct error, never
// round-trip as that literal string and never silently coerce to GLOBAL.
func TestFindByID_CorruptScopeFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := platformconfig.NewRepository(pool)

	const id = "pfc_corrupt_test1"
	testpg.WithConstraintDropped(t, pool, "app_platform_configs", "chk_app_platform_configs_scope", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO app_platform_configs (id, application_code, section, property, scope, value_type, value)
		 VALUES ($1, 'corruptapp', 'sec', 'prop1', 'NOT_A_REAL_SCOPE', 'PLAIN', 'v')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM app_platform_configs WHERE id = $1`, id)
		})
	})

	c, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt scope must fail the read, not round-trip it")
	assert.Nil(t, c)
	assert.Contains(t, err.Error(), "CORRUPT_PLATFORM_CONFIG_SCOPE")
}

// TestFindByID_CorruptValueTypeFailsLoudly mirrors the above for the
// value_type column.
func TestFindByID_CorruptValueTypeFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := platformconfig.NewRepository(pool)

	const id = "pfc_corrupt_test2"
	testpg.WithConstraintDropped(t, pool, "app_platform_configs", "chk_app_platform_configs_value_type", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO app_platform_configs (id, application_code, section, property, scope, value_type, value)
		 VALUES ($1, 'corruptapp', 'sec', 'prop2', 'GLOBAL', 'NOT_A_VALUE_TYPE', 'v')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM app_platform_configs WHERE id = $1`, id)
		})
	})

	c, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt value type must fail the read, not round-trip it")
	assert.Nil(t, c)
	assert.Contains(t, err.Error(), "CORRUPT_PLATFORM_CONFIG_VALUE_TYPE")
}

// TestFindConfigsByApplication_CorruptScopeFailsTheWholeList pins the
// ruling's list semantics explicitly: "a list containing the row fails
// too" — one bad row must not be silently skipped or coerced while the
// rest of the list is returned successfully.
func TestFindConfigsByApplication_CorruptScopeFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := platformconfig.NewRepository(pool)

	const goodID = "pfc_cl_good012345"
	const badID = "pfc_cl_bad0012345"
	_, err := pool.Exec(ctx,
		`INSERT INTO app_platform_configs (id, application_code, section, property, scope, value_type, value)
		 VALUES ($1, 'corruptlistapp', 'sec', 'good', 'GLOBAL', 'PLAIN', 'v')`, goodID)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "app_platform_configs", "chk_app_platform_configs_scope", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO app_platform_configs (id, application_code, section, property, scope, value_type, value)
		 VALUES ($1, 'corruptlistapp', 'sec', 'bad', 'NOT_A_REAL_SCOPE', 'PLAIN', 'v')`, badID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM app_platform_configs WHERE id = $1`, badID)
		})
	})

	rows, err := repo.FindConfigsByApplication(ctx, "corruptlistapp")
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_PLATFORM_CONFIG_SCOPE")
}
