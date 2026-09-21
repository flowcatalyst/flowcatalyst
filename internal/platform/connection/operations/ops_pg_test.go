//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// runAuthorized drives op through the full use-case envelope (Validate →
// Authorize → Execute → atomic commit) as an anchor principal — the common
// case for these tests, which exercise validation, invariants, and
// persistence rather than authorization itself (see
// TestConnectionWrites_RequireAnchor for that). It mirrors how the HTTP
// handler runs the operation.
func runAuthorized[C any, E usecase.DomainEvent](
	uow *usecasepgx.UnitOfWork, op usecaseop.Operation[C, E], cmd C,
) (E, error) {
	return usecaseop.Run(testpg.AnchorCtx(), uow, op, cmd, testpg.TestEC())
}

// mustCreate seeds a connection through the public operation — the same path
// production uses. Codes are hand-unique per test: the fixture never
// truncates between tests, so tests own their rows and never assert
// table-wide. ServiceAccountID is not validated yet (TODO wave-3c), so an
// arbitrary id string suffices. Takes the pool (not a repo) since it needs
// both a connection.Repository and an application.Repository, and
// application.Repository has no meaningful state beyond wrapping the pool.
func mustCreate(t *testing.T, pool *pgxpool.Pool, uow *usecasepgx.UnitOfWork, code, name string) operations.ConnectionCreated {
	t.Helper()
	repo := connection.NewRepository(pool)
	apps := application.NewRepository(pool)
	ev, err := runAuthorized(uow, operations.CreateConnection(repo, apps),
		operations.CreateCommand{Code: code, Name: name, ServiceAccountID: "sva_conntestseed1"})
	require.NoError(t, err)
	return ev
}

// mustCreateApplication seeds an application through its own public
// operation (mirrors principal/operations/ops_pg_test.go's helper of the
// same shape) so CreateConnection's applicationCode-existence check has a
// real row to find. Returns the full event (id + code): tests need the id
// for CanAccessApplication contexts and the code for ApplicationCode fields.
func mustCreateApplication(t *testing.T, uow *usecasepgx.UnitOfWork, code, name string) appops.ApplicationCreated {
	t.Helper()
	ev, err := runAuthorized(uow, appops.CreateApplication(application.NewRepository(testpg.Pool(t))),
		appops.CreateCommand{Code: code, Name: name})
	require.NoError(t, err)
	return ev
}

// appAccessCtx is an all-applications anchor principal. CreateConnection,
// UpdateConnection, and SyncConnections authorize an application link via
// CanAccessApplication — a bare AnchorCtx sets Scope=Anchor but NOT
// AllApplications, so it would be denied. Tests that link a connection to an
// application run under this principal so they can reach any application.
func appAccessCtx() context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_optestrunner1", Scope: auth.ScopeAnchor, AllApplications: true,
	})
}

// ── Create ────────────────────────────────────────────────────────────────

func TestCreateConnection_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := connection.NewRepository(testpg.Pool(t))
	apps := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	desc := "outbound webhook target"
	external := "ext-conncrt-1"
	ev, err := runAuthorized(uow, operations.CreateConnection(repo, apps), operations.CreateCommand{
		Code:             "  CONNCRT-Happy  ", // op must trim + lowercase
		Name:             "  Conn Create Happy  ",
		Description:      &desc,
		ServiceAccountID: "sva_conncrthappy1",
		ExternalID:       &external,
	})
	require.NoError(t, err)

	assert.NotEmpty(t, ev.ConnectionID)
	assert.Equal(t, "conncrt-happy", ev.Code, "code must be trimmed + lowercased")
	assert.Equal(t, "Conn Create Happy", ev.Name, "name must be trimmed")

	got, err := repo.FindByID(ctx, ev.ConnectionID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "conncrt-happy", got.Code)
	assert.Equal(t, "Conn Create Happy", got.Name)
	assert.Equal(t, connection.StatusActive, got.Status, "new connections start ACTIVE")
	assert.Equal(t, "sva_conncrthappy1", got.ServiceAccountID)
	require.NotNil(t, got.Description)
	assert.Equal(t, desc, *got.Description)
	require.NotNil(t, got.ExternalID)
	assert.Equal(t, external, *got.ExternalID)
	assert.Nil(t, got.ClientID)
}

