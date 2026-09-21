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
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	subscriptionops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// mustCreateApplicationWithServiceAccount seeds an application through the
// public operation, then attaches a service account to it. Unlike
// msg_connections.service_account_id (still unvalidated — see
// CreateConnection's TODO(wave-3c)), app_applications.service_account_id IS
// foreign-keyed to iam_principals (migration 028), so the id must name a
// real row — a bare raw-SQL principal insert is enough; SyncConnections
// never reads anything off it beyond the id. serviceAccountID must be
// <=17 chars (iam_principals.id / app_applications.service_account_id are
// both VARCHAR(17)).
func mustCreateApplicationWithServiceAccount(
	t *testing.T, pool *pgxpool.Pool, uow *usecasepgx.UnitOfWork, code, name, serviceAccountID string,
) appops.ApplicationCreated {
	t.Helper()
	ev := mustCreateApplication(t, uow, code, name)
	ctx := context.Background()
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, name, active) VALUES ($1, 'SERVICE', $2, TRUE)
		 ON CONFLICT (id) DO NOTHING`,
		serviceAccountID, name+" SA")
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`UPDATE app_applications SET service_account_id = $1 WHERE id = $2`, serviceAccountID, ev.ApplicationID)
	require.NoError(t, err)
	return ev
}

// insertRawConnection seeds a msg_connections row directly, bypassing every
// use case, so a test can pin an exact (application_code, client_id, code,
// source) combination — including source=API rows outside the scope under
// test, which no public operation can produce (CreateConnection always
// stamps source=UI).
func insertRawConnection(t *testing.T, pool *pgxpool.Pool, id, code, name, serviceAccountID string, applicationCode, clientID *string, source string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_connections (id, code, name, status, service_account_id, application_code, client_id, source)
		 VALUES ($1, $2, $3, 'ACTIVE', $4, $5, $6, $7)`,
		id, code, name, serviceAccountID, applicationCode, clientID, source)
	require.NoError(t, err)
}

func runConnSync(
	t *testing.T, uow *usecasepgx.UnitOfWork,
	connRepo *connection.Repository, apps *application.Repository, subRepo *subscription.Repository,
	cmd operations.SyncConnectionsCommand,
) (operations.ConnectionsSynced, error) {
	t.Helper()
	return usecaseop.Run(appAccessCtx(), uow, operations.SyncConnections(connRepo, apps, subRepo), cmd, testpg.TestEC())
}

// ── Create / idempotent re-sync / update ──────────────────────────────────

