package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// TestMergeConfigsUnionFirstWins covers R5's multi-URL merge: pools are
// keyed by code and queues by URI; the first source to define a key wins and
// later conflicting duplicates are dropped.
func TestMergeConfigsUnionFirstWins(t *testing.T) {
	c5 := uint32(5)
	a := common.RouterConfig{
		ProcessingPools: []common.PoolConfig{{Code: "P1", Concurrency: 5, RateLimitPerMinute: &c5}},
		Queues:          []common.QueueConfig{{Name: "q1", URI: "uri1", Connections: 1}},
	}
	b := common.RouterConfig{
		ProcessingPools: []common.PoolConfig{
			{Code: "P1", Concurrency: 9}, // same code, different value → dropped
			{Code: "P2", Concurrency: 3},
		},
		Queues: []common.QueueConfig{
			{Name: "q1-dup", URI: "uri1", Connections: 2}, // same URI → dropped
			{Name: "q2", URI: "uri2", Connections: 1},
		},
	}

	merged := mergeConfigs([]sourceConfig{{url: "A", cfg: a}, {url: "B", cfg: b}})

	require.Len(t, merged.ProcessingPools, 2)
	assert.Equal(t, "P1", merged.ProcessingPools[0].Code)
	assert.Equal(t, uint32(5), merged.ProcessingPools[0].Concurrency, "first-wins on pool conflict")
	assert.Equal(t, "P2", merged.ProcessingPools[1].Code)

	require.Len(t, merged.Queues, 2)
	assert.Equal(t, "uri1", merged.Queues[0].URI)
	assert.Equal(t, "q1", merged.Queues[0].Name, "first-wins on queue URI conflict")
	assert.Equal(t, "uri2", merged.Queues[1].URI)
}

// TestMergeConfigsSinglePassthrough verifies a single source is returned
// unchanged (no dedup pass).
func TestMergeConfigsSinglePassthrough(t *testing.T) {
	a := common.RouterConfig{Queues: []common.QueueConfig{{Name: "q1", URI: "uri1"}}}
	merged := mergeConfigs([]sourceConfig{{url: "A", cfg: a}})
	assert.Equal(t, a.Queues, merged.Queues)
}

// TestNewConfigSourceParsesCommaSeparated verifies multi-URL parsing +
// retry defaults.
func TestNewConfigSourceParsesCommaSeparated(t *testing.T) {
	cs := NewConfigSource(" http://a/cfg , http://b/cfg ,, http://c/cfg ")
	assert.Equal(t, []string{"http://a/cfg", "http://b/cfg", "http://c/cfg"}, cs.URLs)
	assert.Equal(t, 12, cs.MaxAttempts)
}

// ── R-30: last-known-good config per source ─────────────────────────────

