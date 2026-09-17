//go:build integration

package bridge

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	jsonschema "github.com/google/jsonschema-go/jsonschema"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	edmops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	idpops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/loginattempt"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// ── fake IdP: a real (test) OIDC provider driving a real go-oidc handshake ──
//
// This is the harness docs/spec/oidc-logged-in-event.md's T1-T6 drive
// against: discovery + JWKS + token endpoint are real HTTP, the id_token is
// a genuinely RS256-signed JWT the bridge's go-oidc client verifies for
// real. Only the "browser" leg (handleLogin's redirect to the IdP, and the
// IdP's own login UI) is skipped — a login_state row is seeded directly,
// exactly as if that leg had already happened.
type fakeIdP struct {
	server   *httptest.Server
	signKey  jwk.Key
	clientID string

	issuerURL string
	// idTokenClaims / accessToken are read by the /token handler for the
	// NEXT exchange; each test sets them before driving a callback.
	idTokenClaims map[string]any
	accessToken   string
	// signKeyOverride, when set, signs the NEXT id_token with this key
	// instead of signKey. The JWKS still advertises signKey's public half,
	// so the resulting token is syntactically well-formed but fails
	// signature verification — used to drive T3
	// (docs/spec/sso-login-attempts.md: a bad-signature id_token).
	signKeyOverride jwk.Key
}

func newFakeIdP(t *testing.T, clientID string) *fakeIdP {
	t.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	priv, err := jwk.FromRaw(raw)
	require.NoError(t, err)
	require.NoError(t, priv.Set(jwk.KeyIDKey, "fake-idp-key"))
	require.NoError(t, priv.Set(jwk.AlgorithmKey, jwa.RS256))
	pub, err := priv.PublicKey()
	require.NoError(t, err)
	require.NoError(t, pub.Set(jwk.KeyUsageKey, "sig"))
	set := jwk.NewSet()
	require.NoError(t, set.AddKey(pub))

	f := &fakeIdP{signKey: priv, clientID: clientID, accessToken: "opaque-default-token"}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                f.issuerURL,
			"authorization_endpoint":                f.issuerURL + "/authorize",
			"token_endpoint":                        f.issuerURL + "/token",
			"jwks_uri":                              f.issuerURL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		raw, err := json.Marshal(set)
		require.NoError(t, err)
		_, _ = w.Write(raw)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		idToken := f.mintIDToken(t)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": f.accessToken,
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     idToken,
		})
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	f.issuerURL = f.server.URL
	return f
}

// mintIDToken signs a fresh RS256 id_token carrying the standard claims plus
// whatever f.idTokenClaims holds (set per-test before driving the callback;
// this is how the test controls nonce/email/roles/custom claims).
func (f *fakeIdP) mintIDToken(t *testing.T) string {
	t.Helper()
	tok := jwt.New()
	require.NoError(t, tok.Set(jwt.IssuerKey, f.issuerURL))
	require.NoError(t, tok.Set(jwt.SubjectKey, "fake-idp-subject"))
	require.NoError(t, tok.Set(jwt.AudienceKey, f.clientID))
	require.NoError(t, tok.Set(jwt.ExpirationKey, time.Now().Add(time.Hour)))
	require.NoError(t, tok.Set(jwt.IssuedAtKey, time.Now().Add(-time.Minute)))
	for k, v := range f.idTokenClaims {
		require.NoError(t, tok.Set(k, v))
	}
	key := f.signKey
	if f.signKeyOverride != nil {
		key = f.signKeyOverride
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, key, jws.WithProtectedHeaders(jws.NewHeaders())))
	require.NoError(t, err)
	return string(signed)
}

// opaqueOrJWTAccessToken builds a syntactically-valid (unsigned; access
// tokens are never signature-checked by this platform) 3-part JWT string
// carrying payload as its middle segment — for T3's "JWT access token" case.
func fakeJWTAccessToken(t *testing.T, payload map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return header + "." + base64.RawURLEncoding.EncodeToString(body) + ".sig"
}

