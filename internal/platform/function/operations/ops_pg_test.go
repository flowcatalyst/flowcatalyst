//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// runAuthorized drives op through the full use-case envelope as an anchor
// principal — mirrors dispatchpool/operations/ops_pg_test.go.
func runAuthorized[C any, E usecase.DomainEvent](
	uow *usecasepgx.UnitOfWork, op usecaseop.Operation[C, E], cmd C,
) (E, error) {
	return usecaseop.Run(testpg.AnchorCtx(), uow, op, cmd, testpg.TestEC())
}

// seedApplication inserts a bare app_applications row directly — the
// operations under test only need an application to exist with a known
// code; going through application's own use cases would be out of scope
// here (see function/repository_pg_test.go's identical helper).
func seedApplication(t *testing.T, pool *pgxpool.Pool, id, code string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO app_applications (id, code, name)
		VALUES ($1, $2, $2) ON CONFLICT (id) DO NOTHING`, id, code)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM app_applications WHERE id = $1`, id)
	})
}

// mustCreate seeds a function through the public operation.
func mustCreate(t *testing.T, repo *function.Repository, apps *application.Repository, uow *usecasepgx.UnitOfWork, appID, name string) operations.FunctionCreated {
	t.Helper()
	ev, err := runAuthorized(uow, operations.CreateFunction(repo, apps),
		operations.CreateCommand{ApplicationID: appID, Name: name})
	require.NoError(t, err)
	return ev
}

// ── Create ────────────────────────────────────────────────────────────────

func TestCreateFunction_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	apps := application.NewRepository(pool)
	uow := testpg.NewUoW(t)
	seedApplication(t, pool, "app_opcreatefn001", "opcreatefnapp")

	desc := "a function"
	warm := true
	ev, err := runAuthorized(uow, operations.CreateFunction(repo, apps), operations.CreateCommand{
		ApplicationID: "app_opcreatefn001",
		Name:          "  my-fn  ", // trimmed
		Description:   &desc,
		Warm:          &warm,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, ev.FunctionID)
	assert.Equal(t, "opcreatefnapp.my-fn", ev.Address)

	got, err := repo.FindByID(ctx, ev.FunctionID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "my-fn", got.Name)
	assert.Equal(t, "opcreatefnapp.my-fn", got.Address)
	assert.Equal(t, "opcreatefnapp", got.ApplicationCode)
	assert.True(t, got.Warm)
	assert.Equal(t, desc, *got.Description)
	assert.Nil(t, got.ClientID, "no clientId → platform-owned")
	assert.Equal(t, function.DefaultLimits(), got.Limits)
}

func TestCreateFunction_Validation(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	apps := application.NewRepository(pool)
	uow := testpg.NewUoW(t)
	seedApplication(t, pool, "app_opcreatefnval", "opcreatefnvalapp")

	cases := []struct {
		name string
		cmd  operations.CreateCommand
		code string
	}{
		{"missing application id", operations.CreateCommand{Name: "x"}, "APPLICATION_ID_REQUIRED"},
		{"empty name", operations.CreateCommand{ApplicationID: "app_opcreatefnval"}, "NAME_REQUIRED"},
		{"uppercase name", operations.CreateCommand{ApplicationID: "app_opcreatefnval", Name: "Bad"}, "INVALID_NAME_FORMAT"},
		{"name starts with digit", operations.CreateCommand{ApplicationID: "app_opcreatefnval", Name: "1bad"}, "INVALID_NAME_FORMAT"},
		{"name with underscore", operations.CreateCommand{ApplicationID: "app_opcreatefnval", Name: "bad_name"}, "INVALID_NAME_FORMAT"},
		{"memory too large", operations.CreateCommand{
			ApplicationID: "app_opcreatefnval", Name: "big-mem",
			Limits: &operations.LimitsInput{MemoryMB: int32Ptr(function.MaxMemoryMB + 1)},
		}, "MEMORY_MB_TOO_LARGE"},
		{"zero timeout", operations.CreateCommand{
			ApplicationID: "app_opcreatefnval", Name: "zero-timeout",
			Limits: &operations.LimitsInput{TimeoutMs: int32Ptr(0)},
		}, "TIMEOUT_MS_REQUIRED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.CreateFunction(repo, apps), tc.cmd)
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
}

func TestCreateFunction_UnknownApplication_NotFound(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	apps := application.NewRepository(pool)
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.CreateFunction(repo, apps), operations.CreateCommand{
		ApplicationID: "app_doesnotexist1", Name: "orphan",
	})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Application_NOT_FOUND")
}

func TestCreateFunction_DuplicateAddress_Conflict(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	apps := application.NewRepository(pool)
	uow := testpg.NewUoW(t)
	seedApplication(t, pool, "app_opcreatefndup", "opcreatefndupapp")

	mustCreate(t, repo, apps, uow, "app_opcreatefndup", "dup-fn")

	_, err := runAuthorized(uow, operations.CreateFunction(repo, apps), operations.CreateCommand{
		ApplicationID: "app_opcreatefndup", Name: "dup-fn",
	})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "ADDRESS_EXISTS")
}

