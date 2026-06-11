//go:build integration

package scheduler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestDispatcherTick_OrphanInstance_MarkedDeliveryFailed covers the orphan
// path: a QUEUED instance whose job was deleted while it sat in the queue
// must be marked terminally DELIVERY_FAILED by the next dispatcher tick
// (mirrors the Rust tick — "ScheduledJob no longer exists"), not left
// QUEUED forever. No FK/CASCADE exists by design: instances are firing
// history and outlive their job.
func TestDispatcherTick_OrphanInstance_MarkedDeliveryFailed(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	jobs := scheduledjob.NewRepository(pool)
	instances := scheduledjob.NewInstanceRepository(pool)
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	// Create job → FireNow (MANUAL instance, QUEUED) → delete the job, all
	// through the public operations — the same path production uses.
	created, err := operations.CreateScheduledJob(ctx, jobs, uow, operations.CreateCommand{
		Code:  "sjdsp-orphan",
		Name:  "SJ Dispatcher Orphan",
		Crons: []string{"0 0 * * * *"},
	}, ec)
	require.NoError(t, err)
	jobID := created.Event().ScheduledJobID

	fired, err := operations.FireNow(ctx, jobs, instances, uow,
		operations.FireNowCommand{ID: jobID}, ec)
	require.NoError(t, err)
	instanceID := fired.Event().InstanceID

	inst, err := instances.FindByID(ctx, instanceID)
	require.NoError(t, err)
	require.NotNil(t, inst)
	require.Equal(t, scheduledjob.InstanceStatusQueued, inst.Status, "FireNow must enqueue the instance")

	_, err = operations.DeleteScheduledJob(ctx, jobs, uow,
		operations.DeleteCommand{ID: jobID}, ec)
	require.NoError(t, err)
	gone, err := jobs.FindByID(ctx, jobID)
	require.NoError(t, err)
	require.Nil(t, gone, "job row must be gone — the instance is now an orphan")

	// One dispatcher tick. The dispatcher is constructed directly (same shape
	// NewService wires); tick is the unit run claims QUEUED instances with.
	d := &dispatcher{
		cfg:       Config{DispatchInterval: time.Second, DispatchBatchSize: 32, HTTPTimeout: time.Second},
		jobs:      jobs,
		instances: instances,
		http:      &http.Client{Timeout: time.Second},
		isLeader:  func() bool { return true },
	}
	require.NoError(t, d.tick(ctx))

	got, err := instances.FindByID(ctx, instanceID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, scheduledjob.InstanceStatusDeliveryFailed, got.Status,
		"orphan instance must be terminally DELIVERY_FAILED, not stuck QUEUED")
	require.NotNil(t, got.DeliveryError)
	assert.Equal(t, "ScheduledJob no longer exists", *got.DeliveryError)

	// Terminal means terminal: a second tick must not resurrect or re-touch it.
	require.NoError(t, d.tick(ctx))
	again, err := instances.FindByID(ctx, instanceID)
	require.NoError(t, err)
	require.NotNil(t, again)
	assert.Equal(t, scheduledjob.InstanceStatusDeliveryFailed, again.Status)
}