// ── login fixture: an IdP + ANCHOR-scope email-domain mapping bound to the
// fake IdP, one role, and one IDP-role-mapping translating a raw claim value
// to it — everything the mapping-based (employee-plane) callback needs.

type oidcTestFixture struct {
	t             *testing.T
	pool          *pgxpool.Pool
	endpoint      *LoginEndpoint
	states        *LoginStateRepo
	idp           *fakeIdP
	loginAttempts *loginattempt.Repository

	domain               string
	identityProviderID   string
	identityProviderCode string
	mappingID            string
	// platformRoleName is what a successful IDP role sync grants when the
	// id_token's "roles" claim includes rawIdpRoleName.
	platformRoleName string
	rawIdpRoleName   string

	// lastSessionPrincipalID / lastSessionCode capture what the (overridden)
	// SessionWriter received — nil/zero until a callback completes without
	// an httperror short-circuit.
	lastSessionPrincipalID string
	lastSessionCalled      bool
}

func newOidcTestFixture(t *testing.T) *oidcTestFixture {
	t.Helper()
	suffix := strings.ToLower(randString(6))
	suffix = strings.NewReplacer("-", "a", "_", "b").Replace(suffix)

	pool := testpg.Pool(t)
	principals := principal.NewRepository(pool)
	mappings := emaildomainmapping.NewRepository(pool)
	idps := identityprovider.NewRepository(pool)
	roles := role.NewRepository(pool)
	authRepo := platformauth.NewRepository(pool)
	uow := testpg.NewUoW(t)
	ctx := context.Background()

	clientID := "client-" + suffix
	domain := suffix + ".test"
	idpCode := "idp-" + suffix
	appCode := "app" + suffix
	rawIdpRoleName := "raw-role-" + suffix
	platformRoleName := appCode + ":engineer"

	roleEv, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
		roleops.CreateCommand{ApplicationCode: appCode, RoleName: "engineer", DisplayName: "Engineer"},
		testpg.TestEC())
	require.NoError(t, err)
	require.Equal(t, platformRoleName, roleEv.Name)

	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, authops.CreateIdpRoleMapping(authRepo.IdpRoleMappings),
		authops.CreateIdpRoleMappingCommand{
			IdpType: "OIDC", IdpRoleName: rawIdpRoleName, PlatformRoleName: platformRoleName,
		}, testpg.TestEC())
	require.NoError(t, err)

	fake := newFakeIdP(t, clientID)

	idpResult, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		idpops.CreateIdentityProvider(idpops.Deps{
			Repo: idps,
			MoveDeps: edmops.MoveDeps{
				Mappings:   mappings,
				IDPs:       idps,
				Principals: principals,
			},
		}),
		idpops.CreateCommand{
			Code: idpCode, Name: "Fake IdP " + suffix, Type: "OIDC",
			OIDCIssuerURL: &fake.issuerURL, OIDCClientID: &clientID,
			AllowedEmailDomains: []string{domain},
			MappingScope:        new("ANCHOR"),
			SyncRolesFromIDP:    true,
			AllowedRoleIDs:      []string{roleEv.RoleID},
		}, testpg.TestEC())
	require.NoError(t, err)

	mapping, err := mappings.FindByEmailDomain(ctx, domain)
	require.NoError(t, err)
	require.NotNil(t, mapping)

	states := NewLoginStateRepo(pool)
	b := NewBridge(mappings, idps, nil) // enc nil: the fake IdP is a public client (no secret)

	f := &oidcTestFixture{
		t: t, pool: pool, states: states, idp: fake,
		domain:               domain,
		identityProviderID:   idpResult.IdentityProviderID,
		identityProviderCode: idpCode,
		mappingID:            mapping.ID,
		platformRoleName:     platformRoleName,
		rawIdpRoleName:       rawIdpRoleName,
	}

	attempts := loginattempt.NewRepository(pool)
	f.loginAttempts = attempts

	ep := NewLoginEndpoint(b, states, principals, mappings, roles, authRepo.IdpRoleMappings, uow, authRepo.OAuthClients)
	ep.ExternalBaseURL = "https://fc.test"
	ep.LoginAttempts = attempts
	// The production default SessionWriter passes a nil *http.Request into
	// http.Redirect for a relative return URL, which panics — unrelated to
	// this spec. Override with a plain recorder so these tests exercise the
	// callback's login logic (and the event emit) without tripping that.
	ep.SessionWriter = func(w http.ResponseWriter, _ *http.Request, principalID string, _ string) {
		f.lastSessionCalled = true
		f.lastSessionPrincipalID = principalID
		w.WriteHeader(http.StatusOK)
	}
	f.endpoint = ep
	return f
}

