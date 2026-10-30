//go:build integration

package oauthapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	roledomain "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// fcdev login: authorization code + PKCE for the built-in public client, then
// refresh. Everything below drives the real handlers against the real
// repositories on embedded Postgres; only the session check is faked.

const sessionCookieValue = "fcdev-test-session"

// finderWithExtras serves the real (seeded) client plus ad-hoc test clients.
type finderWithExtras struct {
	real  OAuthClientFinder
	extra map[string]*auth.OAuthClient
}

func (f finderWithExtras) FindByClientID(ctx context.Context, id string) (*auth.OAuthClient, error) {
	if c, ok := f.extra[id]; ok {
		return c, nil
	}
	return f.real.FindByClientID(ctx, id)
}

type loginRig struct {
	t          *testing.T
	s          *State
	principals *principal.Repository
	roles      *roledomain.Repository
	user       ids.PrincipalID
	roleNames  []string
}

func newLoginRig(t *testing.T, email string) *loginRig {
	t.Helper()
	ctx := context.Background()
	pool := testpg.Pool(t)
	require.NoError(t, seed.NewSeeder(pool).SeedFcdevCLIClient(ctx))
	principals := principal.NewRepository(pool)
	roles := roledomain.NewRepository(pool)
	uow := testpg.NewUoW(t)

	local, _, _ := strings.Cut(email, "@")
	app := "fl" + strings.ReplaceAll(strings.TrimPrefix(local, "fcdev-"), "-", "")
	testpg.EnsureApplication(t, app)
	mk := func(name string) string {
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
			roleops.CreateCommand{ApplicationCode: app, RoleName: name, DisplayName: name}, testpg.TestEC())
		require.NoError(t, err)
		return ev.Name
	}
	roleA, roleB := mk("reader"), mk("writer")

	userEv, err := usecaseop.Run(testpg.AnchorCtx(), uow, principalops.CreateUser(principals),
		principalops.CreateCommand{Email: email, Scope: "ANCHOR"}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, principalops.AssignRoles(principals, roles),
		principalops.AssignRolesCommand{UserID: userEv.UserID, Roles: []string{roleA, roleB}}, testpg.TestEC())
	require.NoError(t, err)

	realClients := auth.NewRepository(pool).OAuthClients
	rig := &loginRig{t: t, principals: principals, roles: roles, user: userEv.UserID, roleNames: []string{roleA, roleB}}
	rig.s = &State{
		OAuthClients:  finderWithExtras{real: realClients, extra: map[string]*auth.OAuthClient{}},
		Principals:    principals,
		AuthCodes:     grantstore.NewAuthorizationCodeRepository(pool),
		RefreshTokens: grantstore.NewRefreshTokenRepository(pool),
		PendingAuth:   grantstore.NewPendingAuthRepository(pool),
		Auth:          testAuthService(t),
		ValidateSession: func(tok string) (string, time.Time, bool) {
			if tok == sessionCookieValue {
				return string(userEv.UserID), time.Now(), true
			}
			return "", time.Time{}, false
		},
		// permission = "perm:" + role name, so tests can see exactly which roles contribute
		FlattenPermissions: func(_ context.Context, names []string) ([]string, error) {
			out := make([]string, 0, len(names))
			for _, n := range names {
				out = append(out, "perm:"+n)
			}
			return out, nil
		},
	}
	return rig
}

type pkcePair struct{ verifier, challenge string }

func newPKCE(t *testing.T) pkcePair {
	t.Helper()
	b := make([]byte, 32)
	_, err := rand.Read(b)
	require.NoError(t, err)
	v := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(v))
	return pkcePair{verifier: v, challenge: base64.RawURLEncoding.EncodeToString(sum[:])}
}

const (
	loopbackCB = "http://127.0.0.1:53682/callback"
	fcdevScope = "openid email profile offline_access"
)

func (r *loginRig) authorize(clientID, redirect string, q url.Values, withSession bool) *httptest.ResponseRecorder {
	v := url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {redirect}, "state": {"st4te"}, "scope": {fcdevScope}}
	maps.Copy(v, q)
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+v.Encode(), nil)
	if withSession {
		req.AddCookie(&http.Cookie{Name: "fc_session", Value: sessionCookieValue})
	}
	rr := httptest.NewRecorder()
	r.s.Authorize(rr, req)
	return rr
}

