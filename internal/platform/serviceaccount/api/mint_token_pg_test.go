//go:build integration

package api

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
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

// TestServiceAccountRolesAgreeAcrossRoutes pins the two reads to EACH OTHER,
// not to a literal: GET /{id} used to hardcode an empty Roles slice while
// GET /{id}/roles returned the real set from the linked principal, so one
// route said the account was unroled and its own sibling said otherwise.
func TestServiceAccountRolesAgreeAcrossRoutes(t *testing.T) {
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	roles := role.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)

	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo),
		operations.CreateCommand{Code: "roles-agree-sa", Name: "Roles Agree SA"}, testpg.TestEC())
	require.NoError(t, err)

	roleA, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
		roleops.CreateCommand{ApplicationCode: "agree", RoleName: "alpha", DisplayName: "Alpha"}, testpg.TestEC())
	require.NoError(t, err)
	roleB, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
		roleops.CreateCommand{ApplicationCode: "agree", RoleName: "beta", DisplayName: "Beta"}, testpg.TestEC())
	require.NoError(t, err)

	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, operations.AssignRolesToServiceAccount(saRepo, principals),
		operations.AssignRolesCommand{ServiceAccountID: res.ServiceAccount.ID, Roles: []string{roleA.Name, roleB.Name}},
		testpg.TestEC())
	require.NoError(t, err)

	sa, err := saRepo.FindByID(context.Background(), res.ServiceAccount.ID)
	require.NoError(t, err)
	require.NotNil(t, sa)

	fromAccount := make([]string, 0, len(sa.Roles))
	for _, ra := range sa.Roles {
		fromAccount = append(fromAccount, ra.Role)
	}
	assert.ElementsMatch(t, []string{roleA.Name, roleB.Name}, fromAccount,
		"the account object must report the roles its /roles sub-route returns")

	byCode, err := saRepo.FindByCode(context.Background(), "roles-agree-sa")
	require.NoError(t, err)
	require.NotNil(t, byCode)
	assert.Len(t, byCode.Roles, 2, "FindByCode hydrates the same way as FindByID")
}

// TestMintStampsLastUsedAt covers the field an operator reads before revoking:
// it was declared, stored and rendered, but never written, so it was always
// null and could not distinguish a live account from a dead credential.
func TestMintStampsLastUsedAt(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)

	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo),
		operations.CreateCommand{Code: "lastused-sa", Name: "Last Used SA"}, testpg.TestEC())
	require.NoError(t, err)

	fresh, err := saRepo.FindByID(ctx, res.ServiceAccount.ID)
	require.NoError(t, err)
	require.Nil(t, fresh.LastUsedAt, "a never-used account reports null")

	svc := authservice.NewWithSecret(authservice.Config{
		SecretKey: "lastused-test-secret-lastused-test-secret",
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
		PrincipalID: "prn_lastused_admin", Scope: auth.ScopeAnchor,
	})

	_, err = s.mintToken(anchorCtx, &apicommon.IDInput{ID: res.ServiceAccount.ID})
	require.NoError(t, err)

	used, err := saRepo.FindByID(ctx, res.ServiceAccount.ID)
	require.NoError(t, err)
	require.NotNil(t, used.LastUsedAt, "handing out a bearer is a use")
	assert.WithinDuration(t, time.Now().UTC(), *used.LastUsedAt, time.Minute)
}

// TestTouchLastUsedIsBestEffort pins that bookkeeping cannot break the thing
// being done: an unknown id updates no row and reports no error, so a mint or
// a delivery is never failed by a failed stamp.
func TestTouchLastUsedIsBestEffort(t *testing.T) {
	saRepo := serviceaccount.NewRepository(testpg.Pool(t))
	assert.NoError(t, saRepo.TouchLastUsed(context.Background(), "sa_does_not_exist"))
	assert.NoError(t, saRepo.TouchLastUsed(context.Background(), ""))
}

// TestWebhookCredentialsEncryptedAtRest pins the column contents, not just the
// round-trip: wh_auth_token_ref / wh_signing_secret_ref are named as references
// and used to hold the raw secret, so a database dump handed over live outbound
// bearer tokens and — worse — the HMAC signing secret a subscriber verifies to
// know a delivery is genuinely ours.
func TestWebhookCredentialsEncryptedAtRest(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)

	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo),
		operations.CreateCommand{Code: "encrypted-sa", Name: "Encrypted SA"}, testpg.TestEC())
	require.NoError(t, err)
	require.NotEmpty(t, res.AuthToken)
	require.NotEmpty(t, res.SigningSecret)

	var storedToken, storedSecret *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT wh_auth_token_ref, wh_signing_secret_ref FROM iam_service_accounts WHERE id = $1`,
		res.ServiceAccount.ID).Scan(&storedToken, &storedSecret))

	require.NotNil(t, storedToken)
	require.NotNil(t, storedSecret)
	assert.True(t, strings.HasPrefix(*storedToken, "encrypted:"),
		"the column must hold ciphertext, not the bearer")
	assert.True(t, strings.HasPrefix(*storedSecret, "encrypted:"),
		"the column must hold ciphertext, not the signing key")
	assert.NotContains(t, *storedToken, res.AuthToken)
	assert.NotContains(t, *storedSecret, res.SigningSecret)

	// ...and the delivery path still gets the real value back.
	loaded, err := saRepo.FindByID(ctx, res.ServiceAccount.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded.WebhookCredentials.Token)
	require.NotNil(t, loaded.WebhookCredentials.SigningSecret)
	assert.Equal(t, res.AuthToken, *loaded.WebhookCredentials.Token)
	assert.Equal(t, res.SigningSecret, *loaded.WebhookCredentials.SigningSecret)
}

// TestLegacyPlaintextCredentialsStillResolve covers the rollout: rows written
// before encryption hold a bare value, and must keep working with no backfill.
// The "encrypted:" prefix makes the two forms self-describing.
func TestLegacyPlaintextCredentialsStillResolve(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)

	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo),
		operations.CreateCommand{Code: "legacy-sa", Name: "Legacy SA"}, testpg.TestEC())
	require.NoError(t, err)

	// Simulate a pre-encryption row.
	_, err = pool.Exec(ctx,
		`UPDATE iam_service_accounts
		 SET wh_auth_token_ref = 'legacy-plaintext-token',
		     wh_signing_secret_ref = 'legacy-plaintext-secret'
		 WHERE id = $1`, res.ServiceAccount.ID)
	require.NoError(t, err)

	loaded, err := saRepo.FindByID(ctx, res.ServiceAccount.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded.WebhookCredentials.Token)
	assert.Equal(t, "legacy-plaintext-token", *loaded.WebhookCredentials.Token,
		"a legacy row reads through unchanged rather than failing or blanking")
	assert.Equal(t, "legacy-plaintext-secret", *loaded.WebhookCredentials.SigningSecret)
}
