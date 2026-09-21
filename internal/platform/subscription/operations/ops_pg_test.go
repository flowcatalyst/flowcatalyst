//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	connops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	poolops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
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
// TestCreateSubscription_ResourceScope for that). It mirrors how the HTTP
// handler runs the operation.
func runAuthorized[C any, E usecase.DomainEvent](
	uow *usecasepgx.UnitOfWork, op usecaseop.Operation[C, E], cmd C,
) (E, error) {
	return usecaseop.Run(testpg.AnchorCtx(), uow, op, cmd, testpg.TestEC())
}

// appAccessCtx is an all-applications anchor principal. SyncSubscriptions
// authorizes against the target application (CanAccessApplication) — a bare
// AnchorCtx sets Scope=Anchor but NOT AllApplications, so it would be denied.
// App-scoped sync tests run under this principal so they can reach any
// application.
func appAccessCtx() context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_optestrunner1", Scope: auth.ScopeAnchor, AllApplications: true,
	})
}

// mustCreate seeds a subscription through the public operation — the same
// path production uses. Codes are hand-unique per test: the fixture never
// truncates between tests, so tests own their rows and never assert
// table-wide. Event-type binding codes are free-form patterns (not FK'd),
// so no event type needs to exist.
func mustCreate(t *testing.T, repo *subscription.Repository, uow *usecasepgx.UnitOfWork, code, name string) operations.SubscriptionCreated {
	t.Helper()
	ev, err := runAuthorized(uow, operations.CreateSubscription(repo),
		operations.CreateCommand{
			Code:     code,
			Name:     name,
			Endpoint: "https://seed.example.test/" + code,
			EventTypes: []subscription.EventTypeBinding{
				subscription.NewEventTypeBinding("subtest:orders:order:created"),
			},
		})
	require.NoError(t, err)
	return ev
}

// insertRawConnection seeds a msg_connections row directly, bypassing every
// use case, so a test can pin an exact (application_code, client_id, code)
// combination for the sync's connection lookup — application_code has no FK,
// and service_account_id is unvalidated (CreateConnection's TODO(wave-3c)),
// so neither needs a real backing row. Mirrors
// connection/operations.insertRawConnection (a different package instance;
// not importable across packages).
func insertRawConnection(t *testing.T, pool *pgxpool.Pool, id, code, serviceAccountID string, applicationCode, clientID *string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_connections (id, code, name, status, service_account_id, application_code, client_id, source)
		 VALUES ($1, $2, $3, 'ACTIVE', $4, $5, $6, 'API')`,
		id, code, code, serviceAccountID, applicationCode, clientID)
	require.NoError(t, err)
}

// insertRawSubscription seeds a msg_subscriptions row directly, bypassing
// every use case, so a test can pin an exact (application_code, client_id,
// code, source) combination — including source=API rows outside the scope
// under test, which no public operation can produce (CreateSubscription
// always stamps source=UI and never sets application_code).
func insertRawSubscription(t *testing.T, pool *pgxpool.Pool, id, code, name string, applicationCode, clientID *string, source string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_subscriptions (id, code, application_code, name, client_id, target, source)
		 VALUES ($1, $2, $3, $4, $5, 'https://raw.example.test/hook', $6)`,
		id, code, applicationCode, name, clientID, source)
	require.NoError(t, err)
}

// ── Create ────────────────────────────────────────────────────────────────

func TestCreateSubscription_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := subscription.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	desc := "delivers order events"
	ev, err := runAuthorized(uow, operations.CreateSubscription(repo), operations.CreateCommand{
		Code:             "  SUBCRT-Happy  ", // op must trim + lowercase
		Name:             "  Sub Create Happy  ",
		Endpoint:         "https://orders.example.test/hook",
		Description:      &desc,
		ServiceAccountID: new("sva_subcrthappy1"),
		EventTypes: []subscription.EventTypeBinding{
			{EventTypeCode: "subcrt:orders:order:created", SpecVersion: new("1.0")},
			{EventTypeCode: "subcrt:orders:order:*"},
		},
		CustomConfig:   []subscription.ConfigEntry{{Key: "X-Env", Value: "test"}},
		Mode:           "BLOCK_ON_ERROR",
		TimeoutSeconds: new(int32(60)),
		MaxRetries:     new(int32(5)),
		DelaySeconds:   new(int32(10)),
		MaxAgeSeconds:  new(int32(3600)),
		DataOnly:       new(false),
	})
	require.NoError(t, err)

	assert.NotEmpty(t, ev.SubscriptionID)
	assert.Equal(t, "subcrt-happy", ev.Code, "code must be trimmed + lowercased")
	assert.Equal(t, "Sub Create Happy", ev.Name, "name must be trimmed")

	got, err := repo.FindByID(ctx, ev.SubscriptionID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "subcrt-happy", got.Code)
	assert.Equal(t, "Sub Create Happy", got.Name)
	assert.Equal(t, "https://orders.example.test/hook", got.Endpoint)
	assert.Equal(t, subscription.StatusActive, got.Status, "new subscriptions start ACTIVE")
	assert.Equal(t, subscription.SourceUI, got.Source, "admin create is UI-sourced")
	require.NotNil(t, got.Description)
	assert.Equal(t, desc, *got.Description)
	require.NotNil(t, got.ServiceAccountID)
	assert.Equal(t, "sva_subcrthappy1", *got.ServiceAccountID)
	// created_by persists since migration 035 (pre-035 rows read back
	// NULL — it was never written before).
	require.NotNil(t, got.CreatedBy)
	assert.Equal(t, testpg.TestEC().PrincipalID, *got.CreatedBy)
	assert.Equal(t, common.DispatchBlockOnError, got.Mode)
	assert.Equal(t, int32(60), got.TimeoutSeconds)
	assert.Equal(t, int32(5), got.MaxRetries)
	assert.Equal(t, int32(10), got.DelaySeconds)
	assert.Equal(t, int32(3600), got.MaxAgeSeconds)
	assert.False(t, got.DataOnly)

	require.Len(t, got.EventTypes, 2)
	codes := []string{got.EventTypes[0].EventTypeCode, got.EventTypes[1].EventTypeCode}
	assert.ElementsMatch(t, []string{"subcrt:orders:order:created", "subcrt:orders:order:*"}, codes)
	require.Len(t, got.CustomConfig, 1)
	assert.Equal(t, subscription.ConfigEntry{Key: "X-Env", Value: "test"}, got.CustomConfig[0])
}