func (r *loginRig) pkceAuthorize(p pkcePair) *httptest.ResponseRecorder {
	return r.authorize(auth.FcdevCLIClientID, loopbackCB, url.Values{"code_challenge": {p.challenge}, "code_challenge_method": {"S256"}}, true)
}

func codeFrom(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	require.Equal(t, http.StatusTemporaryRedirect, rr.Code, rr.Body.String())
	u, err := url.Parse(rr.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:53682", u.Host)
	require.Equal(t, "st4te", u.Query().Get("state"))
	code := u.Query().Get("code")
	require.NotEmpty(t, code, rr.Header().Get("Location"))
	return code
}

func (r *loginRig) token(form url.Values) (int, map[string]any) {
	rr := doTokenRequest(r.t, r.s, form)
	var body map[string]any
	require.NoError(r.t, json.Unmarshal(rr.Body.Bytes(), &body))
	return rr.Code, body
}

func (r *loginRig) exchange(code, verifier, redirect string) (int, map[string]any) {
	return r.token(url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect},
		"client_id": {auth.FcdevCLIClientID}, "code_verifier": {verifier},
	})
}

func (r *loginRig) refresh(clientID, tok string) (int, map[string]any) {
	return r.token(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tok}, "client_id": {clientID}})
}

// login runs authorize+token and returns the token response.
func (r *loginRig) login() map[string]any {
	p := newPKCE(r.t)
	code := codeFrom(r.t, r.pkceAuthorize(p))
	st, body := r.exchange(code, p.verifier, loopbackCB)
	require.Equal(r.t, 200, st, "%v", body)
	return body
}

func refreshExpiry(t *testing.T, raw string) (created, expires time.Time) {
	t.Helper()
	rt, err := grantstore.NewRefreshTokenRepository(testpg.Pool(t)).FindByHash(context.Background(), grantstore.HashToken(raw))
	require.NoError(t, err)
	require.NotNil(t, rt)
	return rt.CreatedAt, rt.ExpiresAt
}

func TestFcdevCLIClientSeed(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	sd := seed.NewSeeder(pool)
	require.NoError(t, sd.SeedFcdevCLIClient(ctx))
	require.NoError(t, sd.SeedFcdevCLIClient(ctx), "idempotent")
	repo := auth.NewRepository(pool).OAuthClients

	c, err := repo.FindByClientID(ctx, auth.FcdevCLIClientID)
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, auth.OAuthClientPublic, c.ClientType)
	assert.Nil(t, c.SecretRef)
	assert.True(t, c.PKCERequired)
	assert.True(t, c.APIAccess)
	assert.Nil(t, c.PortalClientID)
	assert.Empty(t, c.ApplicationIDs)
	assert.ElementsMatch(t, auth.FcdevCLIGrantTypes, c.GrantTypes)
	assert.ElementsMatch(t, auth.FcdevCLIRedirectURIs, c.RedirectURIs)

	// A console edit that widens the client is undone at the next start, but
	// an operator's deactivation is respected.
	_, err = pool.Exec(ctx, `UPDATE oauth_clients SET client_type='CONFIDENTIAL', client_secret_ref='x', pkce_required=FALSE,
		api_access=FALSE, active=FALSE WHERE client_id=$1`, auth.FcdevCLIClientID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO oauth_client_redirect_uris (oauth_client_id, redirect_uri) SELECT id, 'https://evil.example/cb' FROM oauth_clients WHERE client_id=$1`, auth.FcdevCLIClientID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO oauth_client_grant_types (oauth_client_id, grant_type) SELECT id, 'client_credentials' FROM oauth_clients WHERE client_id=$1`, auth.FcdevCLIClientID)
	require.NoError(t, err)
	require.NoError(t, sd.SeedFcdevCLIClient(ctx))
	c, err = repo.FindByClientID(ctx, auth.FcdevCLIClientID)
	require.NoError(t, err)
	assert.Equal(t, auth.OAuthClientPublic, c.ClientType)
	assert.Nil(t, c.SecretRef)
	assert.True(t, c.PKCERequired)
	assert.True(t, c.APIAccess)
	assert.ElementsMatch(t, auth.FcdevCLIGrantTypes, c.GrantTypes)
	assert.ElementsMatch(t, auth.FcdevCLIRedirectURIs, c.RedirectURIs)
	assert.False(t, c.Active, "seeding must not re-activate a deactivated client")
	_, err = pool.Exec(ctx, `UPDATE oauth_clients SET active=TRUE WHERE client_id=$1`, auth.FcdevCLIClientID)
	require.NoError(t, err)
}

