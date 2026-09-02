//go:build integration

package scheduledjob_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// TestFindByID_CorruptStatusFailsLoudly is the X-06 read boundary: a row
// whose status column holds a value that isn't one of the known Status
// constants (junk written before write-boundary validation existed, or a
// hand-edited row) must fail the read with a distinct error, never
// round-trip as that literal string and never silently coerce to ACTIVE.
func TestFindByID_CorruptStatusFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := scheduledjob.NewRepository(pool)

	const id = "sjb_corrupt_test1"
	testpg.WithConstraintDropped(t, pool, "msg_scheduled_jobs", "chk_msg_scheduled_jobs_status", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_scheduled_jobs (id, code, name, status, crons)
		 VALUES ($1, 'corrupt-job-test1', 'Corrupt', 'NOT_A_REAL_STATUS', '{}')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_scheduled_jobs WHERE id = $1`, id)
		})
	})

	j, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt status must fail the read, not round-trip it")
	assert.Nil(t, j)
	assert.Contains(t, err.Error(), "CORRUPT_SCHEDULED_JOB_STATUS")
}

// TestFindWithFilters_CorruptStatusFailsTheWholeList pins the ruling's list
// semantics explicitly: "a list containing the row fails too" — one bad row
// must not be silently skipped or coerced while the rest of the list is
// returned successfully.
func TestFindWithFilters_CorruptStatusFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := scheduledjob.NewRepository(pool)

	const goodID = "sjb_cl_good012345"
	const badID = "sjb_cl_bad0012345"
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_scheduled_jobs (id, code, name, status, crons)
		 VALUES ($1, 'corruptlist-good-job', 'Good', 'ACTIVE', '{}')`, goodID)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "msg_scheduled_jobs", "chk_msg_scheduled_jobs_status", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO msg_scheduled_jobs (id, code, name, status, crons)
		 VALUES ($1, 'corruptlist-bad-job', 'Bad', 'NOT_A_REAL_STATUS', '{}')`, badID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_scheduled_jobs WHERE id = $1`, badID)
		})
	})

	rows, err := repo.FindWithFilters(ctx, scheduledjob.ListFilters{})
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_SCHEDULED_JOB_STATUS")
}

// TestInstanceFindByID_CorruptStatusFailsLoudly mirrors the job-level test
// for msg_scheduled_job_instances.status.
func TestInstanceFindByID_CorruptStatusFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := scheduledjob.NewInstanceRepository(pool)

	const id = "sji_corrupt_test1"
	testpg.WithConstraintDropped(t, pool, "msg_scheduled_job_instances", "chk_msg_scheduled_job_instances_status", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_scheduled_job_instances (id, scheduled_job_id, job_code, status)
		 VALUES ($1, 'sjb_corruptins01', 'corrupt-inst-job', 'NOT_A_REAL_STATUS')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_scheduled_job_instances WHERE id = $1`, id)
		})
	})

	inst, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt status must fail the read, not round-trip it")
	assert.Nil(t, inst)
	assert.Contains(t, err.Error(), "CORRUPT_SCHEDULED_JOB_INSTANCE_STATUS")
}

// TestInstanceFindByID_CorruptTriggerKindFailsLoudly mirrors the above for
// the trigger_kind column.
func TestInstanceFindByID_CorruptTriggerKindFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := scheduledjob.NewInstanceRepository(pool)

	const id = "sji_corrupt_test2"
	testpg.WithConstraintDropped(t, pool, "msg_scheduled_job_instances", "chk_msg_scheduled_job_instances_trigger_kind", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_scheduled_job_instances (id, scheduled_job_id, job_code, trigger_kind)
		 VALUES ($1, 'sjb_corruptins02', 'corrupt-inst-job2', 'NOT_A_REAL_TRIGGER')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_scheduled_job_instances WHERE id = $1`, id)
		})
	})

	inst, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt trigger kind must fail the read, not round-trip it")
	assert.Nil(t, inst)
	assert.Contains(t, err.Error(), "CORRUPT_SCHEDULED_JOB_INSTANCE_TRIGGER")
}

// TestInstanceList_CorruptStatusFailsTheWholeList pins the list semantics
// for msg_scheduled_job_instances.
func TestInstanceList_CorruptStatusFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := scheduledjob.NewInstanceRepository(pool)

	jobID := "sjb_corruptins03"
	const goodID = "sji_cl_good012345"
	const badID = "sji_cl_bad0012345"
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_scheduled_job_instances (id, scheduled_job_id, job_code, status)
		 VALUES ($1, $2, 'corruptlist-inst-job', 'QUEUED')`, goodID, jobID)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "msg_scheduled_job_instances", "chk_msg_scheduled_job_instances_status", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO msg_scheduled_job_instances (id, scheduled_job_id, job_code, status)
		 VALUES ($1, $2, 'corruptlist-inst-job', 'NOT_A_REAL_STATUS')`, badID, jobID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_scheduled_job_instances WHERE id = $1`, badID)
		})
	})

	rows, err := repo.List(ctx, scheduledjob.InstanceListFilters{ScheduledJobID: &jobID})
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_SCHEDULED_JOB_INSTANCE_STATUS")
}