// login drives the real /auth/oidc/callback handler: seeds a login_state row
// (standing in for the redirect leg handleLogin would have done), points the
// fake IdP at the given id_token claims + access token for its next /token
// response, and invokes handleCallback directly.
func (f *oidcTestFixture) login(t *testing.T, idTokenClaims map[string]any, accessToken string) *httptest.ResponseRecorder {
	t.Helper()
	nonce, _ := idTokenClaims["nonce"].(string)
	require.NotEmpty(t, nonce, "test bug: idTokenClaims must set \"nonce\"")

	state := randString(16)
	loginState := NewLoginState(state, f.domain, f.identityProviderID, f.mappingID, nonce, "")
	require.NoError(t, f.states.Insert(context.Background(), loginState))

	f.idp.idTokenClaims = idTokenClaims
	f.idp.accessToken = accessToken
	f.lastSessionCalled = false
	f.lastSessionPrincipalID = ""

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state="+state+"&code=fake-code-"+state, nil)
	// docs/spec/sso-login-attempts.md's T1-T5 harness contract: every
	// callback in this file carries these so the login-attempt tests can
	// assert on IP/UA without a bespoke request per case.
	setSSOTestHeaders(req)
	rec := httptest.NewRecorder()
	f.endpoint.handleCallback(rec, req)
	return rec
}

// setSSOTestHeaders applies the fixed X-Forwarded-For / User-Agent pair
// docs/spec/sso-login-attempts.md's test table specifies: rightmost
// X-Forwarded-For hop 198.51.100.7, User-Agent fc-test/1.0.
func setSSOTestHeaders(r *http.Request) {
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 198.51.100.7")
	r.Header.Set("User-Agent", "fc-test/1.0")
}

// recentSSOAttempts returns iam_login_attempts USER_LOGIN rows recorded at or
// after since, most recent first. Tests run sequentially against the shared
// testpg instance (no t.Parallel() in this file), so a `since` timestamp
// captured just before driving one callback reliably isolates that
// callback's own writes (docs/spec/sso-login-attempts.md).
func recentSSOAttempts(t *testing.T, pool *pgxpool.Pool, since time.Time) []loginattempt.LoginAttempt {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT id, attempt_type, outcome, failure_reason, identifier, principal_id,
		        ip_address, user_agent, attempted_at
		   FROM iam_login_attempts
		  WHERE attempt_type = 'USER_LOGIN' AND attempted_at >= $1
		  ORDER BY attempted_at DESC`, since)
	require.NoError(t, err)
	defer rows.Close()
	var out []loginattempt.LoginAttempt
	for rows.Next() {
		var a loginattempt.LoginAttempt
		var attemptType, outcome string
		require.NoError(t, rows.Scan(&a.ID, &attemptType, &outcome, &a.FailureReason, &a.Identifier,
			&a.PrincipalID, &a.IPAddress, &a.UserAgent, &a.AttemptedAt))
		a.AttemptType = loginattempt.ParseAttemptType(attemptType)
		a.Outcome, _ = loginattempt.ParseOutcome(outcome)
		out = append(out, a)
	}
	require.NoError(t, rows.Err())
	return out
}

// loggedInEventData returns the decoded `data` column of every
// platform:iam:user:logged-in row for the given email, most recent last.
func loggedInEventData(t *testing.T, pool *pgxpool.Pool, email string) []map[string]any {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT data FROM msg_events WHERE type = 'platform:iam:user:logged-in' AND data->>'email' = $1 ORDER BY created_at`,
		email)
	require.NoError(t, err)
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var raw []byte
		require.NoError(t, rows.Scan(&raw))
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		out = append(out, m)
	}
	require.NoError(t, rows.Err())
	return out
}