// poolServer serves a RouterConfig with a single pool whose concurrency is
// `code(t)`; while fail reports true it answers 500 instead.
func poolServer(t *testing.T, poolCode string, concurrency func() uint32, fail *atomic.Bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail != nil && fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(common.RouterConfig{
			ProcessingPools: []common.PoolConfig{{Code: poolCode, Concurrency: concurrency()}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func poolByCode(cfg *common.RouterConfig, code string) (common.PoolConfig, bool) {
	for _, p := range cfg.ProcessingPools {
		if p.Code == code {
			return p, true
		}
	}
	return common.PoolConfig{}, false
}

// TestFetchAllSourcesFailedNoCacheIsError pins the unchanged first-boot
// behaviour: a source that has never succeeded contributes nothing, and if
// every source is in that state Fetch still errors (nothing to hold onto).
func TestFetchAllSourcesFailedNoCacheIsError(t *testing.T) {
	failing := &atomic.Bool{}
	failing.Store(true)
	srv := poolServer(t, "P", func() uint32 { return 1 }, failing)

	cs := NewConfigSource(srv.URL)
	cs.MaxAttempts = 1
	cs.RetryDelay = time.Millisecond

	_, err := cs.Fetch(context.Background())
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrUnchanged))
}

// TestFetchServesLastKnownGoodOnSourceFailure is the R-30 pin: in a
// multi-source setup, a source that starts failing keeps contributing its
// last successfully fetched definition to the merge instead of dropping out
// — the pool it defined stays in the merged config (and so stays running),
// exactly as if the source were still answering.
func TestFetchServesLastKnownGoodOnSourceFailure(t *testing.T) {
	var aCalls atomic.Int32
	aFail := &atomic.Bool{}
	srvA := poolServer(t, "A", func() uint32 { return uint32(aCalls.Add(1)) }, aFail)

	bFail := &atomic.Bool{}
	srvB := poolServer(t, "B-DEFAULT-POOL", func() uint32 { return 7 }, bFail)

	cs := NewConfigSource(srvA.URL + "," + srvB.URL)
	cs.MaxAttempts = 1
	cs.RetryDelay = time.Millisecond
	ctx := context.Background()

	cfg1, err := cs.Fetch(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg1)
	_, aOK := poolByCode(cfg1, "A")
	bPool, bOK := poolByCode(cfg1, "B-DEFAULT-POOL")
	require.True(t, aOK && bOK, "both sources must contribute on a clean fetch")
	assert.Equal(t, uint32(7), bPool.Concurrency)

	// B starts failing. A's response changes on every call (concurrency
	// bumps), so the merged bytes differ from the previous fetch and this
	// is NOT the ErrUnchanged short-circuit — it is B's cache actually
	// being consulted.
	bFail.Store(true)
	cfg2, err := cs.Fetch(ctx)
	require.NoError(t, err, "a failing source with a cache must not fail the whole fetch")
	require.NotNil(t, cfg2)
	bPool2, bOK2 := poolByCode(cfg2, "B-DEFAULT-POOL")
	require.True(t, bOK2, "B's last-known-good pool must stay in the merged config while B is failing")
	assert.Equal(t, uint32(7), bPool2.Concurrency, "the cached definition, not a dropped one")
	_, aOK2 := poolByCode(cfg2, "A")
	assert.True(t, aOK2, "the still-healthy source keeps reporting live")
}

// TestFetchStaleConfigWarningOnceThenClearedOnRecovery pins the CONFIGURATION
// warning lifecycle: raised once per failure streak (not once per poll tick),
// and resolved the moment the source recovers.
func TestFetchStaleConfigWarningOnceThenClearedOnRecovery(t *testing.T) {
	fail := &atomic.Bool{}
	srv := poolServer(t, "P", func() uint32 { return 3 }, fail)

	cs := NewConfigSource(srv.URL)
	cs.MaxAttempts = 1
	cs.RetryDelay = time.Millisecond
	ws := NewWarningService(WarningServiceConfig{})
	cs.SetWarnings(ws)
	ctx := context.Background()

	_, err := cs.Fetch(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, ws.Count(), "a healthy source raises nothing")

	fail.Store(true)
	_, err = cs.Fetch(ctx)
	require.True(t, err == nil || errors.Is(err, ErrUnchanged), "cached config must not fail the fetch: %v", err)
	require.Equal(t, 1, ws.Count(), "first failing tick raises exactly one warning")
	warnings := ws.ByCategory(WarningCategoryConfiguration)
	require.Len(t, warnings, 1)
	firstID := warnings[0].ID

	// Still failing on the next tick: must not add a second warning.
	_, err = cs.Fetch(ctx)
	require.True(t, err == nil || errors.Is(err, ErrUnchanged))
	assert.Equal(t, 1, ws.Count(), "a continuing failure streak must not flood /warnings")
	assert.Equal(t, firstID, ws.ByCategory(WarningCategoryConfiguration)[0].ID, "same warning, not a new one")

	fail.Store(false)
	_, err = cs.Fetch(ctx)
	require.True(t, err == nil || errors.Is(err, ErrUnchanged), "recovery must not itself error: %v", err)
	assert.Equal(t, 0, ws.Count(), "recovery resolves the stale-config warning")
}

// ── R-30: the watcher itself also warns, not just ConfigSource ──────────
//
// ConfigSource.Fetch already raises/clears a per-URL "serving stale config"
// warning (tested above). That covers a source that has previously
// succeeded and now fails while another source keeps the merge alive. It
// does NOT cover Watch's own apply(): every source failing with nothing
// cached anywhere (Fetch itself returns an error), or a fetch that succeeds
// but Manager.Reconfigure then rejects the result. Both of those used to
// only slog.Warn, so an operator watching /warnings saw nothing while the
// router quietly kept running on stale-but-untracked config.

// TestWatch_FetchFailureRaisesAndClearsWarning pins the "nothing cached
// anywhere" case: cs.Fetch itself errors, and Watch must surface that on
// /warnings, then clear it once the source recovers.
func TestWatch_FetchFailureRaisesAndClearsWarning(t *testing.T) {
	fail := &atomic.Bool{}
	fail.Store(true)
	srv := poolServer(t, "WPOOL", func() uint32 { return 1 }, fail)

	cs := NewConfigSource(srv.URL)
	cs.MaxAttempts = 1
	cs.RetryDelay = time.Millisecond

	manager := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	ws := NewWarningService(WarningServiceConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Watch(ctx, cs, manager, 20*time.Millisecond, ws)
		close(done)
	}()

	require.Eventually(t, func() bool {
		return len(ws.ByCategory(WarningCategoryConfiguration)) == 1
	}, 2*time.Second, 5*time.Millisecond, "every source failing with nothing cached must raise a CONFIGURATION warning")

	fail.Store(false)
	require.Eventually(t, func() bool {
		return len(ws.ByCategory(WarningCategoryConfiguration)) == 0
	}, 2*time.Second, 5*time.Millisecond, "recovery must clear the watcher's warning")

	cancel()
	<-done
}

// TestWatch_ReconfigureFailureRaisesAndClearsWarning pins the other case: a
// perfectly good fetch whose config Manager.Reconfigure rejects (here, a
// queue URI with no registered backend scheme). This must warn too — it is
// the exact case the ruling calls out ("apply() already holds last-known-good
// correctly ... but only slog.Warns").
func TestWatch_ReconfigureFailureRaisesAndClearsWarning(t *testing.T) {
	var call atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		var cfg common.RouterConfig
		if call.Add(1) == 1 {
			// Well-formed JSON, but a scheme no backend is registered for
			// — Manager.Reconfigure fails building the consumer.
			cfg = common.RouterConfig{Queues: []common.QueueConfig{{Name: "bad", URI: "no-such-scheme://nope"}}}
		} else {
			// A real, working queue. Different content than the first
			// response, so this is a genuine re-fetch, not ErrUnchanged.
			cfg = common.RouterConfig{Queues: []common.QueueConfig{fakeQueueCfg("q-watch-recover")}}
		}
		_ = json.NewEncoder(w).Encode(cfg)
	}))
	t.Cleanup(srv.Close)

	cs := NewConfigSource(srv.URL)
	cs.MaxAttempts = 1
	cs.RetryDelay = time.Millisecond

	manager := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	ws := NewWarningService(WarningServiceConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Watch(ctx, cs, manager, 20*time.Millisecond, ws)
		close(done)
	}()

	require.Eventually(t, func() bool {
		return len(ws.ByCategory(WarningCategoryConfiguration)) == 1
	}, 2*time.Second, 5*time.Millisecond, "a Reconfigure failure must raise a CONFIGURATION warning")

	require.Eventually(t, func() bool {
		return len(ws.ByCategory(WarningCategoryConfiguration)) == 0
	}, 2*time.Second, 5*time.Millisecond, "the next successful apply must clear the warning")

	cancel()
	<-done
}

// TestWatch_NilWarningsIsSafe pins the nil-safety requirement: several
// callers (including this file's other tests, pre-R-30) construct Watch
// without a WarningService at all, and a failing source must not panic.
func TestWatch_NilWarningsIsSafe(t *testing.T) {
	fail := &atomic.Bool{}
	fail.Store(true)
	srv := poolServer(t, "WPOOL", func() uint32 { return 1 }, fail)

	cs := NewConfigSource(srv.URL)
	cs.MaxAttempts = 1
	cs.RetryDelay = time.Millisecond
	manager := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	require.NotPanics(t, func() {
		go func() {
			Watch(ctx, cs, manager, 20*time.Millisecond, nil)
			close(done)
		}()
		time.Sleep(60 * time.Millisecond)
	})
	cancel()
	<-done
}
