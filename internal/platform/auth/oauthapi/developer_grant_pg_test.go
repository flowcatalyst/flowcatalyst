//go:build integration

package oauthapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

func requireAppKeyOAuth(t *testing.T) *encryption.Service {
	t.Helper()
	if os.Getenv("FLOWCATALYST_APP_KEY") == "" {
		t.Skip("FLOWCATALYST_APP_KEY not set; developer-credential grant tests need it")
	}
	enc, err := encryption.FromEnv()
	require.NoError(t, err)
	require.NotNil(t, enc)
	return enc
}

// testStateForDeveloperGrant builds a *State wired to the real (testpg)
// principal repository but a nil-returning OAuthClients finder — every
// request in this file is deliberately for a client_id that ISN'T a
// registered OAuth client, exercising the new principal-as-client_id branch
// of handleClientCredentialsGrant.
func testStateForDeveloperGrant(t *testing.T, enc *encryption.Service) *State {
	t.Helper()
	return &State{
		OAuthClients: fakeClientFinder{client: nil},
		Principals:   principal.NewRepository(testpg.Pool(t)),
		Auth:         testAuthService(t),
		Encryption:   enc,
	}
}

func doTokenRequest(t *testing.T, s *State, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	s.Token(rr, req)
	return rr
}

// devRoleOnce/devRoleName mirror principal/operations' equivalent test
// helper: several tests in this file run in parallel and all need the SAME
// role name ("platform:developer" — the literal constant the grant handler
// checks), so the role catalog row is created exactly once and reused.
var (
	devRoleOnce sync.Once
	devRoleName string
)

func ensureSharedDeveloperRole(t *testing.T) string {
	t.Helper()
	uow := testpg.NewUoW(t)
	roles := role.NewRepository(testpg.Pool(t))
	devRoleOnce.Do(func() {
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow,
			roleops.CreateRole(roles),
			roleops.CreateCommand{ApplicationCode: "platform", RoleName: "developer", DisplayName: "platform developer"},
			testpg.TestEC())
		require.NoError(t, err)
		devRoleName = ev.Name
	})
	require.NotEmpty(t, devRoleName, "ensureSharedDeveloperRole must run before any test that depends on it")
	return devRoleName
}

// seedDeveloperGrantFixture creates a USER principal holding the developer
// role with a real (encrypted) developer secret set, returning the
// principal id and the one-time plaintext secret.
func seedDeveloperGrantFixture(t *testing.T, email string) (principalID, plaintext string) {
	t.Helper()
	uow := testpg.NewUoW(t)
	repo := principal.NewRepository(testpg.Pool(t))
	roles := role.NewRepository(testpg.Pool(t))
	devRole := ensureSharedDeveloperRole(t)

	userEv, err := usecaseop.Run(testpg.AnchorCtx(), uow,
		principalops.CreateUser(repo), principalops.CreateCommand{Email: email, Scope: "ANCHOR"}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow,
		principalops.AssignRoles(repo, roles),
		principalops.AssignRolesCommand{UserID: userEv.UserID, Roles: []string{devRole}}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow,
		principalops.SetDeveloperCredential(repo),
		principalops.SetDeveloperCredentialCommand{PrincipalID: userEv.UserID}, testpg.TestEC())
	require.NoError(t, err)
	secret, ok := principalops.PopDevClientSecret(userEv.UserID)
	require.True(t, ok)

	return userEv.UserID, secret
}

func TestHandleDeveloperCredentialGrant_Success(t *testing.T) {
	enc := requireAppKeyOAuth(t)
	t.Parallel()
	principalID, secret := seedDeveloperGrantFixture(t, "oauth-devgrant-ok@example.com")

	s := testStateForDeveloperGrant(t, enc)
	rr := doTokenRequest(t, s, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {principalID},
		"client_secret": {secret},
	})

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), `"access_token"`)
}

func TestHandleDeveloperCredentialGrant_WrongSecret(t *testing.T) {
	enc := requireAppKeyOAuth(t)
	t.Parallel()
	principalID, _ := seedDeveloperGrantFixture(t, "oauth-devgrant-wrongsecret@example.com")

	s := testStateForDeveloperGrant(t, enc)
	rr := doTokenRequest(t, s, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {principalID},
		"client_secret": {"definitely-not-the-right-secret"},
	})

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid_client")
}

