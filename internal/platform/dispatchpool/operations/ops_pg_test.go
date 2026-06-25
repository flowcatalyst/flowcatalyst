//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

func ptr[T any](v T) *T { return &v }

// runAuthorized drives op through the full use-case envelope (Validate →
// Authorize → Execute → atomic commit) as an anchor principal — the common
// case for these tests, which exercise validation, invariants, and
// persistence rather than authorization itself (see
// TestDispatchPoolWrites_RequirePermission for that). It mirrors how the HTTP
// handler runs the operation.
func runAuthorized[C any, E usecase.DomainEvent](
	uow *usecasepgx.UnitOfWork, op usecaseop.Operation[C, E], cmd C,
) (E, error) {
	return usecaseop.Run(testpg.AnchorCtx(), uow, op, cmd, testpg.TestEC())
}

// appAccessCtx is an all-applications anchor principal. SyncDispatchPools
// authorizes against the target application (CanAccessApplication) — a bare
// AnchorCtx sets Scope=Anchor but NOT AllApplications, so it would be denied.
// App-scoped sync tests run under this principal so they can reach any
// application.
func appAccessCtx() context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_optestrunner1", Scope: auth.ScopeAnchor, AllApplications: true,
	})
}

// mustCreate seeds a dispatch pool through the public operation — the same
// path production uses. Codes are hand-unique per test: the fixture never
// truncates between tests, so tests own their rows and never assert
// table-wide.
func mustCreate(t *testing.T, repo *dispatchpool.Repository, uow *usecasepgx.UnitOfWork, code, name string) operations.DispatchPoolCreated {
	t.Helper()
	ev, err := runAuthorized(uow, operations.CreateDispatchPool(repo),
		operations.CreateCommand{Code: code, Name: name})
	require.NoError(t, err)
	return ev
}

// ── Create ────────────────────────────────────────────────────────────────

func TestCreateDispatchPool_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := dispatchpool.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	// Defaults: nil concurrency → 10, nil rateLimit stays nil (no limiter).
	desc := "router pool"
	ev, err := runAuthorized(uow, operations.CreateDispatchPool(repo), operations.CreateCommand{
		Code:        "  DPCreate-Happy  ", // op must trim + lowercase
		Name:        "DP Create Happy",
		Description: &desc,
	})
	require.NoError(t, err)

	assert.NotEmpty(t, ev.PoolID)
	assert.Equal(t, "dpcreate-happy", ev.Code, "code must be trimmed + lowercased")
	assert.Equal(t, "DP Create Happy", ev.Name)

	got, err := repo.FindByID(ctx, ev.PoolID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "dpcreate-happy", got.Code)
	assert.Equal(t, "DP Create Happy", got.Name)
	assert.Equal(t, dispatchpool.StatusActive, got.Status, "new pools start ACTIVE")
	assert.Equal(t, int32(10), got.Concurrency, "nil concurrency must default to 10")
	assert.Nil(t, got.RateLimit, "nil rateLimit must stay nil (concurrency-only pool)")
	assert.Nil(t, got.ClientID)
	require.NotNil(t, got.Description)
	assert.Equal(t, desc, *got.Description)

	// Explicit values: rateLimit 0 is VALID at create (bound is ≥ 0 — sync's
	// is ≥ 1, pinned in TestSyncDispatchPools_Validation), concurrency 1 is
	// the lower bound.
	ev, err = runAuthorized(uow, operations.CreateDispatchPool(repo), operations.CreateCommand{
		Code:        "dpcreate-explicit",
		Name:        "DP Create Explicit",
		RateLimit:   ptr(int32(0)),
		Concurrency: ptr(int32(1)),
	})
	require.NoError(t, err)

	got, err = repo.FindByID(ctx, ev.PoolID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.RateLimit)
	assert.Equal(t, int32(0), *got.RateLimit)
	assert.Equal(t, int32(1), got.Concurrency)
}

