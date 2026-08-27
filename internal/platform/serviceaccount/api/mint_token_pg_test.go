//go:build integration

package api

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestMain seeds FLOWCATALYST_APP_KEY before the embedded-PG boot:
// create-with-credentials encrypts the OAuth client secret via
// encryption.FromEnv, which reads the env at call time.
func TestMain(m *testing.M) {
	key, err := encryption.GenerateKey()
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("FLOWCATALYST_APP_KEY", key)
	testpg.RunMain(m)
}

// TestMintServiceAccountToken pins the admin token mint: anchor-gated, the
// minted bearer is an authority-bearing (token_use=api) token whose subject
// is the linked SERVICE principal, scope = the flattened ceiling, and a
// deactivated account refuses.
func TestMintServiceAccountToken(t *testing.T) {
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)

	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo),
		operations.CreateCommand{Code: "mint-token-sa", Name: "Mint Token SA"}, testpg.TestEC())
	require.NoError(t, err)

	svc := authservice.NewWithSecret(authservice.Config{
		SecretKey: "mint-token-test-secret-mint-token-test-secret",
		Issuer:    "https://fc.test",
		Audience:  "https://fc.test",
	})

	s := &State{
		Repo:       saRepo,
		Principals: principals,
		Auth:       svc,
		FlattenPermissions: func(_ context.Context, _ []string) ([]string, error) {
			return []string{"platform:events:create"}, nil
		},
	}
	anchorCtx := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_minttest_admin", Scope: auth.ScopeAnchor,
	})

	out, err := s.mintToken(anchorCtx, &apicommon.IDInput{ID: res.ServiceAccount.ID})
	require.NoError(t, err)
	assert.Equal(t, "Bearer", out.Body.TokenType)
	assert.EqualValues(t, 3600, out.Body.ExpiresIn)
	require.NotNil(t, out.Body.Scope)
	assert.Equal(t, "platform:events:create", *out.Body.Scope)

	claims, err := svc.ValidateToken(out.Body.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, res.PrincipalID, claims.Subject, "subject is the linked SERVICE principal")
	assert.Equal(t, authservice.TokenUseAPI, claims.TokenUse, "authority-bearing, not identity")

	// Non-anchor caller refused.
	clientCtx := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_minttest_client", Scope: auth.ScopeClient,
	})
	_, err = s.mintToken(clientCtx, &apicommon.IDInput{ID: res.ServiceAccount.ID})
	require.Error(t, err)

	// Deactivated account refused.
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, operations.DeactivateServiceAccount(saRepo),
		operations.DeactivateCommand{ID: res.ServiceAccount.ID}, testpg.TestEC())
	require.NoError(t, err)
	_, err = s.mintToken(anchorCtx, &apicommon.IDInput{ID: res.ServiceAccount.ID})
	require.Error(t, err)
}

// TestMintServiceAccountTokenCarriesApplicationScope is the regression for an
// app-scoped SA minting a token that claimed no application reach at all: the
// mint loads the principal via FindByServiceAccount, which hydrated roles but
// not the iam_principal_application_access junction, so `applications` was
// always empty however the SA was bound. An app-scoped SA whose token carries
// an empty list is indistinguishable from one confined to nothing — and
// CanAccessApplication has no anchor bypass, so that reads as "no access".
func TestMintServiceAccountTokenCarriesApplicationScope(t *testing.T) {
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)

	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo),
		operations.CreateCommand{Code: "scoped-mint-sa", Name: "Scoped Mint SA"}, testpg.TestEC())
	require.NoError(t, err)

	// Confine the SA to one application, as application service-account
	// provisioning does. The junction carries no FK, so a synthetic id is
	// enough to pin the read path.
	const appID = "app_scopedmint01"
	ctx := context.Background()
	_, err = pool.Exec(ctx,
		`UPDATE iam_principals SET all_applications = false WHERE id = $1`, res.PrincipalID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_principal_application_access (principal_id, application_id) VALUES ($1, $2)`,
		res.PrincipalID, appID)
	require.NoError(t, err)

	svc := authservice.NewWithSecret(authservice.Config{
		SecretKey: "scoped-mint-test-secret-scoped-mint-test-secret",
		Issuer:    "https://fc.test",
		Audience:  "https://fc.test",
	})
	s := &State{
		Repo:       saRepo,
		Principals: principals,
		Auth:       svc,
		FlattenPermissions: func(_ context.Context, _ []string) ([]string, error) {
			return []string{"platform:events:create"}, nil
		},
	}
	anchorCtx := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_scopedmint_admin", Scope: auth.ScopeAnchor,
	})

	out, err := s.mintToken(anchorCtx, &apicommon.IDInput{ID: res.ServiceAccount.ID})
	require.NoError(t, err)
	claims, err := svc.ValidateToken(out.Body.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, []string{appID}, claims.Applications,
		"the SA's application binding must reach the applications claim")
	assert.False(t, claims.AllApplications, "an app-scoped SA is not all-applications")
}