func TestCreateSubscription_Validation(t *testing.T) {
	t.Parallel()
	repo := subscription.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	bindings := []subscription.EventTypeBinding{subscription.NewEventTypeBinding("subcrt:bad:input:case")}
	cases := []struct {
		name string
		cmd  operations.CreateCommand
		code string
	}{
		{"empty code", operations.CreateCommand{
			Name: "X", Endpoint: "https://x.example.test", EventTypes: bindings,
		}, "CODE_REQUIRED"},
		{"underscore code (strict hyphen-only pattern)", operations.CreateCommand{
			Code: "subcrt_bad", Name: "X", Endpoint: "https://x.example.test", EventTypes: bindings,
		}, "INVALID_CODE_FORMAT"},
		{"digit-leading code", operations.CreateCommand{
			Code: "1subcrt-bad", Name: "X", Endpoint: "https://x.example.test", EventTypes: bindings,
		}, "INVALID_CODE_FORMAT"},
		{"empty name", operations.CreateCommand{
			Code: "subcrt-noname", Endpoint: "https://x.example.test", EventTypes: bindings,
		}, "NAME_REQUIRED"},
		{"empty endpoint", operations.CreateCommand{
			Code: "subcrt-noep", Name: "X", EventTypes: bindings,
		}, "INVALID_ENDPOINT"},
		{"non-http endpoint", operations.CreateCommand{
			Code: "subcrt-ftpep", Name: "X", Endpoint: "ftp://files.example.test", EventTypes: bindings,
		}, "INVALID_ENDPOINT"},
		{"no event types", operations.CreateCommand{
			Code: "subcrt-noet", Name: "X", Endpoint: "https://x.example.test",
		}, "EVENT_TYPES_REQUIRED"},
		{"unknown queue value", operations.CreateCommand{
			Code: "subcrt-badq", Name: "X", Endpoint: "https://x.example.test",
			EventTypes: bindings, Queue: new("workers-high"),
		}, "INVALID_QUEUE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.CreateSubscription(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
}

// Re-pointing the connection must actually persist. It did not: connection_id
// was in the upsert's INSERT list but not its DO UPDATE SET, so the update use
// case set it on the entity and the SQL dropped it — while the request DTO
// documented the field as "accepted + persisted".
func TestUpdateSubscription_ConnectionIDPersists(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := subscription.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "subupd-conn", "Before")

	_, err := runAuthorized(uow, operations.UpdateSubscription(repo), operations.UpdateCommand{
		ID: seeded.SubscriptionID, ConnectionID: new("con_subupdconn1"),
	})
	require.NoError(t, err)
	got, err := repo.FindByID(ctx, seeded.SubscriptionID)
	require.NoError(t, err)
	require.NotNil(t, got.ConnectionID, "a re-pointed connection must survive the upsert")
	assert.Equal(t, "con_subupdconn1", *got.ConnectionID)

	// And re-pointing again replaces it, rather than sticking at the first
	// value the row was ever written with.
	_, err = runAuthorized(uow, operations.UpdateSubscription(repo), operations.UpdateCommand{
		ID: seeded.SubscriptionID, ConnectionID: new("con_subupdconn2"),
	})
	require.NoError(t, err)
	got, err = repo.FindByID(ctx, seeded.SubscriptionID)
	require.NoError(t, err)
	require.NotNil(t, got.ConnectionID)
	assert.Equal(t, "con_subupdconn2", *got.ConnectionID)
}

// The queue column is the dispatch priority: which of the client's two
// queues a job raised from this subscription publishes to. The SPA sends
// "default" (lower-case, always) on create, so the lower-case cases here are
// the shipped UI's actual payload, not a curiosity — and the stored form is
// canonical upper-case regardless of how it arrived.
func TestSubscriptionQueuePriority_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := subscription.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	bindings := []subscription.EventTypeBinding{subscription.NewEventTypeBinding("subq:a:b:c")}

	// Omitted entirely → NULL, which the publish path reads as DEFAULT.
	ev, err := runAuthorized(uow, operations.CreateSubscription(repo), operations.CreateCommand{
		Code: "subq-unset", Name: "X", Endpoint: "https://q.example.test", EventTypes: bindings,
	})
	require.NoError(t, err)
	got, err := repo.FindByID(ctx, ev.SubscriptionID)
	require.NoError(t, err)
	assert.Nil(t, got.Queue, "an omitted queue stays unset")

	// The SPA's actual create payload.
	ev, err = runAuthorized(uow, operations.CreateSubscription(repo), operations.CreateCommand{
		Code: "subq-default", Name: "X", Endpoint: "https://q.example.test",
		EventTypes: bindings, Queue: new("default"),
	})
	require.NoError(t, err)
	got, err = repo.FindByID(ctx, ev.SubscriptionID)
	require.NoError(t, err)
	require.NotNil(t, got.Queue)
	assert.Equal(t, "DEFAULT", *got.Queue, "stored canonically upper-case")

	ev, err = runAuthorized(uow, operations.CreateSubscription(repo), operations.CreateCommand{
		Code: "subq-high", Name: "X", Endpoint: "https://q.example.test",
		EventTypes: bindings, Queue: new("high_priority"),
	})
	require.NoError(t, err)
	id := ev.SubscriptionID
	got, err = repo.FindByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, got.Queue)
	assert.Equal(t, "HIGH_PRIORITY", *got.Queue)

	// Update re-points it …
	_, err = runAuthorized(uow, operations.UpdateSubscription(repo), operations.UpdateCommand{
		ID: id, Queue: new("DEFAULT"),
	})
	require.NoError(t, err)
	got, err = repo.FindByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, got.Queue)
	assert.Equal(t, "DEFAULT", *got.Queue)

	// … an omitted queue leaves it alone …
	_, err = runAuthorized(uow, operations.UpdateSubscription(repo), operations.UpdateCommand{
		ID: id, Name: new("Renamed"),
	})
	require.NoError(t, err)
	got, err = repo.FindByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, got.Queue, "an omitted queue must not clear the stored value")

	// … and an explicit blank clears it, the only way back to unset.
	_, err = runAuthorized(uow, operations.UpdateSubscription(repo), operations.UpdateCommand{
		ID: id, Queue: new(""),
	})
	require.NoError(t, err)
	got, err = repo.FindByID(ctx, id)
	require.NoError(t, err)
	assert.Nil(t, got.Queue)

	// Update validates the same way create does.
	_, err = runAuthorized(uow, operations.UpdateSubscription(repo), operations.UpdateCommand{
		ID: id, Queue: new("workers-high"),
	})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVALID_QUEUE")
}

// Conflict is pinned by seeding through the operation itself: the first
// create IS the seed for the second (both anchor-scoped: nil ClientID).
func TestCreateSubscription_DuplicateCode_Conflict(t *testing.T) {
	t.Parallel()
	repo := subscription.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	mustCreate(t, repo, uow, "subdup-code", "First")

	_, err := runAuthorized(uow, operations.CreateSubscription(repo),
		operations.CreateCommand{
			Code: "subdup-code", Name: "Second", Endpoint: "https://dup.example.test",
			EventTypes: []subscription.EventTypeBinding{subscription.NewEventTypeBinding("subdup:a:b:c")},
		})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "CODE_EXISTS")
}

// TestSyncSubscriptions_SameCodeDifferentApplications_Allowed pins the new
// (application_code, client_id, code) key for subscriptions: the same code
// under two different applications must not collide. CreateSubscription
// (the plain create use case) never sets ApplicationCode — only
// SyncSubscriptions does — so this goes through the real production path
// that actually exercises the application dimension of the key.
func TestSyncSubscriptions_SameCodeDifferentApplications_Allowed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	subRepo := subscription.NewRepository(pool)
	connRepo := connection.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)
	uow := testpg.NewUoW(t)

	const code = "subappscope-shared"
	bindings := []operations.SyncEventTypeBindingInput{{EventTypeCode: "subappscope:a:b:c"}}

	_, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncSubscriptions(subRepo, connRepo, poolRepo),
		operations.SyncSubscriptionsCommand{
			ApplicationCode: "subappscope-app-a",
			Subscriptions: []operations.SyncSubscriptionInput{
				{Code: code, Name: "A", Target: "https://a.example.test/hook", EventTypes: bindings},
			},
		}, testpg.TestEC())
	require.NoError(t, err)

	_, err = usecaseop.Run(appAccessCtx(), uow, operations.SyncSubscriptions(subRepo, connRepo, poolRepo),
		operations.SyncSubscriptionsCommand{
			ApplicationCode: "subappscope-app-b",
			Subscriptions: []operations.SyncSubscriptionInput{
				{Code: code, Name: "B", Target: "https://b.example.test/hook", EventTypes: bindings},
			},
		}, testpg.TestEC())
	require.NoError(t, err, "the same code under a different application must not conflict")

	appA := "subappscope-app-a"
	appB := "subappscope-app-b"
	subA, err := subRepo.FindByCode(ctx, code, &appA, nil)
	require.NoError(t, err)
	require.NotNil(t, subA)
	assert.Equal(t, "A", subA.Name)

	subB, err := subRepo.FindByCode(ctx, code, &appB, nil)
	require.NoError(t, err)
	require.NotNil(t, subB)
	assert.Equal(t, "B", subB.Name)
}

