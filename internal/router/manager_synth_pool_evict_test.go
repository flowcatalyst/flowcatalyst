package router

// Pins R-59: a synthesised per-client fallback pool ({identifier}-DEFAULT-POOL,
// see Manager.ensureFallbackPool) with no routed message for at least its TTL
// is stopped and removed by Manager.EvictIdleSynthPools; it is re-synthesised
// on demand by the next message naming it. Configured pools (whether the
// global DEFAULT-POOL or a code Reconfigure defines explicitly) are never
// evicted, no matter how idle.

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// TestEvictIdleSynthPools_EvictsIdleSynthesisedPool: a synthesised fallback
// pool idle past its TTL is stopped and removed from the manager's map.
func TestEvictIdleSynthPools_EvictsIdleSynthesisedPool(t *testing.T) {
	m := defaultPoolManager(t)

	p := m.poolForMessage(msgForPool("acme-DEFAULT-POOL"))
	require.NotNil(t, p)
	require.NotNil(t, m.Pool("acme-DEFAULT-POOL"))

	// One pass is the whole behaviour: a 1ns TTL is already exceeded by the
	// time we call, so the pool is evicted and reported on the FIRST call.
	// Calling twice and asserting on the second reports 0 — the eviction has
	// already happened and there is nothing left to find.
	time.Sleep(time.Millisecond)
	evicted := m.EvictIdleSynthPools(1 * time.Nanosecond)

	assert.Equal(t, 1, evicted)
	assert.Nil(t, m.Pool("acme-DEFAULT-POOL"), "the idle synthesised pool must be removed")
}

// TestEvictIdleSynthPools_ReSynthesisesOnDemand: after eviction, the next
// message naming the code gets a fresh pool — same shape as
// ensureFallbackPool's normal on-demand synthesis.
func TestEvictIdleSynthPools_ReSynthesisesOnDemand(t *testing.T) {
	m := defaultPoolManager(t)

	first := m.poolForMessage(msgForPool("acme-DEFAULT-POOL"))
	require.NotNil(t, first)
	time.Sleep(time.Millisecond)
	require.Equal(t, 1, m.EvictIdleSynthPools(time.Nanosecond))
	require.Nil(t, m.Pool("acme-DEFAULT-POOL"))

	second := m.poolForMessage(msgForPool("acme-DEFAULT-POOL"))
	require.NotNil(t, second)
	assert.NotSame(t, first, second, "eviction must produce a genuinely fresh pool")
	assert.Equal(t, "acme-DEFAULT-POOL", second.Identifier())
}

// TestEvictIdleSynthPools_RecentTrafficIsSpared: a pool routed to within the
// TTL must not be evicted, even if another synthesised pool alongside it is
// idle.
func TestEvictIdleSynthPools_RecentTrafficIsSpared(t *testing.T) {
	m := defaultPoolManager(t)

	m.poolForMessage(msgForPool("idle-DEFAULT-POOL"))
	time.Sleep(20 * time.Millisecond)
	m.poolForMessage(msgForPool("busy-DEFAULT-POOL")) // routed just now

	evicted := m.EvictIdleSynthPools(10 * time.Millisecond)

	assert.Equal(t, 1, evicted)
	assert.Nil(t, m.Pool("idle-DEFAULT-POOL"), "idle past the TTL must be evicted")
	assert.NotNil(t, m.Pool("busy-DEFAULT-POOL"), "recently routed must survive")
}

// TestEvictIdleSynthPools_TouchOnHitPathResetsIdleClock: a second message
// routed to an already-synthesised pool must reset its idle clock — the
// eviction sweep must not be judging solely by creation time.
func TestEvictIdleSynthPools_TouchOnHitPathResetsIdleClock(t *testing.T) {
	m := defaultPoolManager(t)

	m.poolForMessage(msgForPool("acme-DEFAULT-POOL")) // creates it
	time.Sleep(20 * time.Millisecond)
	m.poolForMessage(msgForPool("acme-DEFAULT-POOL")) // hit path; must touch

	evicted := m.EvictIdleSynthPools(10 * time.Millisecond)

	assert.Equal(t, 0, evicted, "a pool touched inside the TTL must not be evicted")
	assert.NotNil(t, m.Pool("acme-DEFAULT-POOL"))
}

