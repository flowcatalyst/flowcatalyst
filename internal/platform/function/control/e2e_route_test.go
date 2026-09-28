//go:build integration

// e2e_route_test.go is the full public-routes round trip
// (docs/function-runner-plan.md §8): a real internal/functions/runner.Runner
// polling a real internal/functions/control.Client against an httptest
// server mounting this package's control routes, a JS function published
// (fixture-seeded, mirroring wiring_pg_test.go's JS-artifact convention) and
// promoted to live, a route added through the real PutRoute operation, and
// a request on runner.PublicHandler() with that route's Host and path
// proven to reach the function.
package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runner"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain"
	fdops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// e2eScript is a minimal JS guest for the shared engine (mirrors
// internal/functions/runner/js_test.go's jsScript): one auth:none endpoint.
const e2eScript = `
globalThis.__fc = {
  describe() {
    return JSON.stringify({ abi: 1, endpoints: [ { path: "/echo", auth: "none" } ] });
  },
  handle(meta, body) {
    return { meta: JSON.stringify({ status: 200, headers: { "Content-Type": ["text/plain"] } }), body: __fc_utf8_encode("hello-e2e") };
  },
};`

// TestE2E_PublicRoute_ServesRealRequest runs the whole stack: platform
// control routes over real HTTP, a real Runner polling them, a route
// materialised through PutRoute, and a public HTTP request served by
// runner.PublicHandler().
func TestE2E_PublicRoute_ServesRealRequest(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	ti := newTestTokenIssuer(t)
	srv := newTestServer(t, s, ti)

	token := ti.mint(t, "sa_fn_runner_e2e_routes", []string{"platform:function:runner:control"})
	client := fncontrol.NewClient(srv.URL, func(context.Context) (string, error) { return token, nil }, nil)

	poolName := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	appCode := "e2ertapp" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, appCode)
	fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, "fn", func(f *function.Function) { f.Pool = &poolName })

	// Build the real describe document + digest via the JS engine (same
	// technique internal/functions/runner/js_test.go's jsVersion uses), and
	// store the artifact bytes where the control plane's artifact route
	// (and so the runner) will find them.
	src := []byte(e2eScript)
	sum := sha256.Sum256(src)
	digest := hex.EncodeToString(sum[:])
	b, err := budget.New(256<<20, 0)
	require.NoError(t, err)
	eng, err := engine.New(context.Background(), engine.Config{Budget: b})
	require.NoError(t, err)
	describeDoc, err := runtimes.NewLoader(eng).Describe(context.Background(), runtimes.JS, src, 64<<20)
	require.NoError(t, err)
	require.NoError(t, eng.Close(context.Background()))
	require.NoError(t, s.Artifacts.Put(context.Background(), digest, bytes.NewReader(src)))

	live := seedVersionWithDigest(t, s.Repo, fn.ID, 1, function.VersionReady, runtimes.JS, describeDoc, digest)
	seedAlias(t, s.Repo, fn.ID, "live", live.ID)

	// Claim a zone and add a route through the real write path.
	uow := testpg.NewUoW(t)
	domains := functiondomain.NewRepository(testpg.Pool(t))
	zone := "e2ert-" + shortID(t) + ".test"
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, fdops.CreateDomain(domains), fdops.CreateCommand{Zone: zone}, testpg.TestEC())
	require.NoError(t, err)
	host := "pub." + zone
	_, err = usecaseop.RunTx(testpg.AnchorCtx(), uow, operations.PutRoute(s.Repo, domains),
		operations.PutRouteCommand{FunctionID: fn.ID, Hostname: host, PathPrefix: "/pub"}, testpg.TestEC())
	require.NoError(t, err)

	// Start a real Runner against the real control-plane client.
	rb, err := budget.New(256<<20, 0)
	require.NoError(t, err)
	rn, err := runner.New(context.Background(), runner.Config{
		Pool: poolName, ControlPlane: client, Budget: rb, CacheDir: t.TempDir(),
	})
	require.NoError(t, err)
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = rn.Run(runCtx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	pubSrv := httptest.NewServer(rn.PublicHandler())
	t.Cleanup(pubSrv.Close)

	// Poll until the runner has reconciled the route (long-poll + prepare +
	// swap take a moment even against a local server).
	deadline := time.Now().Add(10 * time.Second)
	var lastStatus int
	var lastBody []byte
	for time.Now().Before(deadline) {
		req, rerr := http.NewRequest(http.MethodGet, pubSrv.URL+"/pub/echo", nil)
		require.NoError(t, rerr)
		req.Host = host
		resp, derr := pubSrv.Client().Do(req)
		if derr == nil {
			lastBody, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			lastStatus = resp.StatusCode
			if lastStatus == http.StatusOK {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	assert.Equal(t, http.StatusOK, lastStatus, "runner.PublicHandler() must eventually serve the routed request")
	assert.Equal(t, "hello-e2e", string(lastBody))
}
