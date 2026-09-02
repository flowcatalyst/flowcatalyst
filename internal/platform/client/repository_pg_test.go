//go:build integration

package client_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestFindByID_CorruptStatusFailsLoudly is the X-06 read boundary: a row
// whose status column holds a value that isn't one of the known Status
// constants (junk written before write-boundary validation existed, or a
// hand-edited row) must fail the read with a distinct error, never
// round-trip as that literal string and never silently coerce to ACTIVE.
func TestFindByID_CorruptStatusFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := client.NewRepository(pool)

	const id = "clt_corrupt_test1"
	testpg.WithConstraintDropped(t, pool, "tnt_clients", "chk_tnt_clients_status", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO tnt_clients (id, name, identifier, status)
				 VALUES ($1, 'Corrupt', 'corrupt-client-test1', 'NOT_A_REAL_STATUS')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM tnt_clients WHERE id = $1`, id)
		})
	})

	c, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt status must fail the read, not round-trip it")
	assert.Nil(t, c)
	assert.Contains(t, err.Error(), "CORRUPT_CLIENT_STATUS")
}

// TestFindAll_CorruptStatusFailsTheWholeList pins the ruling's list
// semantics explicitly: "a list containing the row fails too" — one bad row
// must not be silently skipped or coerced while the rest of the list is
// returned successfully.
func TestFindAll_CorruptStatusFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := client.NewRepository(pool)

	const goodID = "clt_cl_good012345"
	const badID = "clt_cl_bad0012345"
	_, err := pool.Exec(ctx,
		`INSERT INTO tnt_clients (id, name, identifier, status)
		 VALUES ($1, 'Good', 'corruptlist-good-client', 'ACTIVE')`, goodID)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "tnt_clients", "chk_tnt_clients_status", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO tnt_clients (id, name, identifier, status)
				 VALUES ($1, 'Bad', 'corruptlist-bad-client', 'NOT_A_REAL_STATUS')`, badID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM tnt_clients WHERE id = $1`, badID)
		})
	})

	rows, err := repo.FindAll(ctx)
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_CLIENT_STATUS")
}
