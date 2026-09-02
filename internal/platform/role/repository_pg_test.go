//go:build integration

package role_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestFindByID_CorruptSourceFailsLoudly is the X-06 read boundary: a row
// whose source column holds a value that isn't one of the known Source
// constants (junk written before write-boundary validation existed, or a
// hand-edited row) must fail the read with a distinct error, never
// round-trip as that literal string and never silently coerce to DATABASE.
func TestFindByID_CorruptSourceFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := role.NewRepository(pool)

	const id = "rol_corrupt_test1"
	testpg.WithConstraintDropped(t, pool, "iam_roles", "chk_iam_roles_source", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO iam_roles (id, name, display_name, source)
		 VALUES ($1, 'corrupt:role:test1', 'Corrupt', 'NOT_A_REAL_SOURCE')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM iam_roles WHERE id = $1`, id)
		})
	})

	r, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt source must fail the read, not round-trip it")
	assert.Nil(t, r)
	assert.Contains(t, err.Error(), "CORRUPT_ROLE_SOURCE")
}

// TestFindAll_CorruptSourceFailsTheWholeList pins the ruling's list
// semantics explicitly: "a list containing the row fails too" — one bad row
// must not be silently skipped or coerced while the rest of the list is
// returned successfully.
func TestFindAll_CorruptSourceFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := role.NewRepository(pool)

	const goodID = "rol_cl_good012345"
	const badID = "rol_cl_bad0012345"
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_roles (id, name, display_name, source)
		 VALUES ($1, 'corruptlist:good:role', 'Good', 'CODE')`, goodID)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "iam_roles", "chk_iam_roles_source", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO iam_roles (id, name, display_name, source)
		 VALUES ($1, 'corruptlist:bad:role', 'Bad', 'NOT_A_REAL_SOURCE')`, badID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM iam_roles WHERE id = $1`, badID)
		})
	})

	rows, err := repo.FindAll(ctx)
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_ROLE_SOURCE")
}
