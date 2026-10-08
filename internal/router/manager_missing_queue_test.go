package router

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// The platform creates a dispatch queue on the first message published to it,
// so a queue the router is configured to consume may not exist yet. That is not
// a fault: no consumer, no warning, and a consumer as soon as the queue appears.

const missingQueueScheme = "testmissingqueue"

var queueExists atomic.Bool

func init() {
	queue.RegisterConsumer(missingQueueScheme, func(_ context.Context, cfg common.QueueConfig) (queue.Consumer, error) {
		if !queueExists.Load() {
			return nil, fmt.Errorf("%s: %w", cfg.Name, queue.ErrQueueMissing)
		}
		return &pollErrConsumer{id: cfg.Name}, nil
	})
}

func hasConsumer(m *Manager, name string) bool {
	m.consumerMu.Lock()
	defer m.consumerMu.Unlock()
	_, ok := m.consumers[name]
	return ok
}

func TestMissingQueueIsNeitherAFailureNorAWarningAndIsPickedUpWhenItAppears(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	ws := NewWarningService(DefaultWarningServiceConfig())
	m.SetWarnings(ws)
	queueExists.Store(false)

	cfg := routerCfg([]string{"q-live"})
	cfg.Queues = append(cfg.Queues, common.QueueConfig{Name: "q-new", URI: missingQueueScheme + "://q-new"})

	require.NoError(t, m.Reconfigure(context.Background(), cfg),
		"a queue nobody has published to yet must not fail the apply")
	assert.True(t, polled(t, fakeQueueFor(t, "q-live"), 1, time.Second), "the other queues are consumed")
	assert.False(t, hasConsumer(m, "q-new"), "no consumer for a queue that does not exist")
	assert.Empty(t, ws.Active(60), "a missing queue raises no warning of any kind")

	// Still absent: a recheck changes nothing, and stays silent.
	m.RecheckMissingQueues(context.Background())
	assert.False(t, hasConsumer(m, "q-new"))
	assert.Empty(t, ws.Active(60))

	// The first publish creates the queue; an UNCHANGED config is never
	// re-applied, so the recheck is what starts the consumer.
	queueExists.Store(true)
	m.RecheckMissingQueues(context.Background())
	assert.True(t, hasConsumer(m, "q-new"), "the consumer starts once the queue exists")
	m.consumerMu.Lock()
	_, stillMissing := m.missingQueues["q-new"]
	m.consumerMu.Unlock()
	assert.False(t, stillMissing, "and it is no longer tracked as missing")
}

// A queue that disappears under a running consumer is retired, not reported as
// an erroring or stalled consumer, and comes back through the recheck.
func TestQueueDeletedUnderARunningConsumerIsRetiredQuietly(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	ws := NewWarningService(DefaultWarningServiceConfig())
	m.SetWarnings(ws)

	// A default pool with room, so runConsumer reaches Poll.
	m.pools[defaultPoolCode] = NewPool(
		common.PoolConfig{Code: defaultPoolCode, Concurrency: 8},
		nil, nil, func(string) queue.Consumer { return nil },
	)

	gone := &pollErrConsumer{id: "q-gone", err: queue.ErrQueueMissing}
	qc := common.QueueConfig{Name: "q-gone", URI: missingQueueScheme + "://q-gone"}
	rc, pollCtx := newRunningConsumer(m.consumerRoot(), gone, qc)
	m.consumerMu.Lock()
	m.consumers[qc.Name] = rc
	m.consumersByID[gone.Identifier()] = rc
	m.queues[qc.Name] = qc
	m.consumerMu.Unlock()

	m.wg.Add(1)
	go m.runConsumer(pollCtx, rc)

	require.Eventually(t, func() bool { return !hasConsumer(m, "q-gone") }, 2*time.Second, 10*time.Millisecond,
		"the consumer of a vanished queue is detached")
	assert.Equal(t, int64(1), gone.polls.Load(), "and it stops polling at once")
	assert.Empty(t, ws.Active(60), "no warning")

	m.consumerMu.Lock()
	_, tracked := m.missingQueues["q-gone"]
	m.consumerMu.Unlock()
	assert.True(t, tracked, "it is tracked as missing so the recheck can bring it back")
}