// TestSubscriptions_DuplicateSharedNoClient_RejectedAtDatabase proves the
// actual bug fix in migration 056 for msg_subscriptions: two subscriptions
// with the same code, no application, and no client used to be allowed by
// the OLD idx_msg_subscriptions_code_client index because Postgres treats
// NULLs as distinct in a plain unique index. It goes straight at the
// database with a second raw INSERT — bypassing CreateSubscription's
// find-then-insert check entirely — to prove the constraint itself is what
// now refuses the second row.
func TestSubscriptions_DuplicateSharedNoClient_RejectedAtDatabase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)

	const code = "subappscope-db-dupe"
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_subscriptions (id, code, name, target)
		 VALUES ('sub_dbdupe0000001', $1, 'First', 'https://dbdupe.example.test/hook')`, code)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM msg_subscriptions WHERE id = 'sub_dbdupe0000001'`)
	})

	_, err = pool.Exec(ctx,
		`INSERT INTO msg_subscriptions (id, code, name, target)
		 VALUES ('sub_dbdupe0000002', $1, 'Second', 'https://dbdupe.example.test/hook')`, code)
	require.Error(t, err, "a second shared, clientless subscription with the same code must be rejected by the unique index")
	assert.Contains(t, err.Error(), "uq_msg_subscriptions_app_client_code")
}

// TestCreateSubscription_ResourceScope proves the use case's per-resource
// authorization: the coarse "may write subscriptions" permission is the
// controller's job, but the use case enforces that you can only bind a
// subscription to a client you can access (and that platform-wide
// subscriptions require anchor). A client-scoped principal — even one holding
// the subscription-create permission — is denied a platform-wide and an
// other-client subscription, but allowed one for its own client.
func TestCreateSubscription_ResourceScope(t *testing.T) {
	t.Parallel()
	repo := subscription.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	bindings := []subscription.EventTypeBinding{subscription.NewEventTypeBinding("subscope:a:b:c")}
	ownClient := "cli_subscope_own"
	clientCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_subscope1",
		Scope:       auth.ScopeClient,
		Clients:     []string{ownClient},
		Permissions: []string{"platform:messaging:subscription:create"},
	})

	// Platform-wide (nil ClientID) → cross-client → anchor required → denied.
	_, err := usecaseop.Run(clientCtx, uow, operations.CreateSubscription(repo),
		operations.CreateCommand{
			Code: "subscope-platform", Name: "X", Endpoint: "https://x.example.test",
			EventTypes: bindings,
		}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	// Bound to a client the principal cannot access → denied.
	other := "cli_subscope_other"
	_, err = usecaseop.Run(clientCtx, uow, operations.CreateSubscription(repo),
		operations.CreateCommand{
			Code: "subscope-other", Name: "X", Endpoint: "https://x.example.test",
			ClientID: &other, EventTypes: bindings,
		}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	// Bound to the principal's own client → allowed.
	ev, err := usecaseop.Run(clientCtx, uow, operations.CreateSubscription(repo),
		operations.CreateCommand{
			Code: "subscope-own", Name: "Mine", Endpoint: "https://x.example.test",
			ClientID: &ownClient, EventTypes: bindings,
		}, testpg.TestEC())
	require.NoError(t, err)
	assert.Equal(t, "subscope-own", ev.Code)
}

// ── Update ────────────────────────────────────────────────────────────────

func TestUpdateSubscription_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := subscription.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "subupd-happy", "Before")

	ev, err := runAuthorized(uow, operations.UpdateSubscription(repo), operations.UpdateCommand{
		ID:          seeded.SubscriptionID,
		Name:        new("  After  "), // op must trim
		Description: new("after"),
		Endpoint:    new("https://after.example.test/hook"),
		EventTypes: []subscription.EventTypeBinding{
			subscription.NewEventTypeBinding("subupd:orders:order:updated"),
		},
		Mode:             new("NEXT_ON_ERROR"),
		TimeoutSeconds:   new(int32(90)),
		MaxRetries:       new(int32(7)),
		DelaySeconds:     new(int32(5)),
		MaxAgeSeconds:    new(int32(7200)),
		ServiceAccountID: new("sva_subupdafter1"),
		DataOnly:         new(false),
	})
	require.NoError(t, err)
	assert.Equal(t, seeded.SubscriptionID, ev.SubscriptionID)
	assert.Equal(t, "After", ev.Name)

	got, err := repo.FindByID(ctx, seeded.SubscriptionID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "subupd-happy", got.Code, "code is immutable on update")
	assert.Equal(t, "After", got.Name)
	require.NotNil(t, got.Description)
	assert.Equal(t, "after", *got.Description)
	assert.Equal(t, "https://after.example.test/hook", got.Endpoint)
	require.Len(t, got.EventTypes, 1, "event-type bindings are replaced wholesale")
	assert.Equal(t, "subupd:orders:order:updated", got.EventTypes[0].EventTypeCode)
	assert.Equal(t, common.DispatchNextOnError, got.Mode)
	assert.Equal(t, int32(90), got.TimeoutSeconds)
	assert.Equal(t, int32(7), got.MaxRetries)
	assert.Equal(t, int32(5), got.DelaySeconds)
	assert.Equal(t, int32(7200), got.MaxAgeSeconds)
	require.NotNil(t, got.ServiceAccountID)
	assert.Equal(t, "sva_subupdafter1", *got.ServiceAccountID)
	assert.False(t, got.DataOnly)
	assert.Equal(t, subscription.StatusActive, got.Status, "update must not touch status")
}

func TestUpdateSubscription_Errors(t *testing.T) {
	t.Parallel()
	repo := subscription.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.UpdateCommand
		kind usecase.Kind
		code string
	}{
		{"missing id", operations.UpdateCommand{Name: new("X")}, usecase.KindValidation, "ID_REQUIRED"},
		{"blank name", operations.UpdateCommand{ID: "sub_doesnotexist1", Name: new(" ")}, usecase.KindValidation, "NAME_REQUIRED"},
		{"bad endpoint", operations.UpdateCommand{ID: "sub_doesnotexist1", Endpoint: new("not-a-url")}, usecase.KindValidation, "INVALID_ENDPOINT"},
		{"unknown id", operations.UpdateCommand{ID: "sub_doesnotexist1", Name: new("X")}, usecase.KindNotFound, "Subscription_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.UpdateSubscription(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

// ── Pause / Resume (status round-trip) ────────────────────────────────────

func TestPauseResumeSubscription_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := subscription.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "subpse-roundtrip", "Pause Me")

	paused, err := runAuthorized(uow, operations.PauseSubscription(repo),
		operations.PauseCommand{ID: seeded.SubscriptionID})
	require.NoError(t, err)
	assert.Equal(t, seeded.SubscriptionID, paused.SubscriptionID)

	got, err := repo.FindByID(ctx, seeded.SubscriptionID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, subscription.StatusPaused, got.Status)

	resumed, err := runAuthorized(uow, operations.ResumeSubscription(repo),
		operations.ResumeCommand{ID: seeded.SubscriptionID})
	require.NoError(t, err)
	assert.Equal(t, seeded.SubscriptionID, resumed.SubscriptionID)

	got, err = repo.FindByID(ctx, seeded.SubscriptionID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, subscription.StatusActive, got.Status)
}

