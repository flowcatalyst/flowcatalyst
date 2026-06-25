//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
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

func ptr[T any](v T) *T { return &v }

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
		ServiceAccountID: ptr("sva_subcrthappy1"),
		EventTypes: []subscription.EventTypeBinding{
			{EventTypeCode: "subcrt:orders:order:created", SpecVersion: ptr("1.0")},
			{EventTypeCode: "subcrt:orders:order:*"},
		},
		CustomConfig:   []subscription.ConfigEntry{{Key: "X-Env", Value: "test"}},
		Mode:           "BLOCK_ON_ERROR",
		TimeoutSeconds: ptr(int32(60)),
		MaxRetries:     ptr(int32(5)),
		DelaySeconds:   ptr(int32(10)),
		MaxAgeSeconds:  ptr(int32(3600)),
		DataOnly:       ptr(false),
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
	// created_by persists since migration 035 (Rust never wrote it; its
	// rows read back NULL).
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.CreateSubscription(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
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
		Name:        ptr("  After  "), // op must trim
		Description: ptr("after"),
		Endpoint:    ptr("https://after.example.test/hook"),
		EventTypes: []subscription.EventTypeBinding{
			subscription.NewEventTypeBinding("subupd:orders:order:updated"),
		},
		Mode:             ptr("NEXT_ON_ERROR"),
		TimeoutSeconds:   ptr(int32(90)),
		MaxRetries:       ptr(int32(7)),
		DelaySeconds:     ptr(int32(5)),
		MaxAgeSeconds:    ptr(int32(7200)),
		ServiceAccountID: ptr("sva_subupdafter1"),
		DataOnly:         ptr(false),
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
		{"missing id", operations.UpdateCommand{Name: ptr("X")}, usecase.KindValidation, "ID_REQUIRED"},
		{"blank name", operations.UpdateCommand{ID: "sub_doesnotexist1", Name: ptr(" ")}, usecase.KindValidation, "NAME_REQUIRED"},
		{"bad endpoint", operations.UpdateCommand{ID: "sub_doesnotexist1", Endpoint: ptr("not-a-url")}, usecase.KindValidation, "INVALID_ENDPOINT"},
		{"unknown id", operations.UpdateCommand{ID: "sub_doesnotexist1", Name: ptr("X")}, usecase.KindNotFound, "Subscription_NOT_FOUND"},
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
	poolRepo := dispatchpool.NewRepository(pool)
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()
	const appCode = "subsyncapp1"

	// Real connection for the connectionId binding. ServiceAccountID is an
	// arbitrary string — CreateConnection does not validate it.
	connEv, err := runAuthorized(uow, connops.CreateConnection(connRepo), connops.CreateCommand{
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
					DispatchPoolCode: ptr("subsync-pool1"),
				},
				{
					Code: "subsync-b", Name: "B", Target: "https://b.example.test/hook",
					EventTypes:       []operations.SyncEventTypeBindingInput{{EventTypeCode: "subsync:orders:order:updated"}},
					DispatchPoolCode: ptr("subsync-nosuchpool"),
				},
			},
		}, ec)
	require.NoError(t, err)
	assert.Equal(t, uint32(2), first.Created)
	assert.Equal(t, uint32(0), first.Updated)
	assert.Equal(t, uint32(0), first.Deleted)
	assert.Equal(t, appCode, first.ApplicationCode)
	assert.Equal(t, []string{"subsync-a", "subsync-b"}, first.SyncedCodes)

	subA, err := subRepo.FindByCode(ctx, "subsync-a", nil)
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
	subB, err := subRepo.FindByCode(ctx, "subsync-b", nil)
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

	kept, err := subRepo.FindByCode(ctx, "subsync-a", nil)
	require.NoError(t, err)
	require.NotNil(t, kept)
	assert.Equal(t, "A renamed", kept.Name)
	require.NotNil(t, kept.DispatchPoolCode, "omitted dispatchPoolCode must leave the existing pool link")
	assert.Equal(t, "subsync-pool1", *kept.DispatchPoolCode)

	goneB, err := subRepo.FindByCode(ctx, "subsync-b", nil)
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
					ConnectionID: ptr("con_doesnotexist1"),
					EventTypes:   []operations.SyncEventTypeBindingInput{{EventTypeCode: "subsync:a:b:c"}},
				},
			},
		}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "CONNECTION_NOT_FOUND")
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