func TestSyncConnections_CreateIdempotentUpdate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	connRepo := connection.NewRepository(pool)
	apps := application.NewRepository(pool)
	subRepo := subscription.NewRepository(pool)
	uow := testpg.NewUoW(t)

	app := mustCreateApplicationWithServiceAccount(t, pool, uow, "syncconnapp1", "Sync Conn App", "sva_syncconn1")

	desc := "first"
	first, err := runConnSync(t, uow, connRepo, apps, subRepo, operations.SyncConnectionsCommand{
		ApplicationID:   app.ApplicationID,
		ApplicationCode: app.Code,
		Connections: []operations.SyncConnectionEntry{
			{Code: "syncconn-a", Name: "A", Description: &desc},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, uint32(1), first.Created)
	assert.Equal(t, uint32(0), first.Updated)
	assert.Equal(t, []string{"syncconn-a"}, first.SyncedCodes)

	got, err := connRepo.FindByCode(ctx, "syncconn-a", &app.Code, nil)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "A", got.Name)
	require.NotNil(t, got.Description)
	assert.Equal(t, "first", *got.Description)
	assert.Equal(t, connection.SourceAPI, got.Source, "synced rows are API-sourced")
	assert.Equal(t, "sva_syncconn1", got.ServiceAccountID, "service account defaults from the application")
	assert.Equal(t, connection.StatusActive, got.Status)

	// Re-syncing the exact same payload must not create a duplicate — it
	// upserts the same row (counted as an update, matching SyncSubscriptions'
	// unconditional-update behaviour for matched API rows).
	second, err := runConnSync(t, uow, connRepo, apps, subRepo, operations.SyncConnectionsCommand{
		ApplicationID:   app.ApplicationID,
		ApplicationCode: app.Code,
		Connections: []operations.SyncConnectionEntry{
			{Code: "syncconn-a", Name: "A", Description: &desc},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, uint32(0), second.Created)
	assert.Equal(t, uint32(1), second.Updated)

	// A changed name/description on a later sync actually persists.
	newDesc := "second"
	third, err := runConnSync(t, uow, connRepo, apps, subRepo, operations.SyncConnectionsCommand{
		ApplicationID:   app.ApplicationID,
		ApplicationCode: app.Code,
		Connections: []operations.SyncConnectionEntry{
			{Code: "syncconn-a", Name: "A renamed", Description: &newDesc},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, uint32(1), third.Updated)

	got, err = connRepo.FindByCode(ctx, "syncconn-a", &app.Code, nil)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "A renamed", got.Name)
	require.NotNil(t, got.Description)
	assert.Equal(t, "second", *got.Description)
}

// ── Service account defaulting ────────────────────────────────────────────

func TestSyncConnections_ServiceAccountAlwaysFollowsApplication(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	connRepo := connection.NewRepository(pool)
	apps := application.NewRepository(pool)
	subRepo := subscription.NewRepository(pool)
	uow := testpg.NewUoW(t)

	app := mustCreateApplicationWithServiceAccount(t, pool, uow, "syncconnsa1", "Sync Conn SA", "sva_scfoll-old")

	_, err := runConnSync(t, uow, connRepo, apps, subRepo, operations.SyncConnectionsCommand{
		ApplicationID: app.ApplicationID, ApplicationCode: app.Code,
		Connections: []operations.SyncConnectionEntry{{Code: "syncconnsa-x", Name: "X"}},
	})
	require.NoError(t, err)
	got, err := connRepo.FindByCode(ctx, "syncconnsa-x", &app.Code, nil)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "sva_scfoll-old", got.ServiceAccountID)

	// The application's service account is rotated between syncs; the next
	// sync re-points the connection to the NEW one — there is no
	// per-connection override to pin it to the old value.
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, name, active) VALUES ($1, 'SERVICE', 'New SA', TRUE)`,
		"sva_scfoll-new")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE app_applications SET service_account_id = $1 WHERE id = $2`,
		"sva_scfoll-new", app.ApplicationID)
	require.NoError(t, err)

	_, err = runConnSync(t, uow, connRepo, apps, subRepo, operations.SyncConnectionsCommand{
		ApplicationID: app.ApplicationID, ApplicationCode: app.Code,
		Connections: []operations.SyncConnectionEntry{{Code: "syncconnsa-x", Name: "X"}},
	})
	require.NoError(t, err)
	got, err = connRepo.FindByCode(ctx, "syncconnsa-x", &app.Code, nil)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "sva_scfoll-new", got.ServiceAccountID, "service account must follow the application, not stick to the value at creation")
}

func TestSyncConnections_NoServiceAccount_ValidationError(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	connRepo := connection.NewRepository(pool)
	apps := application.NewRepository(pool)
	subRepo := subscription.NewRepository(pool)
	uow := testpg.NewUoW(t)

	app := mustCreateApplication(t, uow, "syncconnnosa1", "No SA App") // no service account attached

	_, err := runConnSync(t, uow, connRepo, apps, subRepo, operations.SyncConnectionsCommand{
		ApplicationID: app.ApplicationID, ApplicationCode: app.Code,
		Connections: []operations.SyncConnectionEntry{{Code: "syncconnnosa-x", Name: "X"}},
	})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "APPLICATION_SERVICE_ACCOUNT_REQUIRED")
}

// ── UI rows are never touched ──────────────────────────────────────────────