func TestFcdevLoginHappyPath(t *testing.T) {
	rig := newLoginRig(t, "fcdev-happy@login.test")
	body := rig.login()

	access, _ := body["access_token"].(string)
	claims := decodeClaims(t, access)
	assert.Equal(t, "api", claims["token_use"], "the person's own authority, not an identity-only token")
	assert.Equal(t, "ANCHOR", claims["tier"], "tier rules unchanged")
	assert.Equal(t, string(rig.user), claims["sub"])
	assert.Equal(t, "fcdev-happy@login.test", claims["email"])
	assert.ElementsMatch(t, rig.roleNames, claims["roles"])
	scope, _ := claims["scope"].(string)
	for _, r := range rig.roleNames {
		assert.Contains(t, scope, "perm:"+r)
	}
	assert.NotEmpty(t, body["refresh_token"])
	assert.NotEmpty(t, body["id_token"])
	assert.EqualValues(t, rig.s.Auth.AccessTokenTTLSecs(), body["expires_in"], "normal user access-token TTL")

	// the refresh family lasts 7 days by default
	created, expires := refreshExpiry(t, body["refresh_token"].(string))
	assert.InDelta(t, auth.DefaultFcdevCLIRefreshTTL.Seconds(), expires.Sub(created).Seconds(), 5)
	assert.InDelta(t, auth.DefaultFcdevCLIRefreshTTL.Seconds(), body["refresh_expires_in"], 10, "the client is told when the login ends")
}

func TestFcdevLoginRequiresLogin(t *testing.T) {
	rig := newLoginRig(t, "fcdev-nologin@login.test")
	p := newPKCE(t)
	rr := rig.authorize(auth.FcdevCLIClientID, loopbackCB, url.Values{"code_challenge": {p.challenge}, "code_challenge_method": {"S256"}}, false)
	require.Equal(t, http.StatusTemporaryRedirect, rr.Code)
	loc := rr.Header().Get("Location")
	assert.True(t, strings.HasPrefix(loc, "/auth/login?oauth=true"), loc)
	assert.Contains(t, loc, "client_id=fcdev-cli")
	assert.Contains(t, loc, "code_challenge="+p.challenge)
}

func TestFcdevPKCEEnforced(t *testing.T) {
	rig := newLoginRig(t, "fcdev-pkce@login.test")

	// no challenge: refused at authorize
	rr := rig.authorize(auth.FcdevCLIClientID, loopbackCB, nil, true)
	require.Equal(t, http.StatusTemporaryRedirect, rr.Code)
	assert.Contains(t, rr.Header().Get("Location"), "error=invalid_request")
	assert.NotContains(t, rr.Header().Get("Location"), "code=")

	// plain refused
	rr = rig.authorize(auth.FcdevCLIClientID, loopbackCB, url.Values{"code_challenge": {strings.Repeat("a", 43)}, "code_challenge_method": {"plain"}}, true)
	require.Equal(t, http.StatusTemporaryRedirect, rr.Code)
	assert.Contains(t, rr.Header().Get("Location"), "error=invalid_request")
	assert.NotContains(t, rr.Header().Get("Location"), "code=")

	// missing verifier, wrong verifier: refused at the token endpoint
	p := newPKCE(t)
	code := codeFrom(t, rig.pkceAuthorize(p))
	st, body := rig.exchange(code, "", loopbackCB)
	assert.Equal(t, 400, st)
	assert.Equal(t, "invalid_grant", body["error"])

	p = newPKCE(t)
	code = codeFrom(t, rig.pkceAuthorize(p))
	st, body = rig.exchange(code, newPKCE(t).verifier, loopbackCB)
	assert.Equal(t, 400, st)
	assert.Equal(t, "invalid_grant", body["error"])
	// the code is single use: the right verifier afterwards does not revive it
	st, _ = rig.exchange(code, p.verifier, loopbackCB)
	assert.Equal(t, 400, st)

	// a plain-method verifier (verifier == challenge) must not pass for an S256 challenge
	p = newPKCE(t)
	code = codeFrom(t, rig.pkceAuthorize(p))
	st, _ = rig.exchange(code, p.challenge, loopbackCB)
	assert.Equal(t, 400, st)

	// the client is public: presenting a secret is refused
	p = newPKCE(t)
	code = codeFrom(t, rig.pkceAuthorize(p))
	st, body = rig.token(url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {loopbackCB},
		"client_id": {auth.FcdevCLIClientID}, "client_secret": {"guess"}, "code_verifier": {p.verifier},
	})
	assert.Equal(t, 401, st)
	assert.Equal(t, "invalid_client", body["error"])
}

