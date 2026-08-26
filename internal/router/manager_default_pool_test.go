package router

import (
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

func defaultPoolManager(t *testing.T) *Manager {
	t.Helper()
	med := &grMediator{outcome: common.Success(http.StatusOK)}
	m := NewManager(med, NewInFlightTracker())
	m.pools[defaultPoolCode] = NewPool(
		common.PoolConfig{Code: defaultPoolCode, Concurrency: defaultPoolConcurrency},
		med, nil, m.resolveConsumer)
	return m
}

func msgForPool(code string) common.QueuedMessage {
	return common.QueuedMessage{
		Message:         common.Message{ID: "m1", PoolCode: code},
		QueueIdentifier: "q1",
	}
}

// TestSynthesisesPerClientFallbackPool: {identifier}-DEFAULT-POOL codes never
// appear in the router's config — nothing emits processingPools, the router
// polls an external config service — so they only ever arrive from the
// scheduler. Without synthesis every such message would take the unknown-code
// path: routed to the shared DEFAULT-POOL with a warning apiece, losing exactly
// the per-client isolation the namespacing exists to create.
func TestSynthesisesPerClientFallbackPool(t *testing.T) {
	m := defaultPoolManager(t)

	p := m.poolForMessage(msgForPool("acme-DEFAULT-POOL"))

	require.NotNil(t, p)
	assert.Equal(t, "acme-DEFAULT-POOL", p.Identifier(),
		"must get its own pool, not the shared DEFAULT-POOL")
	assert.NotSame(t, m.pools[defaultPoolCode], p)
}

// TestSynthesisedPoolIsReusedNotRebuilt: a second message for the same client
// must land in the same pool, or its concurrency cap would mean nothing.
func TestSynthesisedPoolIsReusedNotRebuilt(t *testing.T) {
	m := defaultPoolManager(t)

	first := m.poolForMessage(msgForPool("acme-DEFAULT-POOL"))
	second := m.poolForMessage(msgForPool("acme-DEFAULT-POOL"))

	assert.Same(t, first, second, "the synthesised pool must be registered and reused")
}

// TestConcurrentSynthesisYieldsOnePool: routing resolves pools under a read
// lock, so synthesis is the one write on that path and has to be
// double-checked. Two messages for a new client arriving together must not end
// up in two pools, each with its own concurrency cap — under -race this also
// fails outright on a concurrent map write.
func TestConcurrentSynthesisYieldsOnePool(t *testing.T) {
	m := defaultPoolManager(t)

	const routers = 16
	pools := make([]*Pool, routers)
	var wg sync.WaitGroup
	for i := 0; i < routers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pools[i] = m.poolForMessage(msgForPool("acme-DEFAULT-POOL"))
		}(i)
	}
	wg.Wait()

	for i, p := range pools {
		require.NotNil(t, p)
		assert.Samef(t, pools[0], p, "router %d got a different pool", i)
	}
}

// TestTwoClientsGetSeparateFallbackPools — the isolation this delivers.
func TestTwoClientsGetSeparateFallbackPools(t *testing.T) {
	m := defaultPoolManager(t)

	acme := m.poolForMessage(msgForPool("acme-DEFAULT-POOL"))
	globex := m.poolForMessage(msgForPool("globex-DEFAULT-POOL"))

	assert.NotSame(t, acme, globex, "each client's fallback traffic must be governed separately")
}

// TestConfigSuppliedFallbackPoolWins: synthesis only fills a gap. A pool of the
// same code supplied by config keeps its own settings.
func TestConfigSuppliedFallbackPoolWins(t *testing.T) {
	m := defaultPoolManager(t)
	configured := NewPool(
		common.PoolConfig{Code: "acme-DEFAULT-POOL", Concurrency: 3},
		m.mediator, nil, m.resolveConsumer)
	m.pools["acme-DEFAULT-POOL"] = configured

	p := m.poolForMessage(msgForPool("acme-DEFAULT-POOL"))

	assert.Same(t, configured, p, "a configured pool must not be replaced by a synthesised one")
	assert.Equal(t, uint32(3), p.Concurrency(), "its configured concurrency must survive")
}

// TestUnknownNonDefaultCodeStillFallsBack: synthesis is limited to fallback
// codes. A genuinely unknown pool code is still a misconfiguration and must
// surface as one rather than silently conjuring a pool.
func TestUnknownNonDefaultCodeStillFallsBack(t *testing.T) {
	m := defaultPoolManager(t)

	p := m.poolForMessage(msgForPool("acme-FAST"))

	assert.Same(t, m.pools[defaultPoolCode], p,
		"an unknown non-fallback code routes to DEFAULT-POOL, as before")
	_, synthesised := m.pools["acme-FAST"]
	assert.False(t, synthesised, "a non-fallback code must never be synthesised")
}

// TestIsDefaultPoolCodeSuffixOnly pins the only structural read allowed on a
// composed code. Both halves may contain hyphens, so the string can never be
// split back apart; the suffix test is safe only because the literal is fixed.
func TestIsDefaultPoolCodeSuffixOnly(t *testing.T) {
	for code, want := range map[string]bool{
		"DEFAULT-POOL":               true,
		"acme-DEFAULT-POOL":          true,
		"multi-part-id-DEFAULT-POOL": true,
		"acme-FAST":                  false,
		"DEFAULT-POOL-EXTRA":         false,
		"":                           false,
	} {
		assert.Equalf(t, want, isDefaultPoolCode(code), "isDefaultPoolCode(%q)", code)
	}
}