func TestCreateConnection_Validation(t *testing.T) {
	t.Parallel()
	repo := connection.NewRepository(testpg.Pool(t))
	apps := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.CreateCommand
		code string
	}{
		{"empty code", operations.CreateCommand{Name: "X", ServiceAccountID: "sva_x"}, "CODE_REQUIRED"},
		{"code starts with digit", operations.CreateCommand{
			Code: "1conncrt-bad", Name: "X", ServiceAccountID: "sva_x",
		}, "INVALID_CODE_FORMAT"},
		{"code with underscore", operations.CreateCommand{
			Code: "conncrt_bad", Name: "X", ServiceAccountID: "sva_x",
		}, "INVALID_CODE_FORMAT"},
		{"empty name", operations.CreateCommand{
			Code: "conncrt-noname", ServiceAccountID: "sva_x",
		}, "NAME_REQUIRED"},
		{"missing service account", operations.CreateCommand{
			Code: "conncrt-nosa", Name: "X",
		}, "SERVICE_ACCOUNT_REQUIRED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.CreateConnection(repo, apps), tc.cmd)
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
}

// Conflict is pinned by seeding through the operation itself: the first
// create IS the seed for the second (both anchor-scoped: nil ClientID).
func TestCreateConnection_DuplicateCode_Conflict(t *testing.T) {
	t.Parallel()
	repo := connection.NewRepository(testpg.Pool(t))
	apps := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	mustCreate(t, testpg.Pool(t), uow, "conndup", "First")

	_, err := runAuthorized(uow, operations.CreateConnection(repo, apps),
		operations.CreateCommand{Code: "conndup", Name: "Second", ServiceAccountID: "sva_conndup2"})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "CODE_EXISTS")
}

// TestCreateConnection_ResourceScope proves the use case's per-resource
// authorization: the coarse "may create connections" permission is the
// controller's job, but the use case enforces that you can only bind a
// connection to a client you can access (and that platform-wide connections
// require anchor). A client-scoped principal is denied a platform-wide and an
// other-client connection, but allowed one for its own client.
func TestCreateConnection_ResourceScope(t *testing.T) {
	t.Parallel()
	repo := connection.NewRepository(testpg.Pool(t))
	apps := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	ownClient := "cli_connscope_own"
	clientCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_connscope1",
		Scope:       auth.ScopeClient,
		Clients:     []string{ownClient},
		Permissions: []string{"platform:messaging:connection:create"},
	})

	// Platform-wide (nil ClientID) → cross-client → anchor required → denied.
	_, err := usecaseop.Run(clientCtx, uow, operations.CreateConnection(repo, apps),
		operations.CreateCommand{Code: "connscope-platform", Name: "X", ServiceAccountID: "sva_x"}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	// Bound to a client the principal cannot access → denied.
	other := "cli_connscope_other"
	_, err = usecaseop.Run(clientCtx, uow, operations.CreateConnection(repo, apps),
		operations.CreateCommand{Code: "connscope-other", Name: "X", ServiceAccountID: "sva_x", ClientID: &other}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	// Bound to the principal's own client → allowed.
	ev, err := usecaseop.Run(clientCtx, uow, operations.CreateConnection(repo, apps),
		operations.CreateCommand{Code: "connscope-own", Name: "Mine", ServiceAccountID: "sva_x", ClientID: &ownClient}, testpg.TestEC())
	require.NoError(t, err)
	assert.Equal(t, "connscope-own", ev.Code)
}

// TestCreateConnection_ApplicationScopedUniqueness pins the new
// (application_code, client_id, code) key end to end through the use case:
// the same code is free to reuse under a different application, but a
// second create for the same (application, client, code) triple is the
// existing CODE_EXISTS conflict — application_code just widens the key the
// conflict check reads, it isn't a new error shape.
func TestCreateConnection_ApplicationScopedUniqueness(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := connection.NewRepository(pool)
	apps := application.NewRepository(pool)
	uow := testpg.NewUoW(t)

	appA := mustCreateApplication(t, uow, "connappscope-a", "App A")
	appB := mustCreateApplication(t, uow, "connappscope-b", "App B")

	// Linking a connection to an application requires access to it
	// (CanAccessApplication) — run as an all-applications principal.
	create := func(cmd operations.CreateCommand) (operations.ConnectionCreated, error) {
		return usecaseop.Run(appAccessCtx(), uow, operations.CreateConnection(repo, apps), cmd, testpg.TestEC())
	}

	// Same code, two different applications → both succeed.
	_, err := create(operations.CreateCommand{Code: "connappscope-shared", Name: "A", ServiceAccountID: "sva_x", ApplicationCode: &appA.Code})
	require.NoError(t, err)
	_, err = create(operations.CreateCommand{Code: "connappscope-shared", Name: "B", ServiceAccountID: "sva_x", ApplicationCode: &appB.Code})
	require.NoError(t, err, "the same code under a different application must not conflict")

	// Same code, same application, same (nil) client → rejected.
	_, err = create(operations.CreateCommand{Code: "connappscope-shared", Name: "A again", ServiceAccountID: "sva_x", ApplicationCode: &appA.Code})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "CODE_EXISTS")

	// An applicationCode that names no application is rejected up front.
	noSuch := "connappscope-no-such-app"
	_, err = create(operations.CreateCommand{Code: "connappscope-newcode", Name: "X", ServiceAccountID: "sva_x", ApplicationCode: &noSuch})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Application_NOT_FOUND")
}