func TestPauseSubscription_Errors(t *testing.T) {
	t.Parallel()
	repo := subscription.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.PauseSubscription(repo),
		operations.PauseCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.PauseSubscription(repo),
		operations.PauseCommand{ID: "sub_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Subscription_NOT_FOUND")
}

func TestResumeSubscription_Errors(t *testing.T) {
	t.Parallel()
	repo := subscription.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.ResumeSubscription(repo),
		operations.ResumeCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.ResumeSubscription(repo),
		operations.ResumeCommand{ID: "sub_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Subscription_NOT_FOUND")
}

// ── Delete ────────────────────────────────────────────────────────────────

func TestDeleteSubscription_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := subscription.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "subdel-happy", "Doomed")

	ev, err := runAuthorized(uow, operations.DeleteSubscription(repo),
		operations.DeleteCommand{ID: seeded.SubscriptionID})
	require.NoError(t, err)
	assert.Equal(t, seeded.SubscriptionID, ev.SubscriptionID)
	assert.Equal(t, "subdel-happy", ev.Code)

	got, err := repo.FindByID(ctx, seeded.SubscriptionID)
	require.NoError(t, err)
	assert.Nil(t, got, "deleted row must be gone")
}

func TestDeleteSubscription_Errors(t *testing.T) {
	t.Parallel()
	repo := subscription.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.DeleteSubscription(repo),
		operations.DeleteCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.DeleteSubscription(repo),
		operations.DeleteCommand{ID: "sub_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Subscription_NOT_FOUND")
}

// ── Sync (app-scoped; pool resolution; API-source-only removal) ───────────

// Sync is scoped by application_code, so a fresh app code keeps this test
// hermetic under t.Parallel().
func TestSyncSubscriptions_UpsertRemoveAndPoolResolution(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	subRepo := subscription.NewRepository(pool)
	connRepo := connection.NewRepository(pool)
	connApps := application.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()
	appCode := "subsyncapp1"

	// Real connection for the connectionId binding. ServiceAccountID is an
	// arbitrary string — CreateConnection does not validate it.
	connEv, err := runAuthorized(uow, connops.CreateConnection(connRepo, connApps), connops.CreateCommand{
		Code: "subsync-conn1", Name: "Sub Sync Conn", ServiceAccountID: "sva_subsync1",
	})
	require.NoError(t, err)
	connID := connEv.ConnectionID

	// Anchor-scoped pool: sync resolves dispatchPoolCode via the global
	// (nil-client) lookup.
	poolEv, err := runAuthorized(uow, poolops.CreateDispatchPool(poolRepo), poolops.CreateCommand{
		Code: "subsync-pool1", Name: "Sub Sync Pool",
	})
	require.NoError(t, err)
	poolID := poolEv.PoolID

	// A UI-authored row inside the application scope: RemoveUnlisted must
	// never touch it. No operation writes application_code on a UI create,
	// so scope the seeded row with a one-column infrastructure update.
	uiRow := mustCreate(t, subRepo, uow, "subsync-ui-kept", "UI Kept")
	_, err = pool.Exec(ctx,
		`UPDATE msg_subscriptions SET application_code = $1 WHERE id = $2`,
		appCode, uiRow.SubscriptionID)
	require.NoError(t, err)

	first, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncSubscriptions(subRepo, connRepo, poolRepo),
		operations.SyncSubscriptionsCommand{
			ApplicationCode: appCode,
			Subscriptions: []operations.SyncSubscriptionInput{
				{
					Code: "subsync-a", Name: "A", Target: "https://a.example.test/hook",
					ConnectionID:     &connID,
					EventTypes:       []operations.SyncEventTypeBindingInput{{EventTypeCode: "subsync:orders:order:created"}},
					DispatchPoolCode: new("subsync-pool1"),
				},
				{
					Code: "subsync-b", Name: "B", Target: "https://b.example.test/hook",
					EventTypes:       []operations.SyncEventTypeBindingInput{{EventTypeCode: "subsync:orders:order:updated"}},
					DispatchPoolCode: new("subsync-nosuchpool"),
				},
			},
		}, ec)
	require.NoError(t, err)
	assert.Equal(t, uint32(2), first.Created)
	assert.Equal(t, uint32(0), first.Updated)
	assert.Equal(t, uint32(0), first.Deleted)
	assert.Equal(t, appCode, first.ApplicationCode)
	assert.Equal(t, []string{"subsync-a", "subsync-b"}, first.SyncedCodes)

	subA, err := subRepo.FindByCode(ctx, "subsync-a", &appCode, nil)
	require.NoError(t, err)
	require.NotNil(t, subA)
	assert.Equal(t, subscription.SourceAPI, subA.Source, "synced rows are API-sourced")
	require.NotNil(t, subA.ApplicationCode)
	assert.Equal(t, appCode, *subA.ApplicationCode)
	require.NotNil(t, subA.ConnectionID)
	assert.Equal(t, connID, *subA.ConnectionID)
	require.NotNil(t, subA.DispatchPoolID, "resolvable dispatchPoolCode must link the pool")
	assert.Equal(t, poolID, *subA.DispatchPoolID)
	require.NotNil(t, subA.DispatchPoolCode)
	assert.Equal(t, "subsync-pool1", *subA.DispatchPoolCode)

	// Pin: an unresolvable dispatchPoolCode is silently left unset — no error.
	subB, err := subRepo.FindByCode(ctx, "subsync-b", &appCode, nil)
	require.NoError(t, err)
	require.NotNil(t, subB)
	assert.Nil(t, subB.DispatchPoolID, "unresolvable pool code must leave the pool ref unset")
	assert.Nil(t, subB.DispatchPoolCode)

	second, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncSubscriptions(subRepo, connRepo, poolRepo),
		operations.SyncSubscriptionsCommand{
			ApplicationCode: appCode,
			Subscriptions: []operations.SyncSubscriptionInput{
				{
					Code: "subsync-a", Name: "A renamed", Target: "https://a.example.test/hook",
					EventTypes: []operations.SyncEventTypeBindingInput{{EventTypeCode: "subsync:orders:order:created"}},
				},
			},
			RemoveUnlisted: true,
		}, ec)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), second.Created)
	assert.Equal(t, uint32(1), second.Updated)
	assert.Equal(t, uint32(1), second.Deleted)

	kept, err := subRepo.FindByCode(ctx, "subsync-a", &appCode, nil)
	require.NoError(t, err)
	require.NotNil(t, kept)
	assert.Equal(t, "A renamed", kept.Name)
	require.NotNil(t, kept.DispatchPoolCode, "omitted dispatchPoolCode must leave the existing pool link")
	assert.Equal(t, "subsync-pool1", *kept.DispatchPoolCode)

	goneB, err := subRepo.FindByCode(ctx, "subsync-b", &appCode, nil)
	require.NoError(t, err)
	assert.Nil(t, goneB, "RemoveUnlisted must hard-delete unlisted API rows")

	stillUI, err := subRepo.FindByID(ctx, uiRow.SubscriptionID)
	require.NoError(t, err)
	require.NotNil(t, stillUI, "RemoveUnlisted must never touch UI-sourced rows")
	assert.Equal(t, subscription.SourceUI, stillUI.Source)
}

