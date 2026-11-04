//go:build integration

package control

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	saops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

const tokenTestSecret = "function-token-test-secret-function-token-test-secret"

// tokenFixture is an application with a linked service account, a function of
// that application declaring (or not) platformToken, and a State that can mint.
type tokenFixture struct {
	s   *State
	svc *authservice.AuthService
	// appSA is the application's linked service account; appPrincipal its
	// SERVICE principal (the token's subject).
	appSA, appPrincipal string
	app                 string
	fn                  *function.Function
}

func tokenDescribe(t *testing.T, declared bool) json.RawMessage {
	t.Helper()
	d := map[string]any{"abi": 1, "endpoints": []map[string]any{{"path": "/x", "auth": "none"}}}
	if declared {
		d["platformToken"] = true
	}
	b, err := json.Marshal(d)
	require.NoError(t, err)
	return b
}

// newServiceAccountWithPrincipal provisions an account and its SERVICE principal.
func newServiceAccountWithPrincipal(t *testing.T, code string) (saID, principalID string) {
	t.Helper()
	pool := testpg.Pool(t)
	res, err := usecaseop.RunTx(testpg.AnchorCtx(), testpg.NewUoW(t),
		saops.CreateServiceAccountWithCredentials(serviceaccount.NewRepository(pool), principal.NewRepository(pool),
			platformauth.NewRepository(pool).OAuthClients, client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
		saops.CreateCommand{Code: code, Name: code}, testpg.TestEC())
	require.NoError(t, err)
	return res.ServiceAccount.ID, res.PrincipalID
}

// newTokenFixture seeds the application, its service account (linked
// unless noLinkedAccount) and one function; mutateFn adjusts the function
// before it is persisted.
func newTokenFixture(t *testing.T, declared, noLinkedAccount bool, mutateFn func(*function.Function)) *tokenFixture {
	t.Helper()
	pool := testpg.Pool(t)
	s := newTestState(t)
	s.Apps = application.NewRepository(pool)
	svc := authservice.NewWithSecret(authservice.Config{SecretKey: tokenTestSecret, Issuer: "https://fc.test", Audience: "https://fc.test"})
	s.Mint = saops.MintDeps{
		Repo: serviceaccount.NewRepository(pool), Principals: principal.NewRepository(pool), Roles: role.NewRepository(pool),
		Auth: svc, Audit: audit.NewRepository(pool),
		FlattenPermissions: func(context.Context, []string) ([]string, error) { return []string{"integral:visit:plan"}, nil },
	}

	appID, app := "app_"+shortID(t), "tok"+shortID(t)
	seedApplication(t, pool, appID, app)
	f := &tokenFixture{s: s, svc: svc, app: app}
	if !noLinkedAccount {
		f.appSA, f.appPrincipal = newServiceAccountWithPrincipal(t, "tok-app-"+shortID(t))
		_, err := pool.Exec(context.Background(), `UPDATE app_applications SET service_account_id = $2 WHERE id = $1`, appID, f.appPrincipal)
		require.NoError(t, err)
	}
	f.fn = seedFunction(t, pool, s.Repo, appID, app, "caller", mutateFn)
	seedVersion(t, s.Repo, f.fn.ID, 1, function.VersionPublished, "js", tokenDescribe(t, declared))
	return f
}

func (f *tokenFixture) call(ctx context.Context, r fncontrol.TokenRequest) (*apicommon.Out[FunctionTokenResponse], error) {
	return f.s.token(ctx, &apicommon.In[FunctionTokenRequest]{Body: FunctionTokenRequest(r)})
}

func (f *tokenFixture) req() fncontrol.TokenRequest {
	return fncontrol.TokenRequest{FunctionID: f.fn.ID, Version: 1}
}

func TestToken_Success(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, true, false, nil)
	out, err := f.call(anchorCtx(), f.req())
	require.NoError(t, err)
	assert.Equal(t, "Bearer", out.Body.TokenType)
	assert.EqualValues(t, 3600, out.Body.ExpiresIn)
	require.NotEmpty(t, out.Body.AccessToken)

	claims, err := f.svc.ValidateToken(out.Body.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, f.appPrincipal, claims.Subject, "the token is the application's service account's")
	assert.Equal(t, authservice.TokenUseAPI, claims.TokenUse, "authority-bearing, as the client_credentials grant mints")
}

// The identity is the APPLICATION's account even when the function names a
// webhook-verifier account of its own.
func TestToken_IsTheApplicationsAccountNotTheFunctions(t *testing.T) {
	t.Parallel()
	otherSA, otherPrincipal := newServiceAccountWithPrincipal(t, "tok-webhook-"+shortID(t))
	f := newTokenFixture(t, true, false, func(fn *function.Function) { fn.ServiceAccountID = &otherSA })
	require.NotEqual(t, f.appSA, otherSA)

	out, err := f.call(anchorCtx(), f.req())
	require.NoError(t, err)
	claims, err := f.svc.ValidateToken(out.Body.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, f.appPrincipal, claims.Subject)
	assert.NotEqual(t, otherPrincipal, claims.Subject, "the function's webhook account must not supply the identity")
}