// TestCreateConnection_ApplicationCode_RequiresApplicationAccess proves the
// use case's application-axis authorization: linking a connection to an
// application requires CanAccessApplication, checked independently of the
// client-scope check CheckScopeAccess already performed. A principal with
// full access to the target CLIENT but no access to the target APPLICATION
// (an application-scoped principal restricted to a different app) must
// still be denied.
func TestCreateConnection_ApplicationCode_RequiresApplicationAccess(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := connection.NewRepository(pool)
	apps := application.NewRepository(pool)
	uow := testpg.NewUoW(t)

	app := mustCreateApplication(t, uow, "connappaccess-app", "App")

	noAccessCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_connappaccess1", Scope: auth.ScopeAnchor, Applications: []string{"app_someotherone"},
	})
	_, err := usecaseop.Run(noAccessCtx, uow, operations.CreateConnection(repo, apps),
		operations.CreateCommand{Code: "connappaccess-x", Name: "X", ServiceAccountID: "sva_x", ApplicationCode: &app.Code},
		testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")

	// The same application, from a principal that can reach it, succeeds.
	_, err = usecaseop.Run(appAccessCtx(), uow, operations.CreateConnection(repo, apps),
		operations.CreateCommand{Code: "connappaccess-x", Name: "X", ServiceAccountID: "sva_x", ApplicationCode: &app.Code},
		testpg.TestEC())
	require.NoError(t, err)
}

// TestCreateConnection_DuplicateSharedNoClient_RejectedAtDatabase proves the
// actual bug fix in migration 056: two connections with the same code, the
// same (absent) application, and no client used to be allowed by the OLD
// idx_msg_connections_code_client index because Postgres treats NULLs as
// distinct in a plain unique index. It goes straight at the database with a
// second raw INSERT — bypassing the use case's find-then-insert check
// entirely — to prove the constraint itself, not the application-level
// pre-check, is what now refuses the second row.
func TestCreateConnection_DuplicateSharedNoClient_RejectedAtDatabase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)

	const code = "connappscope-db-dupe"
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_connections (id, code, name, status, service_account_id)
		 VALUES ('cnx_dbdupe0000001', $1, 'First', 'ACTIVE', 'sva_dbdupe1')`, code)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM msg_connections WHERE id = 'cnx_dbdupe0000001'`)
	})

	_, err = pool.Exec(ctx,
		`INSERT INTO msg_connections (id, code, name, status, service_account_id)
		 VALUES ('cnx_dbdupe0000002', $1, 'Second', 'ACTIVE', 'sva_dbdupe2')`, code)
	require.Error(t, err, "a second shared, clientless connection with the same code must be rejected by the unique index")
	assert.Contains(t, err.Error(), "uq_msg_connections_app_client_code")
}

