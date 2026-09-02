//go:build integration

package process_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
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
	repo := process.NewRepository(pool)

	const id = "prc_corrupt_test1"
	testpg.WithConstraintDropped(t, pool, "msg_processes", "chk_msg_processes_status", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_processes (id, code, name, status, application, subdomain, process_name)
		 VALUES ($1, 'corrupt-code-1', 'Corrupt', 'NOT_A_REAL_STATUS', 'app', 'sub', 'proc')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_processes WHERE id = $1`, id)
		})
	})

	p, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt status must fail the read, not round-trip it")
	assert.Nil(t, p)
	assert.Contains(t, err.Error(), "CORRUPT_PROCESS_STATUS")
}

// TestFindByID_CorruptSourceFailsLoudly mirrors the above for the source
// column.
func TestFindByID_CorruptSourceFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := process.NewRepository(pool)

	const id = "prc_corrupt_test2"
	testpg.WithConstraintDropped(t, pool, "msg_processes", "chk_msg_processes_source", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_processes (id, code, name, source, application, subdomain, process_name)
		 VALUES ($1, 'corrupt-code-2', 'Corrupt', 'NOT_A_REAL_SOURCE', 'app', 'sub', 'proc')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_processes WHERE id = $1`, id)
		})
	})

	p, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt source must fail the read, not round-trip it")
	assert.Nil(t, p)
	assert.Contains(t, err.Error(), "CORRUPT_PROCESS_SOURCE")
}

// TestFindWithFilters_CorruptStatusFailsTheWholeList pins the ruling's list
// semantics explicitly: "a list containing the row fails too" — one bad row
// must not be silently skipped or coerced while the rest of the list is
// returned successfully.
func TestFindWithFilters_CorruptStatusFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := process.NewRepository(pool)

	const goodID = "prc_cl_good012345"
	const badID = "prc_cl_bad0012345"
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_processes (id, code, name, status, application, subdomain, process_name)
		 VALUES ($1, 'corruptlist-good', 'Good', 'CURRENT', 'app', 'sub', 'proc')`, goodID)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "msg_processes", "chk_msg_processes_status", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO msg_processes (id, code, name, status, application, subdomain, process_name)
		 VALUES ($1, 'corruptlist-bad', 'Bad', 'NOT_A_REAL_STATUS', 'app', 'sub', 'proc')`, badID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_processes WHERE id = $1`, badID)
		})
	})

	rows, err := repo.FindWithFilters(ctx, nil, nil, nil)
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_PROCESS_STATUS")
}
