package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// countingSource serves a config document, counting requests. Until `answer`
// is set it responds 500, standing in for a config service that is down.
func countingSource(t *testing.T, queueName string) (srv *httptest.Server, calls *atomic.Int32, answer *atomic.Bool) {
	t.Helper()
	calls = &atomic.Int32{}
	answer = &atomic.Bool{}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		if !answer.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(common.RouterConfig{
			Queues: []common.QueueConfig{fakeQueueCfg(queueName)},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, calls, answer
}

// A source that has never answered is retried at the RETRY cadence until it
// does — not left until the next poll. The poll interval here is longer than
// the test's whole lifetime, so a consumer can only appear if the retry loop
// is what produced it.
func TestWatch_RetriesUntilTheFirstConfigurationApplies(t *testing.T) {
	srv, calls, answer := countingSource(t, "q-first-fetch")

	cs := NewConfigSource(srv.URL)
	cs.MaxAttempts = 1
	cs.RetryDelay = 10 * time.Millisecond

	manager := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Watch(ctx, cs, manager, time.Hour, nil) // a poll tick can never fire
		close(done)
	}()

	// Down: the watcher keeps trying rather than giving up and waiting.
	require.Eventually(t, func() bool { return calls.Load() >= 3 }, 2*time.Second, 5*time.Millisecond,
		"a source that has never answered must be retried, not left until the next poll")
	assert.Equal(t, 0, manager.PoolCount(), "nothing is configured while the source is down")

	// Recovered: the very next retry applies it.
	answer.Store(true)
	require.Eventually(t, func() bool { return len(manager.Consumers()) == 1 }, 2*time.Second, 5*time.Millisecond,
		"the first successful fetch must be applied without waiting for a poll")

	cancel()
	<-done
}

// Once a configuration has landed, the poll interval governs: the retry loop
// is over, so a source that fails later is not hammered at the retry cadence.
func TestWatch_PollIntervalGovernsAfterTheFirstSuccess(t *testing.T) {
	srv, calls, answer := countingSource(t, "q-after-first")
	answer.Store(true)

	cs := NewConfigSource(srv.URL)
	cs.MaxAttempts = 1
	cs.RetryDelay = 5 * time.Millisecond

	manager := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Watch(ctx, cs, manager, time.Hour, nil)
		close(done)
	}()

	require.Eventually(t, func() bool { return len(manager.Consumers()) == 1 }, 2*time.Second, 5*time.Millisecond)
	applied := calls.Load()

	// With the poll an hour away and the retry loop finished, the source is
	// left alone even though it is now failing again.
	answer.Store(false)
	time.Sleep(60 * time.Millisecond)
	assert.Equal(t, applied, calls.Load(),
		"after the first success the poll interval governs; the retry cadence must not keep firing")

	cancel()
	<-done
}

// Shutdown — or leadership loss, which cancels the same context — ends the
// retrying. The pools this watcher would have started belong to whoever holds
// leadership now.
func TestWatch_CancellationStopsTheRetryLoop(t *testing.T) {
	srv, calls, answer := countingSource(t, "q-cancelled")

	cs := NewConfigSource(srv.URL)
	cs.MaxAttempts = 1
	cs.RetryDelay = 5 * time.Millisecond

	manager := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Watch(ctx, cs, manager, time.Hour, nil)
		close(done)
	}()

	require.Eventually(t, func() bool { return calls.Load() >= 2 }, 2*time.Second, 5*time.Millisecond)
	cancel()
	<-done

	// The loop is over: the source answering now changes nothing.
	stopped := calls.Load()
	answer.Store(true)
	time.Sleep(50 * time.Millisecond)
	assert.LessOrEqual(t, calls.Load(), stopped+1,
		"a cancelled watcher must stop fetching (one in-flight attempt may still land)")
	assert.Empty(t, manager.Consumers(), "no consumer may start after the watcher was cancelled")
}

// A fetched configuration the manager REJECTED was never applied, so it must
// not be remembered as the last-applied one: otherwise the next fetch reports
// "unchanged" and the router never retries the configuration it is missing.
func TestWatch_RejectedConfigurationIsRetriedNotRememberedAsApplied(t *testing.T) {
	var call atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// The SAME document every time. Before, the second fetch of it
		// returned ErrUnchanged and the rejection was never retried.
		n := call.Add(1)
		var cfg common.RouterConfig
		if n <= 2 {
			cfg = common.RouterConfig{Queues: []common.QueueConfig{{Name: "bad", URI: "no-such-scheme://nope"}}}
		} else {
			cfg = common.RouterConfig{Queues: []common.QueueConfig{fakeQueueCfg("q-rejected-retry")}}
		}
		_ = json.NewEncoder(w).Encode(cfg)
	}))
	t.Cleanup(srv.Close)

	cs := NewConfigSource(srv.URL)
	cs.MaxAttempts = 1
	cs.RetryDelay = 10 * time.Millisecond

	manager := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Watch(ctx, cs, manager, time.Hour, nil)
		close(done)
	}()

	require.Eventually(t, func() bool { return len(manager.Consumers()) == 1 }, 2*time.Second, 5*time.Millisecond,
		"a rejected configuration must keep being retried until one applies")

	cancel()
	<-done
}
