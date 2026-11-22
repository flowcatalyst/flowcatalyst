//go:build integration

package control

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Owner's decision D4 (2026-11-22): a function's dispatch may name a
// connection only if the function may sign as the connection's service
// account, its application's or its own. Any other account, a shared
// connection's included, is refused 403 DISPATCH_SIGNER_NOT_ALLOWED before
// the job is written.
func TestDispatchSigner_WhoMaySign(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	otherAppID := "app_other" + shortID(t)
	seedApplication(t, pool, otherAppID, "dsg"+shortID(t))

	for _, c := range []struct {
		name string
		// account seeds the connection's account for the fixture's
		// application appID.
		account func(t *testing.T, appID string) *serviceaccount.ServiceAccount
		// own makes the account the function's own service account.
		own bool
		// appConn makes the connection the function's application's
		// (otherwise it is shared, of no application).
		appConn bool
		refused string
	}{
		{
			name: "its application's account through a shared connection",
			account: func(t *testing.T, appID string) *serviceaccount.ServiceAccount {
				return seedServiceAccount(t, pool, saRepo, appID, "dsgapp"+shortID(t), "s")
			},
		},
		{
			name: "its application's account through its application's connection", appConn: true,
			account: func(t *testing.T, appID string) *serviceaccount.ServiceAccount {
				return seedServiceAccount(t, pool, saRepo, appID, "dsgapp2"+shortID(t), "s")
			},
		},
		{
			name: "its own account, bound to no application", own: true,
			account: func(t *testing.T, _ string) *serviceaccount.ServiceAccount {
				return seedPlainServiceAccount(t, "dsgown"+shortID(t), "s", true)
			},
		},
		{
			name: "another application's account through a shared connection", refused: "belongs to another application",
			account: func(t *testing.T, _ string) *serviceaccount.ServiceAccount {
				return seedServiceAccount(t, pool, saRepo, otherAppID, "dsgother"+shortID(t), "s")
			},
		},
		{
			name: "another application's account through its application's connection", appConn: true, refused: "belongs to another application",
			account: func(t *testing.T, _ string) *serviceaccount.ServiceAccount {
				return seedServiceAccount(t, pool, saRepo, otherAppID, "dsgother2"+shortID(t), "s")
			},
		},
		{
			name: "an account of no application that is not its own", refused: "belongs to no application",
			account: func(t *testing.T, _ string) *serviceaccount.ServiceAccount {
				return seedPlainServiceAccount(t, "dsgshared"+shortID(t), "s", true)
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			code := connCode(t, "signer")
			f := newDeclFixture(t, "", code, nil, nil)
			sa := c.account(t, f.appID)
			if c.own {
				_, err := pool.Exec(context.Background(), `UPDATE fng_functions SET service_account_id = $1 WHERE id = $2`, sa.ID, f.creator.ID)
				require.NoError(t, err)
			}
			conn := seedConn(t, f, code, sa.ID, func(cn *connection.Connection) {
				if c.appConn {
					cn.ApplicationCode = &f.app
				}
			})

			out, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
			if c.refused == "" {
				require.NoError(t, err)
				got := f.jobConn(t, out.Body.JobID)
				require.NotNil(t, got)
				assert.Equal(t, conn.ID, *got)
				return
			}
			testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "DISPATCH_SIGNER_NOT_ALLOWED")
			assert.Contains(t, err.Error(), c.refused)
			assert.Contains(t, err.Error(), code, "the refusal names the connection")
			assert.Zero(t, f.jobsFrom(t), "a refused dispatch writes no job")
		})
	}
}

// A connection whose account does not exist signs nothing (its delivery is
// declined with that reason), so it is not refused here; nor is a paused
// connection's, whose job waits.
func TestDispatchSigner_AnAccountThatDoesNotExistIsNotRefused(t *testing.T) {
	t.Parallel()
	code := connCode(t, "ghostsa")
	f := newDeclFixture(t, "", code, nil, nil)
	seedConn(t, f, code, "sac_nosuch"+shortID(t), func(c *connection.Connection) { c.Status = connection.StatusPaused })
	_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
	require.NoError(t, err)
}

// The refusal reaches the guest as NOT_ALLOWED (a 403 on the wire), naming the
// connection.
func TestDispatchSigner_RefusalReachesTheGuest(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	otherAppID := "app_other" + shortID(t)
	seedApplication(t, pool, otherAppID, "dsg"+shortID(t))
	theirs := seedServiceAccount(t, pool, serviceaccount.NewRepository(pool), otherAppID, "dsgwire"+shortID(t), "s")
	code := connCode(t, "wire")
	f := newDeclFixture(t, "", code, nil, nil)
	seedConn(t, f, code, theirs.ID, nil)

	ti := newTestTokenIssuer(t)
	runner := ti.mint(t, "sa_fn_runner_dsg", []string{"platform:function:runner:control"})
	srv := newTestServer(t, f.s, ti)
	client := fncontrol.NewClient(srv.URL, func(context.Context) (string, error) { return runner, nil }, nil)
	_, aerr := client.Dispatch(context.Background(), f.req(abi.Dispatch{}))
	require.NotNil(t, aerr)
	assert.Equal(t, abi.CodeNotAllowed, aerr.Code)
	assert.Contains(t, aerr.Message, code)
	assert.Zero(t, f.jobsFrom(t))
}

func TestDispatchSigner_UnwiredRepositoryIsAnInternalError(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	code := connCode(t, "sgunwired")
	f := newDeclFixture(t, "", code, nil, nil)
	sa := seedServiceAccount(t, pool, serviceaccount.NewRepository(pool), f.appID, "dsgunw"+shortID(t), "s")
	seedConn(t, f, code, sa.ID, nil)
	f.s.ServiceAccounts = nil
	_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}))
	testpg.RequireUsecaseError(t, err, usecase.KindInternal, "DISPATCH_UNWIRED")
	assert.Zero(t, f.jobsFrom(t))
}
