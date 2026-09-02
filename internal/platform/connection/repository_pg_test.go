//go:build integration

package connection_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
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
	repo := connection.NewRepository(pool)

	const id = "cnx_corrupt_test1"
	testpg.WithConstraintDropped(t, pool, "msg_connections", "chk_msg_connections_status", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_connections (id, code, name, status, service_account_id)
		 VALUES ($1, 'corrupt-code-1', 'Corrupt', 'NOT_A_REAL_STATUS', 'sa_corrupttest001')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_connections WHERE id = $1`, id)
		})
	})

	c, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt status must fail the read, not round-trip it")
	assert.Nil(t, c)
	assert.Contains(t, err.Error(), "CORRUPT_CONNECTION_STATUS")
}

// TestFindAll_CorruptStatusFailsTheWholeList pins the ruling's list
// semantics explicitly: "a list containing the row fails too" — one bad row
// must not be silently skipped or coerced while the rest of the list is
// returned successfully.
func TestFindAll_CorruptStatusFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := connection.NewRepository(pool)

	const goodID = "cnx_cl_good012345"
	const badID = "cnx_cl_bad0012345"
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_connections (id, code, name, status, service_account_id)
		 VALUES ($1, 'corruptlist-good-cnx', 'Good', 'ACTIVE', 'sa_corrupttest001')`, goodID)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "msg_connections", "chk_msg_connections_status", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO msg_connections (id, code, name, status, service_account_id)
		 VALUES ($1, 'corruptlist-bad-cnx', 'Bad', 'NOT_A_REAL_STATUS', 'sa_corrupttest001')`, badID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_connections WHERE id = $1`, badID)
		})
	})

	rows, err := repo.FindAll(ctx)
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_CONNECTION_STATUS")
}