// ── Update ────────────────────────────────────────────────────────────────

func TestUpdateConnection_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := connection.NewRepository(testpg.Pool(t))
	apps := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, testpg.Pool(t), uow, "connupd-happy", "Before")

	desc := "after"
	external := "ext-connupd-1"
	status := "PAUSED"
	ev, err := runAuthorized(uow, operations.UpdateConnection(repo, apps), operations.UpdateCommand{
		ID:          seeded.ConnectionID,
		Name:        "After",
		Description: &desc,
		ExternalID:  &external,
		Status:      &status,
	})
	require.NoError(t, err)
	assert.Equal(t, seeded.ConnectionID, ev.ConnectionID)
	assert.Equal(t, "After", ev.Name)

	got, err := repo.FindByID(ctx, seeded.ConnectionID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "After", got.Name)
	require.NotNil(t, got.Description)
	assert.Equal(t, "after", *got.Description)
	require.NotNil(t, got.ExternalID)
	assert.Equal(t, external, *got.ExternalID)
	assert.Equal(t, connection.StatusPaused, got.Status, "status flip via update must persist")
	assert.Equal(t, "connupd-happy", got.Code, "code is immutable on update")
}

// TestUpdateConnection_ApplicationCode proves the set-if-provided semantics
// and the re-pointing collision check: a nil applicationCode leaves the
// existing link untouched, a valid one can be set, an unknown one 404s, and
// re-pointing onto an application that already has this code+client
// combination is the same CODE_EXISTS conflict create uses.
func TestUpdateConnection_ApplicationCode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := connection.NewRepository(pool)
	apps := application.NewRepository(pool)
	uow := testpg.NewUoW(t)

	appA := mustCreateApplication(t, uow, "connupdapp-a", "App A")
	appB := mustCreateApplication(t, uow, "connupdapp-b", "App B")
	seeded := mustCreate(t, pool, uow, "connupdapp-target", "Target")

	// Linking a connection to an application requires access to it
	// (CanAccessApplication) — run as an all-applications principal.
	update := func(cmd operations.UpdateCommand) (operations.ConnectionUpdated, error) {
		return usecaseop.Run(appAccessCtx(), uow, operations.UpdateConnection(repo, apps), cmd, testpg.TestEC())
	}

	// Omitted applicationCode leaves it unset.
	_, err := update(operations.UpdateCommand{ID: seeded.ConnectionID, Name: "Target"})
	require.NoError(t, err)
	got, err := repo.FindByID(ctx, seeded.ConnectionID)
	require.NoError(t, err)
	assert.Nil(t, got.ApplicationCode)

	// Set to a real application.
	_, err = update(operations.UpdateCommand{ID: seeded.ConnectionID, Name: "Target", ApplicationCode: &appA.Code})
	require.NoError(t, err)
	got, err = repo.FindByID(ctx, seeded.ConnectionID)
	require.NoError(t, err)
	require.NotNil(t, got.ApplicationCode)
	assert.Equal(t, appA.Code, *got.ApplicationCode)

	// Unknown application code 404s and leaves the row untouched.
	noSuch := "connupdapp-no-such"
	_, err = update(operations.UpdateCommand{ID: seeded.ConnectionID, Name: "Target", ApplicationCode: &noSuch})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Application_NOT_FOUND")

	// Re-pointing onto an application that already owns this exact
	// (application, client, code) triple conflicts.
	other := mustCreate(t, pool, uow, "connupdapp-target", "Other")
	_, err = update(operations.UpdateCommand{ID: other.ConnectionID, Name: "Other", ApplicationCode: &appA.Code})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "CODE_EXISTS")

	// Re-pointing onto a DIFFERENT application that has no such row succeeds.
	_, err = update(operations.UpdateCommand{ID: other.ConnectionID, Name: "Other", ApplicationCode: &appB.Code})
	require.NoError(t, err)
}

