//go:build integration

package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/echorunner"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func tokenClient(t *testing.T, srvURL string, ti *testTokenIssuer, runtimes ...string) *fncontrol.Client {
	t.Helper()
	token := ti.mint(t, "sa_fn_runner_e2e", []string{"platform:function:runner:control"})
	c := fncontrol.NewClient(srvURL, func(context.Context) (string, error) { return token, nil }, nil)
	c.Runtimes = runtimes
	return c
}

func versionRuntimes(doc *fncontrol.Desired) map[string][]string {
	out := map[string][]string{}
	for _, f := range doc.Functions {
		for _, v := range f.Versions {
			out[f.Address] = append(out[f.Address], v.Runtime)
		}
	}
	return out
}

// TestE2E_RuntimesFilter_EchoRunner: one pool holds a wasm function and an
// echo function. The default (no `runtimes`) and an explicit wasm,js request
// never see the echo version; an echo-only request sees only it; the echo
// runner (the real in-repo runner over the real client) loads exactly its
// version, its heartbeat stores capabilities and flips the version READY.
func TestE2E_RuntimesFilter_EchoRunner(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	ti := newTestTokenIssuer(t)
	srv := newTestServer(t, s, ti)
	pool := testpg.Pool(t)

	poolName := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	appCode := "echoapp" + shortID(t)
	seedApplication(t, pool, appID, appCode)
	wasmFn := seedFunction(t, pool, s.Repo, appID, appCode, "wasm-fn", func(f *function.Function) { f.Pool = &poolName })
	echoFn := seedFunction(t, pool, s.Repo, appID, appCode, "echo-fn", func(f *function.Function) { f.Pool = &poolName })
	wv := seedVersion(t, s.Repo, wasmFn.ID, 1, function.VersionReady, "wasm", describeJSON(t))
	seedAlias(t, s.Repo, wasmFn.ID, "live", wv.ID)
	// Published (a candidate): exactly what a runner must prove loadable. Also
	// live, so it stays in the document once READY: an unaliased READY version
	// leaves it, and the runner's next poll would drop it before the asserts.
	ev := seedVersion(t, s.Repo, echoFn.ID, 1, function.VersionPublished, "echo", describeJSON(t))
	seedAlias(t, s.Repo, echoFn.ID, "live", ev.ID)

	ctx := context.Background()

	// Default: wasm/js only, the echo function (all versions filtered) absent.
	docDefault, _, _, err := tokenClient(t, srv.URL, ti).Desired(ctx, poolName, "", 0)
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{wasmFn.Address: {"wasm"}}, versionRuntimes(docDefault))

	docWasmJS, etagWasmJS, _, err := tokenClient(t, srv.URL, ti, "wasm", "js").Desired(ctx, poolName, "", 0)
	require.NoError(t, err)
	assert.Equal(t, versionRuntimes(docDefault), versionRuntimes(docWasmJS))

	echoClient := tokenClient(t, srv.URL, ti, "echo")
	docEcho, etagEcho, _, err := echoClient.Desired(ctx, poolName, "", 0)
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{echoFn.Address: {"echo"}}, versionRuntimes(docEcho))
	assert.NotEqual(t, etagWasmJS, etagEcho, "each runtime set has its own ETag")

	// A malformed filter is a 400, not a silent default.
	_, _, _, err = tokenClient(t, srv.URL, ti, "Echo!").Desired(ctx, poolName, "", 0)
	var se *fncontrol.ErrStatus
	require.ErrorAs(t, err, &se)
	assert.Equal(t, http.StatusBadRequest, se.Status)

	// The echo runner, end to end.
	runCtx, cancel := context.WithCancel(ctx)
	r := echorunner.New(echorunner.Config{
		Plane: echoClient, Pool: poolName, RunnerID: "echo-e2e-" + shortID(t),
		Version: "9.9.9", Wait: 2 * time.Second, HeartbeatEvery: 100 * time.Millisecond,
	})
	done := make(chan struct{})
	go func() { _ = r.Run(runCtx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	require.Eventually(t, func() bool {
		v, err := s.Repo.GetVersionByNumber(ctx, echoFn.ID, 1)
		return err == nil && v != nil && v.Status == function.VersionReady
	}, 10*time.Second, 50*time.Millisecond, "the echo version must reach READY via the echo runner's LOADED heartbeat")
	held, rev := r.Held()
	assert.Equal(t, 1, held, "the echo runner holds only its own version")
	assert.GreaterOrEqual(t, rev, docEcho.Revision, "the runner applied the document it fetched (revision fence)")
	assert.Zero(t, r.Ignored(), "the platform never sent a foreign-runtime version")

	// Capabilities are stored with the heartbeat report, and the pool's live
	// runner set now advertises echo.
	live, advertised, err := s.Repo.PoolRunnerRuntimes(ctx, poolName, time.Now().Add(-time.Minute))
	require.NoError(t, err)
	assert.Equal(t, 1, live)
	assert.Equal(t, map[string]bool{"echo": true}, advertised)
	var report struct {
		Capabilities fncontrol.RunnerCapabilities `json:"capabilities"`
	}
	var raw []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT report FROM fng_runners WHERE pool = $1`, poolName).Scan(&raw))
	require.NoError(t, json.Unmarshal(raw, &report))
	assert.Equal(t, fncontrol.RunnerCapabilities{Protocol: 1, Runtimes: []string{"echo"}, RunnerKind: "echo", Version: "9.9.9"}, report.Capabilities)

	// The echo runner serves the function it holds, and only that.
	hs := httptest.NewServer(r.Handler())
	defer hs.Close()
	resp, err := http.Post(hs.URL+"/fn/"+echoFn.Address+"/events/tick", "application/json", strings.NewReader(`{"a":1}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp2, err := http.Post(hs.URL+"/fn/"+wasmFn.Address+"/x", "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp2.StatusCode, "the wasm function is not the echo runner's")
}

// TestE2E_Heartbeat_RejectsBadCapabilities: capabilities must name valid runtimes.
func TestE2E_Heartbeat_RejectsBadCapabilities(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	ti := newTestTokenIssuer(t)
	srv := newTestServer(t, s, ti)
	c := tokenClient(t, srv.URL, ti)
	poolName := "pool-" + t.Name()
	for _, rts := range [][]string{nil, {"Bad Runtime"}} {
		err := c.Heartbeat(context.Background(), fncontrol.Heartbeat{
			RunnerID: "r-bad-" + shortID(t), Pool: poolName, Versions: []fncontrol.VersionReport{},
			Capabilities: &fncontrol.RunnerCapabilities{Protocol: 1, Runtimes: rts},
		})
		var se *fncontrol.ErrStatus
		require.ErrorAs(t, err, &se, "runtimes %v", rts)
		assert.Equal(t, http.StatusBadRequest, se.Status)
	}
}