// Every row of the plan's check table, in order.
func TestToken_Refusals(t *testing.T) {
	t.Parallel()

	t.Run("functionId empty is 400 FUNCTION_ID_REQUIRED", func(t *testing.T) {
		f := newTokenFixture(t, true, false, nil)
		_, err := f.call(anchorCtx(), fncontrol.TokenRequest{Version: 1})
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "FUNCTION_ID_REQUIRED")
	})
	t.Run("function not found is 404", func(t *testing.T) {
		f := newTokenFixture(t, true, false, nil)
		_, err := f.call(anchorCtx(), fncontrol.TokenRequest{FunctionID: "fnc_doesnotexist", Version: 1})
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Function_NOT_FOUND")
		assert.Equal(t, 404, statusOf(t, err))
	})
	t.Run("version not found is 404", func(t *testing.T) {
		f := newTokenFixture(t, true, false, nil)
		_, err := f.call(anchorCtx(), fncontrol.TokenRequest{FunctionID: f.fn.ID, Version: 99})
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Function version_NOT_FOUND")
		assert.Equal(t, 404, statusOf(t, err))
	})
	t.Run("not declared is 403 PLATFORM_TOKEN_NOT_DECLARED", func(t *testing.T) {
		f := newTokenFixture(t, false, false, nil)
		_, err := f.call(anchorCtx(), f.req())
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PLATFORM_TOKEN_NOT_DECLARED")
	})
	t.Run("no linked service account is 409 APPLICATION_SERVICE_ACCOUNT_REQUIRED", func(t *testing.T) {
		f := newTokenFixture(t, true, true, nil)
		_, err := f.call(anchorCtx(), f.req())
		testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "APPLICATION_SERVICE_ACCOUNT_REQUIRED")
		assert.Equal(t, 409, statusOf(t, err))
	})
	t.Run("a deactivated account is 400 SERVICE_ACCOUNT_INACTIVE", func(t *testing.T) {
		f := newTokenFixture(t, true, false, nil)
		_, err := testpg.Pool(t).Exec(context.Background(), `UPDATE iam_service_accounts SET active = false WHERE id = $1`, f.appSA)
		require.NoError(t, err)
		_, err = f.call(anchorCtx(), f.req())
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SERVICE_ACCOUNT_INACTIVE")
	})
	t.Run("a deactivated principal is 400 SERVICE_ACCOUNT_INACTIVE", func(t *testing.T) {
		f := newTokenFixture(t, true, false, nil)
		_, err := testpg.Pool(t).Exec(context.Background(), `UPDATE iam_principals SET active = false WHERE id = $1`, f.appPrincipal)
		require.NoError(t, err)
		_, err = f.call(anchorCtx(), f.req())
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SERVICE_ACCOUNT_INACTIVE")
	})
	t.Run("a refusal mints and audits nothing", func(t *testing.T) {
		f := newTokenFixture(t, false, false, nil)
		_, err := f.call(anchorCtx(), f.req())
		require.Error(t, err)
		assert.Zero(t, f.mintAudits(t))
	})
}

// The caller rules are the control routes' own: anchor scope plus the
// function-runner permission.
func TestToken_CallerRules(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, true, false, nil)

	_, err := f.call(context.Background(), f.req())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "UNAUTHENTICATED")

	_, err = f.call(anchorCtx("platform:events:create"), f.req())
	require.Error(t, err, "an anchor caller without the runner permission is refused")
	assert.Zero(t, f.mintAudits(t))
}

// Every mint leaves an audit row: the account, the runner's principal and the
// function's address, never the token.
func TestToken_Audited(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, true, false, nil)
	out, err := f.call(anchorCtx(), f.req())
	require.NoError(t, err)
	require.Equal(t, 1, f.mintAudits(t))

	var entityType, principalID string
	var detail []byte
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT entity_type, principal_id, operation_json FROM aud_logs WHERE entity_id = $1 AND operation = 'TOKEN_MINTED_FOR_FUNCTION'`, f.appSA).
		Scan(&entityType, &principalID, &detail))
	assert.Equal(t, "SERVICE_ACCOUNT", entityType)
	assert.Equal(t, "sa_test_fn_runner", principalID, "the runner's principal")
	assert.JSONEq(t, `{"function":"`+f.fn.Address+`"}`, string(detail))
	assert.False(t, strings.Contains(string(detail), out.Body.AccessToken), "the token is never recorded")
}

// mintAudits counts the function-mint audit rows for the application's account.
func (f *tokenFixture) mintAudits(t *testing.T) int {
	t.Helper()
	if f.appSA == "" {
		return 0
	}
	var n int
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'TOKEN_MINTED_FOR_FUNCTION'`, f.appSA).Scan(&n))
	return n
}

// statusOf is the HTTP status a handler error maps to.
func statusOf(t *testing.T, err error) int {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "expected *usecase.Error, got: %v", err)
	return ue.HTTPStatus()
}