func TestSyncSubscriptions_Validation(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	subRepo := subscription.NewRepository(pool)
	connRepo := connection.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)
	uow := testpg.NewUoW(t)

	bindings := []operations.SyncEventTypeBindingInput{{EventTypeCode: "subsync:bad:input:case"}}
	cases := []struct {
		name string
		cmd  operations.SyncSubscriptionsCommand
		code string
	}{
		{"missing application code", operations.SyncSubscriptionsCommand{}, "APPLICATION_CODE_REQUIRED"},
		{"entry missing code", operations.SyncSubscriptionsCommand{
			ApplicationCode: "subsyncbad",
			Subscriptions: []operations.SyncSubscriptionInput{
				{Name: "X", Target: "https://x.example.test", EventTypes: bindings},
			},
		}, "CODE_REQUIRED"},
		{"entry missing name", operations.SyncSubscriptionsCommand{
			ApplicationCode: "subsyncbad",
			Subscriptions: []operations.SyncSubscriptionInput{
				{Code: "subsync-noname", Target: "https://x.example.test", EventTypes: bindings},
			},
		}, "NAME_REQUIRED"},
		{"entry missing target", operations.SyncSubscriptionsCommand{
			ApplicationCode: "subsyncbad",
			Subscriptions: []operations.SyncSubscriptionInput{
				{Code: "subsync-notarget", Name: "X", EventTypes: bindings},
			},
		}, "TARGET_REQUIRED"},
		{"entry missing event types", operations.SyncSubscriptionsCommand{
			ApplicationCode: "subsyncbad",
			Subscriptions: []operations.SyncSubscriptionInput{
				{Code: "subsync-noet", Name: "X", Target: "https://x.example.test"},
			},
		}, "EVENT_TYPES_REQUIRED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := usecaseop.Run(appAccessCtx(), uow,
				operations.SyncSubscriptions(subRepo, connRepo, poolRepo), tc.cmd, testpg.TestEC())
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
}

// Connection resolution uses usecase.NotFound directly with the exact code
// CONNECTION_NOT_FOUND (not the httperror <Resource>_NOT_FOUND helper).
func TestSyncSubscriptions_ConnectionNotFound(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	subRepo := subscription.NewRepository(pool)
	connRepo := connection.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)
	uow := testpg.NewUoW(t)

	_, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncSubscriptions(subRepo, connRepo, poolRepo),
		operations.SyncSubscriptionsCommand{
			ApplicationCode: "subsyncconn404",
			Subscriptions: []operations.SyncSubscriptionInput{
				{
					Code: "subsync-badconn", Name: "X", Target: "https://x.example.test",
					ConnectionID: new("con_doesnotexist1"),
					EventTypes:   []operations.SyncEventTypeBindingInput{{EventTypeCode: "subsync:a:b:c"}},
				},
			},
		}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "CONNECTION_NOT_FOUND")
}

// TestSyncSubscriptions_ConnectionCode is the code-first route: a connection id
// is minted per environment, so a definition compiled into an app can only
// name the connection by its code. The sync resolves that code, within THIS
// application's own namespace (see TestSyncSubscriptions_ConnectionNamespace_
// NoFallback for the shared namespace and its explicit opt-in), to THIS
// environment's id — and refuses a code that doesn't exist, or an id + code
// pair that name different connections.
func TestSyncSubscriptions_ConnectionCode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	subRepo := subscription.NewRepository(pool)
	connRepo := connection.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)

	appCode := "subsyncconncode"
	const connID, connCode = "cnx_subsynccode1", "subsync-conn-code"
	insertRawConnection(t, pool, connID, connCode, "sac_subsynccode1", &appCode, nil)

	run := func(cmd operations.SyncSubscriptionsCommand) error {
		_, err := usecaseop.Run(appAccessCtx(), testpg.NewUoW(t),
			operations.SyncSubscriptions(subRepo, connRepo, poolRepo), cmd, testpg.TestEC())
		return err
	}
	entry := func(in operations.SyncSubscriptionInput) operations.SyncSubscriptionInput {
		in.Name, in.Target = "X", "https://x.example.test"
		in.EventTypes = []operations.SyncEventTypeBindingInput{{EventTypeCode: "subsync:a:b:c"}}
		return in
	}

	require.NoError(t, run(operations.SyncSubscriptionsCommand{
		ApplicationCode: appCode,
		Subscriptions:   []operations.SyncSubscriptionInput{entry(operations.SyncSubscriptionInput{Code: "subsync-bycode", ConnectionCode: new(connCode)})},
	}))
	got, err := subRepo.FindByCode(ctx, "subsync-bycode", &appCode, nil)
	require.NoError(t, err)
	require.NotNil(t, got.ConnectionID)
	assert.Equal(t, connID, *got.ConnectionID, "the code resolves, within this application's own namespace, to this environment's connection id")

	// Re-sync by code updates the same row rather than losing the connection.
	require.NoError(t, run(operations.SyncSubscriptionsCommand{
		ApplicationCode: appCode,
		Subscriptions:   []operations.SyncSubscriptionInput{entry(operations.SyncSubscriptionInput{Code: "subsync-bycode", ConnectionCode: new(connCode), ConnectionID: new(connID)})},
	}))

	testpg.RequireUsecaseError(t, run(operations.SyncSubscriptionsCommand{
		ApplicationCode: "subsyncconncode404",
		Subscriptions:   []operations.SyncSubscriptionInput{entry(operations.SyncSubscriptionInput{Code: "subsync-badcode", ConnectionCode: new("no-such-connection")})},
	}), usecase.KindNotFound, "CONNECTION_NOT_FOUND")

	testpg.RequireUsecaseError(t, run(operations.SyncSubscriptionsCommand{
		ApplicationCode: appCode,
		Subscriptions:   []operations.SyncSubscriptionInput{entry(operations.SyncSubscriptionInput{Code: "subsync-mismatch", ConnectionCode: new(connCode), ConnectionID: new("con_someotherone1")})},
	}), usecase.KindValidation, "CONNECTION_MISMATCH")
}