// ── T1 ───────────────────────────────────────────────────────────────────

// TestOidcLogin_EmitsExactlyOneLoggedInEvent pins T1: a successful OIDC
// login produces exactly one platform:iam:user:logged-in event for that
// user, with loginMethod OIDC and identityProviderCode equal to the IdP's
// code. Mutant: remove the emit call in handleCallback.
func TestOidcLogin_EmitsExactlyOneLoggedInEvent(t *testing.T) {
	f := newOidcTestFixture(t)
	email := "alice@" + f.domain

	rec := f.login(t, map[string]any{"nonce": "nonce-t1", "email": email}, "opaque-t1")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.True(t, f.lastSessionCalled)

	rows := loggedInEventData(t, f.pool, email)
	require.Len(t, rows, 1, "exactly one logged-in event for this user")
	assert.Equal(t, "OIDC", rows[0]["loginMethod"])
	assert.Equal(t, f.identityProviderCode, rows[0]["identityProviderCode"])
	assert.Equal(t, f.lastSessionPrincipalID, rows[0]["userId"])
}

// ── T2 ───────────────────────────────────────────────────────────────────

// TestOidcLogin_FederatedIdTokenClaimsCustomFieldStrippedNonce pins T2:
// federatedClaims.idToken carries a custom claim the fake IdP issued and
// does NOT carry nonce. Mutant: don't strip nonce (or read only parsed
// fields, which would drop "department" too).
func TestOidcLogin_FederatedIdTokenClaimsCustomFieldStrippedNonce(t *testing.T) {
	f := newOidcTestFixture(t)
	email := "bob@" + f.domain

	rec := f.login(t, map[string]any{
		"nonce": "nonce-t2", "email": email, "department": "engineering",
	}, "opaque-t2")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	rows := loggedInEventData(t, f.pool, email)
	require.Len(t, rows, 1)
	fed, ok := rows[0]["federatedClaims"].(map[string]any)
	require.True(t, ok, "federatedClaims must be an object")
	idTok, ok := fed["idToken"].(map[string]any)
	require.True(t, ok, "idToken must be an object")
	assert.Equal(t, "engineering", idTok["department"], "a custom IdP claim must survive into the event")
	_, hasNonce := idTok["nonce"]
	assert.False(t, hasNonce, "nonce must be stripped from the stored idToken claims")
}

// ── T3 ───────────────────────────────────────────────────────────────────

// TestOidcLogin_AccessTokenDecodedWhenJWTOpaqueOtherwise pins T3: a JWT
// access token yields its payload claim in federatedClaims.accessToken; an
// opaque access token yields {}. Mutant: always {}.
func TestOidcLogin_AccessTokenDecodedWhenJWTOpaqueOtherwise(t *testing.T) {
	f := newOidcTestFixture(t)

	jwtEmail := "carol-jwt@" + f.domain
	jwtAT := fakeJWTAccessToken(t, map[string]any{"scope": "read write", "sub": "svc-account-1"})
	rec := f.login(t, map[string]any{"nonce": "nonce-t3-jwt", "email": jwtEmail}, jwtAT)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	rows := loggedInEventData(t, f.pool, jwtEmail)
	require.Len(t, rows, 1)
	fed := rows[0]["federatedClaims"].(map[string]any)
	at := fed["accessToken"].(map[string]any)
	assert.Equal(t, "read write", at["scope"])
	assert.Equal(t, "svc-account-1", at["sub"])

	opaqueEmail := "carol-opaque@" + f.domain
	rec = f.login(t, map[string]any{"nonce": "nonce-t3-opaque", "email": opaqueEmail}, "just-an-opaque-token-string")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	rows = loggedInEventData(t, f.pool, opaqueEmail)
	require.Len(t, rows, 1)
	fed = rows[0]["federatedClaims"].(map[string]any)
	at = fed["accessToken"].(map[string]any)
	assert.Empty(t, at, "an opaque access token must decode to {}")
}

