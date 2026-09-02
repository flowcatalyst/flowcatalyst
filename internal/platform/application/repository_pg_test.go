//go:build integration

package application_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestFindByID_CorruptTypeFailsLoudly is the X-06 read boundary: a row
// whose type column holds a value that isn't one of the known Type
// constants (junk written before write-boundary validation existed, or a
// hand-edited row) must fail the read with a distinct error, never
// round-trip as that literal string and never silently coerce to
// APPLICATION.
func TestFindByID_CorruptTypeFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := application.NewRepository(pool)

	const id = "app_corrupt_test1"
	// type is CHECK-constrained since migration 051; drop it for the seed
	// insert to simulate a row written before the constraint existed.
	testpg.WithConstraintDropped(t, pool, "app_applications", "chk_app_applications_type", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO app_applications (id, type, code, name)
			 VALUES ($1, 'NOT_A_REAL_TYPE', 'corrupt-app-test1', 'Corrupt')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM app_applications WHERE id = $1`, id)
		})
	})

	a, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt type must fail the read, not round-trip it")
	assert.Nil(t, a)
	assert.Contains(t, err.Error(), "CORRUPT_APPLICATION_TYPE")
}

// TestFindWithFilters_CorruptTypeFailsTheWholeList pins the ruling's list
// semantics explicitly: "a list containing the row fails too" — one bad row
// must not be silently skipped or coerced while the rest of the list is
// returned successfully.
func TestFindWithFilters_CorruptTypeFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := application.NewRepository(pool)

	const goodID = "app_cl_good012345"
	const badID = "app_cl_bad0012345"
	_, err := pool.Exec(ctx,
		`INSERT INTO app_applications (id, type, code, name)
		 VALUES ($1, 'APPLICATION', 'corruptlist-good-app', 'Good')`, goodID)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "app_applications", "chk_app_applications_type", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO app_applications (id, type, code, name)
			 VALUES ($1, 'NOT_A_REAL_TYPE', 'corruptlist-bad-app', 'Bad')`, badID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM app_applications WHERE id = $1`, badID)
		})
	})

	rows, err := repo.FindWithFilters(ctx, nil, nil)
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_APPLICATION_TYPE")
}