// TestCreateFunction_ResourceScope proves the use case's per-resource
// authorization: a client-scoped principal may create a function bound to
// their own client, is denied for another client, and is denied for a
// platform-owned (nil clientId) function (anchor-only) — mirrors
// dispatchpool's TestCreateDispatchPool_ResourceScope.
func TestCreateFunction_ResourceScope(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	apps := application.NewRepository(pool)
	uow := testpg.NewUoW(t)
	seedApplication(t, pool, "app_opcreatefnscp", "opcreatefnscpapp")

	ownClient := "cli_opcreatfnscp1"
	clientCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_opcreatfnscp1",
		Scope:       auth.ScopeClient,
		Clients:     []string{ownClient},
		Permissions: []string{"platform:function:manage"},
	})

	// Platform-owned (nil ClientID) → anchor required → denied.
	_, err := usecaseop.Run(clientCtx, uow, operations.CreateFunction(repo, apps),
		operations.CreateCommand{ApplicationID: "app_opcreatefnscp", Name: "scope-platform"}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	// Bound to a client the principal cannot access → denied.
	other := "cli_opcreatfnscp2"
	_, err = usecaseop.Run(clientCtx, uow, operations.CreateFunction(repo, apps),
		operations.CreateCommand{ApplicationID: "app_opcreatefnscp", Name: "scope-other", ClientID: &other}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	// Bound to the principal's own client → allowed.
	ev, err := usecaseop.Run(clientCtx, uow, operations.CreateFunction(repo, apps),
		operations.CreateCommand{ApplicationID: "app_opcreatefnscp", Name: "scope-own", ClientID: &ownClient}, testpg.TestEC())
	require.NoError(t, err)
	assert.Equal(t, "opcreatefnscpapp.scope-own", ev.Address)
}

// ── Update ────────────────────────────────────────────────────────────────

func TestUpdateFunction_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	apps := application.NewRepository(pool)
	uow := testpg.NewUoW(t)
	seedApplication(t, pool, "app_opupdatefn001", "opupdatefnapp")
	seeded := mustCreate(t, repo, apps, uow, "app_opupdatefn001", "upd-fn")

	newDesc := "updated description"
	warm := true
	ev, err := runAuthorized(uow, operations.UpdateFunction(repo), operations.UpdateCommand{
		ID:          seeded.FunctionID,
		Description: &newDesc,
		Warm:        &warm,
		Limits:      &operations.LimitsInput{MemoryMB: int32Ptr(256)},
	})
	require.NoError(t, err)
	assert.Equal(t, seeded.FunctionID, ev.FunctionID)

	got, err := repo.FindByID(ctx, seeded.FunctionID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, newDesc, *got.Description)
	assert.True(t, got.Warm)
	assert.Equal(t, int32(256), got.Limits.MemoryMB)
	assert.Equal(t, "upd-fn", got.Name, "name is immutable")
	assert.Equal(t, "opupdatefnapp.upd-fn", got.Address, "address is immutable")

	// ClearPool: set then clear.
	poolOverride := "custom"
	_, err = runAuthorized(uow, operations.UpdateFunction(repo), operations.UpdateCommand{
		ID: seeded.FunctionID, Pool: &poolOverride,
	})
	require.NoError(t, err)
	got, err = repo.FindByID(ctx, seeded.FunctionID)
	require.NoError(t, err)
	require.NotNil(t, got.Pool)
	assert.Equal(t, "custom", *got.Pool)

	_, err = runAuthorized(uow, operations.UpdateFunction(repo), operations.UpdateCommand{
		ID: seeded.FunctionID, ClearPool: true,
	})
	require.NoError(t, err)
	got, err = repo.FindByID(ctx, seeded.FunctionID)
	require.NoError(t, err)
	assert.Nil(t, got.Pool, "clearPool must clear the override")
}

func TestUpdateFunction_Errors(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.UpdateCommand
		kind usecase.Kind
		code string
	}{
		{"missing id", operations.UpdateCommand{}, usecase.KindValidation, "ID_REQUIRED"},
		{"unknown id", operations.UpdateCommand{ID: "fnc_doesnotexist1"}, usecase.KindNotFound, "Function_NOT_FOUND"},
		{"memory too large", operations.UpdateCommand{
			ID: "fnc_doesnotexist1", Limits: &operations.LimitsInput{MemoryMB: int32Ptr(function.MaxMemoryMB + 1)},
		}, usecase.KindNotFound, "Function_NOT_FOUND"}, // not-found is checked before limits validation (post-load)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.UpdateFunction(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

// TestUpdateFunction_LimitsCeiling_OnExistingRow proves the ceiling check
// fires for a real row (distinct from the not-found case above).
func TestUpdateFunction_LimitsCeiling_OnExistingRow(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	apps := application.NewRepository(pool)
	uow := testpg.NewUoW(t)
	seedApplication(t, pool, "app_opupdatefnlim", "opupdatefnlimapp")
	seeded := mustCreate(t, repo, apps, uow, "app_opupdatefnlim", "lim-fn")

	_, err := runAuthorized(uow, operations.UpdateFunction(repo), operations.UpdateCommand{
		ID:     seeded.FunctionID,
		Limits: &operations.LimitsInput{MaxConcurrency: int32Ptr(function.MaxMaxConcurrency + 1)},
	})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "MAX_CONCURRENCY_TOO_LARGE")
}