// Pins the underscore union fix: create deliberately validates against
// validate.CodeUnderscorePattern (owner-approved widening from hyphen-only),
// matching sync and the Rust pool_code_pattern.
func TestCreateDispatchPool_UnderscoreCode_Succeeds(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := dispatchpool.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	ev, err := runAuthorized(uow, operations.CreateDispatchPool(repo), operations.CreateCommand{
		Code: "dp_underscore_ok",
		Name: "Underscore Pool",
	})
	require.NoError(t, err, "underscores in pool codes must be accepted")
	assert.Equal(t, "dp_underscore_ok", ev.Code)

	got, err := repo.FindByID(ctx, ev.PoolID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "dp_underscore_ok", got.Code)
}

// Note: an uppercase code like "Bad" can never hit INVALID_CODE_FORMAT on
// create — the op lowercases BEFORE validating (pinned by the happy path).
// Sync does NOT lowercase; see TestSyncDispatchPools_Validation.
func TestCreateDispatchPool_Validation(t *testing.T) {
	t.Parallel()
	repo := dispatchpool.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.CreateCommand
		code string
	}{
		{"empty code", operations.CreateCommand{Name: "X"}, "CODE_REQUIRED"},
		{"code starts with digit", operations.CreateCommand{Code: "1bad", Name: "X"}, "INVALID_CODE_FORMAT"},
		{"code with space", operations.CreateCommand{Code: "bad code", Name: "X"}, "INVALID_CODE_FORMAT"},
		{"empty name", operations.CreateCommand{Code: "dpcrt-noname"}, "NAME_REQUIRED"},
		{"zero concurrency", operations.CreateCommand{
			Code: "dpcrt-conc", Name: "X", Concurrency: ptr(int32(0)),
		}, "INVALID_CONCURRENCY"},
		{"negative rate limit", operations.CreateCommand{
			Code: "dpcrt-rate", Name: "X", RateLimit: ptr(int32(-1)),
		}, "INVALID_RATE_LIMIT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.CreateDispatchPool(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
}

// Conflict is pinned by seeding through the operation itself: the first
// create IS the seed for the second (both anchor-scoped: nil ClientID).
func TestCreateDispatchPool_DuplicateCode_Conflict(t *testing.T) {
	t.Parallel()
	repo := dispatchpool.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	mustCreate(t, repo, uow, "dpdup-pool", "First")

	_, err := runAuthorized(uow, operations.CreateDispatchPool(repo),
		operations.CreateCommand{Code: "dpdup-pool", Name: "Second"})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "CODE_EXISTS")
}

// TestCreateDispatchPool_ResourceScope proves the use case's per-resource
// authorization: the coarse "may write dispatch pools" permission is the
// controller's job, but the use case enforces that you can only bind a pool to
// a client you can access (and that platform-wide pools require anchor). A
// client-scoped principal is denied a platform-wide and an other-client pool,
// but allowed one for its own client.
func TestCreateDispatchPool_ResourceScope(t *testing.T) {
	t.Parallel()
	repo := dispatchpool.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	ownClient := "cli_dpscope_own"
	clientCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_dpscope1",
		Scope:       auth.ScopeClient,
		Clients:     []string{ownClient},
		Permissions: []string{"platform:messaging:dispatch-pool:create"},
	})

	// Platform-wide (nil ClientID) → cross-client → anchor required → denied.
	_, err := usecaseop.Run(clientCtx, uow, operations.CreateDispatchPool(repo),
		operations.CreateCommand{Code: "dpscope-platform", Name: "X"}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	// Bound to a client the principal cannot access → denied.
	other := "cli_dpscope_other"
	_, err = usecaseop.Run(clientCtx, uow, operations.CreateDispatchPool(repo),
		operations.CreateCommand{Code: "dpscope-other", Name: "X", ClientID: &other}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	// Bound to the principal's own client → allowed.
	ev, err := usecaseop.Run(clientCtx, uow, operations.CreateDispatchPool(repo),
		operations.CreateCommand{Code: "dpscope-own", Name: "Mine", ClientID: &ownClient}, testpg.TestEC())
	require.NoError(t, err)
	assert.Equal(t, "dpscope-own", ev.Code)
}

// ── Update ────────────────────────────────────────────────────────────────

func TestUpdateDispatchPool_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := dispatchpool.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "dpupd-happy", "Before")

	desc := "after"
	ev, err := runAuthorized(uow, operations.UpdateDispatchPool(repo), operations.UpdateCommand{
		ID:          seeded.PoolID,
		Name:        ptr("  After  "), // op must trim
		Description: &desc,
		RateLimit:   ptr(int32(60)),
		Concurrency: ptr(int32(4)),
	})
	require.NoError(t, err)
	assert.Equal(t, seeded.PoolID, ev.PoolID)
	assert.Equal(t, "After", ev.Name)

	got, err := repo.FindByID(ctx, seeded.PoolID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "After", got.Name)
	require.NotNil(t, got.Description)
	assert.Equal(t, "after", *got.Description)
	require.NotNil(t, got.RateLimit)
	assert.Equal(t, int32(60), *got.RateLimit)
	assert.Equal(t, int32(4), got.Concurrency)
	assert.Equal(t, "dpupd-happy", got.Code, "code is immutable on update")
}

