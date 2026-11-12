//go:build integration

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/clientselection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Each token class works only on its own transport: the fc_session cookie takes
// a session token, the Authorization header takes an API access token, and the
// routes that mint a session-grade token or change how a principal signs in
// refuse an API bearer. These tests drive the whole router (WirePlatform on
// embedded Postgres) with a session cookie and an API bearer of one principal.

const (
	taSessionRequiredMsg = "This action needs a signed-in browser session; an API token cannot perform it"
	taSameOrigin         = "same-origin"
)

// taSend sends one request through the wired router with whichever credentials
// are given (empty = none), as a same-origin browser would.
func taSend(t *testing.T, x *xoHarness, method, path, body, cookie, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "http://platform.example.com"+path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: platformmw.SessionCookieName, Value: cookie})
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	r.Header.Set("Sec-Fetch-Site", taSameOrigin)
	w := httptest.NewRecorder()
	x.h.ServeHTTP(w, r)
	return w
}

// requireSessionRequired asserts the 403 SESSION_REQUIRED refusal, in either
// error envelope the platform uses ({"code"} for chi handlers, {"error"} for huma).
func requireSessionRequired(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	var env struct {
		Code    string `json:"code"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), w.Body.String())
	code := env.Code
	if code == "" {
		code = env.Error
	}
	require.Equal(t, "SESSION_REQUIRED", code, w.Body.String())
	require.Equal(t, taSessionRequiredMsg, env.Message, w.Body.String())
}

func taSubject(t *testing.T, tok string) string {
	t.Helper()
	mc := jwt.MapClaims{}
	_, _, err := jwt.NewParser().ParseUnverified(tok, mc)
	require.NoError(t, err)
	sub, _ := mc["sub"].(string)
	require.NotEmpty(t, sub)
	return sub
}

func taActiveClient(t *testing.T) string {
	t.Helper()
	id := string(ids.NewClientID())
	_, err := testpg.Pool(t).Exec(context.Background(),
		`INSERT INTO tnt_clients (id, name, identifier, status) VALUES ($1, 'TA client', $2, 'ACTIVE')`,
		id, "ta-"+strings.ToLower(strings.ReplaceAll(id, "_", "-")))
	require.NoError(t, err)
	return id
}

// T2 (end to end): an API token sent as the fc_session cookie authenticates
// nothing, instead of being upgraded to the principal's database authority.
func TestAPITokenInSessionCookieIsAnonymous(t *testing.T) {
	x := newXOHarness(t)

	// control: the real session cookie signs in, and the bearer does too
	require.Equal(t, http.StatusOK, taSend(t, x, http.MethodGet, "/auth/me", "", x.cookie, "").Code)
	require.Equal(t, http.StatusOK, taSend(t, x, http.MethodGet, "/auth/me", "", "", x.bearer).Code)

	w := taSend(t, x, http.MethodGet, "/auth/me", "", x.bearer, "")
	require.Equal(t, http.StatusUnauthorized, w.Code, "an API token in the cookie must not authenticate: %s", w.Body.String())
}

// T3 (end to end): a session token presented as a Bearer is refused with 401.
func TestSessionTokenAsBearerIsRefused(t *testing.T) {
	x := newXOHarness(t)
	w := taSend(t, x, http.MethodGet, "/auth/me", "", "", x.cookie)
	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

// T5: /auth/client/switch.
func TestClientSwitchRules(t *testing.T) {
	x := newXOHarness(t)
	cid := taActiveClient(t)
	body := `{"clientId":"` + cid + `"}`

	// Bearer-authenticated (the API token the principal also holds): refused.
	requireSessionRequired(t, taSend(t, x, http.MethodPost, "/auth/client/switch", body, "", x.bearer))

	// Cookie session of an active principal: unchanged, 200 with a token.
	w := taSend(t, x, http.MethodPost, "/auth/client/switch", body, x.cookie, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out struct {
		Token string `json:"token"`
		Roles []string
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.NotEmpty(t, out.Token)
	require.NotEmpty(t, out.Roles)

	// GET /auth/client/accessible is a read: a bearer still gets it.
	require.Equal(t, http.StatusOK, taSend(t, x, http.MethodGet, "/auth/client/accessible", "", "", x.bearer).Code)

	// The principal is deactivated: the cookie no longer signs in, so the
	// switch cannot be reached through the router either.
	_, err := testpg.Pool(t).Exec(context.Background(),
		`UPDATE iam_principals SET active = FALSE WHERE id = $1`, taSubject(t, x.cookie))
	require.NoError(t, err)
	w = taSend(t, x, http.MethodPost, "/auth/client/switch", body, x.cookie, "")
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "UNAUTHENTICATED")
	require.NotContains(t, w.Body.String(), `"token"`)
}

// T5 (handler level): a session AuthContext for a principal that is no longer
// active is refused by the handler itself, not only by the authenticator in
// front of it; an active one is served.
func TestClientSwitchRefusesInactivePrincipal(t *testing.T) {
	x := newXOHarness(t)
	pool := testpg.Pool(t)
	cfg := EnvCfg{
		JWTIssuer: xoPublicBase, SessionTTLSecs: 3600, AccessTokenTTLSecs: 3600, RefreshTokenTTLSecs: 3600,
		FunctionsArtifactStore: "file://" + t.TempDir(),
	}
	keyPath, err := EnsureSigningKeyFile(filepath.Join(t.TempDir(), "jwt.pem"))
	require.NoError(t, err)
	cfg.JWTSigningKeyPath = keyPath
	repos := buildRepos(pool)
	svcs, err := buildServices(cfg, pool, repos)
	require.NoError(t, err)

	pid := taSubject(t, x.cookie)
	cid := taActiveClient(t)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler { // a session context injected the way a cookie would build it
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := auth.WithContext(req.Context(), &auth.AuthContext{PrincipalID: pid, Scope: auth.ScopeAnchor})
			next.ServeHTTP(w, req.WithContext(platformmw.WithSessionCookieAuth(ctx)))
		})
	})
	clientselection.RegisterRoutes(r, &clientselection.State{
		Principals: repos.principalRepo, Clients: repos.clientRepo, Roles: repos.roleRepo,
		Grants: repos.principalGrantRepo, Auth: svcs.authSvc,
	})
	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/auth/client/switch", strings.NewReader(`{"clientId":"`+cid+`"}`))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	require.Equal(t, http.StatusOK, post().Code)

	_, err = pool.Exec(context.Background(), `UPDATE iam_principals SET active = FALSE WHERE id = $1`, pid)
	require.NoError(t, err)
	w := post()
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), `"token"`)
}

// T6: /oauth/authorize only treats the fc_session cookie, and only a session
// token in it, as a signed-in browser.
func TestAuthorizeIgnoresAPITokens(t *testing.T) {
	x := newXOHarness(t)
	pool := testpg.Pool(t)
	require.NoError(t, seed.NewSeeder(pool).SeedFcdevCLIClient(context.Background()))
	q := url.Values{
		"response_type": {"code"}, "client_id": {platformauth.FcdevCLIClientID},
		"redirect_uri": {"http://127.0.0.1:53682/callback"}, "state": {"st4te"},
		"code_challenge": {strings.Repeat("a", 43)}, "code_challenge_method": {"S256"},
	}
	authorize := func(cookie, bearer string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "http://platform.example.com/oauth/authorize?"+q.Encode(), nil)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: platformmw.SessionCookieName, Value: cookie})
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		x.h.ServeHTTP(w, r)
		return w
	}
	issuesCode := func(w *httptest.ResponseRecorder) bool {
		loc := w.Header().Get("Location")
		return strings.HasPrefix(loc, "http://127.0.0.1:53682/callback") && strings.Contains(loc, "code=")
	}
	codes := func() int {
		var n int
		require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM oauth_oidc_payloads WHERE type = 'AuthorizationCode'`).Scan(&n))
		return n
	}

	// control: a real session cookie gets a code straight away
	w := authorize(x.cookie, "")
	require.True(t, issuesCode(w), "session cookie should be admitted; got %d %q", w.Code, w.Header().Get("Location"))

	for name, c := range map[string]struct{ cookie, bearer string }{
		"api bearer, no cookie":                   {"", x.bearer},
		"api token in the cookie":                 {x.bearer, ""},
		"api token in both the cookie and Bearer": {x.bearer, x.bearer},
	} {
		before := codes()
		w := authorize(c.cookie, c.bearer)
		require.False(t, issuesCode(w), "%s: an authorization code was issued (%s)", name, w.Header().Get("Location"))
		require.Equal(t, before, codes(), "%s: an authorization code was stored", name)
		// the same redirect-to-login path an anonymous request takes
		anon := authorize("", "")
		require.Equal(t, anon.Code, w.Code, name)
		require.Equal(t, anon.Header().Get("Location"), w.Header().Get("Location"), name)
	}
}

