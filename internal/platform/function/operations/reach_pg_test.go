//go:build integration

package operations_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/fnfixture"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain"
	fdops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// operatorCtx is an operator: anchor reach, every permission, and access to
// all applications, as every ordinary user has. testpg.AnchorCtx is anchor
// but NOT all-applications, so on the application axis it is confined.
func operatorCtx() context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID:     "prn_optestrunner1",
		Scope:           auth.ScopeAnchor,
		Permissions:     []string{"platform:*:*:*"},
		AllApplications: true,
	})
}

// reachFixture is two applications, one client both functions belong to, and
// a caller confined to application A (but reaching the client).
type reachFixture struct {
	t         *testing.T
	pool      *pgxpool.Pool
	repo      *function.Repository
	apps      *application.Repository
	uow       *usecasepgx.UnitOfWork
	artifacts artifact.Store
	domains   *functiondomain.Repository

	appA, appB, client string
	fnA, fnB           *function.Function
	confined           context.Context // confined to appA, reaches client
}

func newReachFixture(t *testing.T, seed string) *reachFixture {
	t.Helper()
	pool := testpg.Pool(t)
	store, err := artifact.NewFileStore(t.TempDir())
	require.NoError(t, err)
	f := &reachFixture{
		t: t, pool: pool, repo: function.NewRepository(pool), apps: application.NewRepository(pool),
		uow: testpg.NewUoW(t), artifacts: store, domains: functiondomain.NewRepository(pool),
		appA: "app_rcha" + seed, appB: "app_rchb" + seed, client: "cli_rch" + seed,
	}
	seedApplication(t, pool, f.appA, "rcha"+seed)
	seedApplication(t, pool, f.appB, "rchb"+seed)
	f.fnA = f.create(f.appA, "fn-a")
	f.fnB = f.create(f.appB, "fn-b")
	f.confined = testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID:     "prn_rchconfined" + seed,
		Scope:           auth.ScopeClient,
		Clients:         []string{f.client},
		AllApplications: false,
		Applications:    []string{f.appA},
		Permissions:     []string{"platform:function:*:*"},
	})
	return f
}

// create seeds a client-owned function as the operator.
func (f *reachFixture) create(appID, name string) *function.Function {
	f.t.Helper()
	c := f.client
	ev, err := usecaseop.Run(operatorCtx(), f.uow, operations.CreateFunction(f.repo, f.apps),
		operations.CreateCommand{ApplicationID: appID, Name: name, ClientID: &c}, testpg.TestEC())
	require.NoError(f.t, err)
	fn, err := f.repo.FindByID(context.Background(), ev.FunctionID)
	require.NoError(f.t, err)
	require.NotNil(f.t, fn)
	f.t.Cleanup(func() {
		bg := context.Background()
		for _, q := range []string{
			`DELETE FROM fng_trace_contexts WHERE function_id = $1`,
			`DELETE FROM fng_routes WHERE function_id = $1`,
			`DELETE FROM fng_aliases WHERE function_id = $1`,
			`DELETE FROM fng_settings WHERE function_id = $1`,
			`DELETE FROM fng_versions WHERE function_id = $1`,
			`DELETE FROM fng_functions WHERE id = $1`,
		} {
			_, _ = f.pool.Exec(bg, q, fn.ID)
		}
		_, _ = f.pool.Exec(bg, `DELETE FROM fng_domains WHERE client_id = $1`, f.client)
	})
	return fn
}

// seedVersion inserts a version row directly (as the prune/trace tests do).
func (f *reachFixture) seedVersion(fn *function.Function, n int32, status function.VersionStatus) *function.Version {
	f.t.Helper()
	v := &function.Version{
		ID: tsid.Generate(tsid.FunctionVersion), FunctionID: fn.ID, Number: n,
		Digest:    fmt.Sprintf("%064x", time.Now().UnixNano()+int64(n)),
		SizeBytes: 1, ABI: 1, Runtime: "js", Describe: []byte(`{"abi":1,"endpoints":[]}`),
		Status: status, CreatedAt: time.Now().UTC().Add(-time.Duration(10-n) * time.Hour),
	}
	require.NoError(f.t, f.repo.InsertVersion(context.Background(), v))
	return v
}

func (f *reachFixture) wiring() operations.WiringDeps { return testWiring(f.pool) }