func TestUpdateDispatchPool_Errors(t *testing.T) {
	t.Parallel()
	repo := dispatchpool.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.UpdateCommand
		kind usecase.Kind
		code string
	}{
		{"missing id", operations.UpdateCommand{Name: ptr("X")}, usecase.KindValidation, "ID_REQUIRED"},
		{"blank name", operations.UpdateCommand{ID: "dpl_doesnotexist1", Name: ptr(" ")}, usecase.KindValidation, "NAME_REQUIRED"},
		{"zero concurrency", operations.UpdateCommand{
			ID: "dpl_doesnotexist1", Concurrency: ptr(int32(0)),
		}, usecase.KindValidation, "INVALID_CONCURRENCY"},
		{"negative rate limit", operations.UpdateCommand{
			ID: "dpl_doesnotexist1", RateLimit: ptr(int32(-1)),
		}, usecase.KindValidation, "INVALID_RATE_LIMIT"},
		{"unknown id", operations.UpdateCommand{ID: "dpl_doesnotexist1", Name: ptr("X")}, usecase.KindNotFound, "DispatchPool_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.UpdateDispatchPool(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

// ── Delete ────────────────────────────────────────────────────────────────

func TestDeleteDispatchPool_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := dispatchpool.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "dpdel-happy", "Doomed")

	ev, err := runAuthorized(uow, operations.DeleteDispatchPool(repo),
		operations.DeleteCommand{ID: seeded.PoolID})
	require.NoError(t, err)
	assert.Equal(t, seeded.PoolID, ev.PoolID)
	assert.Equal(t, "dpdel-happy", ev.Code)

	got, err := repo.FindByID(ctx, seeded.PoolID)
	require.NoError(t, err)
	assert.Nil(t, got, "deleted row must be gone")
}

// ── Archive ───────────────────────────────────────────────────────────────

func TestArchiveDispatchPool_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := dispatchpool.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "dparc-happy", "Archive Me")

	ev, err := runAuthorized(uow, operations.ArchiveDispatchPool(repo),
		operations.ArchiveCommand{ID: seeded.PoolID})
	require.NoError(t, err)
	assert.Equal(t, seeded.PoolID, ev.PoolID)
	assert.Equal(t, "dparc-happy", ev.Code)

	got, err := repo.FindByID(ctx, seeded.PoolID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, dispatchpool.StatusArchived, got.Status, "archive must flip ACTIVE → ARCHIVED")
}

// ── Suspend / Activate (status flips) ─────────────────────────────────────

func TestSuspendAndActivateDispatchPool_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := dispatchpool.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "dpsts-happy", "Flip Me")

	suspended, err := runAuthorized(uow, operations.SuspendDispatchPool(repo),
		operations.SuspendCommand{ID: seeded.PoolID})
	require.NoError(t, err)
	assert.Equal(t, seeded.PoolID, suspended.PoolID)
	assert.Equal(t, "dpsts-happy", suspended.Code)

	got, err := repo.FindByID(ctx, seeded.PoolID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, dispatchpool.StatusSuspended, got.Status, "suspend must flip ACTIVE → SUSPENDED")

	activated, err := runAuthorized(uow, operations.ActivateDispatchPool(repo),
		operations.ActivateCommand{ID: seeded.PoolID})
	require.NoError(t, err)
	assert.Equal(t, seeded.PoolID, activated.PoolID)

	got, err = repo.FindByID(ctx, seeded.PoolID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, dispatchpool.StatusActive, got.Status, "activate must flip SUSPENDED → ACTIVE")
}

