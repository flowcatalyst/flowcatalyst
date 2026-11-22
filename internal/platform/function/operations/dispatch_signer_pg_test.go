//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// seedSignerConn persists a shared (application-less, client-less) connection
// code signing with account id.
func (f *testFunctionFixture) seedSignerConn(code, accountID string) {
	f.t.Helper()
	c := connection.New(code, code, accountID)
	tx, err := f.pool.Begin(context.Background())
	require.NoError(f.t, err)
	require.NoError(f.t, connection.NewRepository(f.pool).Persist(context.Background(), c, usecasepgx.WrapTxForBootstrap(tx)))
	require.NoError(f.t, tx.Commit(context.Background()))
	f.t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM msg_connections WHERE id = $1`, c.ID)
	})
}

// seedForeignAccount persists an account of application appID (nil: of no
// application).
func (f *testFunctionFixture) seedForeignAccount(code string, appID *string) *serviceaccount.ServiceAccount {
	f.t.Helper()
	sa := serviceaccount.New(code, code)
	sa.ApplicationID = ids.PtrOf[ids.ApplicationID](appID)
	tx, err := f.pool.Begin(context.Background())
	require.NoError(f.t, err)
	require.NoError(f.t, f.sas.Persist(context.Background(), sa, usecasepgx.WrapTxForBootstrap(tx)))
	require.NoError(f.t, tx.Commit(context.Background()))
	f.t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM iam_service_accounts WHERE id = $1`, sa.ID)
	})
	return sa
}

// Owner's decision D4 (2026-11-22), at publish: a declared dispatch whose
// connection exists and signs with an account the function may not sign as
// (another application's, or one of no application that is not its own) is
// refused 403 DISPATCH_SIGNER_NOT_ALLOWED and no version is stored; its
// application's account, and a connection that does not exist yet, publish.
func TestPublish_DispatchSigner(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optpubsign001", "optpubsignapp", "sign-fn")
	seedApplication(t, f.pool, "app_optpubsign002", "optpubsignother")
	other := "app_optpubsign002"

	mine := f.seedActiveServiceAccount("optpubsign-mine")
	theirs := f.seedForeignAccount("optpubsign-theirs", &other)
	shared := f.seedForeignAccount("optpubsign-shared", nil)
	f.seedSignerConn("optpubsign-c-mine", mine.ID)
	f.seedSignerConn("optpubsign-c-theirs", theirs.ID)
	f.seedSignerConn("optpubsign-c-shared", shared.ID)

	publish := func(conn string) error {
		src := jsArtifact(`{"abi":1,"endpoints":[{"path":"/x","auth":"none"}],"dispatches":[` +
			`{"target":"optpubsignapp.logger","path":"/deliver","connection":"` + conn + `"}]}`)
		_, err := usecaseop.RunTx(operatorCtx(), f.uow, operations.Publish(f.repo, f.artifacts, f.loader, testDispatch(f.pool)),
			operations.PublishCommand{FunctionID: f.fn.ID, Digest: putArtifactBytes(t, f.artifacts, src), Runtime: runtimes.JS}, testpg.TestEC())
		return err
	}
	versions := func() int {
		vs, err := f.repo.ListVersionsByFunction(context.Background(), f.fn.ID)
		require.NoError(t, err)
		return len(vs)
	}

	for _, refused := range []string{"optpubsign-c-theirs", "optpubsign-c-shared"} {
		err := publish(refused)
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, operations.DispatchSignerNotAllowed)
		assert.Contains(t, err.Error(), refused)
	}
	assert.Zero(t, versions(), "a refused publish stores no version")

	require.NoError(t, publish("optpubsign-c-mine"))
	require.NoError(t, publish("optpubsign-c-not-yet"), "a connection that does not exist yet is checked per call")
	assert.Equal(t, 2, versions())

	// The function's own account, bound to no application, may sign.
	_, err := f.pool.Exec(context.Background(), `UPDATE fng_functions SET service_account_id = $1 WHERE id = $2`, shared.ID, f.fn.ID)
	require.NoError(t, err)
	require.NoError(t, publish("optpubsign-c-shared"))
}