// T7: the credential-management routes refuse an API bearer; the same call by a
// cookie session proceeds.
func TestCredentialRoutesRefuseBearer(t *testing.T) {
	x := newXOHarness(t)

	// passkey registration begin
	requireSessionRequired(t, taSend(t, x, http.MethodPost, "/auth/webauthn/register/begin", `{}`, "", x.bearer))
	w := taSend(t, x, http.MethodPost, "/auth/webauthn/register/begin", `{}`, x.cookie, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// passkey registration complete, and revoke
	requireSessionRequired(t, taSend(t, x, http.MethodPost, "/auth/webauthn/register/complete", `{"stateId":"x","name":"n","credential":{}}`, "", x.bearer))
	requireSessionRequired(t, taSend(t, x, http.MethodDelete, "/auth/webauthn/credentials/anything", "", "", x.bearer))

	// 2FA self-service writes
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, "/auth/2fa/methods/totp/begin", ""},
		{http.MethodPost, "/auth/2fa/methods/totp/confirm", `{"code":"123456"}`},
		{http.MethodPost, "/auth/2fa/methods/email/begin", ""},
		{http.MethodPost, "/auth/2fa/methods/email/confirm", `{"code":"123456"}`},
		{http.MethodDelete, "/auth/2fa/methods/totp", ""},
		{http.MethodPost, "/auth/2fa/recovery-codes/regenerate", ""},
		{http.MethodDelete, "/auth/2fa/trusted-devices/abc", ""},
	} {
		requireSessionRequired(t, taSend(t, x, c.method, c.path, c.body, "", x.bearer))
	}
}