// ── T4 ───────────────────────────────────────────────────────────────────

// TestOidcLogin_RolesReflectThisLoginsIdpSync pins T4:
// flowcatalystClaims.roles includes a role granted by THIS login's IDP role
// sync, and applications is its prefix. Mutant: use the principal as loaded
// before the sync (skip the re-read).
func TestOidcLogin_RolesReflectThisLoginsIdpSync(t *testing.T) {
	f := newOidcTestFixture(t)
	email := "dave@" + f.domain

	rec := f.login(t, map[string]any{
		"nonce": "nonce-t4", "email": email, "roles": []string{f.rawIdpRoleName},
	}, "opaque-t4")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	rows := loggedInEventData(t, f.pool, email)
	require.Len(t, rows, 1)
	fc := rows[0]["flowcatalystClaims"].(map[string]any)

	rolesAny := fc["roles"].([]any)
	roles := make([]string, len(rolesAny))
	for i, r := range rolesAny {
		roles[i] = r.(string)
	}
	assert.Contains(t, roles, f.platformRoleName, "the role this login's IDP sync just granted must be in the event")

	appsAny := fc["applications"].([]any)
	apps := make([]string, len(appsAny))
	for i, a := range appsAny {
		apps[i] = a.(string)
	}
	prefix := strings.SplitN(f.platformRoleName, ":", 2)[0]
	assert.Contains(t, apps, prefix, "applications must contain the synced role's prefix")
}

// ── T5 ───────────────────────────────────────────────────────────────────

// TestOidcLogin_FailedLoginEmitsNoEvent pins T5: a failed login (nonce
// mismatch) emits no event. Mutant: emit before the failure checks.
func TestOidcLogin_FailedLoginEmitsNoEvent(t *testing.T) {
	f := newOidcTestFixture(t)
	email := "erin@" + f.domain

	// The id_token's nonce ("actual-nonce") will never match what the login
	// state was seeded with (login() seeds the state's nonce from the
	// claims map itself, so mismatch it by hand here instead).
	state := randString(16)
	loginState := NewLoginState(state, f.domain, f.identityProviderID, f.mappingID, "expected-nonce", "")
	require.NoError(t, f.states.Insert(context.Background(), loginState))
	f.idp.idTokenClaims = map[string]any{"nonce": "wrong-nonce", "email": email}
	f.idp.accessToken = "opaque-t5"

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state="+state+"&code=fake-code-t5", nil)
	rec := httptest.NewRecorder()
	f.endpoint.handleCallback(rec, req)

	require.NotEqual(t, http.StatusOK, rec.Code, "a nonce mismatch must not succeed")
	assert.False(t, f.lastSessionCalled, "a failed login must never reach SessionWriter")
	rows := loggedInEventData(t, f.pool, email)
	assert.Empty(t, rows, "a failed login must emit no event")
}

// ── T6 ───────────────────────────────────────────────────────────────────

// TestOidcLogin_EventValidatesAgainstSeededSchema pins T6: the stored event
// data validates against the seeded platform:iam:user:logged-in schema.
// Mutant: break a required field name.
func TestOidcLogin_EventValidatesAgainstSeededSchema(t *testing.T) {
	f := newOidcTestFixture(t)
	email := "frank@" + f.domain

	rec := f.login(t, map[string]any{
		"nonce": "nonce-t6", "email": email, "roles": []string{f.rawIdpRoleName},
	}, fakeJWTAccessToken(t, map[string]any{"scope": "x"}))
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	rows := loggedInEventData(t, f.pool, email)
	require.Len(t, rows, 1)

	var rawSchema []byte
	for _, def := range seed.PlatformEventTypes() {
		if def.Code == "platform:iam:user:logged-in" {
			rawSchema = def.Schema
			break
		}
	}
	require.NotEmpty(t, rawSchema, "the logged-in event type must have a seeded schema")

	var schema jsonschema.Schema
	require.NoError(t, json.Unmarshal(rawSchema, &schema))
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)
	assert.NoError(t, resolved.Validate(rows[0]), "the stored event data must validate against its seeded schema")
}

