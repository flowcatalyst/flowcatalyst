//go:build integration

package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// newJob creates an ACTIVE job through the public operation.
func newJob(t *testing.T, jobs *scheduledjob.Repository, code, cron string, targetURL *string, timeout *int32) string {
	t.Helper()
	created, err := usecaseop.Run(testpg.AnchorCtx(), testpg.NewUoW(t), operations.CreateScheduledJob(jobs),
		operations.CreateCommand{Code: code, Name: code, Crons: []string{cron}, TargetURL: targetURL, TimeoutSeconds: timeout},
		testpg.TestEC())
	require.NoError(t, err)
	return created.ScheduledJobID
}

func deleteJob(t *testing.T, jobs *scheduledjob.Repository, id string) {
	t.Helper()
	_, err := usecaseop.Run(testpg.AnchorCtx(), testpg.NewUoW(t), operations.DeleteScheduledJob(jobs),
		operations.DeleteCommand{ID: id}, testpg.TestEC())
	require.NoError(t, err)
}

func countInstances(t *testing.T, instances *scheduledjob.InstanceRepository, jobID string) int64 {
	t.Helper()
	n, err := instances.Count(context.Background(), scheduledjob.InstanceListFilters{ScheduledJobID: &jobID})
	require.NoError(t, err)
	return n
}

// Several pollers (split brain / failover overlap: each believes it is leader)
// tick over the same instant. The database tick claim must let exactly one
// instance through.
func TestPoller_TwoSchedulersRacingSameTick_FireOnce(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	jobs := scheduledjob.NewRepository(pool)
	instances := scheduledjob.NewInstanceRepository(pool)

	jobID := newJob(t, jobs, "sjonce-race", "* * * * * *", nil, nil)
	defer deleteJob(t, jobs, jobID)
	time.Sleep(1100 * time.Millisecond) // a whole second slot now lies after created_at
	now := time.Now().UTC()
	fixed := func() time.Time { return now }

	const pollers = 4
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range pollers {
		p := &poller{cfg: DefaultConfig(), jobs: jobs, instances: instances, isLeader: func() bool { return true }, now: fixed}
		wg.Go(func() {
			<-start
			_, err := p.tick(ctx)
			assert.NoError(t, err)
		})
	}
	close(start)
	wg.Wait()
	require.EqualValues(t, 1, countInstances(t, instances, jobID), "concurrent pollers must fire the tick once")

	// Deterministic split brain: a poller holding a stale view of the job (it
	// read last_fired_at before the other fired) re-fires the SAME slot.
	got, err := instances.List(ctx, scheduledjob.InstanceListFilters{ScheduledJobID: &jobID})
	require.NoError(t, err)
	require.Len(t, got, 1)
	dup := got[0]
	dup.ID = "sji-dup-" + jobID[len(jobID)-6:]
	inserted, err := instances.InsertCronTick(ctx, &dup)
	require.NoError(t, err)
	assert.False(t, inserted, "a claimed tick must not insert a second instance")
	require.EqualValues(t, 1, countInstances(t, instances, jobID))

	// A later tick is a different claim and does fire.
	later := *got[0].ScheduledFor
	later = later.Add(time.Second)
	next := got[0]
	next.ID = "sji-nxt-" + jobID[len(jobID)-6:]
	next.ScheduledFor = &later
	inserted, err = instances.InsertCronTick(ctx, &next)
	require.NoError(t, err)
	assert.True(t, inserted)
}

// An instance orphaned IN_FLIGHT by a crash is abandoned (DELIVERY_FAILED with
// an explicit reason + a log row), and is never re-sent.
func TestDispatcher_StuckInFlight_AbandonedWithoutRefire(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	jobs := scheduledjob.NewRepository(pool)
	instances := scheduledjob.NewInstanceRepository(pool)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	url := srv.URL

	jobID := newJob(t, jobs, "sjstuck-job", "0 0 0 1 1 *", &url, nil)
	defer deleteJob(t, jobs, jobID)
	ec := testpg.TestEC()
	mk := func() string {
		fired, err := usecaseop.Run(testpg.AnchorCtx(), testpg.NewUoW(t), operations.FireNow(jobs, instances),
			operations.FireNowCommand{ID: jobID}, ec)
		require.NoError(t, err)
		return fired.InstanceID
	}
	stuck, fresh := mk(), mk()
	// Both go IN_FLIGHT (as if the dispatcher died right after claiming them);
	// only `stuck` has been that way for longer than the grace period.
	require.NoError(t, instances.MarkInFlight(ctx, stuck))
	require.NoError(t, instances.MarkInFlight(ctx, fresh))
	_, err := pool.Exec(ctx, `UPDATE msg_scheduled_job_instances SET in_flight_at = NOW() - INTERVAL '2 hours' WHERE id = $1`, stuck)
	require.NoError(t, err)

	d := &dispatcher{
		cfg: DefaultConfig(), jobs: jobs, instances: instances,
		http: &http.Client{Timeout: time.Second}, isLeader: func() bool { return true },
	}
	d.recoverStuck(ctx)
	require.NoError(t, d.tick(ctx)) // would re-send a QUEUED instance
	d.recoverStuck(ctx)
	require.NoError(t, d.tick(ctx))

	got, err := instances.FindByID(ctx, stuck)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, scheduledjob.InstanceStatusDeliveryFailed, got.Status)
	require.NotNil(t, got.DeliveryError)
	assert.Contains(t, *got.DeliveryError, "ABANDONED")
	assert.Contains(t, *got.DeliveryError, "NOT re-fired")
	assert.EqualValues(t, 0, calls.Load(), "an abandoned instance must not be sent again")

	logs, err := instances.ListLogs(ctx, stuck, 10)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "ERROR", logs[0].Level)
	assert.Contains(t, logs[0].Message, "ABANDONED")

	f, err := instances.FindByID(ctx, fresh)
	require.NoError(t, err)
	assert.Equal(t, scheduledjob.InstanceStatusInFlight, f.Status, "a recent IN_FLIGHT instance is left alone")
}