// TestUpdateConnection_ApplicationCode_RequiresApplicationAccess mirrors
// TestCreateConnection_ApplicationCode_RequiresApplicationAccess for update:
// re-pointing a connection onto an application the caller cannot access is
// denied, independent of the client-scope check.
func TestUpdateConnection_ApplicationCode_RequiresApplicationAccess(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := connection.NewRepository(pool)
	apps := application.NewRepository(pool)
	uow := testpg.NewUoW(t)

	app := mustCreateApplication(t, uow, "connupdappaccess-app", "App")
	seeded := mustCreate(t, pool, uow, "connupdappaccess-target", "Target")

	noAccessCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_connupdappaccess1", Scope: auth.ScopeAnchor, Applications: []string{"app_someotherone"},
	})
	_, err := usecaseop.Run(noAccessCtx, uow, operations.UpdateConnection(repo, apps),
		operations.UpdateCommand{ID: seeded.ConnectionID, Name: "Target", ApplicationCode: &app.Code},
		testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
}

func TestUpdateConnection_Errors(t *testing.T) {
	t.Parallel()
	repo := connection.NewRepository(testpg.Pool(t))
	apps := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.UpdateCommand
		kind usecase.Kind
		code string
	}{
		{"missing id", operations.UpdateCommand{Name: "X"}, usecase.KindValidation, "ID_REQUIRED"},
		{"missing name", operations.UpdateCommand{ID: "con_doesnotexist1", Name: " "}, usecase.KindValidation, "NAME_REQUIRED"},
		{"unknown id", operations.UpdateCommand{ID: "con_doesnotexist1", Name: "X"}, usecase.KindNotFound, "Connection_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.UpdateConnection(repo, apps), tc.cmd)
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

// ── Pause / Activate (status flips) ───────────────────────────────────────

func TestPauseAndActivateConnection_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := connection.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, testpg.Pool(t), uow, "connsts-happy", "Flip Me")

	paused, err := runAuthorized(uow, operations.PauseConnection(repo),
		operations.PauseCommand{ID: seeded.ConnectionID})
	require.NoError(t, err)
	assert.Equal(t, seeded.ConnectionID, paused.ConnectionID)
	assert.Equal(t, "Flip Me", paused.Name)

	got, err := repo.FindByID(ctx, seeded.ConnectionID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, connection.StatusPaused, got.Status, "pause must flip ACTIVE → PAUSED")

	activated, err := runAuthorized(uow, operations.ActivateConnection(repo),
		operations.ActivateCommand{ID: seeded.ConnectionID})
	require.NoError(t, err)
	assert.Equal(t, seeded.ConnectionID, activated.ConnectionID)

	got, err = repo.FindByID(ctx, seeded.ConnectionID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, connection.StatusActive, got.Status, "activate must flip PAUSED → ACTIVE")
}

func TestPauseConnection_Errors(t *testing.T) {
	t.Parallel()
	repo := connection.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.PauseConnection(repo), operations.PauseCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.PauseConnection(repo),
		operations.PauseCommand{ID: "con_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Connection_NOT_FOUND")
}

func TestActivateConnection_Errors(t *testing.T) {
	t.Parallel()
	repo := connection.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.ActivateConnection(repo), operations.ActivateCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.ActivateConnection(repo),
		operations.ActivateCommand{ID: "con_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Connection_NOT_FOUND")
}

// ── Delete ────────────────────────────────────────────────────────────────

func TestDeleteConnection_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := connection.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, testpg.Pool(t), uow, "conndel-happy", "Doomed")

	ev, err := runAuthorized(uow, operations.DeleteConnection(repo),
		operations.DeleteCommand{ID: seeded.ConnectionID})
	require.NoError(t, err)
	assert.Equal(t, seeded.ConnectionID, ev.ConnectionID)
	assert.Equal(t, "conndel-happy", ev.Code)

	got, err := repo.FindByID(ctx, seeded.ConnectionID)
	require.NoError(t, err)
	assert.Nil(t, got, "deleted row must be gone")
}

func TestDeleteConnection_Errors(t *testing.T) {
	t.Parallel()
	repo := connection.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.DeleteConnection(repo), operations.DeleteCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.DeleteConnection(repo),
		operations.DeleteCommand{ID: "con_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Connection_NOT_FOUND")
}