func requireRefused(t *testing.T, err error) {
	t.Helper()
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
}

// T1: create is gated on the application axis.
func TestCreateFunction_ApplicationReach(t *testing.T) {
	t.Parallel()
	f := newReachFixture(t, "t1create1")
	c := f.client
	run := func(ctx context.Context, appID, name string) (operations.FunctionCreated, error) {
		return usecaseop.Run(ctx, f.uow, operations.CreateFunction(f.repo, f.apps),
			operations.CreateCommand{ApplicationID: appID, Name: name, ClientID: &c}, testpg.TestEC())
	}

	_, err := run(f.confined, f.appB, "created-by-confined")
	requireRefused(t, err)
	got, err := f.repo.FindByAddress(context.Background(), "rchb"+"t1create1"+".created-by-confined")
	require.NoError(t, err)
	assert.Nil(t, got, "a refused create must leave no function row")

	ev, err := run(f.confined, f.appA, "created-by-confined")
	require.NoError(t, err, "a confined caller creates under its own application")
	f.t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM fng_functions WHERE id = $1`, ev.FunctionID)
	})

	ev2, err := run(operatorCtx(), f.appB, "created-by-operator")
	require.NoError(t, err, "an all-applications caller creates under any application")
	f.t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM fng_functions WHERE id = $1`, ev2.FunctionID)
	})
}