// All four ID-only ops share identical guard rails — fold them into one
// table (the connection pause/activate pattern, just denser).
func TestDispatchPoolIDOps_Errors(t *testing.T) {
	t.Parallel()
	repo := dispatchpool.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	ops := []struct {
		name string
		call func(id string) error
	}{
		{"delete", func(id string) error {
			_, err := runAuthorized(uow, operations.DeleteDispatchPool(repo), operations.DeleteCommand{ID: id})
			return err
		}},
		{"archive", func(id string) error {
			_, err := runAuthorized(uow, operations.ArchiveDispatchPool(repo), operations.ArchiveCommand{ID: id})
			return err
		}},
		{"suspend", func(id string) error {
			_, err := runAuthorized(uow, operations.SuspendDispatchPool(repo), operations.SuspendCommand{ID: id})
			return err
		}},
		{"activate", func(id string) error {
			_, err := runAuthorized(uow, operations.ActivateDispatchPool(repo), operations.ActivateCommand{ID: id})
			return err
		}},
	}
	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()
			testpg.RequireUsecaseError(t, op.call(""), usecase.KindValidation, "ID_REQUIRED")
			testpg.RequireUsecaseError(t, op.call("dpl_doesnotexist1"), usecase.KindNotFound, "DispatchPool_NOT_FOUND")
		})
	}
}

// ── Sync (GLOBAL matching; upsert; RemoveUnlisted archives) ───────────────

// Plain upsert (no RemoveUnlisted) is safe to run in parallel: it only
// touches pools whose codes it lists, and those codes are unique to this
// test.
func TestSyncDispatchPools_Upsert(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := dispatchpool.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	// rateLimit 1 pins sync's lower bound: ≥ 1 when set (create's is ≥ 0).
	first, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncDispatchPools(repo), operations.SyncDispatchPoolsCommand{
		ApplicationCode: "dpsyncapp",
		Pools: []operations.SyncDispatchPoolInput{
			{Code: "dpsynup-one", Name: "A", Concurrency: 5},
			{Code: "dpsynup-two", Name: "B", Concurrency: 1, RateLimit: ptr(int32(1))},
		},
	}, ec)
	require.NoError(t, err)
	assert.Equal(t, uint32(2), first.Created)
	assert.Equal(t, uint32(0), first.Updated)
	assert.Equal(t, uint32(0), first.Deleted)
	assert.Equal(t, []string{"dpsynup-one", "dpsynup-two"}, first.SyncedCodes)

	second, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncDispatchPools(repo), operations.SyncDispatchPoolsCommand{
		ApplicationCode: "dpsyncapp",
		Pools: []operations.SyncDispatchPoolInput{
			{Code: "dpsynup-one", Name: "A renamed", Concurrency: 7, RateLimit: ptr(int32(60))},
		},
	}, ec)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), second.Created)
	assert.Equal(t, uint32(1), second.Updated)
	assert.Equal(t, uint32(0), second.Deleted, "no RemoveUnlisted → nothing archived")

	one, err := repo.FindByCode(ctx, "dpsynup-one", nil)
	require.NoError(t, err)
	require.NotNil(t, one)
	assert.Equal(t, "A renamed", one.Name)
	assert.Equal(t, int32(7), one.Concurrency)
	require.NotNil(t, one.RateLimit)
	assert.Equal(t, int32(60), *one.RateLimit)

	two, err := repo.FindByCode(ctx, "dpsynup-two", nil)
	require.NoError(t, err)
	require.NotNil(t, two)
	assert.Equal(t, dispatchpool.StatusActive, two.Status, "unlisted pool untouched without RemoveUnlisted")
}

