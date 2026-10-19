package router

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// F29: a client created while the router runs gets its dispatch queue
// consumed within seconds, not at the next five-minute configuration poll.

// docServer serves a mutable RouterConfig the way the platform's router-config
// endpoint does: with an ETag, and 304 for a matching If-None-Match.
type docServer struct {
	srv *httptest.Server

	mu   sync.Mutex
	cfg  common.RouterConfig
	hits atomic.Int32 // requests received
	n304 atomic.Int32 // 304 answers sent
}

func newDocServer(t *testing.T, queues ...string) *docServer {
	t.Helper()
	d := &docServer{cfg: routerCfg(queues)}
	d.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.hits.Add(1)
		d.mu.Lock()
		body, _ := json.Marshal(d.cfg)
		d.mu.Unlock()
		sum := sha256.Sum256(body)
		etag := `"` + hex.EncodeToString(sum[:8]) + `"`
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			d.n304.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(d.srv.Close)
	return d
}

func (d *docServer) addQueue(name string) {
	d.mu.Lock()
	d.cfg.Queues = append(d.cfg.Queues, fakeQueueCfg(name))
	d.mu.Unlock()
}

// The router sends the validator back, and a 304 is an unchanged config: no
// body is read and the cached definition keeps contributing.
func TestFetch_ConditionalRequestsAnswer304AsUnchanged(t *testing.T) {
	d := newDocServer(t, "f29-etag-a")
	cs := NewConfigSource(d.srv.URL)
	cs.MaxAttempts = 1
	ctx := context.Background()

	cfg, err := cs.Fetch(ctx)
	require.NoError(t, err)
	require.Len(t, cfg.Queues, 1)

	_, err = cs.Fetch(ctx)
	require.ErrorIs(t, err, ErrUnchanged)
	assert.EqualValues(t, 1, d.n304.Load(), "the second fetch was conditional and answered 304")

	d.addQueue("f29-etag-b")
	cfg, err = cs.Fetch(ctx)
	require.NoError(t, err, "a changed document is a 200 with a new validator")
	assert.Len(t, cfg.Queues, 2)

	_, err = cs.Fetch(ctx)
	require.ErrorIs(t, err, ErrUnchanged)
	assert.EqualValues(t, 2, d.n304.Load())
}

// A source that never sends an ETag (a third-party config service) is fetched
// in full every time, as before.
func TestFetch_SourceWithoutETagIsFetchedInFull(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		assert.Empty(t, r.Header.Get("If-None-Match"), "nothing to validate against")
		_ = json.NewEncoder(w).Encode(routerCfg([]string{"f29-noetag"}))
	}))
	t.Cleanup(srv.Close)
	cs := NewConfigSource(srv.URL)
	cs.MaxAttempts = 1
	_, err := cs.Fetch(context.Background())
	require.NoError(t, err)
	_, err = cs.Fetch(context.Background())
	require.ErrorIs(t, err, ErrUnchanged, "change detection on the merged body still works")
	assert.EqualValues(t, 2, hits.Load())
}

// pickupDelay runs a Watch against d with the given poll interval, adds
// queue to the document once the first config is applied, and returns how
// long the manager takes to consume it. nudge, when set, is called right after
// the document changes (the platform telling the router).
func pickupDelay(t *testing.T, d *docServer, interval time.Duration, queue string, nudge func(*ConfigSource)) time.Duration {
	t.Helper()
	cs := NewConfigSource(d.srv.URL)
	cs.MaxAttempts = 1
	cs.NudgeDelay = 20 * time.Millisecond
	manager := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Watch(ctx, cs, manager, interval, nil)
		close(done)
	}()
	t.Cleanup(func() { cancel(); <-done })

	require.Eventually(t, func() bool { return hasConsumer(manager, "f29-base") }, 3*time.Second, 5*time.Millisecond)

	started := time.Now()
	d.addQueue(queue)
	if nudge != nil {
		nudge(cs)
	}
	require.Eventually(t, func() bool { return hasConsumer(manager, queue) }, 10*time.Second, 2*time.Millisecond,
		"the new client's queue must be consumed")
	return time.Since(started)
}

// The measurement behind the F29 ledger entry: how long a queue added to the
// document (a new client's) waits to be consumed, by poll interval alone, and
// when the platform nudges the router.
func TestWatch_NewQueuePickupDelay(t *testing.T) {
	const interval = 600 * time.Millisecond

	polled := pickupDelay(t, newDocServer(t, "f29-base"), interval, "f29-polled", nil)
	nudged := pickupDelay(t, newDocServer(t, "f29-base"), time.Hour, "f29-nudged", func(cs *ConfigSource) { cs.Nudge() })

	t.Logf("pick-up delay: poll interval %v -> %v; nudged (interval 1h) -> %v", interval, polled.Round(time.Millisecond), nudged.Round(time.Millisecond))
	assert.Less(t, nudged, 500*time.Millisecond, "a nudge applies the change without waiting for the interval")
	assert.Greater(t, polled, 100*time.Millisecond, "without a nudge the change waits for the next tick")
}

// Nudges are coalesced: a burst of creates costs one extra fetch, not one each.
func TestWatch_NudgesAreCoalesced(t *testing.T) {
	d := newDocServer(t, "f29-base")
	cs := NewConfigSource(d.srv.URL)
	cs.MaxAttempts = 1
	cs.NudgeDelay = 50 * time.Millisecond
	manager := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { Watch(ctx, cs, manager, time.Hour, nil); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	require.Eventually(t, func() bool { return hasConsumer(manager, "f29-base") }, 3*time.Second, 5*time.Millisecond)

	before := d.hits.Load()
	for range 50 {
		cs.Nudge()
	}
	time.Sleep(400 * time.Millisecond)
	assert.LessOrEqual(t, d.hits.Load()-before, int32(3), "50 nudges, at most an apply and its confirming repeat")
}

// A queue added to the config that the broker does not have yet (the platform
// creates it on the first publish) is rechecked on every tick while it is
// young, and only on the slow cadence once it is old, so a large fleet of
// never-used tenant queues is not probed every tick.
func TestRecheckMissingQueues_YoungEveryTickOldOnTheSlowCadence(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	queueExists.Store(false)

	cfg := routerCfg([]string{"f29-live"})
	cfg.Queues = append(cfg.Queues,
		common.QueueConfig{Name: "f29-young", URI: missingQueueScheme + "://f29-young"},
		common.QueueConfig{Name: "f29-old", URI: missingQueueScheme + "://f29-old"})
	require.NoError(t, m.Reconfigure(context.Background(), cfg))

	// Age one of them past the young window.
	m.consumerMu.Lock()
	m.missingSince["f29-old"] = time.Now().Add(-2 * youngMissingQueueAge)
	m.lastFullRecheck = time.Now() // a full probe just ran
	m.consumerMu.Unlock()

	queueExists.Store(true)
	m.RecheckMissingQueuesPaced(context.Background())
	assert.True(t, hasConsumer(m, "f29-young"), "a young missing queue is rechecked on the tick")
	assert.False(t, hasConsumer(m, "f29-old"), "an old one waits for its slow cadence")

	m.consumerMu.Lock()
	m.lastFullRecheck = time.Now().Add(-2 * fullMissingRecheckEvery)
	m.consumerMu.Unlock()
	m.RecheckMissingQueuesPaced(context.Background())
	assert.True(t, hasConsumer(m, "f29-old"), "the slow cadence rechecks everything")
}
