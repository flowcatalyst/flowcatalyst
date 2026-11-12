//go:build integration

package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// A job that is not function-linked is delivered under the customer policy
// and refused at the dial; a function-linked job (function_id set, as only a
// promote does) to the same server goes through the internal policy and is
// delivered. Both rows target the server's loopback address, as one that
// predated the policy split would.
func TestDispatcher_FunctionLinkedJobUsesTheInternalPolicy(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	jobs := scheduledjob.NewRepository(pool)
	instances := scheduledjob.NewInstanceRepository(pool)
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	var hits atomic.Int32
	runner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer runner.Close()
	target := runner.URL + "/fn/x"

	customer := &netguard.Policy{}
	internal := &netguard.Policy{}
	internal.AllowURL(runner.URL)

	fire := func(code string, functionID *string) string {
		created, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateScheduledJob(jobs), operations.CreateCommand{
			Code: code, Name: code, Crons: []string{"0 0 * * * *"}, TargetURL: &target,
		}, ec)
		require.NoError(t, err)
		if functionID != nil {
			_, err = pool.Exec(ctx, `UPDATE msg_scheduled_jobs SET function_id = $1 WHERE id = $2`, *functionID, created.ScheduledJobID)
			require.NoError(t, err)
		}
		fired, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.FireNow(jobs, instances),
			operations.FireNowCommand{ID: created.ScheduledJobID}, ec)
		require.NoError(t, err)
		return fired.InstanceID
	}
	fnID := "fn_policy_test01"
	custInst := fire("sjpol-customer", nil)
	fnInst := fire("sjpol-function", &fnID)

	d := &dispatcher{
		cfg:       Config{DispatchInterval: time.Second, DispatchBatchSize: 32, HTTPTimeout: 5 * time.Second},
		jobs:      jobs,
		instances: instances,
		http:      newDeliveryClient(customer),
		fnHTTP:    newDeliveryClient(internal),
		isLeader:  func() bool { return true },
	}
	require.NoError(t, d.tick(ctx))

	c, err := instances.FindByID(ctx, custInst)
	require.NoError(t, err)
	require.NotNil(t, c)
	require.NotNil(t, c.DeliveryError)
	assert.Contains(t, *c.DeliveryError, "destination not allowed", "the customer job records the guard's refusal")
	assert.NotEqual(t, scheduledjob.InstanceStatusCompleted, c.Status)

	f, err := instances.FindByID(ctx, fnInst)
	require.NoError(t, err)
	require.NotNil(t, f)
	assert.Nil(t, f.DeliveryError, "the function-linked job is delivered")
	assert.EqualValues(t, 1, hits.Load(), "exactly the function-linked job reached the server")
}
