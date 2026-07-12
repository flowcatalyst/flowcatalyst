//go:build integration

package scheduledjob_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestHasActiveInstance_DeliveredIsTerminalWhenNotTrackingCompletion covers a
// real reported bug: a job that fires every minute and never tracks
// completion showed a permanent "Running" badge in the UI. A DELIVERED
// instance is only "active" for jobs that track completion (and haven't
// completed yet) — for jobs that don't, DELIVERED is itself terminal, so an
// old delivered instance must not keep the badge lit forever.
func TestHasActiveInstance_DeliveredIsTerminalWhenNotTrackingCompletion(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	jobs := scheduledjob.NewRepository(pool)
	instances := scheduledjob.NewInstanceRepository(pool)
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	created, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateScheduledJob(jobs), operations.CreateCommand{
		Code:             "sjinst-no-tracking",
		Name:             "SJ Instance No Tracking",
		Crons:            []string{"0 0 * * * *"},
		TracksCompletion: false,
	}, ec)
	require.NoError(t, err)
	jobID := created.ScheduledJobID

	fired, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.FireNow(jobs, instances),
		operations.FireNowCommand{ID: jobID}, ec)
	require.NoError(t, err)
	instanceID := fired.InstanceID

	require.NoError(t, instances.MarkInFlight(ctx, instanceID))
	require.NoError(t, instances.MarkDelivered(ctx, instanceID))

	active, err := instances.HasActiveInstance(ctx, jobID, false)
	require.NoError(t, err)
	assert.False(t, active, "a delivered instance of a non-tracking job must not count as active")
}

// TestHasActiveInstance_DeliveredIsActiveUntilCompletedWhenTrackingCompletion
// covers the other half: for a job that DOES track completion, a delivered
// instance stays "active" until MarkComplete is called.
func TestHasActiveInstance_DeliveredIsActiveUntilCompletedWhenTrackingCompletion(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	jobs := scheduledjob.NewRepository(pool)
	instances := scheduledjob.NewInstanceRepository(pool)
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	created, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateScheduledJob(jobs), operations.CreateCommand{
		Code:             "sjinst-tracking",
		Name:             "SJ Instance Tracking",
		Crons:            []string{"0 0 * * * *"},
		TracksCompletion: true,
	}, ec)
	require.NoError(t, err)
	jobID := created.ScheduledJobID

	fired, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.FireNow(jobs, instances),
		operations.FireNowCommand{ID: jobID}, ec)
	require.NoError(t, err)
	instanceID := fired.InstanceID

	require.NoError(t, instances.MarkInFlight(ctx, instanceID))
	require.NoError(t, instances.MarkDelivered(ctx, instanceID))

	active, err := instances.HasActiveInstance(ctx, jobID, true)
	require.NoError(t, err)
	assert.True(t, active, "a delivered instance of a tracking job must stay active until completed")

	require.NoError(t, instances.MarkComplete(ctx, instanceID, scheduledjob.InstanceStatusCompleted, nil, nil))

	active, err = instances.HasActiveInstance(ctx, jobID, true)
	require.NoError(t, err)
	assert.False(t, active, "a completed instance must no longer count as active")
}