// TestSyncSubscriptions_ConnectionNamespace_NoFallback pins ruling
// 2026-09-21 #4: a connectionCode lookup NEVER falls between the
// application-owned namespace and the shared (application-less) one. A bare
// code only ever resolves an application-owned connection; sharedConnection
// switches to ONLY the shared namespace. A connection existing at the same
// code in the other namespace is invisible to the lookup, not a fallback
// target.
func TestSyncSubscriptions_ConnectionNamespace_NoFallback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	subRepo := subscription.NewRepository(pool)
	connRepo := connection.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)

	appCode := "subns-app"
	run := func(in operations.SyncSubscriptionInput) error {
		in.Name, in.Target = "X", "https://x.example.test"
		in.EventTypes = []operations.SyncEventTypeBindingInput{{EventTypeCode: "subns:a:b:c"}}
		_, err := usecaseop.Run(appAccessCtx(), testpg.NewUoW(t),
			operations.SyncSubscriptions(subRepo, connRepo, poolRepo),
			operations.SyncSubscriptionsCommand{ApplicationCode: appCode, Subscriptions: []operations.SyncSubscriptionInput{in}},
			testpg.TestEC())
		return err
	}

	// Only a SHARED connection exists at this code: a bare code lookup must
	// not fall through to it.
	const onlySharedCode = "subns-only-shared"
	insertRawConnection(t, pool, "cnx_subnsonlysh1", onlySharedCode, "sva_subns1", nil, nil)
	testpg.RequireUsecaseError(t,
		run(operations.SyncSubscriptionInput{Code: "subns-bare-miss", ConnectionCode: new(onlySharedCode)}),
		usecase.KindNotFound, "CONNECTION_NOT_FOUND")
	// sharedConnection:true DOES find it.
	require.NoError(t, run(operations.SyncSubscriptionInput{
		Code: "subns-shared-hit", ConnectionCode: new(onlySharedCode), SharedConnection: true,
	}))
	gotSharedHit, err := subRepo.FindByCode(ctx, "subns-shared-hit", &appCode, nil)
	require.NoError(t, err)
	require.NotNil(t, gotSharedHit.ConnectionID)
	assert.Equal(t, "cnx_subnsonlysh1", *gotSharedHit.ConnectionID)

	// Only an APPLICATION-OWNED connection exists at this code: sharedConnection
	// must not fall through to it.
	const onlyOwnedCode = "subns-only-owned"
	insertRawConnection(t, pool, "cnx_subnsonlyow1", onlyOwnedCode, "sva_subns2", &appCode, nil)
	testpg.RequireUsecaseError(t,
		run(operations.SyncSubscriptionInput{Code: "subns-shared-miss", ConnectionCode: new(onlyOwnedCode), SharedConnection: true}),
		usecase.KindNotFound, "CONNECTION_NOT_FOUND")
	// The bare (default) lookup DOES find it.
	require.NoError(t, run(operations.SyncSubscriptionInput{Code: "subns-bare-hit", ConnectionCode: new(onlyOwnedCode)}))
	gotBareHit, err := subRepo.FindByCode(ctx, "subns-bare-hit", &appCode, nil)
	require.NoError(t, err)
	require.NotNil(t, gotBareHit.ConnectionID)
	assert.Equal(t, "cnx_subnsonlyow1", *gotBareHit.ConnectionID)

	// Both namespaces have a connection at the SAME code: bare and shared
	// resolve to two DIFFERENT rows, proving there's no fallback either way.
	const bothCode = "subns-both"
	insertRawConnection(t, pool, "cnx_subnsbothown1", bothCode, "sva_subns3", &appCode, nil)
	insertRawConnection(t, pool, "cnx_subnsbothshr1", bothCode, "sva_subns4", nil, nil)
	require.NoError(t, run(operations.SyncSubscriptionInput{Code: "subns-both-bare", ConnectionCode: new(bothCode)}))
	require.NoError(t, run(operations.SyncSubscriptionInput{Code: "subns-both-shared", ConnectionCode: new(bothCode), SharedConnection: true}))
	gotBare, err := subRepo.FindByCode(ctx, "subns-both-bare", &appCode, nil)
	require.NoError(t, err)
	gotShared, err := subRepo.FindByCode(ctx, "subns-both-shared", &appCode, nil)
	require.NoError(t, err)
	require.NotNil(t, gotBare.ConnectionID)
	require.NotNil(t, gotShared.ConnectionID)
	assert.Equal(t, "cnx_subnsbothown1", *gotBare.ConnectionID)
	assert.Equal(t, "cnx_subnsbothshr1", *gotShared.ConnectionID)
}

// TestSyncSubscriptions_ConnectionClientPreference pins ruling 2026-09-21 #4's
// client axis: within whichever namespace was chosen, a client-scoped sync
// prefers its OWN client's connection at that code, falling back to a global
// one only when the client has none.
func TestSyncSubscriptions_ConnectionClientPreference(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	subRepo := subscription.NewRepository(pool)
	connRepo := connection.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)

	appCode := "subcli-app"
	clientA := "cli_subcli_a00a"

	run := func(clientID *string, in operations.SyncSubscriptionInput) error {
		in.Name, in.Target = "X", "https://x.example.test"
		in.EventTypes = []operations.SyncEventTypeBindingInput{{EventTypeCode: "subcli:a:b:c"}}
		_, err := usecaseop.Run(appAccessCtx(), testpg.NewUoW(t),
			operations.SyncSubscriptions(subRepo, connRepo, poolRepo),
			operations.SyncSubscriptionsCommand{ApplicationCode: appCode, ClientID: clientID, Subscriptions: []operations.SyncSubscriptionInput{in}},
			testpg.TestEC())
		return err
	}

	// Only a global (application-owned, client-less) connection exists at this
	// code: a client-scoped sync falls back to it.
	const fallbackCode = "subcli-fallback"
	insertRawConnection(t, pool, "cnx_subclifb00001", fallbackCode, "sva_subcli1", &appCode, nil)
	require.NoError(t, run(&clientA, operations.SyncSubscriptionInput{Code: "subcli-fb", ConnectionCode: new(fallbackCode)}))
	gotFallback, err := subRepo.FindByCode(ctx, "subcli-fb", &appCode, &clientA)
	require.NoError(t, err)
	require.NotNil(t, gotFallback.ConnectionID)
	assert.Equal(t, "cnx_subclifb00001", *gotFallback.ConnectionID)

	// Both a global AND the client's own connection exist at the same code:
	// the client's own connection wins.
	const preferCode = "subcli-prefer"
	insertRawConnection(t, pool, "cnx_subcliglob001", preferCode, "sva_subcli2", &appCode, nil)
	insertRawConnection(t, pool, "cnx_subcliownA001", preferCode, "sva_subcli3", &appCode, &clientA)
	require.NoError(t, run(&clientA, operations.SyncSubscriptionInput{Code: "subcli-pref", ConnectionCode: new(preferCode)}))
	gotPreferred, err := subRepo.FindByCode(ctx, "subcli-pref", &appCode, &clientA)
	require.NoError(t, err)
	require.NotNil(t, gotPreferred.ConnectionID)
	assert.Equal(t, "cnx_subcliownA001", *gotPreferred.ConnectionID, "the client's own connection must be preferred over the global one")
}