func TestSyncConnections_LeavesUIRowUntouched(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	connRepo := connection.NewRepository(pool)
	apps := application.NewRepository(pool)
	subRepo := subscription.NewRepository(pool)
	uow := testpg.NewUoW(t)

	app := mustCreateApplicationWithServiceAccount(t, pool, uow, "syncconnui1", "Sync Conn UI", "sva_syncconnui1")
	insertRawConnection(t, pool, "cnx_syncconnui001", "syncconn-ui", "Original UI Name", "sva_uioriginal",
		&app.Code, nil, "UI")

	result, err := runConnSync(t, uow, connRepo, apps, subRepo, operations.SyncConnectionsCommand{
		ApplicationID: app.ApplicationID, ApplicationCode: app.Code,
		Connections: []operations.SyncConnectionEntry{{Code: "syncconn-ui", Name: "Attempted Overwrite"}},
	})
	require.NoError(t, err)
	assert.Equal(t, uint32(0), result.Created, "a UI row at the same key is neither created")
	assert.Equal(t, uint32(0), result.Updated, "nor counted as updated")

	got, err := connRepo.FindByCode(ctx, "syncconn-ui", &app.Code, nil)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Original UI Name", got.Name, "a UI-authored row must never be overwritten by a sync")
	assert.Equal(t, connection.SourceUI, got.Source)
	assert.Equal(t, "sva_uioriginal", got.ServiceAccountID)
}

// ── RemoveUnlisted scoping — the most dangerous behaviour in this feature ──

