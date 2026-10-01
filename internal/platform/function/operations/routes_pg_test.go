//go:build integration

// routes_pg_test.go covers the function-runner public-routes write path
// (docs/function-runner-plan.md §8, "public routes"): PutRoute/DeleteRoute,
// their validation (hostname shape, zone coverage, alias existence,
// cross-function uniqueness), the pool-revision bump every write makes, and
// the fng_routes ON DELETE CASCADE from a function delete.
package operations_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain"
	fdops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// short derives a short, test-unique suffix from the test name so parallel
// tests never collide on the unique (zone) / (hostname, pathPrefix) indexes.
func short(t *testing.T) string {
	h := 0
	for _, r := range t.Name() {
		h = h*31 + int(r)
	}
	if h < 0 {
		h = -h
	}
	n := h % 100000
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func newDomainsRepo(f *testFunctionFixture) *functiondomain.Repository {
	return functiondomain.NewRepository(f.pool)
}

func (f *testFunctionFixture) claimZone(zone string) {
	f.t.Helper()
	domains := newDomainsRepo(f)
	_, err := usecaseop.Run(testpg.AnchorCtx(), f.uow, fdops.CreateDomain(domains), fdops.CreateCommand{Zone: zone}, testpg.TestEC())
	require.NoError(f.t, err)
}

// seedAlias points name at an arbitrary (non-existent-version-row) version
// id — fng_aliases.version_id has no FK, so this is enough to exercise the
// route write path's "alias must name an existing alias" check without a
// full publish/promote.
func (f *testFunctionFixture) seedAlias(name string) {
	f.t.Helper()
	require.NoError(f.t, f.repo.UpsertAlias(context.Background(), &function.Alias{
		FunctionID: f.fn.ID, Name: name, VersionID: tsid.Generate(tsid.FunctionVersion), UpdatedAt: time.Now().UTC(),
	}))
}

func (f *testFunctionFixture) putRoute(hostname, pathPrefix, alias string) (function.Route, error) {
	f.t.Helper()
	return usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.PutRoute(f.repo, newDomainsRepo(f)),
		operations.PutRouteCommand{FunctionID: f.fn.ID, Hostname: hostname, PathPrefix: pathPrefix, Alias: alias}, testpg.TestEC())
}

func (f *testFunctionFixture) deleteRoute(routeID string) error {
	f.t.Helper()
	_, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.DeleteRoute(f.repo),
		operations.DeleteRouteCommand{FunctionID: f.fn.ID, RouteID: routeID}, testpg.TestEC())
	return err
}

func TestPutRoute_HappyPath_CreateThenUpdate(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optroutesfn01", "optroutesfnapp1", "route-fn")
	zone := "routes-" + short(t) + ".test"
	f.claimZone(zone)
	host := "api." + zone

	revBefore, _, err := f.repo.GetPoolRevision(context.Background(), f.fn.RunnerPool())
	require.NoError(t, err)

	rt, err := f.putRoute(host, "webhooks", "")
	require.NoError(t, err)
	assert.NotEmpty(t, rt.ID)
	assert.Equal(t, host, rt.Hostname)
	assert.Equal(t, "/webhooks", rt.PathPrefix, "path prefix must be normalised")
	assert.Nil(t, rt.Alias, "empty alias means live")

	revAfterCreate, _, err := f.repo.GetPoolRevision(context.Background(), f.fn.RunnerPool())
	require.NoError(t, err)
	assert.Greater(t, revAfterCreate, revBefore, "a route create must bump the pool revision")

	f.seedAlias("canary")
	rt2, err := f.putRoute(host, "/webhooks/", "canary")
	require.NoError(t, err)
	assert.Equal(t, rt.ID, rt2.ID, "same (hostname, pathPrefix) on the same function is an UPDATE, not a new row")
	require.NotNil(t, rt2.Alias)
	assert.Equal(t, "canary", *rt2.Alias)

	revAfterUpdate, _, err := f.repo.GetPoolRevision(context.Background(), f.fn.RunnerPool())
	require.NoError(t, err)
	assert.Greater(t, revAfterUpdate, revAfterCreate, "a route update must also bump the pool revision")

	routes, err := f.repo.ListRoutesByFunction(context.Background(), f.fn.ID)
	require.NoError(t, err)
	assert.Len(t, routes, 1, "the update must not create a second row")
}

func TestPutRoute_HostNotClaimed(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optroutesfn02", "optroutesfnapp2", "route-fn")

	_, err := f.putRoute("unclaimed-"+short(t)+".test", "/", "")
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "ROUTE_HOST_NOT_CLAIMED")
}

func TestPutRoute_InvalidHostname(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optroutesfn03", "optroutesfnapp3", "route-fn")

	_, err := f.putRoute("*.bad.com", "/", "")
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVALID_HOSTNAME")
}

func TestPutRoute_AliasMustExist(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optroutesfn04", "optroutesfnapp4", "route-fn")
	zone := "aliaschk-" + short(t) + ".test"
	f.claimZone(zone)

	_, err := f.putRoute("api."+zone, "/", "no-such-alias")
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "ALIAS_NOT_FOUND")
}

