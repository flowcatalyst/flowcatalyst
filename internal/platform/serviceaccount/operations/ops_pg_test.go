//go:build integration

package operations_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// TestMain seeds FLOWCATALYST_APP_KEY before the embedded-PG boot:
// create-with-credentials encrypts the OAuth client secret via
// encryption.FromEnv, which reads the env at call time. os.Setenv (not
// t.Setenv) because every test here runs t.Parallel().
func TestMain(m *testing.M) {
	key, err := encryption.GenerateKey()
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("FLOWCATALYST_APP_KEY", key)
	testpg.RunMain(m)
}

// runOp drives op through the full use-case envelope (Validate → Authorize →
// Execute → atomic commit) as an anchor principal — these operations are all
// usecaseop.Public, so the coarse permission lives at the controller; the
// anchor ctx mirrors how the HTTP handler runs them.
func runOp[C any, E usecase.DomainEvent](
	uow *usecasepgx.UnitOfWork, op usecaseop.Operation[C, E], cmd C,
) (E, error) {
	return usecaseop.Run(testpg.AnchorCtx(), uow, op, cmd, testpg.TestEC())
}

// mustCreate seeds a service account through the public operation. Codes
// are hand-unique per test: the fixture never truncates, so tests own
// their rows and never assert table-wide.
func mustCreate(t *testing.T, repo *serviceaccount.Repository, uow *usecasepgx.UnitOfWork, code, name string) operations.ServiceAccountCreated {
	t.Helper()
	ev, err := runOp(uow, operations.CreateServiceAccount(repo),
		operations.CreateCommand{Code: code, Name: name})
	require.NoError(t, err)
	return ev
}

// mustProvision seeds via create-with-credentials — the only operation
// that also mints the linked SERVICE principal, which assign-roles needs.
func mustProvision(t *testing.T, saRepo *serviceaccount.Repository, principals *principal.Repository, oauthRepo *platformauth.OAuthClientRepo, uow *usecasepgx.UnitOfWork, code, name string) operations.CreateWithCredentialsResult {
	t.Helper()
	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo),
		operations.CreateCommand{Code: code, Name: name}, testpg.TestEC())
	require.NoError(t, err)
	return res
}

// ── Create ────────────────────────────────────────────────────────────────

func TestCreateServiceAccount_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := serviceaccount.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	desc := "machine account"
	scope := "anchor"
	appID := "app_sacreatehappy"
	ev, err := runOp(uow, operations.CreateServiceAccount(repo), operations.CreateCommand{
		Code:          "SACreate-Happy", // lower-cased by the op
		Name:          "  Create Happy  ",
		Description:   &desc,
		Scope:         &scope,
		ApplicationID: &appID,
		ClientIDs:     []string{"clt_sacreatehappy"},
	})
	require.NoError(t, err)

	assert.NotEmpty(t, ev.ServiceAccountID)
	assert.Equal(t, "sacreate-happy", ev.Code, "code is lower-cased")
	assert.Equal(t, "Create Happy", ev.Name, "name is trimmed")

	got, err := repo.FindByID(ctx, ev.ServiceAccountID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "sacreate-happy", got.Code)
	assert.Equal(t, "Create Happy", got.Name)
	assert.True(t, got.Active)
	require.NotNil(t, got.Description)
	assert.Equal(t, desc, *got.Description)
	require.NotNil(t, got.ApplicationID)
	assert.Equal(t, appID, *got.ApplicationID)
	assert.Equal(t, serviceaccount.AuthNone, got.WebhookCredentials.AuthType,
		"no credentials in cmd → NONE")
	// scope + client_ids persist since migration 035 (pre-035 / Rust rows
	// read back NULL).
	require.NotNil(t, got.Scope)
	assert.Equal(t, scope, *got.Scope)
	assert.Equal(t, []string{"clt_sacreatehappy"}, got.ClientIDs)
}

// Without applicationId the standalone path stays unconfined — the
// NewService default (AllApplications=true, no bindings) is preserved.
func TestCreateServiceAccountWithCredentials_NoApplicationID_Unconfined(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)

	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo),
		operations.CreateCommand{
			Code: "sawithcreds-unconfined",
			Name: "Unconfined",
		}, testpg.TestEC())
	require.NoError(t, err)

	p, err := principals.FindByID(ctx, res.PrincipalID)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.True(t, p.AllApplications, "no applicationId → unrestricted on the application axis")
	assert.Empty(t, p.AccessibleApplicationIDs)
}