// TestSyncConnections_RemoveUnlisted_ScopedToApplicationAndClient seeds six
// pre-existing rows that all look like plausible deletion candidates —
// same code, various (application, client, source) combinations — and
// proves a sync targeting exactly one (application, client) pair with
// RemoveUnlisted removes ONLY the one row in that exact scope, never a
// sibling client's row, the application's shared (client-less) row, another
// application's row, a truly shared (application-less) connection, or a
// UI-authored row even inside the deletion scope.
func TestSyncConnections_RemoveUnlisted_ScopedToApplicationAndClient(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	connRepo := connection.NewRepository(pool)
	apps := application.NewRepository(pool)
	subRepo := subscription.NewRepository(pool)
	uow := testpg.NewUoW(t)

	app := mustCreateApplicationWithServiceAccount(t, pool, uow, "rmscope-app-main", "RM Scope Main", "sva_rmscope1")
	otherAppCode := "rmscope-app-other" // never created as a real row — application_code has no FK
	clientA := "cli_rmscope_a"
	clientB := "cli_rmscope_b"
	const sharedCode = "rmscope-code"

	// The one row that must be removed: (this app, client A), API-sourced,
	// not listed in the sync payload.
	insertRawConnection(t, pool, "cnx_rmscope000001", sharedCode, "Target", "sva_x", &app.Code, &clientA, "API")
	// A UI row at the SAME (app, client) scope, same code family but its own
	// code (can't collide on the unique key) — must survive even though it's
	// inside the exact scope RemoveUnlisted sweeps.
	insertRawConnection(t, pool, "cnx_rmscope000002", "rmscope-ui-kept", "UI Kept", "sva_x", &app.Code, &clientA, "UI")
	// Same app, DIFFERENT client — must survive.
	insertRawConnection(t, pool, "cnx_rmscope000003", sharedCode, "Client B row", "sva_x", &app.Code, &clientB, "API")
	// Same app, NO client (the application's shared partition) — must survive.
	insertRawConnection(t, pool, "cnx_rmscope000004", sharedCode, "No client row", "sva_x", &app.Code, nil, "API")
	// DIFFERENT application, same client — must survive.
	insertRawConnection(t, pool, "cnx_rmscope000005", sharedCode, "Other app row", "sva_x", &otherAppCode, &clientA, "API")
	// NO application at all (a truly shared connection), same client — must survive.
	insertRawConnection(t, pool, "cnx_rmscope000006", sharedCode, "Shared conn row", "sva_x", nil, &clientA, "API")

	result, err := runConnSync(t, uow, connRepo, apps, subRepo, operations.SyncConnectionsCommand{
		ApplicationID:   app.ApplicationID,
		ApplicationCode: app.Code,
		ClientID:        &clientA,
		Connections:     nil, // nothing listed: everything in scope is "unlisted"
		RemoveUnlisted:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, uint32(0), result.Created)
	assert.Equal(t, uint32(0), result.Updated)
	assert.Equal(t, uint32(1), result.Deleted, "exactly one row is in the deletion scope")

	gone, err := connRepo.FindByCode(ctx, sharedCode, &app.Code, &clientA)
	require.NoError(t, err)
	assert.Nil(t, gone, "the unlisted API row in the exact synced scope must be removed")

	uiKept, err := connRepo.FindByCode(ctx, "rmscope-ui-kept", &app.Code, &clientA)
	require.NoError(t, err)
	require.NotNil(t, uiKept, "a UI row in the same scope must survive")
	assert.Equal(t, connection.SourceUI, uiKept.Source)

	clientBRow, err := connRepo.FindByCode(ctx, sharedCode, &app.Code, &clientB)
	require.NoError(t, err)
	require.NotNil(t, clientBRow, "a sibling client's row must survive")

	noClientRow, err := connRepo.FindByCode(ctx, sharedCode, &app.Code, nil)
	require.NoError(t, err)
	require.NotNil(t, noClientRow, "the application's shared (client-less) row must survive")

	otherAppRow, err := connRepo.FindByCode(ctx, sharedCode, &otherAppCode, &clientA)
	require.NoError(t, err)
	require.NotNil(t, otherAppRow, "another application's row must survive")

	sharedConnRow, err := connRepo.FindByCode(ctx, sharedCode, nil, &clientA)
	require.NoError(t, err)
	require.NotNil(t, sharedConnRow, "a truly shared (application-less) connection must survive")
}

// ── Delete-while-referenced guard ─────────────────────────────────────────

func TestSyncConnections_RemoveUnlisted_RefusesWhenReferenced(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	connRepo := connection.NewRepository(pool)
	apps := application.NewRepository(pool)
	subRepo := subscription.NewRepository(pool)
	uow := testpg.NewUoW(t)

	app := mustCreateApplicationWithServiceAccount(t, pool, uow, "syncconnref1", "Sync Conn Ref", "sva_syncconnref1")

	// Create the connection through the sync itself, then a live subscription
	// that targets it.
	_, err := runConnSync(t, uow, connRepo, apps, subRepo, operations.SyncConnectionsCommand{
		ApplicationID: app.ApplicationID, ApplicationCode: app.Code,
		Connections: []operations.SyncConnectionEntry{{Code: "syncconnref-target", Name: "Target"}},
	})
	require.NoError(t, err)
	conn, err := connRepo.FindByCode(ctx, "syncconnref-target", &app.Code, nil)
	require.NoError(t, err)
	require.NotNil(t, conn)

	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, subscriptionops.CreateSubscription(subRepo),
		subscriptionops.CreateCommand{
			Code: "syncconnref-referrer", Name: "Referrer", Endpoint: "https://ref.example.test/hook",
			ConnectionID: &conn.ID,
			EventTypes:   []subscription.EventTypeBinding{subscription.NewEventTypeBinding("syncconnref:a:b:c")},
		}, testpg.TestEC())
	require.NoError(t, err)

	// A sync that omits the connection with RemoveUnlisted must refuse the
	// WHOLE sync rather than silently orphaning the subscription.
	_, err = runConnSync(t, uow, connRepo, apps, subRepo, operations.SyncConnectionsCommand{
		ApplicationID: app.ApplicationID, ApplicationCode: app.Code,
		Connections:    nil,
		RemoveUnlisted: true,
	})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "CONNECTION_REFERENCED")

	// Nothing was removed.
	stillThere, err := connRepo.FindByCode(ctx, "syncconnref-target", &app.Code, nil)
	require.NoError(t, err)
	require.NotNil(t, stillThere, "the referenced connection must survive a refused sync")
}