// TestPutRoute_RouteTaken_AcrossFunctions proves the cross-function
// uniqueness check: two DIFFERENT functions (same platform owner, so both
// pass the zone-coverage check) cannot both claim the same
// (hostname, pathPrefix).
func TestPutRoute_RouteTaken_AcrossFunctions(t *testing.T) {
	t.Parallel()
	fA := newTestFunctionFixture(t, "app_optroutesfn05", "optroutesfnapp5", "route-fn-a")
	zone := "taken-" + short(t) + ".test"
	fA.claimZone(zone)
	host := "api." + zone

	_, err := fA.putRoute(host, "/", "")
	require.NoError(t, err)

	// A second function, same application — reuse the same fixture's repo
	// but a distinct function row.
	ev, err := usecaseop.Run(testpg.AnchorCtx(), fA.uow, operations.CreateFunction(fA.repo, fA.apps),
		operations.CreateCommand{ApplicationID: fA.appID, Name: "route-fn-b"}, testpg.TestEC())
	require.NoError(t, err)
	fnB, err := fA.repo.FindByID(context.Background(), ev.FunctionID)
	require.NoError(t, err)
	fB := &testFunctionFixture{t: t, pool: fA.pool, repo: fA.repo, apps: fA.apps, uow: fA.uow, appID: fA.appID, appCode: fA.appCode, fn: fnB}

	_, err = fB.putRoute(host, "/", "")
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "ROUTE_TAKEN")
}

func TestDeleteRoute_HappyPath(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optroutesfn06", "optroutesfnapp6", "route-fn")
	zone := "del-" + short(t) + ".test"
	f.claimZone(zone)
	rt, err := f.putRoute("api."+zone, "/", "")
	require.NoError(t, err)

	revBefore, _, err := f.repo.GetPoolRevision(context.Background(), f.fn.RunnerPool())
	require.NoError(t, err)

	require.NoError(t, f.deleteRoute(rt.ID))

	revAfter, _, err := f.repo.GetPoolRevision(context.Background(), f.fn.RunnerPool())
	require.NoError(t, err)
	assert.Greater(t, revAfter, revBefore, "a route delete must bump the pool revision")

	got, err := f.repo.FindRouteByID(context.Background(), rt.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestDeleteRoute_NotFound(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optroutesfn07", "optroutesfnapp7", "route-fn")
	err := f.deleteRoute(tsid.Generate(tsid.FunctionRoute))
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "FunctionRoute_NOT_FOUND")
}

// TestDeleteFunction_CascadesRoutes proves the fng_routes ON DELETE CASCADE
// FK (migration 062): deleting the function row removes its routes.
func TestDeleteFunction_CascadesRoutes(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optroutesfn08", "optroutesfnapp8", "route-fn")
	zone := "cascade-" + short(t) + ".test"
	f.claimZone(zone)
	rt, err := f.putRoute("api."+zone, "/", "")
	require.NoError(t, err)

	_, err = usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.DeleteFunction(f.repo, f.wiring),
		operations.DeleteCommand{ID: f.fn.ID}, testpg.TestEC())
	require.NoError(t, err)

	got, err := f.repo.FindRouteByID(context.Background(), rt.ID)
	require.NoError(t, err)
	assert.Nil(t, got, "the route row must be gone once its function is deleted")
}

func (f *testFunctionFixture) putRouteWithPrefixes(hostname, alias string, prefixes []string) (function.Route, error) {
	f.t.Helper()
	return usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.PutRoute(f.repo, newDomainsRepo(f)),
		operations.PutRouteCommand{FunctionID: f.fn.ID, Hostname: hostname, PathPrefix: "/", Alias: alias, AliasPrefixes: prefixes}, testpg.TestEC())
}

// TestPutRoute_AliasPrefixes proves prefixes persist (sorted), round-trip
// through the pool document query, can be cleared by a later PUT, and are
// refused when malformed or set on a route pinned to a named alias.
func TestPutRoute_AliasPrefixes(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optroutesfn11", "optroutesfnapp11", "route-fn")
	zone := "prefixes-" + short(t) + ".test"
	f.claimZone(zone)
	host := "myapp." + zone

	rt, err := f.putRouteWithPrefixes(host, "", []string{"staging", "qa"})
	require.NoError(t, err)
	assert.Equal(t, []string{"qa", "staging"}, rt.AliasPrefixes)

	routes, _, err := f.repo.ListRoutesByPool(context.Background(), f.fn.RunnerPool())
	require.NoError(t, err)
	var found bool
	for _, r := range routes {
		if r.ID == rt.ID {
			found = true
			assert.Equal(t, []string{"qa", "staging"}, r.AliasPrefixes)
		}
	}
	assert.True(t, found, "route must appear in the pool's control document query")

	cleared, err := f.putRouteWithPrefixes(host, "", nil)
	require.NoError(t, err)
	assert.Equal(t, rt.ID, cleared.ID)
	assert.Empty(t, cleared.AliasPrefixes)

	for _, bad := range [][]string{{"live"}, {"Qa"}, {"qa-x"}, {"qa", "qa"}, {""}} {
		_, err = f.putRouteWithPrefixes(host, "", bad)
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVALID_ALIAS_PREFIX")
	}

	f.seedAlias("canary")
	_, err = f.putRouteWithPrefixes(host, "canary", []string{"qa"})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ALIAS_PREFIXES_REQUIRE_LIVE")
}