// TestUpdateFunction_ResourceScope proves per-resource authorization runs
// post-load: a client-scoped principal may update their own client's
// function, is denied for another client's, and is denied for a
// platform-owned one.
func TestUpdateFunction_ResourceScope(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	apps := application.NewRepository(pool)
	uow := testpg.NewUoW(t)
	seedApplication(t, pool, "app_opupdatefnscp", "opupdatefnscpapp")

	platformFn := mustCreate(t, repo, apps, uow, "app_opupdatefnscp", "scope-platform")

	ownClient := "cli_opupdatfnscp1"
	ownEv, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateFunction(repo, apps),
		operations.CreateCommand{ApplicationID: "app_opupdatefnscp", Name: "scope-own", ClientID: &ownClient}, testpg.TestEC())
	require.NoError(t, err)

	otherClient := "cli_opupdatfnscp2"
	otherEv, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateFunction(repo, apps),
		operations.CreateCommand{ApplicationID: "app_opupdatefnscp", Name: "scope-other", ClientID: &otherClient}, testpg.TestEC())
	require.NoError(t, err)

	clientCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_opupdatfnscp1",
		Scope:       auth.ScopeClient,
		Clients:     []string{ownClient},
		Permissions: []string{"platform:function:manage"},
	})

	newDesc := "d"
	_, err = usecaseop.Run(clientCtx, uow, operations.UpdateFunction(repo),
		operations.UpdateCommand{ID: platformFn.FunctionID, Description: &newDesc}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	_, err = usecaseop.Run(clientCtx, uow, operations.UpdateFunction(repo),
		operations.UpdateCommand{ID: otherEv.FunctionID, Description: &newDesc}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	_, err = usecaseop.Run(clientCtx, uow, operations.UpdateFunction(repo),
		operations.UpdateCommand{ID: ownEv.FunctionID, Description: &newDesc}, testpg.TestEC())
	require.NoError(t, err)
}

// ── Delete ────────────────────────────────────────────────────────────────

func TestDeleteFunction_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	apps := application.NewRepository(pool)
	uow := testpg.NewUoW(t)
	seedApplication(t, pool, "app_opdeletefn001", "opdeletefnapp")
	seeded := mustCreate(t, repo, apps, uow, "app_opdeletefn001", "del-fn")

	ev, err := runAuthorized(uow, operations.DeleteFunction(repo), operations.DeleteCommand{ID: seeded.FunctionID})
	require.NoError(t, err)
	assert.Equal(t, seeded.FunctionID, ev.FunctionID)

	got, err := repo.FindByID(ctx, seeded.FunctionID)
	require.NoError(t, err)
	assert.Nil(t, got, "deleted row must be gone")
}

func TestDeleteFunction_Errors(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.DeleteFunction(repo), operations.DeleteCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.DeleteFunction(repo), operations.DeleteCommand{ID: "fnc_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Function_NOT_FOUND")
}

// TestDeleteFunction_ResourceScope_AnchorOnlyForPlatformOwned is the
// explicit authz-denial pin the WP3 task calls for: a client-scoped
// principal may delete their own client's function but is denied for a
// platform-owned (nil clientId) one, even holding the coarse manage
// permission.
func TestDeleteFunction_ResourceScope_AnchorOnlyForPlatformOwned(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	apps := application.NewRepository(pool)
	uow := testpg.NewUoW(t)
	seedApplication(t, pool, "app_opdeletefnscp", "opdeletefnscpapp")

	platformFn := mustCreate(t, repo, apps, uow, "app_opdeletefnscp", "scope-platform")

	ownClient := "cli_opdeletfnscp1"
	ownEv, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateFunction(repo, apps),
		operations.CreateCommand{ApplicationID: "app_opdeletefnscp", Name: "scope-own", ClientID: &ownClient}, testpg.TestEC())
	require.NoError(t, err)

	clientCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_opdeletfnscp1",
		Scope:       auth.ScopeClient,
		Clients:     []string{ownClient},
		Permissions: []string{"platform:function:manage"},
	})

	_, err = usecaseop.Run(clientCtx, uow, operations.DeleteFunction(repo),
		operations.DeleteCommand{ID: platformFn.FunctionID}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	// The platform-owned function must still exist.
	got, err := repo.FindByID(ctx, platformFn.FunctionID)
	require.NoError(t, err)
	assert.NotNil(t, got)

	// The client's own function may be deleted.
	_, err = usecaseop.Run(clientCtx, uow, operations.DeleteFunction(repo),
		operations.DeleteCommand{ID: ownEv.FunctionID}, testpg.TestEC())
	require.NoError(t, err)
}

func int32Ptr(v int32) *int32 { return &v }
