//go:build integration

package operations_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// TestMain seeds FLOWCATALYST_APP_KEY before the embedded-PG boot:
// CONFIDENTIAL client creation / secret rotation encrypt the secret via
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

// runAuthorized drives op through the full use-case envelope (Validate →
// Authorize → Execute → atomic commit) as an anchor principal. These auth
// resources (OAuth clients, anchor domains, auth configs, IDP role mappings)
// are platform-level config: the operations are intentionally open
// (Authorize: Public) and the anchor gate lives on the controller, so there
// is no use-case authorization-denial test here. It mirrors how the HTTP
// handler runs the operation.
func runAuthorized[C any, E usecase.DomainEvent](
	uow *usecasepgx.UnitOfWork, op usecaseop.Operation[C, E], cmd C,
) (E, error) {
	return usecaseop.Run(testpg.AnchorCtx(), uow, op, cmd, testpg.TestEC())
}

// ══ AnchorDomain ══════════════════════════════════════════════════════════

func mustCreateAnchor(t *testing.T, repo *auth.AnchorDomainRepo, uow *usecasepgx.UnitOfWork, domain string) operations.AnchorDomainCreated {
	t.Helper()
	ev, err := runAuthorized(uow, operations.CreateAnchorDomain(repo),
		operations.CreateAnchorDomainCommand{Domain: domain})
	require.NoError(t, err)
	return ev
}

func TestCreateAnchorDomain_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := auth.NewRepository(testpg.Pool(t)).AnchorDomains
	uow := testpg.NewUoW(t)

	ev, err := runAuthorized(uow, operations.CreateAnchorDomain(repo),
		operations.CreateAnchorDomainCommand{Domain: "  ADCreate-Happy.Example.COM  "})
	require.NoError(t, err)
	assert.NotEmpty(t, ev.AnchorDomainID)
	assert.Equal(t, "adcreate-happy.example.com", ev.Domain, "domain is trimmed + lower-cased")

	got, err := repo.FindByID(ctx, ev.AnchorDomainID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "adcreate-happy.example.com", got.Domain)
}

func TestCreateAnchorDomain_Validation(t *testing.T) {
	t.Parallel()
	repo := auth.NewRepository(testpg.Pool(t)).AnchorDomains
	uow := testpg.NewUoW(t)

	cases := []struct {
		name, domain, code string
	}{
		{"empty", "", "DOMAIN_REQUIRED"},
		{"no dot", "nodots", "INVALID_DOMAIN"},
		{"embedded space", "has space.com", "INVALID_DOMAIN"},
		{"at sign", "user@example.com", "INVALID_DOMAIN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.CreateAnchorDomain(repo),
				operations.CreateAnchorDomainCommand{Domain: tc.domain})
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
}

func TestCreateAnchorDomain_Duplicate_Conflict(t *testing.T) {
	t.Parallel()
	repo := auth.NewRepository(testpg.Pool(t)).AnchorDomains
	uow := testpg.NewUoW(t)
	mustCreateAnchor(t, repo, uow, "addup-conflict.example.com")

	// Case-insensitive: the lookup lower-cases, so a re-cased dup still conflicts.
	_, err := runAuthorized(uow, operations.CreateAnchorDomain(repo),
		operations.CreateAnchorDomainCommand{Domain: "ADDup-Conflict.Example.com"})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "DOMAIN_EXISTS")
}

func TestUpdateAnchorDomain_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := auth.NewRepository(testpg.Pool(t)).AnchorDomains
	uow := testpg.NewUoW(t)
	seeded := mustCreateAnchor(t, repo, uow, "adupd-before.example.com")

	ev, err := runAuthorized(uow, operations.UpdateAnchorDomain(repo), operations.UpdateAnchorDomainCommand{
		ID: seeded.AnchorDomainID, Domain: "ADUpd-After.Example.com",
	})
	require.NoError(t, err)
	assert.Equal(t, seeded.AnchorDomainID, ev.AnchorDomainID)
	assert.Equal(t, "adupd-after.example.com", ev.Domain)

	got, err := repo.FindByID(ctx, seeded.AnchorDomainID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "adupd-after.example.com", got.Domain)
}

