package router

// Pins R-34: default-broker pools (FC_DEFAULT_BROKER=postgres, no remote
// FLOWCATALYST_CONFIG_URL) must be leadership-gated, and must recover after
// a leadership loss→regain rather than staying stopped until a process
// restart. See Server.DefaultConfig's doc comment in server.go for the
// mechanism.

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/standby"
)

// fakeElection is a minimal leaderElection a test can drive directly,
// without a live Redis instance backing standby.Election.
type fakeElection struct {
	leader atomic.Bool
	ch     chan standby.LeadershipChange
}

func newFakeElection(leader bool) *fakeElection {
	e := &fakeElection{ch: make(chan standby.LeadershipChange, 4)}
	e.leader.Store(leader)
	return e
}

func (e *fakeElection) IsLeader() bool                             { return e.leader.Load() }
func (e *fakeElection) Subscribe() <-chan standby.LeadershipChange { return e.ch }
func (e *fakeElection) Start(context.Context) error                { return nil }
func (e *fakeElection) Stop(context.Context) error                 { return nil }

// setLeader flips leadership and publishes the transition, matching
// standby.Election.setLeader's contract (only a genuine flip is
// published).
func (e *fakeElection) setLeader(v bool) {
	if e.leader.Swap(v) == v {
		return
	}
	e.ch <- standby.LeadershipChange{IsLeader: v, At: time.Now()}
}

// TestGateOnLeadership_FollowerDoesNotStartPools pins half (a) of R-34: a
// follower must never call startPools, so it never touches a broker.
func TestGateOnLeadership_FollowerDoesNotStartPools(t *testing.T) {
	tracker := NewInFlightTracker()
	manager := newTestManager(t, &grMediator{outcome: common.Success(http.StatusOK)}, tracker)
	traffic, err := NewTrafficStrategy(context.Background(), TrafficConfig{})
	require.NoError(t, err)

	election := newFakeElection(false) // follower from the start

	var calls int32
	startPools := func(c context.Context) {
		atomic.AddInt32(&calls, 1)
		_ = manager.Reconfigure(c, routerCfg([]string{"q-follower"}))
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		gateOnLeadership(ctx, election, manager, traffic, tracker, time.Second, startPools)
		close(done)
	}()

	// Give the goroutine a chance to run its synchronous initial apply().
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls), "a follower must never start pools")
	assert.Equal(t, 0, manager.PoolCount(), "no pools should exist on a follower")

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("gateOnLeadership did not return after ctx cancel")
	}
}

// TestServerRun_DefaultBroker_RegainAfterLeadershipLoss is the regression
// test for the worse half of R-34: on leadership loss, gateOnLeadership's
// loss branch calls Manager.Shutdown, which empties m.pools/m.consumers.
// Before the fix, a regain's startPools saw ConfigSource == nil and
// PoolCount() == 0 and just logged a warning — Manager.Reconfigure was
// never called again, so a default-broker router that lost and regained
// leadership stopped processing permanently until restarted.
func TestServerRun_DefaultBroker_RegainAfterLeadershipLoss(t *testing.T) {
	srv, err := NewServer(ServerConfig{
		DrainTimeout: 300 * time.Millisecond,
		// StandbyEnabled is deliberately left false: NewServer would try to
		// dial Redis. election is wired in directly below instead, exactly
		// as it would be assigned from standby.New in the real path — Run
		// only ever checks s.election != nil.
	})
	require.NoError(t, err)

	cfg := routerCfg([]string{"q-regain"}, common.PoolConfig{Code: "RPOOL", Concurrency: 2})
	srv.DefaultConfig = &cfg

	election := newFakeElection(true) // starts as leader
	srv.election = election

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- srv.Run(ctx) }()

	// Leader from the start: the default-broker config must be applied and
	// polling must begin.
	require.Eventually(t, func() bool { return srv.Manager.PoolCount() > 0 }, 2*time.Second, 10*time.Millisecond,
		"leader must bootstrap the default-broker pools")
	q := fakeQueueFor(t, "q-regain")
	require.True(t, polled(t, q, 1, time.Second), "router must be polling while leader")

	// Lose leadership: gateOnLeadership's loss branch drains and calls
	// Manager.Shutdown, which empties the pool/consumer maps.
	election.setLeader(false)
	require.Eventually(t, func() bool { return srv.Manager.PoolCount() == 0 }, 2*time.Second, 10*time.Millisecond,
		"leadership loss must stop the pools")

	// Regain leadership. This is the regression: startPools must
	// re-Reconfigure from DefaultConfig, not just find PoolCount() == 0
	// and warn.
	election.setLeader(true)
	require.Eventually(t, func() bool { return srv.Manager.PoolCount() > 0 }, 2*time.Second, 10*time.Millisecond,
		"regain must re-bootstrap the default-broker pools, not stay stopped")

	// Reconfigure rebuilds the consumer from scratch (Shutdown removed the
	// old one from the fake registry's backing map), so re-fetch it and
	// confirm the router is actually polling again — a re-created pool
	// with a dead consumer would satisfy PoolCount() > 0 without fixing
	// anything.
	q2 := fakeQueueFor(t, "q-regain")
	require.True(t, polled(t, q2, 1, time.Second), "router must resume polling after regain")

	cancel()
	select {
	case e := <-runErr:
		require.NoError(t, e)
	case <-time.After(3 * time.Second):
		t.Fatal("Server.Run did not return after ctx cancel")
	}
}