func TestFcdevRedirectMatching(t *testing.T) {
	rig := newLoginRig(t, "fcdev-redirect@login.test")
	p := newPKCE(t)
	q := url.Values{"code_challenge": {p.challenge}, "code_challenge_method": {"S256"}}

	for _, ok := range []string{
		"http://127.0.0.1:1/callback", "http://127.0.0.1:65535/callback", "http://[::1]:40000/callback",
	} {
		rr := rig.authorize(auth.FcdevCLIClientID, ok, q, true)
		require.Equal(t, http.StatusTemporaryRedirect, rr.Code, ok)
		assert.True(t, strings.HasPrefix(rr.Header().Get("Location"), ok+"?code="), ok)
	}
	for _, bad := range []string{
		"http://127.0.0.1.evil.com:5000/callback", "http://evil.com@127.0.0.1:5000/callback",
		"http://localhost:5000/callback", "https://127.0.0.1:5000/callback",
		"http://127.0.0.1:5000/callback/x", "http://127.0.0.1:5000/other", "http://127.0.0.1:0/callback",
		"http://127.0.0.1:65536/callback", "http://127.0.0.1:5000/callback?next=https://evil.com",
		"https://evil.example/cb",
	} {
		rr := rig.authorize(auth.FcdevCLIClientID, bad, q, true)
		assert.Equal(t, http.StatusBadRequest, rr.Code, "%s must be refused without a redirect", bad)
		assert.Empty(t, rr.Header().Get("Location"), bad)
	}

	// the same port-less registration on any other client gets exact matching only
	rig.s.OAuthClients.(finderWithExtras).extra["imposter"] = &auth.OAuthClient{
		ClientID: "imposter", Active: true, PKCERequired: true,
		RedirectURIs: auth.FcdevCLIRedirectURIs, GrantTypes: auth.FcdevCLIGrantTypes,
	}
	rr := rig.authorize("imposter", loopbackCB, q, true)
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	// the token endpoint insists the redirect_uri equals the one the code was issued for
	code := codeFrom(t, rig.pkceAuthorize(p))
	st, body := rig.exchange(code, p.verifier, "http://127.0.0.1:53683/callback")
	assert.Equal(t, 400, st)
	assert.Equal(t, "invalid_grant", body["error"])
}

func TestFcdevRefreshRotationAndAbsoluteExpiry(t *testing.T) {
	rig := newLoginRig(t, "fcdev-refresh@login.test")
	pool := testpg.Pool(t)
	body := rig.login()
	rt1 := body["refresh_token"].(string)
	created, exp1 := refreshExpiry(t, rt1)

	// refresh: new tokens, same absolute deadline
	time.Sleep(1100 * time.Millisecond) // a sliding expiry would visibly move
	st, b2 := rig.refresh(auth.FcdevCLIClientID, rt1)
	require.Equal(t, 200, st, "%v", b2)
	rt2 := b2["refresh_token"].(string)
	assert.NotEqual(t, rt1, rt2, "refresh tokens rotate")
	assert.Equal(t, "api", decodeClaims(t, b2["access_token"].(string))["token_use"])
	// the remaining time reported on refresh counts DOWN, it is not reset
	assert.Less(t, b2["refresh_expires_in"], body["refresh_expires_in"])
	_, exp2 := refreshExpiry(t, rt2)
	assert.True(t, exp2.Equal(exp1), "a refresh must not extend the absolute lifetime: %s vs %s", exp1, exp2)
	assert.InDelta(t, auth.DefaultFcdevCLIRefreshTTL.Seconds(), exp2.Sub(created).Seconds(), 5)

	// a third generation too
	st, b3 := rig.refresh(auth.FcdevCLIClientID, rt2)
	require.Equal(t, 200, st)
	rt3 := b3["refresh_token"].(string)
	_, exp3 := refreshExpiry(t, rt3)
	assert.True(t, exp3.Equal(exp1))

	// reuse detection: replaying an already-rotated token burns the whole family
	st, body = rig.refresh(auth.FcdevCLIClientID, rt1)
	assert.Equal(t, 400, st)
	assert.Equal(t, "invalid_grant", body["error"])
	st, _ = rig.refresh(auth.FcdevCLIClientID, rt3)
	assert.Equal(t, 400, st, "the family is revoked after a replay")

	// absolute expiry: once the deadline passes, refresh is refused
	body = rig.login()
	rt := body["refresh_token"].(string)
	_, err := pool.Exec(context.Background(),
		`UPDATE oauth_oidc_payloads SET expires_at = NOW() - INTERVAL '1 minute'
		  WHERE type='RefreshToken' AND payload->>'tokenHash' = $1`, grantstore.HashToken(rt))
	require.NoError(t, err)
	st, body = rig.refresh(auth.FcdevCLIClientID, rt)
	assert.Equal(t, 400, st)
	assert.Equal(t, "invalid_grant", body["error"])
}

