package router

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

const (
	failBuildScheme  = "testbuildfail"
	panicBuildScheme = "testbuildpanic"
)

var (
	failBuilds  atomic.Int64
	panicBuilds atomic.Int64
	errNoBroker = errors.New("broker unreachable")
)

func init() {
	queue.RegisterConsumer(failBuildScheme, func(context.Context, common.QueueConfig) (queue.Consumer, error) {
		failBuilds.Add(1)
		return nil, errNoBroker
	})
	queue.RegisterConsumer(panicBuildScheme, func(context.Context, common.QueueConfig) (queue.Consumer, error) {
		panicBuilds.Add(1)
		panic("broker client bug")
	})
}

// One queue whose consumer cannot be built must not cost the queues after
// it. Reconfigure used to return at the first failure, so every queue later
// in the (random) map order silently went unconsumed. It now builds every
// queue it can, reports the failures together, and raises one ERROR
// warning; a later Reconfigure retries only the missing ones.
func TestReconfigureKeepsGoingPastAFailedConsumer(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	ws := NewWarningService(DefaultWarningServiceConfig())
	m.SetWarnings(ws)

	cfg := routerCfg([]string{"q-good-1", "q-good-2"})
	cfg.Queues = append(cfg.Queues,
		common.QueueConfig{Name: "q-down", URI: failBuildScheme + "://q-down"},
		common.QueueConfig{Name: "q-panics", URI: panicBuildScheme + "://q-panics"},
	)
	failBefore, panicBefore := failBuilds.Load(), panicBuilds.Load()

	err := m.Reconfigure(context.Background(), cfg)
	var rerr *ReconfigureError
	require.ErrorAs(t, err, &rerr)
	failedQueues := []string{}
	for _, f := range rerr.Failed {
		failedQueues = append(failedQueues, f.Queue)
	}
	assert.ElementsMatch(t, []string{"q-down", "q-panics"}, failedQueues)
	assert.ErrorIs(t, err, errNoBroker, "each queue's own error stays reachable")

	for _, name := range []string{"q-good-1", "q-good-2"} {
		assert.True(t, polled(t, fakeQueueFor(t, name), 1, time.Second),
			"%s must be consumed although another queue failed", name)
	}
	assert.NotNil(t, m.Pool(defaultPoolCode), "pools are applied whatever the queues do")

	warned := false
	for _, w := range ws.ByCategory(WarningCategoryConfiguration) {
		if w.Severity == WarningError && strings.Contains(w.Message, "q-down") && strings.Contains(w.Message, "q-panics") {
			warned = true
		}
	}
	assert.True(t, warned, "running without configured queues must raise an ERROR warning naming them")

	// The next apply retries the missing queues, and only those.
	err = m.Reconfigure(context.Background(), cfg)
	require.ErrorAs(t, err, &rerr)
	assert.Equal(t, failBefore+2, failBuilds.Load(), "the failed queue is retried on the next apply")
	assert.Equal(t, panicBefore+2, panicBuilds.Load(), "the panicking queue is retried on the next apply")
}
