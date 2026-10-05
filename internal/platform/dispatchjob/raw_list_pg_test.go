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

// TestFindWithFilters_MessageGroupAndProjectedColumns pins the 2026-09-22
// grid additions on the read projection: an exact message_group filter, and
// descriptor + metadata coming back on list rows (migration 057 projected
// them; the readSelect must name them or the grid shows nothing).
func TestFindWithFilters_MessageGroupAndProjectedColumns(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)

	const code = "grouptest:jobs:list"
	seed := func(id, group, descriptor string) {
		t.Helper()
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_dispatch_jobs_read
			     (id, code, target_url, kind, protocol, mode, status, max_retries, updated_at,
			      message_group, descriptor, metadata)
			 VALUES ($1, $2, 'http://example.invalid/hook',
			         'EVENT', 'HTTP_WEBHOOK', 'IMMEDIATE', 'PENDING', 3, NOW(),
			         $3, $4, '[{"key":"tenant","value":"acme"}]')`,
			id, code, group, descriptor)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_dispatch_jobs_read WHERE id = $1`, id)
		})
	}
	seed("djgrouptest01", "order-1", "Notify Value of orders")
	seed("djgrouptest02", "order-2", "Notify Value of orders")

	group := "order-1"
	rows, err := repo.FindWithFilters(ctx, dispatchjob.FilterParams{Codes: []string{code}, MessageGroup: &group})
	require.NoError(t, err)
	require.Len(t, rows, 1, "messageGroup is an exact filter")
	assert.Equal(t, "djgrouptest01", rows[0].ID)
	require.NotNil(t, rows[0].Descriptor)
	assert.Equal(t, "Notify Value of orders", *rows[0].Descriptor)
	require.Len(t, rows[0].Metadata, 1, "metadata is projected onto list rows")
	assert.Equal(t, "tenant", rows[0].Metadata[0].Key)
	assert.Equal(t, "acme", rows[0].Metadata[0].Value)
}