// TestSyncSubscriptions_ConnectionScopeMismatch pins ruling 2026-09-21 #5: a
// global subscription may never bind to a client-owned connection. Naming it
// by connectionId surfaces CONNECTION_SCOPE_MISMATCH (the id lookup doesn't
// filter by client, so it finds the row and the explicit check catches it).
// Naming the SAME connection by connectionCode instead surfaces
// CONNECTION_NOT_FOUND — not a scope mismatch — because a global sync's code
// lookup is restricted to a NULL client_id by construction (ruling #4: "a
// global subscription may use ONLY a global connection"), so a client-owned
// row is never even a candidate match; there's nothing to compare scopes
// against.
func TestSyncSubscriptions_ConnectionScopeMismatch(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	subRepo := subscription.NewRepository(pool)
	connRepo := connection.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)

	appCode := "subscmis-app"
	clientA := "cli_subscmis_a0a"
	const code = "subscmis-conn"
	insertRawConnection(t, pool, "cnx_subscmisown1", code, "sva_subscmis1", &appCode, &clientA)

	run := func(in operations.SyncSubscriptionInput) error {
		in.Name, in.Target = "X", "https://x.example.test"
		in.EventTypes = []operations.SyncEventTypeBindingInput{{EventTypeCode: "subscmis:a:b:c"}}
		_, err := usecaseop.Run(appAccessCtx(), testpg.NewUoW(t),
			operations.SyncSubscriptions(subRepo, connRepo, poolRepo),
			// No ClientID: a global subscription.
			operations.SyncSubscriptionsCommand{ApplicationCode: appCode, Subscriptions: []operations.SyncSubscriptionInput{in}},
			testpg.TestEC())
		return err
	}

	testpg.RequireUsecaseError(t,
		run(operations.SyncSubscriptionInput{Code: "subscmis-byid", ConnectionID: new("cnx_subscmisown1")}),
		usecase.KindValidation, "CONNECTION_SCOPE_MISMATCH")

	testpg.RequireUsecaseError(t,
		run(operations.SyncSubscriptionInput{Code: "subscmis-bycode", ConnectionCode: new(code)}),
		usecase.KindNotFound, "CONNECTION_NOT_FOUND")
}

// TestSyncSubscriptions_ConnectionIdCannotCrossApplications closes the id
// path's hole on the application axis: a connection signs deliveries with its
// application's credentials, so a sync for one application must not be able to
// bind a subscription to another application's connection just by knowing its
// id. A shared (application-less) connection stays usable by id from anywhere.
func TestSyncSubscriptions_ConnectionIdCannotCrossApplications(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	subRepo := subscription.NewRepository(pool)
	connRepo := connection.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)

	otherApp := "subxapp-other"
	insertRawConnection(t, pool, "cnx_subxappothr1", "subxapp-theirs", "sva_subxapp1", &otherApp, nil)
	insertRawConnection(t, pool, "cnx_subxappshrd1", "subxapp-shared", "sva_subxapp2", nil, nil)

	run := func(code, connID string) error {
		_, err := usecaseop.Run(appAccessCtx(), testpg.NewUoW(t),
			operations.SyncSubscriptions(subRepo, connRepo, poolRepo),
			operations.SyncSubscriptionsCommand{ApplicationCode: "subxapp-mine", Subscriptions: []operations.SyncSubscriptionInput{{
				Code: code, Name: "X", Target: "https://x.example.test", ConnectionID: &connID,
				EventTypes: []operations.SyncEventTypeBindingInput{{EventTypeCode: "subxapp:a:b:c"}},
			}}}, testpg.TestEC())
		return err
	}

	testpg.RequireUsecaseError(t, run("subxapp-borrow", "cnx_subxappothr1"),
		usecase.KindValidation, "CONNECTION_SCOPE_MISMATCH")

	require.NoError(t, run("subxapp-useshared", "cnx_subxappshrd1"))
	subs, err := subRepo.FindByApplicationAndClient(ctx, "subxapp-mine", nil)
	require.NoError(t, err)
	require.Len(t, subs, 1, "only the shared-connection subscription was created")
	require.NotNil(t, subs[0].ConnectionID)
	assert.Equal(t, "cnx_subxappshrd1", *subs[0].ConnectionID)
}

// TestSyncSubscriptions_SharedConnectionRequiresCode pins ruling 2026-09-21
// #4's validation edge: sharedConnection is meaningless without a code to
// resolve within that namespace.
func TestSyncSubscriptions_SharedConnectionRequiresCode(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	subRepo := subscription.NewRepository(pool)
	connRepo := connection.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)

	_, err := usecaseop.Run(appAccessCtx(), testpg.NewUoW(t),
		operations.SyncSubscriptions(subRepo, connRepo, poolRepo),
		operations.SyncSubscriptionsCommand{
			ApplicationCode: "subsharedreq-app",
			Subscriptions: []operations.SyncSubscriptionInput{
				{
					Code: "subsharedreq-x", Name: "X", Target: "https://x.example.test",
					EventTypes:       []operations.SyncEventTypeBindingInput{{EventTypeCode: "subsharedreq:a:b:c"}},
					SharedConnection: true,
				},
			},
		}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SHARED_CONNECTION_REQUIRES_CODE")
}

// TestSyncSubscriptions_SameCodeAcrossClientPartitions proves the same
// subscription code exists independently under client A, client B, and
// globally for one application — each partition is updated only by its own
// sync, never by another partition's.
func TestSyncSubscriptions_SameCodeAcrossClientPartitions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	subRepo := subscription.NewRepository(pool)
	connRepo := connection.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)

	appCode := "subpart-app"
	clientA := "cli_subpart_a0a"
	clientB := "cli_subpart_b0b"
	const code = "subpart-shared-code"
	bindings := []operations.SyncEventTypeBindingInput{{EventTypeCode: "subpart:a:b:c"}}

	run := func(clientID *string, name string) {
		_, err := usecaseop.Run(appAccessCtx(), testpg.NewUoW(t),
			operations.SyncSubscriptions(subRepo, connRepo, poolRepo),
			operations.SyncSubscriptionsCommand{
				ApplicationCode: appCode,
				ClientID:        clientID,
				Subscriptions: []operations.SyncSubscriptionInput{
					{Code: code, Name: name, Target: "https://x.example.test/" + name, EventTypes: bindings},
				},
			}, testpg.TestEC())
		require.NoError(t, err)
	}

	run(&clientA, "A1")
	run(&clientB, "B1")
	run(nil, "G1")
	// Re-syncing client A's partition must not touch the others.
	run(&clientA, "A2")

	subA, err := subRepo.FindByCode(ctx, code, &appCode, &clientA)
	require.NoError(t, err)
	require.NotNil(t, subA)
	assert.Equal(t, "A2", subA.Name)

	subB, err := subRepo.FindByCode(ctx, code, &appCode, &clientB)
	require.NoError(t, err)
	require.NotNil(t, subB)
	assert.Equal(t, "B1", subB.Name, "client B's row must be untouched by client A's sync")

	subG, err := subRepo.FindByCode(ctx, code, &appCode, nil)
	require.NoError(t, err)
	require.NotNil(t, subG)
	assert.Equal(t, "G1", subG.Name, "the global row must be untouched by client A's sync")
}

