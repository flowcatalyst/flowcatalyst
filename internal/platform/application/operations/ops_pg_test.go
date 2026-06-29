//go:build integration

package operations_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// TestMain seeds FLOWCATALYST_APP_KEY before the embedded-PG boot: provisioning
// a service account creates a CONFIDENTIAL OAuth client whose secret is
// encrypted via encryption.FromEnv, which reads the env at call time. os.Setenv
// (not t.Setenv) because the tests here run t.Parallel().
func TestMain(m *testing.M) {
	key, err := encryption.GenerateKey()
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("FLOWCATALYST_APP_KEY", key)
	testpg.RunMain(m)
}

func ptr(s string) *string { return &s }

// runAuthorized drives op through the full use-case envelope (Validate →
// Authorize → Execute → atomic commit) as an anchor principal — the common
// case for these tests, which exercise validation, invariants, and
// persistence rather than authorization itself. It mirrors how the HTTP
// handler runs the operation.
func runAuthorized[C any, E usecase.DomainEvent](
	uow *usecasepgx.UnitOfWork, op usecaseop.Operation[C, E], cmd C,
) (E, error) {
	return usecaseop.Run(testpg.AnchorCtx(), uow, op, cmd, testpg.TestEC())
}

// mustCreateApp seeds an application through the public operation — the
// same path production uses. Codes are hand-unique per test: the fixture
// never truncates between tests, so tests own their rows and never assert
// table-wide.
func mustCreateApp(t *testing.T, repo *application.Repository, uow *usecasepgx.UnitOfWork, code, name string) operations.ApplicationCreated {
	t.Helper()
	ev, err := runAuthorized(uow, operations.CreateApplication(repo),
		operations.CreateCommand{Code: code, Name: name})
	require.NoError(t, err)
	return ev
}

// mustCreateClient seeds a real client via client/operations — the
// enable/disable ops verify the client row exists, so a fabricated id
// won't do.
func mustCreateClient(t *testing.T, uow *usecasepgx.UnitOfWork, name, identifier string) clientops.ClientCreated {
	t.Helper()
	repo := client.NewRepository(testpg.Pool(t))
	ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(repo),
		clientops.CreateCommand{Name: name, Identifier: identifier}, testpg.TestEC())
	require.NoError(t, err)
	return ev
}

// ── Create ────────────────────────────────────────────────────────────────

func TestCreateApplication_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	ev, err := runAuthorized(uow, operations.CreateApplication(repo), operations.CreateCommand{
		Code:        "  AppCreate-Happy1  ",
		Name:        "  First App  ",
		Description: ptr("the first"),
		Website:     ptr("https://example.com"),
	})
	require.NoError(t, err)

	assert.NotEmpty(t, ev.ApplicationID)
	assert.Equal(t, "appcreate-happy1", ev.Code, "code is lowercased + trimmed")
	assert.Equal(t, "First App", ev.Name, "name is trimmed")

	got, err := repo.FindByID(ctx, ev.ApplicationID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "appcreate-happy1", got.Code)
	assert.Equal(t, "First App", got.Name)
	assert.Equal(t, application.TypeApplication, got.Type, "default type is APPLICATION")
	assert.True(t, got.Active, "new applications start active")
	require.NotNil(t, got.Description)
	assert.Equal(t, "the first", *got.Description)
	require.NotNil(t, got.Website)
	assert.Equal(t, "https://example.com", *got.Website)
}

// Underscores are explicitly allowed (real codes like logistics_portal use
// them — the Rust reference enforced no pattern at all).
func TestCreateApplication_UnderscoreCodeAllowed(t *testing.T) {
	t.Parallel()
	repo := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	ev := mustCreateApp(t, repo, uow, "app_with_underscores", "Underscore App")
	assert.Equal(t, "app_with_underscores", ev.Code)

	got, err := repo.FindByCode(context.Background(), "app_with_underscores")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, ev.ApplicationID, got.ID)
}

