//go:build integration

package operations_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// F31: a scheduled-job sync audits each job under entity type "Scheduledjob"
// (create and update alike) and its one rollup under "Scheduledjobs" keyed by
// the application code, the way every other sync's rollup is ("Roles",
// "Connections", ...). The rollup used to be keyed "synced".
func TestSyncScheduledJobs_AuditEntityTypesAreConsistent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := scheduledjob.NewRepository(pool)
	uow := testpg.NewUoW(t)
	start := time.Now().UTC().Add(-time.Second)
	clientID := "cli_f31audit01"

	sync := func(name string) {
		t.Helper()
		_, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.SyncScheduledJobs(repo), operations.SyncScheduledJobsCommand{
			ApplicationCode: "f31audit", ClientID: &clientID,
			Jobs: []operations.ScheduledJobSyncEntry{{
				Code: "f31-a", Name: name, Crons: validCrons, Timezone: "UTC", DeliveryMaxAttempts: 3,
			}},
		}, testpg.TestEC())
		require.NoError(t, err)
	}
	sync("A")  // create
	sync("A2") // update

	type row struct{ entityType, entityID string }
	var rows []row
	r, err := pool.Query(ctx, `SELECT entity_type, entity_id FROM aud_logs
		WHERE performed_at >= $1 AND operation_json::text LIKE '%f31audit%' ORDER BY performed_at, id`, start)
	require.NoError(t, err)
	defer r.Close()
	for r.Next() {
		var x row
		require.NoError(t, r.Scan(&x.entityType, &x.entityID))
		rows = append(rows, x)
	}
	require.NoError(t, r.Err())

	job, err := repo.FindByCode(ctx, "f31-a", &clientID)
	require.NoError(t, err)
	require.NotNil(t, job)
	// Per sync: the job row, then the rollup.
	assert.Equal(t, []row{
		{"Scheduledjob", job.ID},
		{"Scheduledjobs", "f31audit"},
		{"Scheduledjob", job.ID},
		{"Scheduledjobs", "f31audit"},
	}, rows)
}