func TestCreateServiceAccount_Validation(t *testing.T) {
	t.Parallel()
	repo := serviceaccount.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.CreateCommand
		code string
	}{
		{"empty code", operations.CreateCommand{Name: "X"}, "CODE_REQUIRED"},
		{"underscore in code", operations.CreateCommand{Code: "bad_code", Name: "X"}, "INVALID_CODE_FORMAT"},
		{"digit-leading code", operations.CreateCommand{Code: "9starts-digit", Name: "X"}, "INVALID_CODE_FORMAT"},
		{"empty name", operations.CreateCommand{Code: "sacreate-noname", Name: " "}, "NAME_REQUIRED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runOp(uow, operations.CreateServiceAccount(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
}

func TestCreateServiceAccount_DuplicateCode_Conflict(t *testing.T) {
	t.Parallel()
	repo := serviceaccount.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	mustCreate(t, repo, uow, "sadup-conflict", "First")

	_, err := runOp(uow, operations.CreateServiceAccount(repo),
		operations.CreateCommand{Code: "sadup-conflict", Name: "Second"})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "CODE_EXISTS")
}

// ── CreateWithCredentials ─────────────────────────────────────────────────

// One transaction must yield: the SA row, a linked SERVICE principal, a
// CONFIDENTIAL oauth client, and one-time plaintext credentials in the
// returned result struct (NOT a Committed — secrets are never re-readable).
func TestCreateServiceAccountWithCredentials_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)

	scope := "anchor"
	appID := "app_sawithcreds01"
	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo),
		operations.CreateCommand{
			Code:          "sawithcreds-happy",
			Name:          "With Creds",
			Scope:         &scope,
			ApplicationID: &appID,
		}, testpg.TestEC())
	require.NoError(t, err)

	// Returned one-time credentials are all non-empty.
	require.NotNil(t, res.ServiceAccount)
	assert.NotEmpty(t, res.PrincipalID)
	assert.NotEmpty(t, res.OAuthClientRowID)
	assert.NotEmpty(t, res.OAuthClientID)
	assert.NotEmpty(t, res.OAuthClientSecret)
	assert.NotEmpty(t, res.SigningSecret)
	assert.Regexp(t, `^fc_[0-9a-z]{32}$`, res.AuthToken, "bearer token format (Rust parity)")

	// Service-account row: webhook credentials minted as BEARER + both secrets.
	sa, err := saRepo.FindByID(ctx, res.ServiceAccount.ID)
	require.NoError(t, err)
	require.NotNil(t, sa)
	assert.Equal(t, "sawithcreds-happy", sa.Code)
	assert.True(t, sa.Active)
	assert.Equal(t, serviceaccount.AuthBearer, sa.WebhookCredentials.AuthType)
	require.NotNil(t, sa.WebhookCredentials.Token)
	assert.Equal(t, res.AuthToken, *sa.WebhookCredentials.Token)
	require.NotNil(t, sa.WebhookCredentials.SigningSecret)
	assert.Equal(t, res.SigningSecret, *sa.WebhookCredentials.SigningSecret)
	require.NotNil(t, sa.ApplicationID)
	assert.Equal(t, appID, *sa.ApplicationID)
	require.NotNil(t, sa.Scope, "scope persists since migration 035")
	assert.Equal(t, scope, *sa.Scope)

	// Since migration 035 the standalone path confines its SA principal the
	// same way the application-provision flow does whenever applicationId is
	// supplied: AllApplications=false plus a single app-access binding, so
	// the token's `applications` claim carries exactly that app.
	p, err := principals.FindByID(ctx, res.PrincipalID)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, principal.TypeService, p.Type)
	require.NotNil(t, p.ServiceAccountID)
	assert.Equal(t, sa.ID, *p.ServiceAccountID)
	assert.Equal(t, principal.ScopeAnchor, p.Scope, "SERVICE principals are anchor-tier")
	assert.True(t, p.Active)
	assert.False(t, p.AllApplications, "applicationId supplied → principal is app-scoped")
	assert.Equal(t, []string{appID}, p.AccessibleApplicationIDs, "single app-access binding written")

	// OAuth client row: CONFIDENTIAL, owned by the principal, with the
	// client_credentials/refresh_token grants and an encrypted secret ref.
	oc, err := oauthRepo.FindByID(ctx, res.OAuthClientRowID)
	require.NoError(t, err)
	require.NotNil(t, oc)
	assert.Equal(t, res.OAuthClientID, oc.ClientID)
	assert.Equal(t, "With Creds Client", oc.ClientName)
	assert.Equal(t, platformauth.OAuthClientConfidential, oc.ClientType)
	assert.True(t, oc.Active)
	require.NotNil(t, oc.PrincipalID)
	assert.Equal(t, res.PrincipalID, *oc.PrincipalID)
	assert.ElementsMatch(t, []string{"client_credentials", "refresh_token"}, oc.GrantTypes)
	assert.Equal(t, []string{"openid"}, oc.Scopes)

	// The stored ref decrypts back to the returned plaintext (the
	// /oauth/token decrypt-and-compare contract). NOTE: this path stores the
	// raw envelope WITHOUT the "encrypted:" prefix that auth/operations'
	// generateSecret adds — pinned so a future unification flips it knowingly.
	require.NotNil(t, oc.SecretRef)
	assert.NotRegexp(t, `^encrypted:`, *oc.SecretRef)
	enc, err := encryption.FromEnv()
	require.NoError(t, err)
	require.NotNil(t, enc)
	plain, err := enc.Decrypt(*oc.SecretRef)
	require.NoError(t, err)
	assert.Equal(t, res.OAuthClientSecret, plain)
}