// HAZARD — deliberately NOT parallel: sync matches pools GLOBALLY (not
// app-scoped) and RemoveUnlisted archives EVERY non-listed, non-archived
// pool in the database. Running serially means it completes before any
// paused t.Parallel() bodies create their pools; it still only asserts on
// pools it created itself, never table-wide.
func TestSyncDispatchPools_RemoveUnlisted_Archives(t *testing.T) {
	ctx := context.Background()
	repo := dispatchpool.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	_, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncDispatchPools(repo), operations.SyncDispatchPoolsCommand{
		ApplicationCode: "dpsyncrm",
		Pools: []operations.SyncDispatchPoolInput{
			{Code: "dpsyncrm-keep", Name: "Keep", Concurrency: 2},
			{Code: "dpsyncrm-drop", Name: "Drop", Concurrency: 2},
		},
	}, ec)
	require.NoError(t, err)

	second, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncDispatchPools(repo), operations.SyncDispatchPoolsCommand{
		ApplicationCode: "dpsyncrm",
		Pools: []operations.SyncDispatchPoolInput{
			{Code: "dpsyncrm-keep", Name: "Keep renamed", Concurrency: 3},
		},
		RemoveUnlisted: true,
	}, ec)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), second.Created)
	assert.Equal(t, uint32(1), second.Updated)
	// Removal is global, so other tests' rows could inflate the count —
	// assert only the lower bound our own dropped pool guarantees.
	assert.GreaterOrEqual(t, second.Deleted, uint32(1))

	kept, err := repo.FindByCode(ctx, "dpsyncrm-keep", nil)
	require.NoError(t, err)
	require.NotNil(t, kept)
	assert.Equal(t, "Keep renamed", kept.Name)
	assert.Equal(t, dispatchpool.StatusActive, kept.Status)

	dropped, err := repo.FindByCode(ctx, "dpsyncrm-drop", nil)
	require.NoError(t, err)
	require.NotNil(t, dropped, "RemoveUnlisted archives, never hard-deletes")
	assert.Equal(t, dispatchpool.StatusArchived, dropped.Status, "unlisted pool must be ARCHIVED")
}

func TestSyncDispatchPools_Validation(t *testing.T) {
	t.Parallel()
	repo := dispatchpool.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.SyncDispatchPoolsCommand
		code string
	}{
		{"missing application code", operations.SyncDispatchPoolsCommand{}, "APPLICATION_CODE_REQUIRED"},
		// Sync does NOT lowercase codes (deliberate Rust parity — create
		// DOES): the uppercase letters fail the pattern outright, proving
		// no normalization happens before validation.
		{"uppercase code rejected", operations.SyncDispatchPoolsCommand{
			ApplicationCode: "dpsyncval",
			Pools:           []operations.SyncDispatchPoolInput{{Code: "DPSync-Mixed", Name: "X", Concurrency: 1}},
		}, "INVALID_POOL_CODE"},
		{"code starts with digit", operations.SyncDispatchPoolsCommand{
			ApplicationCode: "dpsyncval",
			Pools:           []operations.SyncDispatchPoolInput{{Code: "1bad", Name: "X", Concurrency: 1}},
		}, "INVALID_POOL_CODE"},
		{"missing name", operations.SyncDispatchPoolsCommand{
			ApplicationCode: "dpsyncval",
			Pools:           []operations.SyncDispatchPoolInput{{Code: "dpsyncval-noname", Concurrency: 1}},
		}, "NAME_REQUIRED"},
		// Sync's rateLimit bound is ≥ 1 when set — rateLimit 0 is an error
		// here but VALID at create (pinned in TestCreateDispatchPool_HappyPath).
		{"zero rate limit", operations.SyncDispatchPoolsCommand{
			ApplicationCode: "dpsyncval",
			Pools: []operations.SyncDispatchPoolInput{
				{Code: "dpsyncval-rate", Name: "X", Concurrency: 1, RateLimit: ptr(int32(0))},
			},
		}, "INVALID_RATE_LIMIT"},
		{"zero concurrency", operations.SyncDispatchPoolsCommand{
			ApplicationCode: "dpsyncval",
			Pools:           []operations.SyncDispatchPoolInput{{Code: "dpsyncval-conc", Name: "X"}},
		}, "INVALID_CONCURRENCY"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncDispatchPools(repo), tc.cmd, testpg.TestEC())
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
}