// T2: every mutating family refuses a function of another application, and
// leaves it unchanged.
func TestFunctionMutations_RefuseOtherApplication(t *testing.T) {
	t.Parallel()
	f := newReachFixture(t, "t2mutate1")
	bg := context.Background()
	fnB := f.fnB
	vB := f.seedVersion(fnB, 1, function.VersionPublished)
	f.seedVersion(fnB, 2, function.VersionPublished)
	f.seedVersion(fnB, 3, function.VersionPublished)

	runTx := func(t *testing.T, name string, fn func() error) {
		t.Run(name, func(t *testing.T) { requireRefused(t, fn()) })
	}

	desc := "changed by the confined caller"
	runTx(t, "update", func() error {
		_, err := usecaseop.Run(f.confined, f.uow, operations.UpdateFunction(f.repo),
			operations.UpdateCommand{ID: fnB.ID, Description: &desc}, testpg.TestEC())
		return err
	})
	runTx(t, "publish", func() error {
		_, err := usecaseop.RunTx(f.confined, f.uow, operations.Publish(f.repo, f.artifacts, newTestLoader(t)),
			operations.PublishCommand{FunctionID: fnB.ID, Digest: fmt.Sprintf("%064x", 7)}, testpg.TestEC())
		return err
	})
	runTx(t, "retire", func() error {
		_, err := usecaseop.RunTx(f.confined, f.uow, operations.RetireVersion(f.repo),
			operations.RetireCommand{FunctionID: fnB.ID, Number: vB.Number}, testpg.TestEC())
		return err
	})
	runTx(t, "alias-promote", func() error {
		_, err := usecaseop.RunTx(f.confined, f.uow, operations.PutAlias(f.repo, f.wiring()),
			operations.PutAliasCommand{FunctionID: fnB.ID, Name: "canary", Number: vB.Number}, testpg.TestEC())
		return err
	})
	runTx(t, "alias-delete", func() error {
		_, err := usecaseop.RunTx(f.confined, f.uow, operations.DeleteAlias(f.repo, f.wiring()),
			operations.DeleteAliasCommand{FunctionID: fnB.ID, Name: "canary"}, testpg.TestEC())
		return err
	})
	runTx(t, "settings-put", func() error {
		_, err := usecaseop.RunTx(f.confined, f.uow, operations.PutSetting(f.repo),
			operations.PutSettingCommand{FunctionID: fnB.ID, Kind: function.SettingConfig, Key: "GREETING", Value: "hi"}, testpg.TestEC())
		return err
	})
	runTx(t, "settings-delete", func() error {
		_, err := usecaseop.RunTx(f.confined, f.uow, operations.DeleteSetting(f.repo),
			operations.DeleteSettingCommand{FunctionID: fnB.ID, Kind: function.SettingConfig, Key: "GREETING"}, testpg.TestEC())
		return err
	})
	runTx(t, "route-put", func() error {
		_, err := usecaseop.RunTx(f.confined, f.uow, operations.PutRoute(f.repo, f.domains),
			operations.PutRouteCommand{FunctionID: fnB.ID, Hostname: "api.t2mutate1.test", PathPrefix: "/x"}, testpg.TestEC())
		return err
	})
	runTx(t, "route-delete", func() error {
		_, err := usecaseop.RunTx(f.confined, f.uow, operations.DeleteRoute(f.repo),
			operations.DeleteRouteCommand{FunctionID: fnB.ID, RouteID: "rt_nonexistent"}, testpg.TestEC())
		return err
	})
	runTx(t, "trace-enable", func() error {
		_, err := usecaseop.RunTx(f.confined, f.uow, operations.EnableTraceContext(f.repo, function.DefaultTraceLimits()),
			operations.EnableTraceCommand{FunctionID: fnB.ID, Context: "orders", Minutes: 5, SampleRate: 1}, testpg.TestEC())
		return err
	})
	runTx(t, "trace-disable", func() error {
		_, err := usecaseop.RunTx(f.confined, f.uow, operations.DisableTraceContext(f.repo),
			operations.DisableTraceCommand{FunctionID: fnB.ID, Context: "orders"}, testpg.TestEC())
		return err
	})
	runTx(t, "prune", func() error {
		_, err := usecaseop.RunTx(f.confined, f.uow, operations.PruneVersions(f.repo),
			operations.PruneCommand{FunctionID: fnB.ID, Keep: 1}, testpg.TestEC())
		return err
	})
	runTx(t, "delete", func() error {
		_, err := usecaseop.RunTx(f.confined, f.uow, operations.DeleteFunction(f.repo, f.wiring()),
			operations.DeleteCommand{ID: fnB.ID}, testpg.TestEC())
		return err
	})

	// The row is untouched by every refused family.
	got, err := f.repo.FindByID(bg, fnB.ID)
	require.NoError(t, err)
	require.NotNil(t, got, "delete was refused: the function remains")
	assert.Equal(t, fnB.Description, got.Description, "update was refused")
	vs, err := f.repo.ListVersionsByFunction(bg, fnB.ID)
	require.NoError(t, err)
	require.Len(t, vs, 3, "publish and prune were refused: the versions are as seeded")
	for _, v := range vs {
		assert.Equal(t, function.VersionPublished, v.Status, "retire was refused")
	}
	aliases, err := f.repo.ListAliases(bg, fnB.ID)
	require.NoError(t, err)
	assert.Empty(t, aliases)
	settings, err := f.repo.ListSettings(bg, fnB.ID)
	require.NoError(t, err)
	assert.Empty(t, settings)
	routes, err := f.repo.ListRoutesByFunction(bg, fnB.ID)
	require.NoError(t, err)
	assert.Empty(t, routes)
	var traces int
	require.NoError(t, f.pool.QueryRow(bg, `SELECT COUNT(*) FROM fng_trace_contexts WHERE function_id = $1`, fnB.ID).Scan(&traces))
	assert.Zero(t, traces)
}

// T3: the reads refuse a function of another application.
func TestFunctionReads_RefuseOtherApplication(t *testing.T) {
	t.Parallel()
	f := newReachFixture(t, "t3reads01")
	bg := context.Background()
	vB := f.seedVersion(f.fnB, 1, function.VersionPublished)

	t.Run("list-versions", func(t *testing.T) {
		_, _, err := operations.ListVersions(f.confined, f.repo, f.fnB.ID)
		requireRefused(t, err)
	})
	t.Run("get-version", func(t *testing.T) {
		_, _, err := operations.GetVersion(f.confined, f.repo, f.fnB.ID, vB.Number)
		requireRefused(t, err)
	})
	t.Run("list-aliases", func(t *testing.T) {
		_, _, err := operations.ListAliases(f.confined, f.repo, f.fnB.ID)
		requireRefused(t, err)
	})
	t.Run("list-settings", func(t *testing.T) {
		_, _, err := operations.ListSettings(f.confined, f.repo, f.fnB.ID)
		requireRefused(t, err)
	})
	t.Run("list-routes", func(t *testing.T) {
		_, _, err := operations.ListRoutes(f.confined, f.repo, f.fnB.ID)
		requireRefused(t, err)
	})
	t.Run("list-trace-contexts", func(t *testing.T) {
		_, _, err := operations.ListTraceContexts(f.confined, f.repo, function.DefaultTraceLimits(), f.fnB.ID)
		requireRefused(t, err)
	})

	// The operator reads the same function.
	_, _, err := operations.ListVersions(operatorCtx(), f.repo, f.fnB.ID)
	require.NoError(t, err)
	_ = bg
}