// ── docs/spec/sso-login-attempts.md T1-T5 ─────────────────────────────────
//
// SSO logins write iam_login_attempts rows on the callbacks the platform
// itself accepts or refuses (employee plane only — a portal-flow state
// writes nothing, success or failure). These reuse the same fake-IdP
// callback harness above; f.login()/setSSOTestHeaders supply the fixed
// X-Forwarded-For / User-Agent pair the spec's test table specifies.

// TestSSOLoginAttempt_SuccessRecordsOneRow pins T1: a successful SSO login
// writes exactly one USER_LOGIN SUCCESS row with identifier = the email,
// the resolved principal id, and the request's IP/UA. Mutant: remove the
// success write.
func TestSSOLoginAttempt_SuccessRecordsOneRow(t *testing.T) {
	f := newOidcTestFixture(t)
	email := "gina@" + f.domain
	since := time.Now().UTC()

	rec := f.login(t, map[string]any{"nonce": "nonce-sso-t1", "email": email}, "opaque-sso-t1")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.True(t, f.lastSessionCalled)

	rows := recentSSOAttempts(t, f.pool, since)
	require.Len(t, rows, 1, "exactly one login-attempt row for this callback")
	row := rows[0]
	assert.Equal(t, loginattempt.OutcomeSuccess, row.Outcome)
	require.NotNil(t, row.Identifier)
	assert.Equal(t, email, *row.Identifier)
	require.NotNil(t, row.PrincipalID)
	assert.Equal(t, f.lastSessionPrincipalID, *row.PrincipalID)
	require.NotNil(t, row.IPAddress)
	assert.Equal(t, "198.51.100.7", *row.IPAddress, "must be the rightmost X-Forwarded-For hop")
	require.NotNil(t, row.UserAgent)
	assert.Equal(t, "fc-test/1.0", *row.UserAgent)
}

// TestSSOLoginAttempt_EmailDomainMismatchRecordsFailure pins T2: a refused
// callback (email-domain mismatch, the cheapest of the table's refusals to
// drive) writes one FAILURE row with the table's reason and the verified
// email as identifier; principal id is null. Mutant: remove that failure
// write.
func TestSSOLoginAttempt_EmailDomainMismatchRecordsFailure(t *testing.T) {
	f := newOidcTestFixture(t)
	// A verified email whose domain does NOT match the login's mapped
	// domain — the mapping-based (non-provider-direct) branch's
	// EMAIL_DOMAIN_MISMATCH check.
	email := "hank@not-" + f.domain
	since := time.Now().UTC()

	rec := f.login(t, map[string]any{"nonce": "nonce-sso-t2", "email": email}, "opaque-sso-t2")
	require.NotEqual(t, http.StatusOK, rec.Code, "an email-domain mismatch must be refused")
	assert.False(t, f.lastSessionCalled)

	rows := recentSSOAttempts(t, f.pool, since)
	require.Len(t, rows, 1, "exactly one login-attempt row for this refusal")
	row := rows[0]
	assert.Equal(t, loginattempt.OutcomeFailure, row.Outcome)
	require.NotNil(t, row.FailureReason)
	assert.Equal(t, "SSO: email domain not allowed", *row.FailureReason)
	require.NotNil(t, row.Identifier)
	assert.Equal(t, email, *row.Identifier)
	assert.Nil(t, row.PrincipalID, "principal id must be null on every recorded failure")
}

