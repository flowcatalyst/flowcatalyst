//go:build integration

package openapispecs_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/openapispecs"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestFindByID_CorruptStatusFailsLoudly is the X-06 read boundary: a row
// whose status column holds a value that isn't one of the known Status
// constants (junk written before write-boundary validation existed, or a
// hand-edited row) must fail the read with a distinct error, never
// round-trip as that literal string and never silently coerce to CURRENT.
func TestFindByID_CorruptStatusFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := openapispecs.NewRepository(pool)

	const appID = "app_corruptspec01"
	_, err := pool.Exec(ctx,
		`INSERT INTO app_applications (id, type, code, name) VALUES ($1, 'APPLICATION', 'corrupt-spec-app', 'App')`, appID)
	require.NoError(t, err)

	const id = "oas_corrupt_test1"
	testpg.WithConstraintDropped(t, pool, "app_application_openapi_specs", "chk_app_application_openapi_specs_status", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO app_application_openapi_specs (id, application_id, version, status, spec, spec_hash)
		 VALUES ($1, $2, 'v1', 'NOT_A_REAL_STATUS', '{}'::jsonb, 'hash1')`, id, appID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM app_application_openapi_specs WHERE id = $1`, id)
		})
	})

	spec, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt status must fail the read, not round-trip it")
	assert.Nil(t, spec)
	assert.Contains(t, err.Error(), "CORRUPT_OPENAPI_SPEC_STATUS")
}

// TestFindAllByApplication_CorruptStatusFailsTheWholeList pins the ruling's
// list semantics explicitly: "a list containing the row fails too" — one
// bad row must not be silently skipped or coerced while the rest of the
// list is returned successfully.
func TestFindAllByApplication_CorruptStatusFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := openapispecs.NewRepository(pool)

	const appID = "app_corruptspec02"
	_, err := pool.Exec(ctx,
		`INSERT INTO app_applications (id, type, code, name) VALUES ($1, 'APPLICATION', 'corruptlist-spec-app', 'App')`, appID)
	require.NoError(t, err)

	const goodID = "oas_cl_good012345"
	const badID = "oas_cl_bad0012345"
	_, err = pool.Exec(ctx,
		`INSERT INTO app_application_openapi_specs (id, application_id, version, status, spec, spec_hash)
		 VALUES ($1, $2, 'v1', 'ARCHIVED', '{}'::jsonb, 'hash-good')`, goodID, appID)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "app_application_openapi_specs", "chk_app_application_openapi_specs_status", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO app_application_openapi_specs (id, application_id, version, status, spec, spec_hash)
		 VALUES ($1, $2, 'v2', 'NOT_A_REAL_STATUS', '{}'::jsonb, 'hash-bad')`, badID, appID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM app_application_openapi_specs WHERE id = $1`, badID)
		})
	})

	rows, err := repo.FindAllByApplication(ctx, appID)
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_OPENAPI_SPEC_STATUS")
}