// The job's timeoutSeconds, not a fixed 10 s, bounds the delivery call.
func TestDispatcher_HonoursJobTimeoutSeconds(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	jobs := scheduledjob.NewRepository(pool)
	instances := scheduledjob.NewInstanceRepository(pool)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select { // slower than the job's timeout, faster than the default
		case <-time.After(4 * time.Second):
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	url := srv.URL
	timeout := int32(1)
	jobID := newJob(t, jobs, "sjtimeout-job", "0 0 0 1 1 *", &url, &timeout)
	defer deleteJob(t, jobs, jobID)
	fired, err := usecaseop.Run(testpg.AnchorCtx(), testpg.NewUoW(t), operations.FireNow(jobs, instances),
		operations.FireNowCommand{ID: jobID}, testpg.TestEC())
	require.NoError(t, err)

	d := &dispatcher{
		cfg: DefaultConfig(), jobs: jobs, instances: instances,
		http: &http.Client{}, isLeader: func() bool { return true },
	}
	began := time.Now()
	require.NoError(t, d.tick(ctx))
	elapsed := time.Since(began)
	assert.Less(t, elapsed, 3*time.Second, "call must be cut at the job's 1s timeout, not wait for the 4s response")
	assert.GreaterOrEqual(t, elapsed, 900*time.Millisecond)

	got, err := instances.FindByID(ctx, fired.InstanceID)
	require.NoError(t, err)
	require.NotNil(t, got.DeliveryError)
	assert.Contains(t, *got.DeliveryError, "deadline exceeded")
}

// A schedule shorter than the 30 s poll interval fires at its slots, and the
// firing is delivered promptly (the poller wakes the dispatcher), even though
// the configured poll is 30 s and dispatch is 5 s.
func TestPoller_SubPollIntervalScheduleFiresOnTime(t *testing.T) {
	pool := testpg.Pool(t)
	jobs := scheduledjob.NewRepository(pool)
	instances := scheduledjob.NewInstanceRepository(pool)

	var mu sync.Mutex
	var received []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		received = append(received, time.Now())
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	url := srv.URL
	jobID := newJob(t, jobs, "sjsub30-job", "*/2 * * * * *", &url, nil)
	defer deleteJob(t, jobs, jobID)

	cfg := DefaultConfig() // PollInterval 30s, DispatchInterval 5s
	wake := make(chan struct{}, 1)
	p := &poller{cfg: cfg, jobs: jobs, instances: instances, isLeader: func() bool { return true }, wake: wake}
	d := &dispatcher{
		cfg: cfg, jobs: jobs, instances: instances, http: &http.Client{Timeout: 2 * time.Second},
		isLeader: func() bool { return true }, wake: wake,
	}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() { p.run(ctx) })
	wg.Go(func() { d.run(ctx) })
	time.Sleep(9 * time.Second)
	cancel()
	wg.Wait()

	list, err := instances.List(context.Background(), scheduledjob.InstanceListFilters{ScheduledJobID: &jobID})
	require.NoError(t, err)
	// 9 s of a 2 s schedule: the slots at +2/+4/+6/+8 s, minus startup slop. A
	// 30 s poll would have produced at most one firing.
	assert.GreaterOrEqual(t, len(list), 3, "sub-30s schedule must fire repeatedly")
	for _, inst := range list {
		require.NotNil(t, inst.ScheduledFor)
		assert.Zero(t, inst.ScheduledFor.Second()%2, "fires at its own slots")
		assert.Less(t, inst.FiredAt.Sub(*inst.ScheduledFor), 1500*time.Millisecond,
			"queued within tolerance of its slot (slot %s, queued %s)", inst.ScheduledFor, inst.FiredAt)
	}
	mu.Lock()
	defer mu.Unlock()
	assert.GreaterOrEqual(t, len(received), 3, "firings delivered on time, not on the 5s dispatch tick")
}
