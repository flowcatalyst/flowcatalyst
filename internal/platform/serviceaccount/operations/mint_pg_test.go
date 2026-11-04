//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// TestMintForFunction: the token is what the account's client_credentials
// grant would give (subject = the SERVICE principal, authority-bearing, scope =
// the flattened ceiling, the grant's lifetime), minted for a caller that holds
// NO service-account permission (a runner: no role ceiling), stamps the account
// used, and is audited with the runner and the function, never the token.
func TestMintForFunction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)
	res := mustProvision(t, saRepo, principals, platformauth.NewRepository(pool).OAuthClients, uow, "mint-for-fn-sa", "Mint For Function SA")

	svc := authservice.NewWithSecret(authservice.Config{
		SecretKey: "mint-for-fn-test-secret-mint-for-fn-test-secret", Issuer: "https://fc.test", Audience: "https://fc.test",
	})
	deps := operations.MintDeps{
		Repo: saRepo, Principals: principals, Roles: role.NewRepository(pool), Auth: svc, Audit: audit.NewRepository(pool),
		FlattenPermissions: func(context.Context, []string) ([]string, error) { return []string{"a:b:c:d", "e:f:g:h"}, nil },
	}
	runner := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_mintfn_runner", Scope: auth.ScopeAnchor, Permissions: []string{"platform:function:runner:control"},
	})

	minted, err := operations.MintForFunction(runner, deps, res.ServiceAccount.ID, "app.caller")
	require.NoError(t, err)
	assert.EqualValues(t, 3600, minted.ExpiresIn)
	assert.Equal(t, "a:b:c:d e:f:g:h", minted.Scope)
	claims, err := svc.ValidateToken(minted.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, res.PrincipalID, claims.Subject)
	assert.Equal(t, authservice.TokenUseAPI, claims.TokenUse)

	used, err := saRepo.FindByID(ctx, res.ServiceAccount.ID)
	require.NoError(t, err)
	assert.NotNil(t, used.LastUsedAt, "a mint is a use of the account")

	var principalID string
	var detail []byte
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT principal_id, operation_json FROM aud_logs WHERE entity_type = 'SERVICE_ACCOUNT' AND entity_id = $1 AND operation = 'TOKEN_MINTED_FOR_FUNCTION'`,
		res.ServiceAccount.ID).Scan(&principalID, &detail))
	assert.Equal(t, "prn_mintfn_runner", principalID)
	assert.JSONEq(t, `{"function":"app.caller"}`, string(detail))
	assert.NotContains(t, string(detail), minted.AccessToken)

	// Refusals: an unknown account, a deactivated one, an unwired minter. None audits.
	_, err = operations.MintForFunction(runner, deps, "sac_doesnotexist", "app.caller")
	require.Error(t, err)
	_, err = operations.MintForFunction(runner, operations.MintDeps{Repo: saRepo, Principals: principals}, res.ServiceAccount.ID, "app.caller")
	testpg.RequireUsecaseError(t, err, usecase.KindInternal, "TOKEN")
	_, err = pool.Exec(ctx, `UPDATE iam_service_accounts SET active = false WHERE id = $1`, res.ServiceAccount.ID)
	require.NoError(t, err)
	_, err = operations.MintForFunction(runner, deps, res.ServiceAccount.ID, "app.caller")
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SERVICE_ACCOUNT_INACTIVE")
	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'TOKEN_MINTED_FOR_FUNCTION'`, res.ServiceAccount.ID).Scan(&n))
	assert.Equal(t, 1, n, "only the successful mint is audited")
}