func TestHandleDeveloperCredentialGrant_RoleRevoked(t *testing.T) {
	enc := requireAppKeyOAuth(t)
	t.Parallel()
	principalID, secret := seedDeveloperGrantFixture(t, "oauth-devgrant-revoked@example.com")

	// Revoke the role (not the secret) — the live re-check at mint time must
	// reject this even though the secret itself is still technically valid.
	uow := testpg.NewUoW(t)
	repo := principal.NewRepository(testpg.Pool(t))
	roles := role.NewRepository(testpg.Pool(t))
	_, err := usecaseop.Run(testpg.AnchorCtx(), uow,
		principalops.AssignRoles(repo, roles),
		principalops.AssignRolesCommand{UserID: principalID, Roles: []string{}}, testpg.TestEC())
	require.NoError(t, err)

	s := testStateForDeveloperGrant(t, enc)
	rr := doTokenRequest(t, s, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {principalID},
		"client_secret": {secret},
	})

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid_client")
}

func TestHandleDeveloperCredentialGrant_InactivePrincipal(t *testing.T) {
	enc := requireAppKeyOAuth(t)
	t.Parallel()
	principalID, secret := seedDeveloperGrantFixture(t, "oauth-devgrant-inactive@example.com")

	uow := testpg.NewUoW(t)
	repo := principal.NewRepository(testpg.Pool(t))
	_, err := usecaseop.Run(testpg.AnchorCtx(), uow,
		principalops.DeactivateUser(repo),
		principalops.DeactivateCommand{ID: principalID}, testpg.TestEC())
	require.NoError(t, err)

	s := testStateForDeveloperGrant(t, enc)
	rr := doTokenRequest(t, s, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {principalID},
		"client_secret": {secret},
	})

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid_client")
}

func TestHandleDeveloperCredentialGrant_NoCredentialSet(t *testing.T) {
	enc := requireAppKeyOAuth(t)
	t.Parallel()
	uow := testpg.NewUoW(t)
	repo := principal.NewRepository(testpg.Pool(t))
	roles := role.NewRepository(testpg.Pool(t))
	devRole := ensureSharedDeveloperRole(t)

	userEv, err := usecaseop.Run(testpg.AnchorCtx(), uow,
		principalops.CreateUser(repo), principalops.CreateCommand{Email: "oauth-devgrant-nocred@example.com", Scope: "ANCHOR"}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow,
		principalops.AssignRoles(repo, roles),
		principalops.AssignRolesCommand{UserID: userEv.UserID, Roles: []string{devRole}}, testpg.TestEC())
	require.NoError(t, err)

	s := testStateForDeveloperGrant(t, enc)
	rr := doTokenRequest(t, s, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {userEv.UserID},
		"client_secret": {"anything"},
	})

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid_client")
}

// TestHandleDeveloperCredentialGrant_UnknownPrincipal uses a client_id that
// looks like a real Principal TSID (prn_ prefix) but was never created —
// confirms the branch fails closed on a lookup miss, not just wrong
// credentials for a real principal.
func TestHandleDeveloperCredentialGrant_UnknownPrincipal(t *testing.T) {
	enc := requireAppKeyOAuth(t)
	t.Parallel()
	s := testStateForDeveloperGrant(t, enc)
	rr := doTokenRequest(t, s, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"prn_doesnotexist000"},
		"client_secret": {"anything"},
	})

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid_client")
}

// TestHandleDeveloperCredentialGrant_FreshSecretIsStoredHashed pins the
// write path: SetDeveloperCredential (seedDeveloperGrantFixture) must store
// dev_client_secret_ref as "hashed:v1:…", not the older reversibly-encrypted
// form — and that stored hash must actually verify against the returned
// plaintext with nothing left to migrate.
func TestHandleDeveloperCredentialGrant_FreshSecretIsStoredHashed(t *testing.T) {
	enc := requireAppKeyOAuth(t)
	t.Parallel()
	principalID, plaintext := seedDeveloperGrantFixture(t, "oauth-devgrant-freshhash@example.com")

	repo := principal.NewRepository(testpg.Pool(t))
	p, err := repo.FindByID(context.Background(), principalID)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.NotNil(t, p.UserIdentity)
	require.NotNil(t, p.UserIdentity.DevClientSecretRef)
	require.True(t, strings.HasPrefix(*p.UserIdentity.DevClientSecretRef, "hashed:v1:"),
		"a freshly-set developer secret must be stored hashed, got %q", *p.UserIdentity.DevClientSecretRef)
	ok, rehash := enc.VerifySecret(*p.UserIdentity.DevClientSecretRef, plaintext)
	assert.True(t, ok, "stored hash must verify against the disclosed plaintext")
	assert.False(t, rehash, "a freshly-hashed ref under the current key needs no migration")
}

