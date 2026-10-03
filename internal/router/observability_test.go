package router

import (
	"context"
	"net/http"
	"runtime/pprof"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// byIDMediator answers each message by id, and records the pprof labels its
// context carried (the labels processOne sets on the worker goroutine).
type byIDMediator struct {
	outcomes map[string]common.MediationOutcome
	mu       sync.Mutex
	labels   map[string]map[string]string
}

func (m *byIDMediator) Mediate(ctx context.Context, msg *common.Message) common.MediationOutcome {
	got := map[string]string{}
	for _, k := range []string{"message_id", "group", "pool", "queue"} {
		if v, ok := pprof.Label(ctx, k); ok {
			got[k] = v
		}
	}
	m.mu.Lock()
	if m.labels == nil {
		m.labels = map[string]map[string]string{}
	}
	m.labels[msg.ID] = got
	m.mu.Unlock()
	if o, ok := m.outcomes[msg.ID]; ok {
		return o
	}
	return common.Success(http.StatusOK)
}

// The event-time counters behind fc_messages_submitted_total,
// fc_messages_processed_total{result} and fc_messages_rejected_total{reason}.
func TestPoolEventCounters(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
	med := &byIDMediator{outcomes: map[string]common.MediationOutcome{
		"cfg":  common.ErrorConfig(http.StatusNotFound, "not found"),
		"down": common.ErrorConnection("refused"),
	}}
	pool := NewPool(common.PoolConfig{Code: "EV", Concurrency: 4}, med, nil, func(string) queue.Consumer { return cons })
	for _, id := range []string{"ok", "cfg", "down"} {
		pool.submit(context.Background(), releaseMsg(id, "", common.DispatchImmediate))
	}
	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}
	require.Eventually(t, func() bool { return pool.EventCounts().Processed["ERROR_CONFIG"] == 1 },
		time.Second, 5*time.Millisecond)

	ev := pool.EventCounts()
	assert.Equal(t, "EV", ev.Pool)
	assert.Equal(t, uint64(3), ev.Submitted)
	assert.Equal(t, uint64(1), ev.Processed["SUCCESS"])
	assert.Equal(t, uint64(1), ev.Processed["ERROR_CONFIG"])
	assert.Equal(t, uint64(1), ev.Processed["ERROR_CONNECTION"])
	assert.Equal(t, uint64(1), ev.Rejected["released"])
	assert.True(t, ProcessedSuccess("SUCCESS"))
	assert.False(t, ProcessedSuccess("ERROR_CONFIG"))
}

// A worker's goroutine carries pprof labels naming its message, group, pool
// and queue while it delivers, so a goroutine dump (debug=1) or a CPU
// profile says which message each worker is on.
func TestProcessOneLabelsTheWorkerGoroutine(t *testing.T) {
	med := &byIDMediator{}
	pool := NewPool(common.PoolConfig{Code: "LBL", Concurrency: 1}, med, nil,
		func(string) queue.Consumer { return &cascadeConsumer{} })
	qm := releaseMsg("lbl-1", "grp-7", common.DispatchNextOnError)
	qm.QueueIdentifier = "q-lbl"
	pool.processOne(context.Background(), qm)

	med.mu.Lock()
	defer med.mu.Unlock()
	assert.Equal(t, map[string]string{
		"message_id": "lbl-1", "group": "grp-7", "pool": "LBL", "queue": "q-lbl",
	}, med.labels["lbl-1"])
}

// Consumer poll counters survive in the Manager across consumer instances
// and surface in EventCounters.
func TestManagerCountsConsumerPolls(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	require.NoError(t, m.Reconfigure(context.Background(), routerCfg([]string{"q-count"})))
	require.True(t, polled(t, fakeQueueFor(t, "q-count"), 1, time.Second))
	require.Eventually(t, func() bool {
		for _, c := range m.EventCounters().Consumers {
			if c.Queue == "q-count" && c.Polls > 0 {
				return true
			}
		}
		return false
	}, time.Second, 5*time.Millisecond)
}

func TestProfilerLabelsCanBeSwitchedOff(t *testing.T) {
	SetProfilerLabels(false)
	defer SetProfilerLabels(true)
	med := &byIDMediator{}
	pool := NewPool(common.PoolConfig{Code: "NOLBL", Concurrency: 1}, med, nil,
		func(string) queue.Consumer { return &cascadeConsumer{} })
	qm := releaseMsg("nolbl-1", "grp-7", common.DispatchNextOnError)
	pool.processOne(context.Background(), qm)

	med.mu.Lock()
	defer med.mu.Unlock()
	assert.Empty(t, med.labels["nolbl-1"])
}