func TestCreateServiceAccountWithCredentials_Errors(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.CreateCommand
		kind usecase.Kind
		code string
	}{
		{"empty code", operations.CreateCommand{Name: "X"}, usecase.KindValidation, "CODE_REQUIRED"},
		{"bad code", operations.CreateCommand{Code: "Bad Code", Name: "X"}, usecase.KindValidation, "INVALID_CODE_FORMAT"},
		{"empty name", operations.CreateCommand{Code: "sawcerr-noname"}, usecase.KindValidation, "NAME_REQUIRED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
				operations.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo),
				tc.cmd, testpg.TestEC())
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}

	t.Run("duplicate code", func(t *testing.T) {
		t.Parallel()
		mustCreate(t, saRepo, uow, "sawcdup-conflict", "First")
		_, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
			operations.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo),
			operations.CreateCommand{Code: "sawcdup-conflict", Name: "Second"}, testpg.TestEC())
		testpg.RequireUsecaseError(t, err, usecase.KindConflict, "CODE_EXISTS")
	})
}

// ── Update ────────────────────────────────────────────────────────────────

func TestUpdateServiceAccount_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := serviceaccount.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "saupd-happy", "Before")

	newName := "  After  "
	newDesc := "after"
	ev, err := runOp(uow, operations.UpdateServiceAccount(repo), operations.UpdateCommand{
		ID: seeded.ServiceAccountID, Name: &newName, Description: &newDesc,
	})
	require.NoError(t, err)
	assert.Equal(t, seeded.ServiceAccountID, ev.ServiceAccountID)
	assert.Equal(t, "After", ev.Name, "name is trimmed")

	got, err := repo.FindByID(ctx, seeded.ServiceAccountID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "After", got.Name)
	require.NotNil(t, got.Description)
	assert.Equal(t, "after", *got.Description)
	assert.Equal(t, "saupd-happy", got.Code, "code is immutable on update")
}