func TestUpdateAnchorDomain_Errors(t *testing.T) {
	t.Parallel()
	repo := auth.NewRepository(testpg.Pool(t)).AnchorDomains
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.UpdateAnchorDomainCommand
		kind usecase.Kind
		code string
	}{
		{"missing id", operations.UpdateAnchorDomainCommand{Domain: "x.example.com"}, usecase.KindValidation, "ID_REQUIRED"},
		// Update folds "empty" into INVALID_DOMAIN (create distinguishes DOMAIN_REQUIRED).
		{"empty domain", operations.UpdateAnchorDomainCommand{ID: "and_doesnotexist1"}, usecase.KindValidation, "INVALID_DOMAIN"},
		{"no dot", operations.UpdateAnchorDomainCommand{ID: "and_doesnotexist1", Domain: "nodots"}, usecase.KindValidation, "INVALID_DOMAIN"},
		{"unknown id", operations.UpdateAnchorDomainCommand{ID: "and_doesnotexist1", Domain: "x.example.com"}, usecase.KindNotFound, "AnchorDomain_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.UpdateAnchorDomain(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

func TestDeleteAnchorDomain_HappyPathAndErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := auth.NewRepository(testpg.Pool(t)).AnchorDomains
	uow := testpg.NewUoW(t)
	seeded := mustCreateAnchor(t, repo, uow, "addel-happy.example.com")

	ev, err := runAuthorized(uow, operations.DeleteAnchorDomain(repo),
		operations.DeleteAnchorDomainCommand{ID: seeded.AnchorDomainID})
	require.NoError(t, err)
	assert.Equal(t, "addel-happy.example.com", ev.Domain)

	got, err := repo.FindByID(ctx, seeded.AnchorDomainID)
	require.NoError(t, err)
	assert.Nil(t, got, "deleted row must be gone")

	_, err = runAuthorized(uow, operations.DeleteAnchorDomain(repo),
		operations.DeleteAnchorDomainCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.DeleteAnchorDomain(repo),
		operations.DeleteAnchorDomainCommand{ID: "and_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "AnchorDomain_NOT_FOUND")
}

// ══ AuthConfig ════════════════════════════════════════════════════════════

func mustCreateConfig(t *testing.T, repo *auth.ClientAuthConfigRepo, uow *usecasepgx.UnitOfWork, emailDomain string) operations.AuthConfigCreated {
	t.Helper()
	ev, err := runAuthorized(uow, operations.CreateAuthConfig(repo),
		operations.CreateAuthConfigCommand{
			EmailDomain: emailDomain, ConfigType: "CLIENT", AuthProvider: "INTERNAL",
		})
	require.NoError(t, err)
	return ev
}

func TestCreateAuthConfig_HappyPath_OIDC(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := auth.NewRepository(testpg.Pool(t)).ClientAuthConfigs
	uow := testpg.NewUoW(t)

	issuer := "https://idp.accreate-happy.example.com"
	clientID := "accreate-happy-oidc"
	primary := "clt_accreatehappy"
	ev, err := runAuthorized(uow, operations.CreateAuthConfig(repo), operations.CreateAuthConfigCommand{
		EmailDomain:         "ACCreate-Happy.Example.com", // lower-cased by the op
		ConfigType:          "PARTNER",
		AuthProvider:        "OIDC",
		PrimaryClientID:     &primary,
		AdditionalClientIDs: []string{"clt_accreateextra"},
		GrantedClientIDs:    []string{"clt_accreategrant"},
		OIDCIssuerURL:       &issuer,
		OIDCClientID:        &clientID,
		OIDCMultiTenant:     true,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, ev.AuthConfigID)
	assert.Equal(t, "accreate-happy.example.com", ev.EmailDomain)

	got, err := repo.FindByID(ctx, ev.AuthConfigID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "accreate-happy.example.com", got.EmailDomain)
	assert.Equal(t, auth.ConfigPartner, got.ConfigType)
	assert.Equal(t, auth.ProviderOIDC, got.AuthProvider)
	require.NotNil(t, got.PrimaryClientID)
	assert.Equal(t, primary, *got.PrimaryClientID)
	assert.Equal(t, []string{"clt_accreateextra"}, got.AdditionalClientIDs)
	assert.Equal(t, []string{"clt_accreategrant"}, got.GrantedClientIDs)
	require.NotNil(t, got.OIDCIssuerURL)
	assert.Equal(t, issuer, *got.OIDCIssuerURL)
	require.NotNil(t, got.OIDCClientID)
	assert.Equal(t, clientID, *got.OIDCClientID)
	assert.True(t, got.OIDCMultiTenant)
}

func TestCreateAuthConfig_Validation(t *testing.T) {
	t.Parallel()
	repo := auth.NewRepository(testpg.Pool(t)).ClientAuthConfigs
	uow := testpg.NewUoW(t)
	issuer := "https://idp.example.com"

	cases := []struct {
		name string
		cmd  operations.CreateAuthConfigCommand
		code string
	}{
		{"empty email domain", operations.CreateAuthConfigCommand{ConfigType: "CLIENT"}, "INVALID_EMAIL_DOMAIN"},
		{"dotless email domain", operations.CreateAuthConfigCommand{EmailDomain: "nodot", ConfigType: "CLIENT"}, "INVALID_EMAIL_DOMAIN"},
		{"bad config type", operations.CreateAuthConfigCommand{EmailDomain: "acval.example.com", ConfigType: "WEIRD"}, "INVALID_CONFIG_TYPE"},
		{"oidc missing issuer", operations.CreateAuthConfigCommand{
			EmailDomain: "acval.example.com", ConfigType: "CLIENT", AuthProvider: "OIDC",
		}, "OIDC_ISSUER_REQUIRED"},
		{"oidc missing client id", operations.CreateAuthConfigCommand{
			EmailDomain: "acval.example.com", ConfigType: "CLIENT", AuthProvider: "OIDC", OIDCIssuerURL: &issuer,
		}, "OIDC_CLIENT_ID_REQUIRED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.CreateAuthConfig(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
}

func TestCreateAuthConfig_Duplicate_Conflict(t *testing.T) {
	t.Parallel()
	repo := auth.NewRepository(testpg.Pool(t)).ClientAuthConfigs
	uow := testpg.NewUoW(t)
	mustCreateConfig(t, repo, uow, "acdup-conflict.example.com")

	_, err := runAuthorized(uow, operations.CreateAuthConfig(repo), operations.CreateAuthConfigCommand{
		EmailDomain: "ACDup-Conflict.Example.com", ConfigType: "CLIENT", AuthProvider: "INTERNAL",
	})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "DOMAIN_ALREADY_CONFIGURED")
}

func TestUpdateAuthConfig_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := auth.NewRepository(testpg.Pool(t)).ClientAuthConfigs
	uow := testpg.NewUoW(t)
	seeded := mustCreateConfig(t, repo, uow, "acupd-happy.example.com")

	primary := "clt_acupdprimary"
	multi := true
	ev, err := runAuthorized(uow, operations.UpdateAuthConfig(repo), operations.UpdateAuthConfigCommand{
		ID:               seeded.AuthConfigID,
		PrimaryClientID:  &primary,
		GrantedClientIDs: []string{"clt_acupdgrant"},
		OIDCMultiTenant:  &multi,
	})
	require.NoError(t, err)
	assert.Equal(t, seeded.AuthConfigID, ev.AuthConfigID)
	assert.Equal(t, "acupd-happy.example.com", ev.EmailDomain,
		"email domain is immutable on update")

	got, err := repo.FindByID(ctx, seeded.AuthConfigID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.PrimaryClientID)
	assert.Equal(t, primary, *got.PrimaryClientID)
	assert.Equal(t, []string{"clt_acupdgrant"}, got.GrantedClientIDs)
	assert.True(t, got.OIDCMultiTenant)
	assert.Equal(t, auth.ProviderInternal, got.AuthProvider, "unset fields stay untouched")
}

func TestUpdateAuthConfig_Errors(t *testing.T) {
	t.Parallel()
	repo := auth.NewRepository(testpg.Pool(t)).ClientAuthConfigs
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.UpdateAuthConfig(repo),
		operations.UpdateAuthConfigCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.UpdateAuthConfig(repo),
		operations.UpdateAuthConfigCommand{ID: "cac_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "AuthConfig_NOT_FOUND")
}

func TestDeleteAuthConfig_HappyPathAndErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := auth.NewRepository(testpg.Pool(t)).ClientAuthConfigs
	uow := testpg.NewUoW(t)
	seeded := mustCreateConfig(t, repo, uow, "acdel-happy.example.com")

	ev, err := runAuthorized(uow, operations.DeleteAuthConfig(repo),
		operations.DeleteAuthConfigCommand{ID: seeded.AuthConfigID})
	require.NoError(t, err)
	assert.Equal(t, "acdel-happy.example.com", ev.EmailDomain)

	got, err := repo.FindByID(ctx, seeded.AuthConfigID)
	require.NoError(t, err)
	assert.Nil(t, got, "deleted row must be gone")

	_, err = runAuthorized(uow, operations.DeleteAuthConfig(repo),
		operations.DeleteAuthConfigCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.DeleteAuthConfig(repo),
		operations.DeleteAuthConfigCommand{ID: "cac_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "AuthConfig_NOT_FOUND")
}

// ══ IdpRoleMapping ════════════════════════════════════════════════════════

func TestCreateIdpRoleMapping_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := auth.NewRepository(testpg.Pool(t)).IdpRoleMappings
	uow := testpg.NewUoW(t)

	ev, err := runAuthorized(uow, operations.CreateIdpRoleMapping(repo), operations.CreateIdpRoleMappingCommand{
		IdpType: "keycloak", IdpRoleName: "irm-create-upstream", PlatformRoleName: "irmcreate:admin",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, ev.MappingID)
	assert.Equal(t, "keycloak", ev.IdpType)
	assert.Equal(t, "irm-create-upstream", ev.IdpRoleName)
	assert.Equal(t, "irmcreate:admin", ev.PlatformRoleName)

	got, err := repo.FindByID(ctx, ev.MappingID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "irm-create-upstream", got.IdpRoleName)
	assert.Equal(t, "irmcreate:admin", got.PlatformRoleName)
	// idp_type persists since migration 035 (pre-035 / Rust rows read back
	// ""). FindByIdpRole deliberately still doesn't filter on it.
	assert.Equal(t, "keycloak", got.IdpType)
}

func TestCreateIdpRoleMapping_Validation(t *testing.T) {
	t.Parallel()
	repo := auth.NewRepository(testpg.Pool(t)).IdpRoleMappings
	uow := testpg.NewUoW(t)

	// All three fields share the one FIELD_REQUIRED code.
	cases := []struct {
		name string
		cmd  operations.CreateIdpRoleMappingCommand
	}{
		{"missing idpType", operations.CreateIdpRoleMappingCommand{IdpRoleName: "r", PlatformRoleName: "p"}},
		{"missing idpRoleName", operations.CreateIdpRoleMappingCommand{IdpType: "t", PlatformRoleName: "p"}},
		{"missing platformRoleName", operations.CreateIdpRoleMappingCommand{IdpType: "t", IdpRoleName: "r"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.CreateIdpRoleMapping(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, "FIELD_REQUIRED")
		})
	}
}

func TestDeleteIdpRoleMapping_HappyPathAndErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := auth.NewRepository(testpg.Pool(t)).IdpRoleMappings
	uow := testpg.NewUoW(t)

	seeded, err := runAuthorized(uow, operations.CreateIdpRoleMapping(repo), operations.CreateIdpRoleMappingCommand{
		IdpType: "entra", IdpRoleName: "irm-delete-upstream", PlatformRoleName: "irmdelete:viewer",
	})
	require.NoError(t, err)

	ev, err := runAuthorized(uow, operations.DeleteIdpRoleMapping(repo),
		operations.DeleteIdpRoleMappingCommand{ID: seeded.MappingID})
	require.NoError(t, err)
	assert.Equal(t, "irm-delete-upstream", ev.IdpRoleName)

	got, err := repo.FindByID(ctx, seeded.MappingID)
	require.NoError(t, err)
	assert.Nil(t, got, "deleted row must be gone")

	_, err = runAuthorized(uow, operations.DeleteIdpRoleMapping(repo),
		operations.DeleteIdpRoleMappingCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.DeleteIdpRoleMapping(repo),
		operations.DeleteIdpRoleMappingCommand{ID: "irm_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "IdpRoleMapping_NOT_FOUND")
}

// ══ OAuthClient ═══════════════════════════════════════════════════════════

func mustCreateClient(t *testing.T, repo *auth.OAuthClientRepo, uow *usecasepgx.UnitOfWork, clientID, name, clientType string) operations.OAuthClientCreated {
	t.Helper()
	ev, err := runAuthorized(uow, operations.CreateOAuthClient(repo),
		operations.CreateOAuthClientCommand{ClientID: clientID, ClientName: name, ClientType: clientType})
	require.NoError(t, err)
	return ev
}

func TestCreateOAuthClient_PublicHappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := auth.NewRepository(testpg.Pool(t)).OAuthClients
	uow := testpg.NewUoW(t)

	ev, err := runAuthorized(uow, operations.CreateOAuthClient(repo), operations.CreateOAuthClientCommand{
		ClientID:               "oc-create-public",
		ClientName:             "Public SPA",
		ClientType:             "PUBLIC",
		RedirectURIs:           []string{"https://spa.example.com/callback"},
		PostLogoutRedirectURIs: []string{"https://spa.example.com/bye"},
		GrantTypes:             []string{"authorization_code"},
		Scopes:                 []string{"openid", "profile"},
		AllowedOrigins:         []string{"https://spa.example.com"},
		ApplicationIDs:         []string{"app_occreatepub01"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, ev.OAuthClientID)
	assert.Equal(t, "oc-create-public", ev.ClientID)
	assert.Equal(t, "Public SPA", ev.ClientName)

	got, err := repo.FindByID(ctx, ev.OAuthClientID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, auth.OAuthClientPublic, got.ClientType)
	assert.True(t, got.Active)
	assert.True(t, got.PKCERequired, "PKCE defaults to required")
	assert.Equal(t, []string{"https://spa.example.com/callback"}, got.RedirectURIs)
	assert.Equal(t, []string{"https://spa.example.com/bye"}, got.PostLogoutRedirectURIs)
	assert.Equal(t, []string{"authorization_code"}, got.GrantTypes)
	assert.ElementsMatch(t, []string{"openid", "profile"}, got.Scopes)
	assert.Equal(t, []string{"https://spa.example.com"}, got.AllowedOrigins)
	assert.Equal(t, []string{"app_occreatepub01"}, got.ApplicationIDs)

	// PUBLIC clients mint no secret: no ref at rest, nothing stashed.
	assert.Nil(t, got.SecretRef)
	_, ok := operations.PopStashedSecret(ev.OAuthClientID)
	assert.False(t, ok, "PUBLIC create must not stash a secret")
}

func TestCreateOAuthClient_ConfidentialSecretStash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := auth.NewRepository(testpg.Pool(t)).OAuthClients
	uow := testpg.NewUoW(t)

	ev := mustCreateClient(t, repo, uow, "oc-create-conf", "Server Client", "CONFIDENTIAL")

	// One-shot stash, keyed by the row id: first pop returns the plaintext,
	// second pop misses — the handler's "show it exactly once" contract.
	plaintext, ok := operations.PopStashedSecret(ev.OAuthClientID)
	require.True(t, ok, "first pop must return the minted secret")
	assert.NotEmpty(t, plaintext)

	again, ok := operations.PopStashedSecret(ev.OAuthClientID)
	assert.False(t, ok, "second pop must miss — stash is one-shot")
	assert.Empty(t, again)

	// At rest: "encrypted:"-prefixed envelope (Rust wire parity) that
	// decrypts back to the popped plaintext (decrypt-and-compare contract).
	got, err := repo.FindByID(ctx, ev.OAuthClientID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, auth.OAuthClientConfidential, got.ClientType)
	require.NotNil(t, got.SecretRef)
	assert.True(t, strings.HasPrefix(*got.SecretRef, "encrypted:"))
	enc, err := encryption.FromEnv()
	require.NoError(t, err)
	require.NotNil(t, enc)
	decrypted, err := enc.Decrypt(*got.SecretRef)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestCreateOAuthClient_Validation(t *testing.T) {
	t.Parallel()
	repo := auth.NewRepository(testpg.Pool(t)).OAuthClients
	uow := testpg.NewUoW(t)

	// Omitted clientId is NOT an error: the backend generates a branded
	// TSID, exactly like the service-account provision flows.
	ev, err := runAuthorized(uow, operations.CreateOAuthClient(repo),
		operations.CreateOAuthClientCommand{ClientName: "Generated ID", ClientType: "PUBLIC"})
	require.NoError(t, err)
	assert.Regexp(t, `^oac_`, ev.ClientID, "omitted clientId → backend-generated branded TSID")

	_, err = runAuthorized(uow, operations.CreateOAuthClient(repo),
		operations.CreateOAuthClientCommand{ClientID: "oc-val-noname"})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "CLIENT_NAME_REQUIRED")
}

func TestCreateOAuthClient_DuplicateClientID_Conflict(t *testing.T) {
	t.Parallel()
	repo := auth.NewRepository(testpg.Pool(t)).OAuthClients
	uow := testpg.NewUoW(t)
	mustCreateClient(t, repo, uow, "oc-dup-conflict", "First", "PUBLIC")

	_, err := runAuthorized(uow, operations.CreateOAuthClient(repo),
		operations.CreateOAuthClientCommand{ClientID: "oc-dup-conflict", ClientName: "Second"})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "CLIENT_ID_EXISTS")
}

func TestUpdateOAuthClient_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := auth.NewRepository(testpg.Pool(t)).OAuthClients
	uow := testpg.NewUoW(t)
	seeded := mustCreateClient(t, repo, uow, "oc-upd-happy", "Before", "PUBLIC")

	newName := "  After  "
	pkce := false
	ev, err := runAuthorized(uow, operations.UpdateOAuthClient(repo), operations.UpdateOAuthClientCommand{
		ID:           seeded.OAuthClientID,
		ClientName:   &newName,
		RedirectURIs: []string{"https://after.example.com/cb"},
		PKCERequired: &pkce,
	})
	require.NoError(t, err)
	assert.Equal(t, seeded.OAuthClientID, ev.OAuthClientID)
	assert.Equal(t, "After", ev.ClientName, "name is trimmed")

	got, err := repo.FindByID(ctx, seeded.OAuthClientID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "After", got.ClientName)
	assert.Equal(t, []string{"https://after.example.com/cb"}, got.RedirectURIs)
	assert.False(t, got.PKCERequired)
	assert.Equal(t, "oc-upd-happy", got.ClientID, "client_id is immutable on update")
}

func TestUpdateOAuthClient_Errors(t *testing.T) {
	t.Parallel()
	repo := auth.NewRepository(testpg.Pool(t)).OAuthClients
	uow := testpg.NewUoW(t)
	name := "X"
	blank := " "

	cases := []struct {
		name string
		cmd  operations.UpdateOAuthClientCommand
		kind usecase.Kind
		code string
	}{
		{"missing id", operations.UpdateOAuthClientCommand{ClientName: &name}, usecase.KindValidation, "ID_REQUIRED"},
		{"blank name", operations.UpdateOAuthClientCommand{ID: "oac_doesnotexist1", ClientName: &blank}, usecase.KindValidation, "CLIENT_NAME_REQUIRED"},
		{"unknown id", operations.UpdateOAuthClientCommand{ID: "oac_doesnotexist1", ClientName: &name}, usecase.KindNotFound, "OAuthClient_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.UpdateOAuthClient(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

func TestDeactivateActivateOAuthClient_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := auth.NewRepository(testpg.Pool(t)).OAuthClients
	uow := testpg.NewUoW(t)
	seeded := mustCreateClient(t, repo, uow, "oc-actcycle", "Cycle", "PUBLIC")

	deactivated, err := runAuthorized(uow, operations.DeactivateOAuthClient(repo),
		operations.DeactivateOAuthClientCommand{ID: seeded.OAuthClientID})
	require.NoError(t, err)
	assert.Equal(t, seeded.OAuthClientID, deactivated.OAuthClientID)

	got, err := repo.FindByID(ctx, seeded.OAuthClientID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.False(t, got.Active, "deactivate must flip Active → false")

	activated, err := runAuthorized(uow, operations.ActivateOAuthClient(repo),
		operations.ActivateOAuthClientCommand{ID: seeded.OAuthClientID})
	require.NoError(t, err)
	assert.Equal(t, seeded.OAuthClientID, activated.OAuthClientID)

	got, err = repo.FindByID(ctx, seeded.OAuthClientID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Active, "activate must flip Active → true")
}

func TestActivateDeactivateOAuthClient_Errors(t *testing.T) {
	t.Parallel()
	repo := auth.NewRepository(testpg.Pool(t)).OAuthClients
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.ActivateOAuthClient(repo),
		operations.ActivateOAuthClientCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")
	_, err = runAuthorized(uow, operations.ActivateOAuthClient(repo),
		operations.ActivateOAuthClientCommand{ID: "oac_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "OAuthClient_NOT_FOUND")

	_, err = runAuthorized(uow, operations.DeactivateOAuthClient(repo),
		operations.DeactivateOAuthClientCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")
	_, err = runAuthorized(uow, operations.DeactivateOAuthClient(repo),
		operations.DeactivateOAuthClientCommand{ID: "oac_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "OAuthClient_NOT_FOUND")
}

func TestDeleteOAuthClient_HappyPathAndErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := auth.NewRepository(testpg.Pool(t)).OAuthClients
	uow := testpg.NewUoW(t)
	seeded := mustCreateClient(t, repo, uow, "oc-del-happy", "Doomed", "PUBLIC")

	ev, err := runAuthorized(uow, operations.DeleteOAuthClient(repo),
		operations.DeleteOAuthClientCommand{ID: seeded.OAuthClientID})
	require.NoError(t, err)
	assert.Equal(t, "oc-del-happy", ev.ClientID)

	got, err := repo.FindByID(ctx, seeded.OAuthClientID)
	require.NoError(t, err)
	assert.Nil(t, got, "deleted row must be gone")

	_, err = runAuthorized(uow, operations.DeleteOAuthClient(repo),
		operations.DeleteOAuthClientCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.DeleteOAuthClient(repo),
		operations.DeleteOAuthClientCommand{ID: "oac_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "OAuthClient_NOT_FOUND")
}

func TestRotateOAuthClientSecret_HappyPathAndStash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := auth.NewRepository(testpg.Pool(t)).OAuthClients
	uow := testpg.NewUoW(t)
	seeded := mustCreateClient(t, repo, uow, "oc-rotate-happy", "Rotate Me", "CONFIDENTIAL")

	// Drain the create-time stash so the pops below see only the rotation.
	createSecret, ok := operations.PopStashedSecret(seeded.OAuthClientID)
	require.True(t, ok)
	before, err := repo.FindByID(ctx, seeded.OAuthClientID)
	require.NoError(t, err)
	require.NotNil(t, before.SecretRef)
	oldRef := *before.SecretRef

	ev, err := runAuthorized(uow, operations.RotateOAuthClientSecret(repo),
		operations.RotateOAuthClientSecretCommand{ID: seeded.OAuthClientID})
	require.NoError(t, err)
	assert.Equal(t, seeded.OAuthClientID, ev.OAuthClientID)

	rotated, ok := operations.PopStashedSecret(seeded.OAuthClientID)
	require.True(t, ok, "first pop must return the rotated secret")
	assert.NotEmpty(t, rotated)
	assert.NotEqual(t, createSecret, rotated, "rotation must mint a NEW secret")

	_, ok = operations.PopStashedSecret(seeded.OAuthClientID)
	assert.False(t, ok, "second pop must miss — stash is one-shot")

	after, err := repo.FindByID(ctx, seeded.OAuthClientID)
	require.NoError(t, err)
	require.NotNil(t, after.SecretRef)
	assert.NotEqual(t, oldRef, *after.SecretRef, "stored ref must change on rotation")
	assert.True(t, strings.HasPrefix(*after.SecretRef, "encrypted:"))
	enc, err := encryption.FromEnv()
	require.NoError(t, err)
	require.NotNil(t, enc)
	decrypted, err := enc.Decrypt(*after.SecretRef)
	require.NoError(t, err)
	assert.Equal(t, rotated, decrypted)
}

func TestRotateOAuthClientSecret_PublicClient_Conflict(t *testing.T) {
	t.Parallel()
	repo := auth.NewRepository(testpg.Pool(t)).OAuthClients
	uow := testpg.NewUoW(t)
	seeded := mustCreateClient(t, repo, uow, "oc-rotate-public", "No Secret", "PUBLIC")

	_, err := runAuthorized(uow, operations.RotateOAuthClientSecret(repo),
		operations.RotateOAuthClientSecretCommand{ID: seeded.OAuthClientID})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "NOT_CONFIDENTIAL")
}

func TestRotateOAuthClientSecret_Errors(t *testing.T) {
	t.Parallel()
	repo := auth.NewRepository(testpg.Pool(t)).OAuthClients
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.RotateOAuthClientSecret(repo),
		operations.RotateOAuthClientSecretCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.RotateOAuthClientSecret(repo),
		operations.RotateOAuthClientSecretCommand{ID: "oac_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "OAuthClient_NOT_FOUND")
}