func TestFcdevRefreshTTLIsConfigurable(t *testing.T) {
	rig := newLoginRig(t, "fcdev-ttl@login.test")
	rig.s.FcdevRefreshTTL = 36 * time.Hour
	body := rig.login()
	created, exp := refreshExpiry(t, body["refresh_token"].(string))
	assert.InDelta(t, (36 * time.Hour).Seconds(), exp.Sub(created).Seconds(), 5)
}

func TestFcdevRefreshBoundToClient(t *testing.T) {
	rig := newLoginRig(t, "fcdev-bound@login.test")
	rig.s.OAuthClients.(finderWithExtras).extra["other-public"] = &auth.OAuthClient{
		ClientID: "other-public", Active: true, ClientType: auth.OAuthClientPublic,
		RedirectURIs: []string{"https://other.example/cb"}, GrantTypes: []string{"authorization_code", "refresh_token"},
	}
	rt := rig.login()["refresh_token"].(string)

	// another client cannot redeem fcdev's refresh token
	st, body := rig.refresh("other-public", rt)
	assert.Equal(t, 400, st)
	assert.Equal(t, "invalid_grant", body["error"])

	// and fcdev cannot redeem another client's refresh token
	otherRaw, entity, err := grantstore.GenerateTokenPair(string(rig.user))
	require.NoError(t, err)
	cid := "other-public"
	entity.OAuthClientID = &cid
	entity.Scopes = []string{"openid", "offline_access"}
	require.NoError(t, rig.s.RefreshTokens.Insert(context.Background(), entity))
	st, body = rig.refresh(auth.FcdevCLIClientID, otherRaw)
	assert.Equal(t, 400, st)
	assert.Equal(t, "invalid_grant", body["error"])

	// the failed attempts did not burn the legitimate token
	st, _ = rig.refresh(auth.FcdevCLIClientID, rt)
	assert.Equal(t, 200, st)
}

func TestFcdevDeactivatedUserAndRemovedRoles(t *testing.T) {
	rig := newLoginRig(t, "fcdev-cutoff@login.test")
	uow := testpg.NewUoW(t)

	// removed roles take effect at the next refresh
	rt := rig.login()["refresh_token"].(string)
	_, err := usecaseop.Run(testpg.AnchorCtx(), uow, principalops.AssignRoles(rig.principals, rig.roles),
		principalops.AssignRolesCommand{UserID: rig.user, Roles: rig.roleNames[:1]}, testpg.TestEC())
	require.NoError(t, err)
	st, body := rig.refresh(auth.FcdevCLIClientID, rt)
	require.Equal(t, 200, st, "%v", body)
	claims := decodeClaims(t, body["access_token"].(string))
	assert.ElementsMatch(t, rig.roleNames[:1], claims["roles"])
	scope, _ := claims["scope"].(string)
	assert.Contains(t, scope, "perm:"+rig.roleNames[0])
	assert.NotContains(t, scope, "perm:"+rig.roleNames[1])
	rt = body["refresh_token"].(string)

	// a code issued before the account is deactivated cannot be redeemed after
	p := newPKCE(t)
	code := codeFrom(t, rig.pkceAuthorize(p))

	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, principalops.DeactivateUser(rig.principals),
		principalops.DeactivateCommand{ID: rig.user}, testpg.TestEC())
	require.NoError(t, err)

	st, body = rig.exchange(code, p.verifier, loopbackCB)
	assert.Equal(t, 400, st)
	assert.Equal(t, "invalid_grant", body["error"])
	st, body = rig.refresh(auth.FcdevCLIClientID, rt)
	assert.Equal(t, 400, st)
	assert.Equal(t, "invalid_grant", body["error"])
	assert.Contains(t, body["error_description"], "not active")
}