func TestUpdateServiceAccount_Errors(t *testing.T) {
	t.Parallel()
	repo := serviceaccount.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	name := "X"
	blank := " "

	cases := []struct {
		name string
		cmd  operations.UpdateCommand
		kind usecase.Kind
		code string
	}{
		{"missing id", operations.UpdateCommand{Name: &name}, usecase.KindValidation, "ID_REQUIRED"},
		{"blank name", operations.UpdateCommand{ID: "sa_doesnotexist1", Name: &blank}, usecase.KindValidation, "NAME_REQUIRED"},
		{"unknown id", operations.UpdateCommand{ID: "sa_doesnotexist1", Name: &name}, usecase.KindNotFound, "ServiceAccount_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runOp(uow, operations.UpdateServiceAccount(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

// ── Delete ────────────────────────────────────────────────────────────────

func TestDeleteServiceAccount_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := serviceaccount.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "sadel-happy", "Doomed")

	ev, err := runOp(uow, operations.DeleteServiceAccount(repo),
		operations.DeleteCommand{ID: seeded.ServiceAccountID})
	require.NoError(t, err)
	assert.Equal(t, seeded.ServiceAccountID, ev.ServiceAccountID)
	assert.Equal(t, "sadel-happy", ev.Code)

	got, err := repo.FindByID(ctx, seeded.ServiceAccountID)
	require.NoError(t, err)
	assert.Nil(t, got, "deleted row must be gone")
}

func TestDeleteServiceAccount_Errors(t *testing.T) {
	t.Parallel()
	repo := serviceaccount.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runOp(uow, operations.DeleteServiceAccount(repo),
		operations.DeleteCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runOp(uow, operations.DeleteServiceAccount(repo),
		operations.DeleteCommand{ID: "sa_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "ServiceAccount_NOT_FOUND")
}

// ── Deactivate ────────────────────────────────────────────────────────────

func TestDeactivateServiceAccount_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := serviceaccount.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "sadeact-happy", "Sleepy")

	ev, err := runOp(uow, operations.DeactivateServiceAccount(repo),
		operations.DeactivateCommand{ID: seeded.ServiceAccountID})
	require.NoError(t, err)
	assert.Equal(t, seeded.ServiceAccountID, ev.ServiceAccountID)

	got, err := repo.FindByID(ctx, seeded.ServiceAccountID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.False(t, got.Active, "deactivate must flip Active → false")
}

func TestDeactivateServiceAccount_Errors(t *testing.T) {
	t.Parallel()
	repo := serviceaccount.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runOp(uow, operations.DeactivateServiceAccount(repo),
		operations.DeactivateCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runOp(uow, operations.DeactivateServiceAccount(repo),
		operations.DeactivateCommand{ID: "sa_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "ServiceAccount_NOT_FOUND")
}

// ── AssignRoles ───────────────────────────────────────────────────────────

// Roles live on the linked SERVICE principal (iam_principal_roles), not the
// SA row — so the seed must be create-with-credentials (which mints the
// principal) and the post-state reload goes through FindByServiceAccount.
func TestAssignRolesToServiceAccount_HappyPathAndReplace(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)
	res := mustProvision(t, saRepo, principals, oauthRepo, uow, "saroles-happy", "Roles Happy")
	saID := res.ServiceAccount.ID

	first, err := runOp(uow, operations.AssignRolesToServiceAccount(saRepo, principals),
		operations.AssignRolesCommand{ServiceAccountID: saID, Roles: []string{"saroles:admin", "saroles:viewer"}})
	require.NoError(t, err)
	assert.Equal(t, saID, first.ServiceAccountID)
	assert.ElementsMatch(t, []string{"saroles:admin", "saroles:viewer"}, first.RolesAdded)
	assert.Empty(t, first.RolesRemoved)

	p, err := principals.FindByServiceAccount(ctx, saID)
	require.NoError(t, err)
	require.NotNil(t, p)
	gotRoles := make([]string, 0, len(p.Roles))
	for _, ra := range p.Roles {
		gotRoles = append(gotRoles, ra.Role)
		require.NotNil(t, ra.AssignmentSource)
		assert.Equal(t, "ADMIN_ASSIGNED", *ra.AssignmentSource)
	}
	assert.ElementsMatch(t, []string{"saroles:admin", "saroles:viewer"}, gotRoles)

	// Declarative replace: the event carries the set-difference.
	second, err := runOp(uow, operations.AssignRolesToServiceAccount(saRepo, principals),
		operations.AssignRolesCommand{ServiceAccountID: saID, Roles: []string{"saroles:viewer", "saroles:auditor"}})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"saroles:auditor"}, second.RolesAdded)
	assert.ElementsMatch(t, []string{"saroles:admin"}, second.RolesRemoved)

	p, err = principals.FindByServiceAccount(ctx, saID)
	require.NoError(t, err)
	require.NotNil(t, p)
	gotRoles = gotRoles[:0]
	for _, ra := range p.Roles {
		gotRoles = append(gotRoles, ra.Role)
	}
	assert.ElementsMatch(t, []string{"saroles:viewer", "saroles:auditor"}, gotRoles,
		"assignment is wholesale replace, not append")
}

func TestAssignRolesToServiceAccount_Errors(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)

	_, err := runOp(uow, operations.AssignRolesToServiceAccount(saRepo, principals),
		operations.AssignRolesCommand{Roles: []string{"x"}})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SERVICE_ACCOUNT_ID_REQUIRED")

	_, err = runOp(uow, operations.AssignRolesToServiceAccount(saRepo, principals),
		operations.AssignRolesCommand{ServiceAccountID: "sa_doesnotexist1", Roles: []string{"x"}})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "ServiceAccount_NOT_FOUND")
}

// ── RegenerateAuthToken ───────────────────────────────────────────────────

func TestRegenerateAuthToken_HappyPathAndStash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := serviceaccount.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "saregen-token", "Token Regen")

	ev, err := runOp(uow, operations.RegenerateAuthToken(repo),
		operations.RegenerateAuthTokenCommand{ServiceAccountID: seeded.ServiceAccountID})
	require.NoError(t, err)
	assert.Equal(t, seeded.ServiceAccountID, ev.ServiceAccountID)
	assert.Equal(t, "saregen-token", ev.Code)

	// One-shot stash: first pop yields the plaintext, second pop is empty —
	// the HTTP handler's "show it exactly once" contract.
	token, ok := operations.PopStashedSecret(seeded.ServiceAccountID, "token")
	require.True(t, ok, "first pop must return the stashed token")
	assert.Regexp(t, `^fc_[0-9a-z]{32}$`, token)

	again, ok := operations.PopStashedSecret(seeded.ServiceAccountID, "token")
	assert.False(t, ok, "second pop must miss — stash is one-shot")
	assert.Empty(t, again)

	got, err := repo.FindByID(ctx, seeded.ServiceAccountID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, serviceaccount.AuthBearer, got.WebhookCredentials.AuthType,
		"regenerate forces BEARER_TOKEN")
	require.NotNil(t, got.WebhookCredentials.Token)
	assert.Equal(t, token, *got.WebhookCredentials.Token, "persisted token == stashed plaintext")
}