// TestSSOLoginAttempt_BadSignatureRecordsNullIdentifier pins T3: an
// id_token signed with a key OTHER than the one the IdP's JWKS advertises
// fails verification and writes one FAILURE row with the table's reason and
// a NULL identifier — the presented (unverified) token's email claim must
// never be trusted as the identifier. Mutant: record the unverified token's
// email as identifier.
func TestSSOLoginAttempt_BadSignatureRecordsNullIdentifier(t *testing.T) {
	f := newOidcTestFixture(t)
	email := "ivan@" + f.domain
	since := time.Now().UTC()

	badKeyRaw, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	badKey, err := jwk.FromRaw(badKeyRaw)
	require.NoError(t, err)
	require.NoError(t, badKey.Set(jwk.AlgorithmKey, jwa.RS256))
	f.idp.signKeyOverride = badKey

	rec := f.login(t, map[string]any{"nonce": "nonce-sso-t3", "email": email}, "opaque-sso-t3")
	require.NotEqual(t, http.StatusOK, rec.Code, "a bad-signature id_token must be refused")
	assert.False(t, f.lastSessionCalled)

	rows := recentSSOAttempts(t, f.pool, since)
	require.Len(t, rows, 1, "exactly one login-attempt row for this refusal")
	row := rows[0]
	assert.Equal(t, loginattempt.OutcomeFailure, row.Outcome)
	require.NotNil(t, row.FailureReason)
	assert.Equal(t, "SSO: id_token verification failed", *row.FailureReason)
	assert.Nil(t, row.Identifier, "an unverified token's claims must never become the identifier")
	assert.Nil(t, row.PrincipalID)
}

// TestSSOLoginAttempt_UnknownStateRecordsNoRow pins T4: an unknown/expired
// state writes no login-attempt row at all — it's infrastructure/replay
// noise, not an identity being refused. Mutant: record on that branch.
func TestSSOLoginAttempt_UnknownStateRecordsNoRow(t *testing.T) {
	f := newOidcTestFixture(t)
	since := time.Now().UTC()

	req := httptest.NewRequest(http.MethodGet,
		"/auth/oidc/callback?state=never-existed-"+randString(8)+"&code=fake-code", nil)
	setSSOTestHeaders(req)
	rec := httptest.NewRecorder()
	f.endpoint.handleCallback(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, f.lastSessionCalled)

	rows := recentSSOAttempts(t, f.pool, since)
	assert.Empty(t, rows, "an unknown/expired state must write no login-attempt row")
}

// TestSSOLoginAttempt_PortalFlowRecordsNoRow pins T5: a login state that
// routes to the portal completion writes no row at all — success or
// failure. Driven cheaply as a provider-direct, portal-flagged state that
// also fails the table's EMAIL_DOMAIN_MISMATCH refusal: this is a real call
// site that WOULD record for an employee-plane login, so the assertion
// genuinely exercises the portal-plane guard rather than merely avoiding
// every call site by accident. Mutant: drop the plane check.
func TestSSOLoginAttempt_PortalFlowRecordsNoRow(t *testing.T) {
	f := newOidcTestFixture(t)
	// Domain deliberately outside the IdP's allowed_email_domains (=
	// {f.domain}) so the providerDirect EMAIL_DOMAIN_MISMATCH branch fires
	// before the portal hand-off — proving the plane check, not just that
	// the portal branch itself never records.
	email := "julia@not-" + f.domain
	since := time.Now().UTC()

	state := randString(16)
	// mappingID "" marks provider-direct, exactly as handlePortalOIDCLogin
	// builds a portal state.
	loginState := NewLoginState(state, "", f.identityProviderID, "", "nonce-sso-t5", "")
	// oauth_oidc_login_states.portal_client_id is VARCHAR(17) (TSID-shaped in
	// production); any non-empty value is enough to mark the state
	// portal-flagged for this test, which never resolves it against a real
	// client (e.Portal is unset in this fixture).
	portalClientID := "portal-test-id-1"
	loginState.PortalClientID = &portalClientID
	require.NoError(t, f.states.Insert(context.Background(), loginState))

	f.idp.idTokenClaims = map[string]any{"nonce": "nonce-sso-t5", "email": email}
	f.idp.accessToken = "opaque-sso-t5"

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state="+state+"&code=fake-code-"+state, nil)
	setSSOTestHeaders(req)
	rec := httptest.NewRecorder()
	f.endpoint.handleCallback(rec, req)

	require.NotEqual(t, http.StatusOK, rec.Code, "the email-domain mismatch must still be refused")
	assert.False(t, f.lastSessionCalled)

	rows := recentSSOAttempts(t, f.pool, since)
	assert.Empty(t, rows, "a portal-flow login state must write no login-attempt row, success or failure")
}