func TestCreateApplication_Validation(t *testing.T) {
	t.Parallel()
	repo := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.CreateCommand
		code string
	}{
		{"missing code", operations.CreateCommand{Name: "X"}, "CODE_REQUIRED"},
		{"leading digit", operations.CreateCommand{Code: "1bad", Name: "X"}, "INVALID_CODE_FORMAT"},
		{"missing name", operations.CreateCommand{Code: "appcrtval"}, "NAME_REQUIRED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.CreateApplication(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
}

// Conflict is pinned by seeding through the operation itself: the first
// create IS the seed for the second.
func TestCreateApplication_DuplicateCode_Conflict(t *testing.T) {
	t.Parallel()
	repo := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	mustCreateApp(t, repo, uow, "appdup1", "First")

	_, err := runAuthorized(uow, operations.CreateApplication(repo),
		operations.CreateCommand{Code: "appdup1", Name: "Second"})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "CODE_EXISTS")
}

// ── Update ────────────────────────────────────────────────────────────────

func TestUpdateApplication_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreateApp(t, repo, uow, "appupd1", "Before")

	ev, err := runAuthorized(uow, operations.UpdateApplication(repo), operations.UpdateCommand{
		ID:          seeded.ApplicationID,
		Name:        ptr("  After  "),
		Description: ptr("after"),
	})
	require.NoError(t, err)
	assert.Equal(t, seeded.ApplicationID, ev.ApplicationID)
	assert.Equal(t, "After", ev.Name)

	got, err := repo.FindByID(ctx, seeded.ApplicationID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "After", got.Name, "name is trimmed")
	require.NotNil(t, got.Description)
	assert.Equal(t, "after", *got.Description)
	assert.Equal(t, "appupd1", got.Code, "code is immutable on update")
}

func TestUpdateApplication_Errors(t *testing.T) {
	t.Parallel()
	repo := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.UpdateCommand
		kind usecase.Kind
		code string
	}{
		{"missing id", operations.UpdateCommand{Name: ptr("X")}, usecase.KindValidation, "ID_REQUIRED"},
		{"blank name", operations.UpdateCommand{ID: "app_doesnotexist1", Name: ptr("  ")}, usecase.KindValidation, "NAME_REQUIRED"},
		{"unknown id", operations.UpdateCommand{ID: "app_doesnotexist1", Name: ptr("X")}, usecase.KindNotFound, "Application_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.UpdateApplication(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

// ── Delete ────────────────────────────────────────────────────────────────

func TestDeleteApplication_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreateApp(t, repo, uow, "appdel1", "Doomed")

	ev, err := runAuthorized(uow, operations.DeleteApplication(repo),
		operations.DeleteCommand{ID: seeded.ApplicationID})
	require.NoError(t, err)
	assert.Equal(t, seeded.ApplicationID, ev.ApplicationID)
	assert.Equal(t, "appdel1", ev.Code)

	got, err := repo.FindByID(ctx, seeded.ApplicationID)
	require.NoError(t, err)
	assert.Nil(t, got, "deleted row must be gone")
}

func TestDeleteApplication_Errors(t *testing.T) {
	t.Parallel()
	repo := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.DeleteApplication(repo),
		operations.DeleteCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.DeleteApplication(repo),
		operations.DeleteCommand{ID: "app_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Application_NOT_FOUND")
}

// ── Activate / Deactivate ─────────────────────────────────────────────────

func TestDeactivateAndActivateApplication_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreateApp(t, repo, uow, "appact1", "Toggle Me")

	deactivated, err := runAuthorized(uow, operations.DeactivateApplication(repo),
		operations.DeactivateCommand{ID: seeded.ApplicationID})
	require.NoError(t, err)
	assert.Equal(t, seeded.ApplicationID, deactivated.ApplicationID)

	got, err := repo.FindByID(ctx, seeded.ApplicationID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.False(t, got.Active, "deactivate must flip Active → false")

	reactivated, err := runAuthorized(uow, operations.ActivateApplication(repo),
		operations.ActivateCommand{ID: seeded.ApplicationID})
	require.NoError(t, err)
	assert.Equal(t, seeded.ApplicationID, reactivated.ApplicationID)

	got, err = repo.FindByID(ctx, seeded.ApplicationID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Active, "activate must flip Active → true")
}

func TestActivateApplication_Errors(t *testing.T) {
	t.Parallel()
	repo := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.ActivateApplication(repo),
		operations.ActivateCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.ActivateApplication(repo),
		operations.ActivateCommand{ID: "app_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Application_NOT_FOUND")
}

func TestDeactivateApplication_Errors(t *testing.T) {
	t.Parallel()
	repo := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.DeactivateApplication(repo),
		operations.DeactivateCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.DeactivateApplication(repo),
		operations.DeactivateCommand{ID: "app_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Application_NOT_FOUND")
}

// ── Enable / Disable for client ───────────────────────────────────────────

func TestEnableApplicationForClient_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	apps := application.NewRepository(pool)
	clients := client.NewRepository(pool)
	configs := application.NewClientConfigRepo(pool)
	uow := testpg.NewUoW(t)

	app := mustCreateApp(t, apps, uow, "appencl1", "Enable Me")
	cl := mustCreateClient(t, uow, "Enable Client", "appencl-client1")

	ev, err := runAuthorized(uow, operations.EnableApplicationForClient(apps, clients, configs),
		operations.EnableForClientCommand{ApplicationID: app.ApplicationID, ClientID: cl.ClientID})
	require.NoError(t, err)

	assert.Equal(t, app.ApplicationID, ev.ApplicationID)
	assert.Equal(t, cl.ClientID, ev.ClientID)
	assert.NotEmpty(t, ev.ConfigID)

	cfg, err := configs.FindByApplicationAndClient(ctx, app.ApplicationID, cl.ClientID)
	require.NoError(t, err)
	require.NotNil(t, cfg, "enable must create the app_client_configs row")
	assert.Equal(t, ev.ConfigID, cfg.ID)
	assert.True(t, cfg.Enabled)
}