// TestEvictIdleSynthPools_NeverEvictsConfiguredPools: the global DEFAULT-POOL
// and a pool config explicitly defines (even one whose code matches the
// fallback suffix) are never evicted, regardless of idle time.
func TestEvictIdleSynthPools_NeverEvictsConfiguredPools(t *testing.T) {
	m := defaultPoolManager(t) // seeds m.pools[defaultPoolCode] directly, bypassing Reconfigure

	require.NoError(t, m.Reconfigure(t.Context(), routerCfg(nil,
		common.PoolConfig{Code: "acme-DEFAULT-POOL", Concurrency: 3},
	)))
	require.NotNil(t, m.Pool("acme-DEFAULT-POOL"))
	require.NotNil(t, m.Pool(defaultPoolCode))

	evicted := m.EvictIdleSynthPools(1 * time.Nanosecond)

	assert.Equal(t, 0, evicted, "Reconfigure-owned pools must never be evicted")
	assert.NotNil(t, m.Pool("acme-DEFAULT-POOL"))
	assert.NotNil(t, m.Pool(defaultPoolCode))
}

// TestEvictIdleSynthPools_ConfigTakesOwnershipOfASynthesisedCode: a pool this
// Manager synthesised on demand, then later defined by config with the SAME
// code, must stop being eviction-eligible — config always wins.
func TestEvictIdleSynthPools_ConfigTakesOwnershipOfASynthesisedCode(t *testing.T) {
	m := defaultPoolManager(t)

	synthesised := m.poolForMessage(msgForPool("acme-DEFAULT-POOL"))
	require.NotNil(t, synthesised)

	require.NoError(t, m.Reconfigure(t.Context(), routerCfg(nil,
		common.PoolConfig{Code: "acme-DEFAULT-POOL", Concurrency: 7},
	)))
	assert.Same(t, synthesised, m.Pool("acme-DEFAULT-POOL"), "Reconfigure updates the existing pool in place, doesn't replace it")

	evicted := m.EvictIdleSynthPools(1 * time.Nanosecond)

	assert.Equal(t, 0, evicted, "a code config now owns must survive eviction")
	assert.NotNil(t, m.Pool("acme-DEFAULT-POOL"))
	assert.Equal(t, uint32(7), m.Pool("acme-DEFAULT-POOL").Concurrency(), "the configured settings must stick")
}

// TestEvictIdleSynthPools_ZeroTTLIsANoOp: a non-positive TTL disables the
// sweep rather than evicting everything immediately.
func TestEvictIdleSynthPools_ZeroTTLIsANoOp(t *testing.T) {
	m := defaultPoolManager(t)
	m.poolForMessage(msgForPool("acme-DEFAULT-POOL"))

	assert.Equal(t, 0, m.EvictIdleSynthPools(0))
	assert.NotNil(t, m.Pool("acme-DEFAULT-POOL"))
}

// TestEvictIdleSynthPools_ShutdownClearsTracking: Shutdown resets both the
// pool map and the synth-pool tracking map together, so a later Reconfigure
// on the same Manager (leadership regain) starts from a clean slate rather
// than carrying stale entries for pools that no longer exist.
func TestEvictIdleSynthPools_ShutdownClearsTracking(t *testing.T) {
	m := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, NewInFlightTracker())
	m.poolForMessage(msgForPool("acme-DEFAULT-POOL"))
	require.NotNil(t, m.Pool("acme-DEFAULT-POOL"))

	require.NoError(t, m.Shutdown(t.Context()))
	assert.Nil(t, m.Pool("acme-DEFAULT-POOL"))

	// Re-synthesises cleanly after Shutdown, same as after a normal eviction.
	p := m.poolForMessage(msgForPool("acme-DEFAULT-POOL"))
	require.NotNil(t, p)
	assert.Equal(t, 0, m.EvictIdleSynthPools(time.Hour), "freshly synthesised, well inside any TTL")
}