// TestSyncSubscriptions_RemoveUnlisted_ScopedToApplicationAndClient is the
// single most important test in this feature: it seeds five pre-existing
// rows that all look like plausible deletion candidates — same code, various
// (application, client, source) combinations — and proves a sync targeting
// exactly one (application, client) pair with RemoveUnlisted removes ONLY the
// one row in that exact scope, never a sibling client's row, the
// application's global (client-less) row, another application's row, or a
// UI-authored row even inside the deletion scope. It then mirrors the case
// for a client-less sync, proving backward compatibility: a sync with no
// clientId behaves exactly as before — RemoveUnlisted only ever sweeps the
// application's global partition.
func TestSyncSubscriptions_RemoveUnlisted_ScopedToApplicationAndClient(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	subRepo := subscription.NewRepository(pool)
	connRepo := connection.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)

	// application_code feeds aud_logs.entity_id (VARCHAR(17)) via the rollup
	// event's Subject(), so these must fit that width.
	appCode := "rmsub-app-main"
	otherAppCode := "rmsub-app-other" // application_code has no FK
	clientA := "cli_rmsubscope_a"
	clientB := "cli_rmsubscope_b"
	const sharedCode = "rmsubscope-code"

	// The one row that must be removed: (this app, client A), API-sourced,
	// not listed in the sync payload.
	insertRawSubscription(t, pool, "sub_rmsubscope01", sharedCode, "Target", &appCode, &clientA, "API")
	// A UI row at the SAME (app, client) scope, same code family but its own
	// code (can't collide on the unique key) — must survive even though it's
	// inside the exact scope RemoveUnlisted sweeps.
	insertRawSubscription(t, pool, "sub_rmsubscope02", "rmsubscope-ui-kept", "UI Kept", &appCode, &clientA, "UI")
	// Same app, DIFFERENT client — must survive.
	insertRawSubscription(t, pool, "sub_rmsubscope03", sharedCode, "Client B row", &appCode, &clientB, "API")
	// Same app, NO client (the application's global partition) — must survive.
	insertRawSubscription(t, pool, "sub_rmsubscope04", sharedCode, "No client row", &appCode, nil, "API")
	// DIFFERENT application, same client — must survive.
	insertRawSubscription(t, pool, "sub_rmsubscope05", sharedCode, "Other app row", &otherAppCode, &clientA, "API")

	result, err := usecaseop.Run(appAccessCtx(), testpg.NewUoW(t),
		operations.SyncSubscriptions(subRepo, connRepo, poolRepo),
		operations.SyncSubscriptionsCommand{
			ApplicationCode: appCode,
			ClientID:        &clientA,
			Subscriptions:   nil, // nothing listed: everything in scope is "unlisted"
			RemoveUnlisted:  true,
		}, testpg.TestEC())
	require.NoError(t, err)
	assert.Equal(t, uint32(0), result.Created)
	assert.Equal(t, uint32(0), result.Updated)
	assert.Equal(t, uint32(1), result.Deleted, "exactly one row is in the deletion scope")

	gone, err := subRepo.FindByCode(ctx, sharedCode, &appCode, &clientA)
	require.NoError(t, err)
	assert.Nil(t, gone, "the unlisted API row in the exact synced scope must be removed")

	uiKept, err := subRepo.FindByCode(ctx, "rmsubscope-ui-kept", &appCode, &clientA)
	require.NoError(t, err)
	require.NotNil(t, uiKept, "a UI row in the same scope must survive")
	assert.Equal(t, subscription.SourceUI, uiKept.Source)

	clientBRow, err := subRepo.FindByCode(ctx, sharedCode, &appCode, &clientB)
	require.NoError(t, err)
	require.NotNil(t, clientBRow, "a sibling client's row must survive")

	noClientRow, err := subRepo.FindByCode(ctx, sharedCode, &appCode, nil)
	require.NoError(t, err)
	require.NotNil(t, noClientRow, "the application's global (client-less) row must survive")

	otherAppRow, err := subRepo.FindByCode(ctx, sharedCode, &otherAppCode, &clientA)
	require.NoError(t, err)
	require.NotNil(t, otherAppRow, "another application's row must survive")

	// ── Mirror case: a client-less sync only ever sweeps the global partition ──
	result2, err := usecaseop.Run(appAccessCtx(), testpg.NewUoW(t),
		operations.SyncSubscriptions(subRepo, connRepo, poolRepo),
		operations.SyncSubscriptionsCommand{
			ApplicationCode: appCode,
			Subscriptions:   nil,
			RemoveUnlisted:  true,
		}, testpg.TestEC())
	require.NoError(t, err)
	assert.Equal(t, uint32(1), result2.Deleted, "the client-less sync removes only the global row")

	goneGlobal, err := subRepo.FindByCode(ctx, sharedCode, &appCode, nil)
	require.NoError(t, err)
	assert.Nil(t, goneGlobal, "the global row must now be gone")

	stillClientB, err := subRepo.FindByCode(ctx, sharedCode, &appCode, &clientB)
	require.NoError(t, err)
	require.NotNil(t, stillClientB, "client B's row must survive the client-less sync")

	stillOtherApp, err := subRepo.FindByCode(ctx, sharedCode, &otherAppCode, &clientA)
	require.NoError(t, err)
	require.NotNil(t, stillOtherApp, "another application's row must survive the client-less sync")
}

// TestSyncSubscriptions_Authorization_ClientScope covers the remaining two
// authorization branches TestSyncSubscriptions_RequiresAppAccess doesn't: a
// client named in the request that the caller cannot access is forbidden
// even though the caller can reach the application, and a non-anchor caller
// with application access and NO client is allowed (mirrors
// connection.TestSyncConnections_Authorization's ruling).
func TestSyncSubscriptions_Authorization_ClientScope(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	subRepo := subscription.NewRepository(pool)
	connRepo := connection.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)

	run := func(ctx context.Context, cmd operations.SyncSubscriptionsCommand) (operations.SubscriptionsSynced, error) {
		return usecaseop.Run(ctx, testpg.NewUoW(t), operations.SyncSubscriptions(subRepo, connRepo, poolRepo), cmd, testpg.TestEC())
	}
	bindings := []operations.SyncEventTypeBindingInput{{EventTypeCode: "subauthcli:a:b:c"}}

	otherClient := "cli_subauthcli_oth"
	appOnlyCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_subauthcli_apponly", Scope: auth.ScopeClient, Applications: []string{"app_subauthcli"},
	})
	_, err := run(appOnlyCtx, operations.SyncSubscriptionsCommand{
		ApplicationID: "app_subauthcli", ApplicationCode: "subauthcli-app", ClientID: &otherClient,
		Subscriptions: []operations.SyncSubscriptionInput{{Code: "subauthcli-x", Name: "X", Target: "https://x.example.test", EventTypes: bindings}},
	})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")

	// Ruling (2026-09-21): a NON-anchor caller with application access and NO
	// client is allowed — a client-less subscription sync needs no anchor
	// tier, exactly like the connection sync (ownership, not reach, is the
	// fence).
	_, err = run(appOnlyCtx, operations.SyncSubscriptionsCommand{
		ApplicationID: "app_subauthcli", ApplicationCode: "subauthcli-app",
		Subscriptions: []operations.SyncSubscriptionInput{{Code: "subauthcli-allowed", Name: "X", Target: "https://x.example.test", EventTypes: bindings}},
	})
	require.NoError(t, err)
}

// TestSyncSubscriptions_RequiresAppAccess proves the use case's resource-level
// authorization: a principal without access to the target application is
// denied before any write (the coarse "may sync" permission is the
// controller's separate gate).
func TestSyncSubscriptions_RequiresAppAccess(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	subRepo := subscription.NewRepository(pool)
	connRepo := connection.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)
	uow := testpg.NewUoW(t)

	noAccessCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_noappaccess", Scope: auth.ScopeClient, Applications: []string{"app_other"},
	})
	_, err := usecaseop.Run(noAccessCtx, uow, operations.SyncSubscriptions(subRepo, connRepo, poolRepo),
		operations.SyncSubscriptionsCommand{
			ApplicationID:   "app_subsyncnoaccess",
			ApplicationCode: "subsyncnoaccess",
			Subscriptions: []operations.SyncSubscriptionInput{
				{
					Code: "subsync-x", Name: "X", Target: "https://x.example.test",
					EventTypes: []operations.SyncEventTypeBindingInput{{EventTypeCode: "subsync:a:b:c"}},
				},
			},
		}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
}