func TestEnableApplicationForClient_Errors(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	apps := application.NewRepository(pool)
	clients := client.NewRepository(pool)
	configs := application.NewClientConfigRepo(pool)
	uow := testpg.NewUoW(t)

	// Application_NOT_FOUND is checked before the client lookup, so the
	// unknown-client case needs a real application.
	app := mustCreateApp(t, apps, uow, "appenclerr1", "Enable Errors")

	cases := []struct {
		name string
		cmd  operations.EnableForClientCommand
		kind usecase.Kind
		code string
	}{
		{"missing application id", operations.EnableForClientCommand{ClientID: "clt_doesnotexist1"}, usecase.KindValidation, "APPLICATION_ID_REQUIRED"},
		{"missing client id", operations.EnableForClientCommand{ApplicationID: "app_doesnotexist1"}, usecase.KindValidation, "CLIENT_ID_REQUIRED"},
		{"unknown application", operations.EnableForClientCommand{ApplicationID: "app_doesnotexist1", ClientID: "clt_doesnotexist1"}, usecase.KindNotFound, "Application_NOT_FOUND"},
		{"unknown client", operations.EnableForClientCommand{ApplicationID: app.ApplicationID, ClientID: "clt_doesnotexist1"}, usecase.KindNotFound, "Client_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.EnableApplicationForClient(apps, clients, configs), tc.cmd)
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

func TestDisableApplicationForClient_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	apps := application.NewRepository(pool)
	clients := client.NewRepository(pool)
	configs := application.NewClientConfigRepo(pool)
	uow := testpg.NewUoW(t)

	app := mustCreateApp(t, apps, uow, "appdiscl1", "Disable Me")
	cl := mustCreateClient(t, uow, "Disable Client", "appdiscl-client1")

	enabled, err := runAuthorized(uow, operations.EnableApplicationForClient(apps, clients, configs),
		operations.EnableForClientCommand{ApplicationID: app.ApplicationID, ClientID: cl.ClientID})
	require.NoError(t, err)

	disabled, err := runAuthorized(uow, operations.DisableApplicationForClient(configs),
		operations.DisableForClientCommand{ApplicationID: app.ApplicationID, ClientID: cl.ClientID})
	require.NoError(t, err)
	assert.Equal(t, enabled.ConfigID, disabled.ConfigID,
		"disable flips the SAME config row, not a new one")

	cfg, err := configs.FindByApplicationAndClient(ctx, app.ApplicationID, cl.ClientID)
	require.NoError(t, err)
	require.NotNil(t, cfg, "disable keeps the row (soft flag, not delete)")
	assert.False(t, cfg.Enabled)

	// Round-trip: re-enabling flips the existing disabled row back.
	reenabled, err := runAuthorized(uow, operations.EnableApplicationForClient(apps, clients, configs),
		operations.EnableForClientCommand{ApplicationID: app.ApplicationID, ClientID: cl.ClientID})
	require.NoError(t, err)
	assert.Equal(t, cfg.ID, reenabled.ConfigID, "re-enable reuses the existing row")

	cfg, err = configs.FindByApplicationAndClient(ctx, app.ApplicationID, cl.ClientID)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.True(t, cfg.Enabled)
}

func TestDisableApplicationForClient_Errors(t *testing.T) {
	t.Parallel()
	configs := application.NewClientConfigRepo(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.DisableForClientCommand
		kind usecase.Kind
		code string
	}{
		{"missing application id", operations.DisableForClientCommand{ClientID: "clt_doesnotexist1"}, usecase.KindValidation, "APPLICATION_ID_REQUIRED"},
		{"missing client id", operations.DisableForClientCommand{ApplicationID: "app_doesnotexist1"}, usecase.KindValidation, "CLIENT_ID_REQUIRED"},
		{"no config row", operations.DisableForClientCommand{ApplicationID: "app_doesnotexist1", ClientID: "clt_doesnotexist1"}, usecase.KindNotFound, "ClientConfig_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.DisableApplicationForClient(configs), tc.cmd)
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

// ── UpdateClientApplications (bulk replace) ───────────────────────────────

func TestUpdateClientApplications_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	apps := application.NewRepository(pool)
	clients := client.NewRepository(pool)
	configs := application.NewClientConfigRepo(pool)
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	appA := mustCreateApp(t, apps, uow, "appbulk-a1", "Bulk A")
	appB := mustCreateApp(t, apps, uow, "appbulk-b1", "Bulk B")
	appC := mustCreateApp(t, apps, uow, "appbulk-c1", "Bulk C")
	cl := mustCreateClient(t, uow, "Bulk Client", "appbulk-client1")

	// enabledByApp reloads the client's configs as appID → Enabled.
	enabledByApp := func() map[string]bool {
		t.Helper()
		rows, err := configs.FindByClient(ctx, cl.ClientID)
		require.NoError(t, err)
		out := make(map[string]bool, len(rows))
		for _, row := range rows {
			out[row.ApplicationID] = row.Enabled
		}
		return out
	}

	// 1. Initial set: [A, B] — both freshly created enabled.
	first, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.UpdateClientApplications(apps, clients, configs),
		operations.UpdateClientApplicationsCommand{
			ClientID:              cl.ClientID,
			EnabledApplicationIDs: []string{appA.ApplicationID, appB.ApplicationID},
		}, ec)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{appA.ApplicationID, appB.ApplicationID}, first.EnabledAdded)
	assert.Empty(t, first.DisabledRemoved)
	assert.Equal(t, map[string]bool{appA.ApplicationID: true, appB.ApplicationID: true}, enabledByApp())

	// 2. Replace with [B, C]: A flips to disabled (row kept), B untouched,
	//    C freshly enabled.
	second, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.UpdateClientApplications(apps, clients, configs),
		operations.UpdateClientApplicationsCommand{
			ClientID:              cl.ClientID,
			EnabledApplicationIDs: []string{appB.ApplicationID, appC.ApplicationID},
		}, ec)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{appC.ApplicationID}, second.EnabledAdded)
	assert.ElementsMatch(t, []string{appA.ApplicationID}, second.DisabledRemoved)
	assert.Equal(t, map[string]bool{
		appA.ApplicationID: false,
		appB.ApplicationID: true,
		appC.ApplicationID: true,
	}, enabledByApp(), "disable keeps the row; the desired set is enabled")

	// 3. Idempotent re-apply: empty diff still emits the rollup event.
	third, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.UpdateClientApplications(apps, clients, configs),
		operations.UpdateClientApplicationsCommand{
			ClientID:              cl.ClientID,
			EnabledApplicationIDs: []string{appB.ApplicationID, appC.ApplicationID},
		}, ec)
	require.NoError(t, err)
	assert.Empty(t, third.EnabledAdded)
	assert.Empty(t, third.DisabledRemoved)
}