// seedLegacyDeveloperGrantFixture creates a USER principal holding the
// developer role, then writes dev_client_secret_ref directly as an older
// bare (unprefixed) reversibly-encrypted envelope — the exact shape
// generateDevClientSecret produced before this change, and one of the two
// legacy shapes VerifySecret must still accept (see docs/spec/
// encryption.md §3: a bare envelope with no "encrypted:" prefix). Bypasses
// SetDeveloperCredential (which now always mints the hashed form) so the
// migration path has something pre-existing to migrate.
func seedLegacyDeveloperGrantFixture(t *testing.T, enc *encryption.Service, email string) (principalID, plaintext string) {
	t.Helper()
	uow := testpg.NewUoW(t)
	repo := principal.NewRepository(testpg.Pool(t))
	roles := role.NewRepository(testpg.Pool(t))
	devRole := ensureSharedDeveloperRole(t)

	userEv, err := usecaseop.Run(testpg.AnchorCtx(), uow,
		principalops.CreateUser(repo), principalops.CreateCommand{Email: email, Scope: "ANCHOR"}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow,
		principalops.AssignRoles(repo, roles),
		principalops.AssignRolesCommand{UserID: userEv.UserID, Roles: []string{devRole}}, testpg.TestEC())
	require.NoError(t, err)

	plaintext = "legacy-dev-secret-" + userEv.UserID
	legacyRef, err := enc.Encrypt(plaintext)
	require.NoError(t, err)
	_, err = testpg.Pool(t).Exec(context.Background(),
		`UPDATE iam_principals SET dev_client_secret_ref = $1, dev_client_secret_updated_at = NOW() WHERE id = $2`,
		legacyRef, userEv.UserID)
	require.NoError(t, err)

	return userEv.UserID, plaintext
}

// TestHandleDeveloperCredentialGrant_LegacyEncryptedMigratesPersistedValue
// is the transparent-migration contract end to end: a developer secret
// minted before this change (a bare reversibly-encrypted envelope) still
// authenticates through the real HTTP handler AND the row's persisted
// dev_client_secret_ref is hashed:v1:… afterward — asserted by re-reading
// the row from Postgres, not by checking that some rewrite function was
// invoked.
func TestHandleDeveloperCredentialGrant_LegacyEncryptedMigratesPersistedValue(t *testing.T) {
	enc := requireAppKeyOAuth(t)
	t.Parallel()
	principalID, plaintext := seedLegacyDeveloperGrantFixture(t, enc, "oauth-devgrant-legacymigrate@example.com")

	repo := principal.NewRepository(testpg.Pool(t))
	before, err := repo.FindByID(context.Background(), principalID)
	require.NoError(t, err)
	require.NotNil(t, before.UserIdentity.DevClientSecretRef)
	require.False(t, strings.HasPrefix(*before.UserIdentity.DevClientSecretRef, "hashed:v1:"),
		"fixture must seed the legacy (non-hashed) shape")

	s := testStateForDeveloperGrant(t, enc)
	rr := doTokenRequest(t, s, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {principalID},
		"client_secret": {plaintext},
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	after, err := repo.FindByID(context.Background(), principalID)
	require.NoError(t, err)
	require.NotNil(t, after.UserIdentity.DevClientSecretRef)
	newRef := *after.UserIdentity.DevClientSecretRef
	require.True(t, strings.HasPrefix(newRef, "hashed:v1:"),
		"a successful legacy-shape authentication must rewrite dev_client_secret_ref to hashed:v1:…, got %q", newRef)
	ok, rehash := enc.VerifySecret(newRef, plaintext)
	assert.True(t, ok, "rewritten ref must verify against the same plaintext")
	assert.False(t, rehash, "rewritten ref must be keyed under the current key with nothing left to migrate")
}

// TestHandleDeveloperCredentialGrant_LegacyEncryptedWrongSecretFails: wrong
// secret against a legacy-shape ref must still fail closed, and must not
// touch the stored ref.
func TestHandleDeveloperCredentialGrant_LegacyEncryptedWrongSecretFails(t *testing.T) {
	enc := requireAppKeyOAuth(t)
	t.Parallel()
	principalID, _ := seedLegacyDeveloperGrantFixture(t, enc, "oauth-devgrant-legacywrong@example.com")

	repo := principal.NewRepository(testpg.Pool(t))
	before, err := repo.FindByID(context.Background(), principalID)
	require.NoError(t, err)
	beforeRef := *before.UserIdentity.DevClientSecretRef

	s := testStateForDeveloperGrant(t, enc)
	rr := doTokenRequest(t, s, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {principalID},
		"client_secret": {"definitely-wrong"},
	})
	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	after, err := repo.FindByID(context.Background(), principalID)
	require.NoError(t, err)
	assert.Equal(t, beforeRef, *after.UserIdentity.DevClientSecretRef,
		"a failed verify must never rewrite the stored ref")
}