func TestFcdevRevokeEndsTheSession(t *testing.T) {
	rig := newLoginRig(t, "fcdev-revoke@login.test")
	rt := rig.login()["refresh_token"].(string)

	req := httptest.NewRequest(http.MethodPost, "/oauth/revoke",
		strings.NewReader(url.Values{"token": {rt}, "client_id": {auth.FcdevCLIClientID}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	rig.s.Revoke(rr, req)
	require.Equal(t, 200, rr.Code, rr.Body.String())

	st, body := rig.refresh(auth.FcdevCLIClientID, rt)
	assert.Equal(t, 400, st)
	assert.Equal(t, "invalid_grant", body["error"])
}

func TestFcdevDeactivatedClientIsRefused(t *testing.T) {
	rig := newLoginRig(t, "fcdev-clientoff@login.test")
	pool := testpg.Pool(t)
	ctx := context.Background()
	rt := rig.login()["refresh_token"].(string)
	_, err := pool.Exec(ctx, `UPDATE oauth_clients SET active=FALSE WHERE client_id=$1`, auth.FcdevCLIClientID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE oauth_clients SET active=TRUE WHERE client_id=$1`, auth.FcdevCLIClientID)
	})
	st, _ := rig.refresh(auth.FcdevCLIClientID, rt)
	assert.Equal(t, 401, st)
	rr := rig.pkceAuthorize(newPKCE(t))
	assert.Equal(t, 400, rr.Code)
}

func (r *loginRig) revoke(clientID, tok string) int {
	req := httptest.NewRequest(http.MethodPost, "/oauth/revoke",
		strings.NewReader(url.Values{"token": {tok}, "client_id": {clientID}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	r.s.Revoke(rr, req)
	return rr.Code
}

// A client may only revoke tokens issued to it (RFC 7009 §2.1): another
// client's revoke answers 200 but the token stays live; the legacy
// behaviour returns with RevokeAnyClient.
func TestRevokeIsScopedToTheIssuedClient(t *testing.T) {
	rig := newLoginRig(t, "fcdev-revoke-scope@login.test")
	rig.s.OAuthClients.(finderWithExtras).extra["other-public"] = &auth.OAuthClient{
		ClientID: "other-public", Active: true,
	}
	rt := rig.login()["refresh_token"].(string)
	live := func() bool {
		v, err := rig.s.RefreshTokens.FindValidByHash(context.Background(), grantstore.HashToken(rt))
		require.NoError(t, err)
		return v != nil
	}

	require.Equal(t, 200, rig.revoke("other-public", rt))
	assert.True(t, live(), "another client must not revoke the token")

	rig.s.RevokeAnyClient = true
	require.Equal(t, 200, rig.revoke("other-public", rt))
	assert.False(t, live(), "legacy flag restores cross-client revoke")
}

// IntrospectClientAuthOnly turns Bearer authentication on /oauth/introspect
// into invalid_client; by default a Bearer is evaluated as a token.
func TestIntrospectClientAuthOnlyRefusesBearer(t *testing.T) {
	rig := newLoginRig(t, "fcdev-introspect@login.test")
	call := func() map[string]any {
		req := httptest.NewRequest(http.MethodPost, "/oauth/introspect", strings.NewReader(url.Values{"token": {"x"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Authorization", "Bearer not-a-real-token")
		rr := httptest.NewRecorder()
		rig.s.Introspect(rr, req)
		var body map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &body)
		return body
	}
	assert.Equal(t, "invalid_token", call()["error"])
	rig.s.IntrospectClientAuthOnly = true
	assert.Equal(t, "invalid_client", call()["error"])
}