func TestUpdateClientApplications_Errors(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	apps := application.NewRepository(pool)
	clients := client.NewRepository(pool)
	configs := application.NewClientConfigRepo(pool)
	uow := testpg.NewUoW(t)

	// The client lookup runs before per-app validation, so the app-side
	// cases need a real client.
	cl := mustCreateClient(t, uow, "Bulk Errors Client", "appbulkerr-client1")

	cases := []struct {
		name string
		cmd  operations.UpdateClientApplicationsCommand
		kind usecase.Kind
		code string
	}{
		{"missing client id", operations.UpdateClientApplicationsCommand{}, usecase.KindValidation, "CLIENT_ID_REQUIRED"},
		{"unknown client", operations.UpdateClientApplicationsCommand{ClientID: "clt_doesnotexist1"}, usecase.KindNotFound, "Client_NOT_FOUND"},
		{"blank application id", operations.UpdateClientApplicationsCommand{ClientID: cl.ClientID, EnabledApplicationIDs: []string{"  "}}, usecase.KindValidation, "APPLICATION_ID_REQUIRED"},
		{"unknown application", operations.UpdateClientApplicationsCommand{ClientID: cl.ClientID, EnabledApplicationIDs: []string{"app_doesnotexist1"}}, usecase.KindNotFound, "Application_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.UpdateClientApplications(apps, clients, configs), tc.cmd)
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

// ── Provision service account ───────────────────────────────────────────────

// TestProvisionServiceAccount_AssignsRoleAndScopesClient locks in the three
// guarantees of application SA provisioning: the SERVICE principal is granted
// the application-service role, it is pinned to its own application (not
// all-applications), and its OAuth client is limited to that application.
func TestProvisionServiceAccount_AssignsRoleAndScopesClient(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	appRepo := application.NewRepository(pool)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)

	app := mustCreateApp(t, appRepo, uow, "appprov-sa1", "Prov SA App")

	result, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.ProvisionServiceAccount(appRepo, saRepo, principals, oauthRepo),
		operations.ProvisionServiceAccountCommand{ApplicationID: app.ApplicationID},
		testpg.TestEC())
	require.NoError(t, err)
	require.NotEmpty(t, result.ServicePrincipalID)
	require.NotEmpty(t, result.OAuthClientRowID)

	// 1 + app scope. The SERVICE principal carries the application-service role
	// and is confined to its own application.
	p, err := principals.FindByID(ctx, result.ServicePrincipalID)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.False(t, p.AllApplications, "provisioned SA is app-scoped, not all-applications")
	assert.Equal(t, []string{app.ApplicationID}, p.AccessibleApplicationIDs)
	roleNames := make([]string, 0, len(p.Roles))
	for _, ra := range p.Roles {
		roleNames = append(roleNames, ra.Role)
	}
	assert.Contains(t, roleNames, "platform:application-service",
		"SA principal is granted the application-service role at provision")

	// 2. The OAuth client is limited to the application it was provisioned under.
	oc, err := oauthRepo.FindByID(ctx, result.OAuthClientRowID)
	require.NoError(t, err)
	require.NotNil(t, oc)
	assert.Equal(t, []string{app.ApplicationID}, oc.ApplicationIDs,
		"SA OAuth client is scoped to its application")
}