// T7 (2FA): a cookie session starts an email-code enrolment; the bearer is refused;
// the reads stay open to a bearer.
func TestTwoFactorSelfServiceBearerRules(t *testing.T) {
	x := newXOHarness(t)

	requireSessionRequired(t, taSend(t, x, http.MethodPost, "/auth/2fa/methods/totp/begin", "", "", x.bearer))
	requireSessionRequired(t, taSend(t, x, http.MethodPost, "/auth/2fa/methods/email/begin", "", "", x.bearer))
	w := taSend(t, x, http.MethodPost, "/auth/2fa/methods/email/begin", "", x.cookie, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	for _, p := range []string{"/auth/2fa/status", "/auth/2fa/trusted-devices"} {
		w := taSend(t, x, http.MethodGet, p, "", "", x.bearer)
		require.Equal(t, http.StatusOK, w.Code, "%s: %s", p, w.Body.String())
	}
}

// T4 (cookie half): a real session cookie still authenticates, with authority
// resolved from the database rather than from the token, and only the cookie
// marker is set; an API bearer of the same principal keeps its own (narrower)
// claims and only the Bearer marker.
func TestSessionCookieAndBearerAuthority(t *testing.T) {
	x := newXOHarness(t)
	pool := testpg.Pool(t)
	keyPath, err := EnsureSigningKeyFile(filepath.Join(t.TempDir(), "jwt.pem"))
	require.NoError(t, err)
	cfg := EnvCfg{
		JWTIssuer: xoPublicBase, JWTSigningKeyPath: keyPath, SessionTTLSecs: 3600,
		AccessTokenTTLSecs: 3600, RefreshTokenTTLSecs: 3600, FunctionsArtifactStore: "file://" + t.TempDir(),
	}
	repos := buildRepos(pool)
	svcs, err := buildServices(cfg, pool, repos)
	require.NoError(t, err)

	// The harness principal, under THIS service set's key.
	pid := ids.PrincipalID(taSubject(t, x.cookie))
	cookie, err := svcs.authProvider.MintSessionToken(context.Background(), string(pid), 0)
	require.NoError(t, err)
	p, err := repos.principalRepo.FindByID(context.Background(), pid)
	require.NoError(t, err)
	narrow := []string{"platform:iam:role:view"}
	bearer, err := svcs.authSvc.GenerateAccessTokenWithScope(p, narrow)
	require.NoError(t, err)

	var seen struct {
		ac     *auth.AuthContext
		cookie bool
		bearer bool
	}
	h := platformmw.Authenticator(platformmw.AuthConfig{Provider: svcs.authProvider})(http.HandlerFunc(
		func(_ http.ResponseWriter, r *http.Request) {
			seen.ac = auth.FromContext(r.Context())
			seen.cookie = platformmw.AuthenticatedBySessionCookie(r.Context())
			seen.bearer = platformmw.AuthenticatedByBearer(r.Context())
		}))
	send := func(cookieVal, bearerVal string) {
		seen.ac, seen.cookie, seen.bearer = nil, false, false
		r := httptest.NewRequest(http.MethodGet, "/api/thing", nil)
		if cookieVal != "" {
			r.AddCookie(&http.Cookie{Name: platformmw.SessionCookieName, Value: cookieVal})
		}
		if bearerVal != "" {
			r.Header.Set("Authorization", "Bearer "+bearerVal)
		}
		h.ServeHTTP(httptest.NewRecorder(), r)
	}

	send(cookie, "")
	require.NotNil(t, seen.ac)
	require.True(t, seen.cookie)
	require.False(t, seen.bearer)
	require.ElementsMatch(t, xoPerms, seen.ac.Permissions, "cookie authority is the database's")

	send("", bearer)
	require.NotNil(t, seen.ac)
	require.True(t, seen.bearer)
	require.False(t, seen.cookie)
	require.ElementsMatch(t, narrow, seen.ac.Permissions, "a bearer keeps its own narrowed claims")

	// the bearer planted in the cookie jar buys nothing
	send(bearer, "")
	require.Nil(t, seen.ac)

	// a deactivated principal's cookie no longer signs in
	_, err = pool.Exec(context.Background(), `UPDATE iam_principals SET active = FALSE WHERE id = $1`, string(pid))
	require.NoError(t, err)
	send(cookie, "")
	require.Nil(t, seen.ac)
}
