//go:build integration

// routes_test.go covers docs/function-runner-plan.md §8 ("public routes")
// on the control-plane side: buildDesired renders Desired.Routes for the
// right pool only, and a route change (through the real PutRoute operation,
// not a raw revision bump) changes the ETag and wakes a waiting long-poll —
// exactly like a promote/alias/settings change already does (desired_test.go).
package control

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain"
	fdops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestDesired_IncludesRoutesForRightPoolOnly proves buildDesired's Routes
// rendering: a route owned by a function in THIS pool appears (Address from
// the function, Alias as stored — "" for live), a route owned by a function
// in a DIFFERENT pool does not.
func TestDesired_IncludesRoutesForRightPoolOnly(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	pool := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	appCode := "routesapp" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, appCode)

	fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, "fn", func(f *function.Function) { f.Pool = &pool })
	seedRoute(t, s.Repo, fn.ID, "api.routes-"+shortID(t)+".test", "/webhooks", "")
	seedRoute(t, s.Repo, fn.ID, "api2.routes-"+shortID(t)+".test", "/", "canary")

	otherPool := "other-" + pool
	otherFn := seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, "other-fn", func(f *function.Function) { f.Pool = &otherPool })
	seedRoute(t, s.Repo, otherFn.ID, "other.routes-"+shortID(t)+".test", "/", "")

	doc, err := s.buildDesired(context.Background(), pool)
	require.NoError(t, err)

	require.Len(t, doc.Routes, 2, "only this pool's routes")
	byHost := map[string]fncontrol.Route{}
	for _, r := range doc.Routes {
		byHost[r.Hostname] = r
	}
	r1, ok := byHost["api.routes-"+shortID(t)+".test"]
	require.True(t, ok)
	assert.Equal(t, fn.Address, r1.Address)
	assert.Equal(t, "/webhooks", r1.PathPrefix)
	assert.Equal(t, "", r1.Alias, "a live route reports an empty alias")

	r2, ok := byHost["api2.routes-"+shortID(t)+".test"]
	require.True(t, ok)
	assert.Equal(t, "canary", r2.Alias)

	for _, r := range doc.Routes {
		assert.NotEqual(t, "other.routes-"+shortID(t)+".test", r.Hostname, "a different pool's route must not appear")
	}
}

// TestDesiredHandler_RouteChangeChangesETagAndWakesLongPoll runs the real
// PutRoute operation (validation, ROUTE_HOST_NOT_CLAIMED coverage check,
// and the same-transaction pool-revision bump + NOTIFY) against a held
// long-poll, proving the whole path end to end — not just a raw
// BumpPoolRevisionTx call, unlike TestDesiredHandler_WakesOnNotifyWithinASecond.
func TestDesiredHandler_RouteChangeChangesETagAndWakesLongPoll(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	s := newTestState(t)
	s.Listener = NewListener(pool, nil)
	s.ReRenderInterval = 10 * time.Second

	listenCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.Listener.Run(listenCtx)
	waitForListener(t, pool)

	poolName := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	appCode := "rtwakeapp" + shortID(t)
	seedApplication(t, pool, appID, appCode)
	fn := seedFunction(t, pool, s.Repo, appID, appCode, "fn", func(f *function.Function) { f.Pool = &poolName })
	live := seedVersion(t, s.Repo, fn.ID, 1, function.VersionReady, "wasm", describeJSON(t))
	seedAlias(t, s.Repo, fn.ID, "live", live.ID)

	ctx := anchorCtx()
	first, err := s.desired(ctx, &desiredInput{Pool: poolName})
	require.NoError(t, err)
	require.Empty(t, first.Body.Routes)

	done := make(chan *desiredOutput, 1)
	go func() {
		out, err := s.desired(ctx, &desiredInput{Pool: poolName, Wait: 30, IfNoneMatch: first.ETag})
		require.NoError(t, err)
		done <- out
	}()

	// Give the goroutine a moment to register its Wait(), then claim a zone
	// and write a real route through the operation under test — the same
	// path PUT /api/functions/{id}/routes drives.
	time.Sleep(100 * time.Millisecond)
	zone := "wake-" + shortID(t) + ".test"
	domains := functiondomain.NewRepository(pool)
	uow := testpg.NewUoW(t)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, fdops.CreateDomain(domains), fdops.CreateCommand{Zone: zone}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.RunTx(testpg.AnchorCtx(), uow, operations.PutRoute(s.Repo, domains),
		operations.PutRouteCommand{FunctionID: fn.ID, Hostname: "api." + zone, PathPrefix: "/"}, testpg.TestEC())
	require.NoError(t, err)

	select {
	case out := <-done:
		assert.Equal(t, 200, out.Status)
		assert.NotEqual(t, first.ETag, out.ETag)
		require.Len(t, out.Body.Routes, 1)
		assert.Equal(t, "api."+zone, out.Body.Routes[0].Hostname)
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("desired() did not wake within ~1s of a route write")
	}
}
