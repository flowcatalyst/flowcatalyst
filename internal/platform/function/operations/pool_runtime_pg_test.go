//go:build integration

package operations_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// echoVersion publishes a JS version and re-labels it with the made-up
// runtime "echo" (publish itself only knows wasm/js describe readers; this also
// proves migration 083 lets a new runtime tag into fng_versions), marked READY.
func (f *testFunctionFixture) echoVersion(describe string) int32 {
	f.t.Helper()
	res := f.publish(putArtifactBytes(f.t, f.artifacts, jsArtifact(describe)), runtimes.JS)
	_, err := f.pool.Exec(context.Background(), `UPDATE fng_versions SET runtime = 'echo' WHERE id = $1`, res.Version.ID)
	require.NoError(f.t, err)
	f.markReady(res.Version.Number)
	return res.Version.Number
}

func (f *testFunctionFixture) heartbeat(runner string, caps *fncontrol.RunnerCapabilities) {
	f.t.Helper()
	report, err := json.Marshal(fncontrol.Heartbeat{RunnerID: runner, Pool: f.fn.RunnerPool(), Capabilities: caps})
	require.NoError(f.t, err)
	require.NoError(f.t, f.repo.UpsertRunnerHeartbeat(context.Background(), runner, f.fn.RunnerPool(), report))
	f.t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM fng_runners WHERE id = $1`, runner)
	})
}

// TestAlias_PoolLacksRuntime: promoting a version of a runtime no live runner
// of the pool advertises is refused; a runner that advertises it lets the
// promote through, and the delivery URL comes from the per-runtime override.
func TestAlias_PoolLacksRuntime(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_oppoolrt0001", "oppoolrtapp", "rt-fn")
	f.seedActiveServiceAccount("oppoolrtapp-sa")
	_, err := f.pool.Exec(context.Background(), `UPDATE fng_functions SET pool = 'rtpool' WHERE id = $1`, f.fn.ID)
	require.NoError(t, err)
	f.fn, err = f.repo.FindByID(context.Background(), f.fn.ID)
	require.NoError(t, err)
	f.wiring.RunnerURLTemplate = "http://wasm-{pool}:8095"
	f.wiring.RunnerURLByRuntime = map[string]string{"echo": "http://echo-{pool}:9000"}

	n := f.echoVersion(jsDescribeWithSub("oppoolrtapp:orders:order:created"))
	require.NoError(t, setConfig(f, "GREETING", "hi"))

	// No live runner at all: a non-built-in runtime is still refused.
	_, err = f.putAlias("live", n)
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "POOL_LACKS_RUNTIME")

	// A wasm/js-only runner (one that predates capabilities) does not help.
	f.heartbeat("rt-legacy-runner", nil)
	_, err = f.putAlias("live", n)
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "POOL_LACKS_RUNTIME")
	_, err = f.putAlias("canary", n)
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "POOL_LACKS_RUNTIME")

	// A live runner advertising echo lets it through.
	f.heartbeat("rt-echo-runner", &fncontrol.RunnerCapabilities{Protocol: 1, Runtimes: []string{"echo"}, RunnerKind: "echo"})
	got, err := f.putAlias("live", n)
	require.NoError(t, err)
	assert.Equal(t, n, got.Version.Number)

	subs, err := subscription.NewRepository(f.pool).FindByFunctionID(context.Background(), f.fn.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Equal(t, "http://echo-rtpool:9000/fn/"+f.fn.Address+"/on-order", subs[0].Endpoint, "the echo override, not the default template")
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM msg_dispatch_pools WHERE code = $1`, got.Wiring.DispatchPoolCode)
	})
}

// A wasm/js version in a pool whose only live runner advertises another
// runtime is refused too; with no live runner at all it keeps promoting
// (unchanged behaviour for the built-in runtimes).
func TestAlias_PoolLacksRuntime_BuiltinRuntimes(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_oppoolrt0002", "oppoolrt2app", "rt2-fn")
	_, err := f.pool.Exec(context.Background(), `UPDATE fng_functions SET pool = 'rtpool2' WHERE id = $1`, f.fn.ID)
	require.NoError(t, err)
	f.fn, err = f.repo.FindByID(context.Background(), f.fn.ID)
	require.NoError(t, err)

	res := f.publish(putArtifactBytes(t, f.artifacts, jsArtifact(`{"abi":1,"endpoints":[{"method":"POST","path":"/x","auth":"webhook"}]}`)), runtimes.JS)
	f.markReady(res.Version.Number)

	f.heartbeat("rt2-echo-only", &fncontrol.RunnerCapabilities{Protocol: 1, Runtimes: []string{"echo"}})
	_, err = f.putAlias("canary", res.Version.Number)
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "POOL_LACKS_RUNTIME")

	f.heartbeat("rt2-js-runner", &fncontrol.RunnerCapabilities{Protocol: 1, Runtimes: []string{"wasm", "js"}})
	_, err = f.putAlias("canary", res.Version.Number)
	require.NoError(t, err)
}