// ── Authorization ──────────────────────────────────────────────────────────

func TestSyncConnections_Authorization(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	connRepo := connection.NewRepository(pool)
	apps := application.NewRepository(pool)
	subRepo := subscription.NewRepository(pool)
	uow := testpg.NewUoW(t)

	app := mustCreateApplicationWithServiceAccount(t, pool, uow, "syncconnauth1", "Sync Conn Auth", "sva_syncconnauth1")

	run := func(ctx context.Context, cmd operations.SyncConnectionsCommand) (operations.ConnectionsSynced, error) {
		return usecaseop.Run(ctx, uow, operations.SyncConnections(connRepo, apps, subRepo), cmd, testpg.TestEC())
	}

	// No application access at all → forbidden.
	noAppAccessCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_syncconnauth_noapp", Scope: auth.ScopeClient, Applications: []string{"app_other"},
	})
	_, err := run(noAppAccessCtx, operations.SyncConnectionsCommand{
		ApplicationID: app.ApplicationID, ApplicationCode: app.Code,
		Connections: []operations.SyncConnectionEntry{{Code: "syncconnauth-x", Name: "X"}},
	})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")

	// Application access, but a client is named the caller cannot reach →
	// forbidden.
	otherClient := "cli_syncauth_oth"
	appOnlyCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_syncconnauth_apponly", Scope: auth.ScopeClient, Applications: []string{app.ApplicationID},
	})
	_, err = run(appOnlyCtx, operations.SyncConnectionsCommand{
		ApplicationID: app.ApplicationID, ApplicationCode: app.Code, ClientID: &otherClient,
		Connections: []operations.SyncConnectionEntry{{Code: "syncconnauth-x", Name: "X"}},
	})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")

	// Ruling (2026-09-21): a NON-anchor caller with application access and NO
	// client is allowed — a client-less connection sync needs no anchor tier,
	// unlike the scheduled-job sync (ownership, not reach, is the fence).
	_, err = run(appOnlyCtx, operations.SyncConnectionsCommand{
		ApplicationID: app.ApplicationID, ApplicationCode: app.Code,
		Connections: []operations.SyncConnectionEntry{{Code: "syncconnauth-allowed", Name: "X"}},
	})
	require.NoError(t, err)
}

// ── Validation ───────────────────────────────────────────────────────────

func TestSyncConnections_Validation(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	connRepo := connection.NewRepository(pool)
	apps := application.NewRepository(pool)
	subRepo := subscription.NewRepository(pool)
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.SyncConnectionsCommand
		code string
	}{
		{"missing application code", operations.SyncConnectionsCommand{}, "APPLICATION_CODE_REQUIRED"},
		{"entry missing code", operations.SyncConnectionsCommand{
			ApplicationCode: "syncconnvalbad",
			Connections:     []operations.SyncConnectionEntry{{Name: "X"}},
		}, "CODE_REQUIRED"},
		{"entry bad code format", operations.SyncConnectionsCommand{
			ApplicationCode: "syncconnvalbad",
			Connections:     []operations.SyncConnectionEntry{{Code: "Bad_Code", Name: "X"}},
		}, "INVALID_CODE_FORMAT"},
		{"entry missing name", operations.SyncConnectionsCommand{
			ApplicationCode: "syncconnvalbad",
			Connections:     []operations.SyncConnectionEntry{{Code: "syncconnval-noname"}},
		}, "NAME_REQUIRED"},
		{"duplicate code in request", operations.SyncConnectionsCommand{
			ApplicationCode: "syncconnvalbad",
			Connections: []operations.SyncConnectionEntry{
				{Code: "syncconnval-dup", Name: "First"},
				{Code: "syncconnval-dup", Name: "Second"},
			},
		}, "DUPLICATE_CODE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runConnSync(t, uow, connRepo, apps, subRepo, tc.cmd)
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
}
