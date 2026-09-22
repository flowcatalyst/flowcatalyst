//go:build integration

package dispatchjob_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// TestFindRecentRaw_AgreesWithFindByID pins GET /bff/debug/dispatch-jobs's
// query against the sqlc FindByID row it is mapped onto. FindRecentRaw is a
// hand-written SELECT scanned with pgx.RowToStructByName[DispatchJobFindByIDRow];
// a column added to the sqlc query (queue, 2026-09-18) but not to this
// SELECT makes the scan fail for EVERY row — the debug page 500s outright,
// and nothing else reads this path, so it went unnoticed until the owner hit
// it (2026-09-22). The events repo has the same shape of pin (TestFindRawByID).
func TestFindRecentRaw_AgreesWithFindByID(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)

	id := tsid.GenerateUntyped()
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_dispatch_jobs (id, code, target_url, status, queue, payload, metadata)
		 VALUES ($1, 'rawlist:test', 'http://example.invalid/hook', 'PENDING', 'HIGH_PRIORITY',
		         '{"n":1}', '{"k":"v"}')`, id)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM msg_dispatch_jobs WHERE id = $1`, id)
	})

	byID, err := repo.FindByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, byID)

	recent, err := repo.FindRecentRaw(ctx, 1000)
	require.NoError(t, err, "the raw list must scan — every column the FindByID row has must be selected")
	var fromList *dispatchjob.DispatchJob
	for i := range recent {
		if recent[i].ID == id {
			fromList = &recent[i]
			break
		}
	}
	require.NotNil(t, fromList, "seeded row must appear in FindRecentRaw")
	assert.Equal(t, *byID, *fromList, "FindByID and FindRecentRaw must agree on the row's shape — queue included")
}