// T4: a confined caller keeps everything it could do on its own application.
func TestFunctionOperations_ConfinedCallerOnOwnApplication(t *testing.T) {
	t.Parallel()
	f := newReachFixture(t, "t4regres1")
	bg := context.Background()
	fnA := f.fnA

	desc := "own function"
	_, err := usecaseop.Run(f.confined, f.uow, operations.UpdateFunction(f.repo),
		operations.UpdateCommand{ID: fnA.ID, Description: &desc}, testpg.TestEC())
	require.NoError(t, err, "update")

	_, err = usecaseop.RunTx(f.confined, f.uow, operations.PutSetting(f.repo),
		operations.PutSettingCommand{FunctionID: fnA.ID, Kind: function.SettingConfig, Key: "GREETING", Value: "hi"}, testpg.TestEC())
	require.NoError(t, err, "settings write")
	_, settings, err := operations.ListSettings(f.confined, f.repo, fnA.ID)
	require.NoError(t, err, "settings read")
	assert.Len(t, settings, 1)

	digest := putArtifactBytes(t, f.artifacts, fnfixture.Wasm)
	res, err := usecaseop.RunTx(f.confined, f.uow, operations.Publish(f.repo, f.artifacts, newTestLoader(t)),
		operations.PublishCommand{FunctionID: fnA.ID, Digest: digest}, testpg.TestEC())
	require.NoError(t, err, "publish")
	require.NoError(t, f.repo.SetVersionReady(bg, res.Version.ID, time.Now().UTC()))

	_, err = usecaseop.RunTx(f.confined, f.uow, operations.PutAlias(f.repo, f.wiring()),
		operations.PutAliasCommand{FunctionID: fnA.ID, Name: "canary", Number: res.Version.Number}, testpg.TestEC())
	require.NoError(t, err, "alias promote")
	_, aliases, err := operations.ListAliases(f.confined, f.repo, fnA.ID)
	require.NoError(t, err, "alias read")
	assert.Len(t, aliases, 1)

	zone := "t4regres1.test"
	c := f.client
	_, err = usecaseop.Run(operatorCtx(), f.uow, fdops.CreateDomain(f.domains),
		fdops.CreateCommand{Zone: zone, ClientID: &c}, testpg.TestEC())
	require.NoError(t, err)
	rt, err := usecaseop.RunTx(f.confined, f.uow, operations.PutRoute(f.repo, f.domains),
		operations.PutRouteCommand{FunctionID: fnA.ID, Hostname: "api." + zone, PathPrefix: "/x"}, testpg.TestEC())
	require.NoError(t, err, "route write")
	_, err = usecaseop.RunTx(f.confined, f.uow, operations.DeleteRoute(f.repo),
		operations.DeleteRouteCommand{FunctionID: fnA.ID, RouteID: rt.ID}, testpg.TestEC())
	require.NoError(t, err, "route delete")

	_, err = usecaseop.RunTx(f.confined, f.uow, operations.EnableTraceContext(f.repo, function.DefaultTraceLimits()),
		operations.EnableTraceCommand{FunctionID: fnA.ID, Context: "http", Minutes: 5, SampleRate: 1}, testpg.TestEC())
	require.NoError(t, err, "trace enable")
	_, _, err = operations.ListTraceContexts(f.confined, f.repo, function.DefaultTraceLimits(), fnA.ID)
	require.NoError(t, err, "trace read")

	_, err = usecaseop.RunTx(f.confined, f.uow, operations.PruneVersions(f.repo),
		operations.PruneCommand{FunctionID: fnA.ID, Keep: 1, DryRun: true}, testpg.TestEC())
	require.NoError(t, err, "prune")

	_, err = usecaseop.RunTx(f.confined, f.uow, operations.DeleteFunction(f.repo, f.wiring()),
		operations.DeleteCommand{ID: fnA.ID}, testpg.TestEC())
	require.NoError(t, err, "delete")
	gone, err := f.repo.FindByID(bg, fnA.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
}
