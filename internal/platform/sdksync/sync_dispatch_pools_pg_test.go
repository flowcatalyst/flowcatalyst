//go:build integration

package sdksync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	poolops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// Dispatch pool sync by an application service account (F19). The built-in
// platform:application-service role carries platform:messaging:dispatch-pool:sync
// so an application's deploy can sync its own pools. Pools are global rows
// matched by code, so the grant is safe only with ownership: an application
// may create pools and change the pools it owns, never another application's
// or a platform pool.

func appServiceRolePermissions(t *testing.T) []string {
	t.Helper()
	for _, r := range seed.PlatformRoles() {
		if r.Name == "platform:application-service" {
			return r.Permissions
		}
	}
	t.Fatal("platform:application-service role not found")
	return nil
}

type poolSyncFixture struct {
	t     *testing.T
	s     *State
	repo  *dispatchpool.Repository
	appID map[string]string
}

func newPoolSyncFixture(t *testing.T, codes ...string) *poolSyncFixture {
	t.Helper()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	f := &poolSyncFixture{
		t:     t,
		repo:  dispatchpool.NewRepository(pool),
		appID: map[string]string{},
	}
	f.s = &State{Apps: application.NewRepository(pool), DispatchPools: f.repo, UoW: uow}
	for _, c := range codes {
		app, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(application.NewRepository(pool)),
			appops.CreateCommand{Code: c, Name: c}, testpg.TestEC())
		require.NoError(t, err)
		f.appID[c] = app.ApplicationID
	}
	return f
}

// appSA is an application service account bound to the given applications,
// holding exactly the built-in application-service role's permissions.
func (f *poolSyncFixture) appSA(scope auth.Scope, apps ...string) context.Context {
	bound := make([]string, 0, len(apps))
	for _, a := range apps {
		bound = append(bound, f.appID[a])
	}
	return auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_poolsyncsa001", Scope: scope, PrincipalType: "SERVICE",
		Applications: bound, Permissions: appServiceRolePermissions(f.t),
	})
}

// admin is an all-applications anchor holding the messaging admin permission.
func (f *poolSyncFixture) admin() context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_poolsyncadm01", Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions: []string{"platform:messaging:dispatch-pool:sync"},
	})
}

func (f *poolSyncFixture) sync(ctx context.Context, appCode string, remove bool, codes ...string) error {
	pools := make([]syncDispatchPoolInputRequest, 0, len(codes))
	for _, c := range codes {
		pools = append(pools, syncDispatchPoolInputRequest{Code: c, Name: "name of " + c})
	}
	_, err := f.s.syncDispatchPools(ctx, &syncDispatchPoolsInput{
		AppCode: appCode, RemoveUnlisted: remove, Body: syncDispatchPoolsRequest{Pools: pools},
	})
	return err
}

func (f *poolSyncFixture) pool(code string) *dispatchpool.DispatchPool {
	f.t.Helper()
	p, err := f.repo.FindByCode(context.Background(), code, nil)
	require.NoError(f.t, err)
	return p
}

func requireForbiddenApp(t *testing.T, err error, appCode string) {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "want a refusal, got %v", err)
	assert.Equal(t, usecase.KindAuthorization, ue.Kind)
	assert.Equal(t, "FORBIDDEN", ue.Code)
	assert.Equal(t, "Not authorised for application '"+appCode+"'", ue.Message)
}

func TestSyncDispatchPools_ApplicationServiceAccount_OwnApplication(t *testing.T) {
	f := newPoolSyncFixture(t, "dpsownapp")
	sa := f.appSA(auth.ScopeAnchor, "dpsownapp")

	require.NoError(t, f.sync(sa, "dpsownapp", false, "dpsownapp-high", "dpsownapp-low"))
	require.NotNil(t, f.pool("dpsownapp-high"))
	require.NotNil(t, f.pool("dpsownapp-low"))

	// Re-sync (an update of pools it created) works too.
	require.NoError(t, f.sync(sa, "dpsownapp", false, "dpsownapp-high"))
}

func TestSyncDispatchPools_ApplicationServiceAccount_OtherApplicationRefused(t *testing.T) {
	f := newPoolSyncFixture(t, "dpsmine", "dpstheirs")
	sa := f.appSA(auth.ScopeAnchor, "dpsmine")

	err := f.sync(sa, "dpstheirs", false, "dpstheirs-new")
	requireForbiddenApp(t, err, "dpstheirs")
	assert.Nil(t, f.pool("dpstheirs-new"), "nothing written for the other application")

	// Not bound to anything (an unbound caller) and CLIENT-scoped: the same
	// refusal as the other per-application syncs.
	unbound := f.appSA(auth.ScopeAnchor)
	requireForbiddenApp(t, f.sync(unbound, "dpsmine", false, "dpsmine-x"), "dpsmine")
	clientScoped := f.appSA(auth.ScopeClient)
	requireForbiddenApp(t, f.sync(clientScoped, "dpsmine", false, "dpsmine-x"), "dpsmine")
	assert.Nil(t, f.pool("dpsmine-x"))
}

