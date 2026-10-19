//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// seedPlainAccount inserts an account NOT linked to any application (the np
// shape: a plain account), optionally scoped to clients and optionally inactive.
func seedPlainAccount(t *testing.T, f *testFunctionFixture, code string, clients []string, active bool) *serviceaccount.ServiceAccount {
	t.Helper()
	sa := serviceaccount.New(code, code)
	secret := "sig_" + code
	sa.WebhookCredentials = serviceaccount.WebhookCredentials{AuthType: serviceaccount.AuthHMAC, SigningSecret: &secret}
	if clients != nil {
		sa.ClientIDs = clients
	}
	sa.Active = active
	tx, err := f.pool.Begin(context.Background())
	require.NoError(t, err)
	require.NoError(t, f.sas.Persist(context.Background(), sa, usecasepgx.WrapTxForBootstrap(tx)))
	require.NoError(t, tx.Commit(context.Background()))
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM iam_service_accounts WHERE id = $1`, sa.ID)
	})
	return sa
}

func TestFunctionServiceAccount_SetClearAndValidate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newTestFunctionFixture(t, "app_fnsacmd00001", "fnsacmdapp", "sa-fn")
	acct := seedPlainAccount(t, f, "fnsa-plain-1", nil, true)
	inactive := seedPlainAccount(t, f, "fnsa-inactive-1", nil, false)

	require.Nil(t, f.fn.ServiceAccountID, "unset by default")

	// Set by CODE: stored as the id.
	code := acct.Code
	ev, err := runAuthorized(f.uow, operations.UpdateFunction(f.repo), operations.UpdateCommand{ID: f.fn.ID, ServiceAccount: &code})
	require.NoError(t, err)
	assert.Equal(t, &acct.ID, ev.ServiceAccountID)
	assert.Nil(t, ev.PreviousServiceAccountID)
	got, err := f.repo.FindByID(ctx, f.fn.ID)
	require.NoError(t, err)
	require.NotNil(t, got.ServiceAccountID)
	assert.Equal(t, acct.ID, *got.ServiceAccountID)

	// The event payload reports the change.
	data, err := ev.ToDataJSON()
	require.NoError(t, err)
	assert.Contains(t, string(data), `"serviceAccountChange":{"from":null,"to":"`+acct.ID+`"}`)

	// Set by ID is the same; an unrelated update leaves it alone.
	desc := "d"
	_, err = runAuthorized(f.uow, operations.UpdateFunction(f.repo), operations.UpdateCommand{ID: f.fn.ID, Description: &desc})
	require.NoError(t, err)
	got, _ = f.repo.FindByID(ctx, f.fn.ID)
	require.NotNil(t, got.ServiceAccountID, "an unrelated update must not clear the account")

	// Validation failures leave the stored value untouched.
	for _, tc := range []struct {
		name, value, code string
		kind              usecase.Kind
	}{
		{"unknown", "no-such-account", "SERVICE_ACCOUNT_NOT_FOUND", usecase.KindValidation},
		{"inactive", inactive.Code, "SERVICE_ACCOUNT_INACTIVE", usecase.KindValidation},
		{"empty", "  ", "SERVICE_ACCOUNT_REQUIRED", usecase.KindValidation},
	} {
		v := tc.value
		_, err := runAuthorized(f.uow, operations.UpdateFunction(f.repo), operations.UpdateCommand{ID: f.fn.ID, ServiceAccount: &v})
		testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
	}
	both := acct.Code
	_, err = runAuthorized(f.uow, operations.UpdateFunction(f.repo), operations.UpdateCommand{ID: f.fn.ID, ServiceAccount: &both, ClearServiceAccount: true})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SERVICE_ACCOUNT_CONFLICT")
	got, _ = f.repo.FindByID(ctx, f.fn.ID)
	require.NotNil(t, got.ServiceAccountID)
	assert.Equal(t, acct.ID, *got.ServiceAccountID)

	// Clear returns the function to the application fallback.
	ev, err = runAuthorized(f.uow, operations.UpdateFunction(f.repo), operations.UpdateCommand{ID: f.fn.ID, ClearServiceAccount: true})
	require.NoError(t, err)
	assert.Nil(t, ev.ServiceAccountID)
	require.NotNil(t, ev.PreviousServiceAccountID)
	got, _ = f.repo.FindByID(ctx, f.fn.ID)
	assert.Nil(t, got.ServiceAccountID)
}

func TestFunctionServiceAccount_AtCreate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newTestFunctionFixture(t, "app_fnsacrt00001", "fnsacrtapp", "sa-seed")
	acct := seedPlainAccount(t, f, "fnsa-create-1", nil, true)

	code := acct.Code
	ev, err := runAuthorized(f.uow, operations.CreateFunction(f.repo, application.NewRepository(f.pool)),
		operations.CreateCommand{ApplicationID: f.appID, Name: "with-account", ServiceAccount: &code})
	require.NoError(t, err)
	got, err := f.repo.FindByID(ctx, ev.FunctionID)
	require.NoError(t, err)
	require.NotNil(t, got.ServiceAccountID)
	assert.Equal(t, acct.ID, *got.ServiceAccountID)

	bad := "ghost"
	_, err = runAuthorized(f.uow, operations.CreateFunction(f.repo, application.NewRepository(f.pool)),
		operations.CreateCommand{ApplicationID: f.appID, Name: "bad-account", ServiceAccount: &bad})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SERVICE_ACCOUNT_NOT_FOUND")
	missing, err := f.repo.FindByAddress(ctx, function.BuildAddress("fnsacrtapp", "bad-account"))
	require.NoError(t, err)
	assert.Nil(t, missing, "a rejected create persists nothing")

	// Omitted: nil (the application fallback).
	ev2 := mustCreate(t, f.repo, f.apps, f.uow, f.appID, "no-account")
	got2, _ := f.repo.FindByID(ctx, ev2.FunctionID)
	assert.Nil(t, got2.ServiceAccountID)
}

func TestFunctionServiceAccount_CallerReach(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_fnsarch00001", "fnsarchapp", "sa-reach")
	own, other := "cli_fnsarch00001", "cli_fnsarch00002"
	platformAcct := seedPlainAccount(t, f, "fnsa-platform-r", nil, true)
	ownAcct := seedPlainAccount(t, f, "fnsa-own-r", []string{own}, true)
	otherAcct := seedPlainAccount(t, f, "fnsa-other-r", []string{other}, true)

	ev, err := usecaseop.Run(testpg.AnchorCtx(), f.uow, operations.CreateFunction(f.repo, f.apps),
		operations.CreateCommand{ApplicationID: f.appID, Name: "client-fn", ClientID: &own}, testpg.TestEC())
	require.NoError(t, err)

	clientCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_fnsarch00001", Scope: auth.ScopeClient, Clients: []string{own},
		Permissions: []string{"platform:function:function:manage"}, AllApplications: true,
	})
	set := func(code string) error {
		_, err := usecaseop.Run(clientCtx, f.uow, operations.UpdateFunction(f.repo),
			operations.UpdateCommand{ID: ev.FunctionID, ServiceAccount: &code}, testpg.TestEC())
		return err
	}
	testpg.RequireUsecaseError(t, set(platformAcct.Code), usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	testpg.RequireUsecaseError(t, set(otherAcct.Code), usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	require.NoError(t, set(ownAcct.Code))
}