func TestRegenerateAuthToken_Errors(t *testing.T) {
	t.Parallel()
	repo := serviceaccount.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runOp(uow, operations.RegenerateAuthToken(repo),
		operations.RegenerateAuthTokenCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SERVICE_ACCOUNT_ID_REQUIRED")

	_, err = runOp(uow, operations.RegenerateAuthToken(repo),
		operations.RegenerateAuthTokenCommand{ServiceAccountID: "sa_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "ServiceAccount_NOT_FOUND")
}

// ── RegenerateSigningSecret ───────────────────────────────────────────────

func TestRegenerateSigningSecret_HappyPathAndStash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := serviceaccount.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "saregen-secret", "Secret Regen")

	ev, err := runOp(uow, operations.RegenerateSigningSecret(repo),
		operations.RegenerateSigningSecretCommand{ServiceAccountID: seeded.ServiceAccountID})
	require.NoError(t, err)
	assert.Equal(t, seeded.ServiceAccountID, ev.ServiceAccountID)
	assert.Equal(t, "saregen-secret", ev.Code)

	secret, ok := operations.PopStashedSecret(seeded.ServiceAccountID, "signing_secret")
	require.True(t, ok, "first pop must return the stashed secret")
	assert.NotEmpty(t, secret)

	again, ok := operations.PopStashedSecret(seeded.ServiceAccountID, "signing_secret")
	assert.False(t, ok, "second pop must miss — stash is one-shot")
	assert.Empty(t, again)

	got, err := repo.FindByID(ctx, seeded.ServiceAccountID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.WebhookCredentials.SigningSecret)
	assert.Equal(t, secret, *got.WebhookCredentials.SigningSecret,
		"persisted signing secret == stashed plaintext")
}

func TestRegenerateSigningSecret_Errors(t *testing.T) {
	t.Parallel()
	repo := serviceaccount.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runOp(uow, operations.RegenerateSigningSecret(repo),
		operations.RegenerateSigningSecretCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SERVICE_ACCOUNT_ID_REQUIRED")

	_, err = runOp(uow, operations.RegenerateSigningSecret(repo),
		operations.RegenerateSigningSecretCommand{ServiceAccountID: "sa_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "ServiceAccount_NOT_FOUND")
}