func TestSyncDispatchPools_ApplicationServiceAccount_CannotTakeOverAPool(t *testing.T) {
	f := newPoolSyncFixture(t, "dpstakea", "dpstakeb")
	uow := testpg.NewUoW(t)

	// Another application's pool, an admin-created platform pool, and a pool
	// created by an anchor sync with no owner: none may be changed by an
	// application that does not own it.
	require.NoError(t, f.sync(f.appSA(auth.ScopeAnchor, "dpstakeb"), "dpstakeb", false, "dpstakeb-pool"))
	_, err := usecaseop.Run(testpg.AnchorCtx(), uow, poolops.CreateDispatchPool(f.repo),
		poolops.CreateCommand{Code: "dpstake-platform", Name: "Platform pool"}, testpg.TestEC())
	require.NoError(t, err)

	sa := f.appSA(auth.ScopeAnchor, "dpstakea")
	for _, code := range []string{"dpstakeb-pool", "dpstake-platform"} {
		before := f.pool(code)
		err := f.sync(sa, "dpstakea", false, code)
		testpgRequire(t, err, usecase.KindAuthorization, "DISPATCH_POOL_NOT_OWNED")
		after := f.pool(code)
		assert.Equal(t, before.Name, after.Name, "%s left untouched", code)
		assert.Equal(t, before.Concurrency, after.Concurrency, "%s left untouched", code)
	}
	// One owned code mixed with a foreign one: all-or-nothing.
	err = f.sync(sa, "dpstakea", false, "dpstakea-fresh", "dpstakeb-pool")
	testpgRequire(t, err, usecase.KindAuthorization, "DISPATCH_POOL_NOT_OWNED")
	assert.Nil(t, f.pool("dpstakea-fresh"))
}

func TestSyncDispatchPools_ApplicationServiceAccount_CannotArchiveOthers(t *testing.T) {
	f := newPoolSyncFixture(t, "dpsarchmine", "dpsarchtheirs")
	require.NoError(t, f.sync(f.appSA(auth.ScopeAnchor, "dpsarchtheirs"), "dpsarchtheirs", false, "dpsarchtheirs-pool"))
	uow := testpg.NewUoW(t)
	_, err := usecaseop.Run(testpg.AnchorCtx(), uow, poolops.CreateDispatchPool(f.repo),
		poolops.CreateCommand{Code: "dpsarch-platform", Name: "Platform pool"}, testpg.TestEC())
	require.NoError(t, err)

	sa := f.appSA(auth.ScopeAnchor, "dpsarchmine")
	require.NoError(t, f.sync(sa, "dpsarchmine", false, "dpsarchmine-keep", "dpsarchmine-drop"))

	// The sweep archives only the pools the application owns.
	require.NoError(t, f.sync(sa, "dpsarchmine", true, "dpsarchmine-keep"))
	assert.Equal(t, dispatchpool.StatusArchived, f.pool("dpsarchmine-drop").Status)
	assert.Equal(t, dispatchpool.StatusActive, f.pool("dpsarchmine-keep").Status)
	assert.Equal(t, dispatchpool.StatusActive, f.pool("dpsarchtheirs-pool").Status, "another application's pool")
	assert.Equal(t, dispatchpool.StatusActive, f.pool("dpsarch-platform").Status, "a platform pool")

	// Targeting the other application's code is refused outright.
	requireForbiddenApp(t, f.sync(sa, "dpsarchtheirs", true, "dpsarchtheirs-pool"), "dpsarchtheirs")
	assert.Equal(t, dispatchpool.StatusActive, f.pool("dpsarchtheirs-pool").Status)

	// A non-anchor account holding the permission cannot sweep at all.
	clientScoped := f.appSA(auth.ScopeClient, "dpsarchmine")
	testpgRequire(t, f.sync(clientScoped, "dpsarchmine", true, "dpsarchmine-keep"),
		usecase.KindAuthorization, "ANCHOR_REQUIRED_FOR_PLATFORM_SWEEP")
}

func TestSyncDispatchPools_Admin_StillSyncsAnyApplication(t *testing.T) {
	f := newPoolSyncFixture(t, "dpsadmx", "dpsadmy")
	adm := f.admin()

	require.NoError(t, f.sync(adm, "dpsadmx", false, "dpsadmx-pool"))
	require.NoError(t, f.sync(adm, "dpsadmy", false, "dpsadmy-pool"))
	// An admin may also update a pool another application owns.
	require.NoError(t, f.sync(adm, "dpsadmx", false, "dpsadmy-pool"))
	// And sweep.
	require.NoError(t, f.sync(adm, "dpsadmx", true, "dpsadmx-pool"))
	assert.Equal(t, dispatchpool.StatusArchived, f.pool("dpsadmy-pool").Status)
}

func TestSyncDispatchPools_AdminClaimsUnownedPoolForTheApplication(t *testing.T) {
	f := newPoolSyncFixture(t, "dpsclaim")
	uow := testpg.NewUoW(t)
	_, err := usecaseop.Run(testpg.AnchorCtx(), uow, poolops.CreateDispatchPool(f.repo),
		poolops.CreateCommand{Code: "dpsclaim-pool", Name: "Pre-existing"}, testpg.TestEC())
	require.NoError(t, err)

	sa := f.appSA(auth.ScopeAnchor, "dpsclaim")
	testpgRequire(t, f.sync(sa, "dpsclaim", false, "dpsclaim-pool"), usecase.KindAuthorization, "DISPATCH_POOL_NOT_OWNED")

	// One sync by an admin hands the unowned pool to the application...
	require.NoError(t, f.sync(f.admin(), "dpsclaim", false, "dpsclaim-pool"))
	// ...after which the application's own account syncs it.
	require.NoError(t, f.sync(sa, "dpsclaim", false, "dpsclaim-pool"))
}

func testpgRequire(t *testing.T, err error, kind usecase.Kind, code string) {
	t.Helper()
	testpg.RequireUsecaseError(t, err, kind, code)
}
