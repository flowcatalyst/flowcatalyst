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
			"issuer":                                 f.issuerURL,
			"authorization_endpoint":                 f.issuerURL + "/authorize",
			"token_endpoint":                          f.issuerURL + "/token",
			"jwks_uri":                                f.issuerURL + "/jwks",
			"id_token_signing_alg_values_supported":   []string{"RS256"},
			"response_types_supported":                []string{"code"},
			"subject_types_supported":                 []string{"public"},
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
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, f.signKey, jws.WithProtectedHeaders(jws.NewHeaders())))
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
	t        *testing.T
	pool     *pgxpool.Pool
	endpoint *LoginEndpoint
	states   *LoginStateRepo
	idp      *fakeIdP

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

	ep := NewLoginEndpoint(b, states, principals, mappings, roles, authRepo.IdpRoleMappings, uow, authRepo.OAuthClients)
	ep.ExternalBaseURL = "https://fc.test"
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
	rec := httptest.NewRecorder()
	f.endpoint.handleCallback(rec, req)
	return rec
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
