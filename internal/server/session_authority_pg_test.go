//go:build integration

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/passwordreset"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/tsid"
)

// Browser sessions and the sign-in methods. These tests drive the whole router
// (WirePlatform on embedded Postgres) with real fc_session cookies:
//
//   - changing sign-in methods (passkeys, 2FA factors) needs a recent sign-in
//     (FC_AUTH_STEPUP_MAX_AGE_SECS);
//   - a password change, a password reset, an administrator's password or 2FA
//     reset and a deactivation end every session minted before them
//     (iam_principals.sessions_valid_after), and the browser that changed its own
//     password is handed a fresh cookie.

const (
	saPassword    = "Tangerine-Lighthouse-42"
	saNewPassword = "Quartz-Meridian-77-Hollow"
)

type saHarness struct {
	t    *testing.T
	h    http.Handler
	cfg  EnvCfg
	svcs *serviceSet
	repo *repoSet
}

func newSAHarness(t *testing.T, stepUpSecs int) *saHarness {
	t.Helper()
	pool := testpg.Pool(t)
	keyPath, err := EnsureSigningKeyFile(filepath.Join(t.TempDir(), "jwt.pem"))
	require.NoError(t, err)
	cfg := EnvCfg{
		JWTIssuer:              xoPublicBase,
		JWTSigningKeyPath:      keyPath,
		SessionTTLSecs:         3600,
		AccessTokenTTLSecs:     3600,
		RefreshTokenTTLSecs:    3600,
		FunctionsArtifactStore: "file://" + t.TempDir(),
		AuthStepUpMaxAgeSecs:   stepUpSecs,
	}
	r := chi.NewRouter()
	_, err = WirePlatform(r, pool, cfg, dispatch.Settings{})
	require.NoError(t, err)
	repos := buildRepos(pool)
	svcs, err := buildServices(cfg, pool, repos)
	require.NoError(t, err)
	return &saHarness{t: t, h: r, cfg: cfg, svcs: svcs, repo: repos}
}

// newUser inserts an active anchor user holding the XO role, with a password.
func (x *saHarness) newUser() (pid, email string) {
	x.t.Helper()
	ctx := context.Background()
	pool := testpg.Pool(x.t)
	id := ids.NewPrincipalID()
	email = strings.ToLower(string(id)) + "@sa.example.com"
	hash, err := passwordhash.Hash(saPassword)
	require.NoError(x.t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, name, active, email, email_domain, idp_type, password_hash)
		 VALUES ($1, 'USER', 'ANCHOR', 'SA User', TRUE, $2, 'sa.example.com', 'INTERNAL', $3)`, id, email, hash)
	require.NoError(x.t, err)
	roleID := tsid.Generate(tsid.Role)
	roleName := "satest:user-" + strings.ToLower(string(id))
	_, err = pool.Exec(ctx, `INSERT INTO iam_roles (id, name, display_name) VALUES ($1, $2, 'SA User')`, roleID, roleName)
	require.NoError(x.t, err)
	for _, p := range xoPerms {
		_, err = pool.Exec(ctx, `INSERT INTO iam_role_permissions (role_id, permission) VALUES ($1, $2)`, roleID, p)
		require.NoError(x.t, err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source) VALUES ($1, $2, 'TEST')`, id, roleName)
	require.NoError(x.t, err)
	return string(id), email
}

// cookieAt mints a session cookie for pid exactly as sessiontoken.Mint does
// (same claims, same key), but issued at iat.
func (x *saHarness) cookieAt(pid string, iat time.Time) string {
	x.t.Helper()
	mc := jwt.MapClaims{
		"iss": x.cfg.JWTIssuer, "sub": pid,
		"iat": iat.Unix(), "nbf": iat.Unix(), "exp": iat.Add(24 * time.Hour).Unix(),
		"tier": "ANCHOR", "all_applications": false,
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodRS256, mc).SignedString(x.svcs.authProvider.SigningKey())
	require.NoError(x.t, err)
	return s
}

func (x *saHarness) mintNow(pid string) string {
	x.t.Helper()
	tok, err := x.svcs.authProvider.MintSessionToken(context.Background(), pid, time.Hour)
	require.NoError(x.t, err)
	return tok
}

func (x *saHarness) bearer(pid string, perms ...string) string {
	x.t.Helper()
	p, err := x.repo.principalRepo.FindByID(context.Background(), ids.PrincipalID(pid))
	require.NoError(x.t, err)
	tok, err := x.svcs.authSvc.GenerateAccessTokenWithScope(p, perms)
	require.NoError(x.t, err)
	return tok
}

func (x *saHarness) send(method, path, body, cookie, bearer string) *httptest.ResponseRecorder {
	x.t.Helper()
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
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	w := httptest.NewRecorder()
	x.h.ServeHTTP(w, r)
	return w
}

func (x *saHarness) status(cookie string) int {
	return x.send(http.MethodGet, "/auth/me", "", cookie, "").Code
}

func setCookieValue(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == platformmw.SessionCookieName {
			return c.Value
		}
	}
	return ""
}

func errCode(w *httptest.ResponseRecorder) (code, message string) {
	var env struct {
		Code, Error, Message string
	}
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	code = env.Code
	if code == "" {
		code = env.Error
	}
	return code, env.Message
}

const reauthMsg = "Sign in again to change your sign-in methods"

func requireReauth(t *testing.T, w *httptest.ResponseRecorder, what string) {
	t.Helper()
	code, msg := errCode(w)
	require.Equal(t, http.StatusForbidden, w.Code, "%s: %s", what, w.Body.String())
	require.Equal(t, "REAUTH_REQUIRED", code, "%s: %s", what, w.Body.String())
	require.Equal(t, reauthMsg, msg, what)
}

func requireNotReauth(t *testing.T, w *httptest.ResponseRecorder, what string) {
	t.Helper()
	code, _ := errCode(w)
	require.NotEqual(t, "REAUTH_REQUIRED", code, "%s: %s", what, w.Body.String())
}

// TA1: the sign-in-method write routes need a recent sign-in.
func TestStepUpRequiredToChangeSignInMethods(t *testing.T) {
	x := newSAHarness(t, 900)
	pid, _ := x.newUser()
	old := x.cookieAt(pid, time.Now().Add(-20*time.Minute))
	fresh := x.cookieAt(pid, time.Now().Add(-1*time.Minute))

	writes := []struct{ method, path, body string }{
		{http.MethodPost, "/auth/webauthn/register/begin", `{}`},
		{http.MethodPost, "/auth/webauthn/register/complete", `{"stateId":"x","name":"n","credential":{}}`},
		{http.MethodDelete, "/auth/webauthn/credentials/anything", ""},
		{http.MethodPost, "/auth/2fa/methods/totp/begin", ""},
		{http.MethodPost, "/auth/2fa/methods/totp/confirm", `{"code":"123456"}`},
		{http.MethodPost, "/auth/2fa/methods/email/begin", ""},
		{http.MethodPost, "/auth/2fa/methods/email/confirm", `{"code":"123456"}`},
		{http.MethodDelete, "/auth/2fa/methods/totp", ""},
		{http.MethodPost, "/auth/2fa/recovery-codes/regenerate", ""},
		{http.MethodDelete, "/auth/2fa/trusted-devices/abc", ""},
	}
	for _, c := range writes {
		what := c.method + " " + c.path
		requireReauth(t, x.send(c.method, c.path, c.body, old, ""), what+" (20 min old session)")
		requireNotReauth(t, x.send(c.method, c.path, c.body, fresh, ""), what+" (1 min old session)")
	}
	// A fresh session really does the work.
	require.Equal(t, http.StatusOK, x.send(http.MethodPost, "/auth/webauthn/register/begin", `{}`, fresh, "").Code)
	require.Equal(t, http.StatusOK, x.send(http.MethodPost, "/auth/2fa/methods/email/begin", "", fresh, "").Code)

	// Reads are not gated.
	for _, p := range []string{"/auth/me", "/auth/2fa/status", "/auth/2fa/trusted-devices", "/auth/webauthn/credentials"} {
		require.Equal(t, http.StatusOK, x.send(http.MethodGet, p, "", old, "").Code, "GET %s with the old session", p)
	}

	// The old session still works as a session: only sign-in method changes are gated.
	require.Equal(t, http.StatusOK, x.status(old))
}

func TestStepUpDisabledWithZeroWindow(t *testing.T) {
	x := newSAHarness(t, 0)
	pid, _ := x.newUser()
	old := x.cookieAt(pid, time.Now().Add(-20*time.Hour))
	require.Equal(t, http.StatusOK, x.send(http.MethodPost, "/auth/webauthn/register/begin", `{}`, old, "").Code)
	require.Equal(t, http.StatusOK, x.send(http.MethodPost, "/auth/2fa/methods/email/begin", "", old, "").Code)
	requireNotReauth(t, x.send(http.MethodPost, "/auth/2fa/recovery-codes/regenerate", "", old, ""), "regenerate")
}

// A context that was neither built from the cookie nor from a Bearer (the dev test
// headers, a test injecting an AuthContext) is not subject to the rule.
func TestStepUpIgnoresInjectedContext(t *testing.T) {
	x := newSAHarness(t, 900)
	pid, _ := x.newUser()
	inject := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.WithContext(r.Context(), &auth.AuthContext{PrincipalID: pid, Scope: auth.ScopeAnchor, Permissions: xoPerms})
		x.h.ServeHTTP(w, r.WithContext(ctx))
	})
	r := httptest.NewRequest(http.MethodPost, "http://platform.example.com/auth/webauthn/register/begin", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	inject.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	r = httptest.NewRequest(http.MethodPost, "http://platform.example.com/auth/2fa/methods/email/begin", nil)
	w = httptest.NewRecorder()
	inject.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func (x *saHarness) validAfter(pid string) *time.Time {
	x.t.Helper()
	var at *time.Time
	require.NoError(x.t, testpg.Pool(x.t).QueryRow(context.Background(),
		`SELECT sessions_valid_after FROM iam_principals WHERE id = $1`, pid).Scan(&at))
	return at
}

// TB1: a password change ends the sessions minted before it, and the browser
// that made the change stays signed in on the cookie from the response.
func TestPasswordChangeEndsOtherSessions(t *testing.T) {
	x := newSAHarness(t, 0)
	pid, _ := x.newUser()
	captured := x.cookieAt(pid, time.Now().Add(-5*time.Second))
	browser := x.cookieAt(pid, time.Now().Add(-6*time.Second))
	require.Equal(t, http.StatusOK, x.status(captured))
	require.Nil(t, x.validAfter(pid))

	w := x.send(http.MethodPost, "/auth/change-password",
		`{"currentPassword":"`+saPassword+`","newPassword":"`+saNewPassword+`"}`, browser, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	reissued := setCookieValue(t, w)
	require.NotEmpty(t, reissued, "the response must carry a fresh session cookie")
	require.NotNil(t, x.validAfter(pid))

	// the old cookies (the one used for the change, and one captured elsewhere) no longer sign in
	require.Equal(t, http.StatusUnauthorized, x.status(captured), "a session captured before the change must end")
	require.Equal(t, http.StatusUnauthorized, x.status(browser), "the cookie the change was made with must end")
	// the cookie from the change response signs in (this is the "still signed in" case)
	require.Equal(t, http.StatusOK, x.status(reissued), "the browser that changed the password stays signed in")
}

// Not a cookie-authenticated change: no cookie is issued.
func TestPasswordChangeByInjectedContextIssuesNoCookie(t *testing.T) {
	x := newSAHarness(t, 0)
	pid, _ := x.newUser()
	inject := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.WithContext(r.Context(), &auth.AuthContext{PrincipalID: pid, Scope: auth.ScopeAnchor, Permissions: xoPerms})
		x.h.ServeHTTP(w, r.WithContext(ctx))
	})
	r := httptest.NewRequest(http.MethodPost, "http://platform.example.com/auth/change-password",
		strings.NewReader(`{"currentPassword":"`+saPassword+`","newPassword":"`+saNewPassword+`"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	inject.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Empty(t, setCookieValue(t, w))
	require.NotNil(t, x.validAfter(pid))
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// TB1: confirming a password reset (employee plane) ends earlier sessions.
func TestPasswordResetConfirmEndsSessions(t *testing.T) {
	x := newSAHarness(t, 0)
	pid, _ := x.newUser()
	captured := x.cookieAt(pid, time.Now().Add(-5*time.Second))
	require.Equal(t, http.StatusOK, x.status(captured))

	raw := "reset-token-" + strings.Repeat("a", 31)
	require.NoError(t, x.repo.resetTokenRepo.Insert(context.Background(),
		passwordreset.New(pid, sha256Hex(raw), time.Now().Add(15*time.Minute))))
	w := x.send(http.MethodPost, "/auth/password-reset/confirm", `{"token":"`+raw+`","password":"`+saNewPassword+`"}`, "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	require.Equal(t, http.StatusUnauthorized, x.status(captured))
	require.Equal(t, http.StatusOK, x.status(x.mintNow(pid)), "a session started after the reset signs in")
}

// TB1: an administrator resetting a user's password, or the user's 2FA, ends the
// user's sessions; so does deactivating them (and re-activating does not revive them).
func TestAdminActionsEndSessions(t *testing.T) {
	x := newSAHarness(t, 0)
	admin, _ := x.newUser()

	// The resets are bounded by the role ceiling (the target holds a role the
	// caller must cover), so the administrator here is a super-admin.
	t.Run("reset password", func(t *testing.T) {
		pid, _ := x.newUser()
		captured := x.cookieAt(pid, time.Now().Add(-5*time.Second))
		require.Equal(t, http.StatusOK, x.status(captured))
		w := x.send(http.MethodPost, "/api/principals/"+pid+"/reset-password",
			`{"newPassword":"`+saNewPassword+`"}`, "", x.bearer(admin, "platform:*:*:*"))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Equal(t, http.StatusUnauthorized, x.status(captured))
		require.Equal(t, http.StatusOK, x.status(x.mintNow(pid)))
	})

	t.Run("reset 2fa", func(t *testing.T) {
		pid, _ := x.newUser()
		captured := x.cookieAt(pid, time.Now().Add(-5*time.Second))
		require.Equal(t, http.StatusOK, x.status(captured))
		w := x.send(http.MethodPost, "/api/principals/"+pid+"/reset-2fa", "", "", x.bearer(admin, "platform:*:*:*"))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Equal(t, http.StatusUnauthorized, x.status(captured))
		require.Equal(t, http.StatusOK, x.status(x.mintNow(pid)))
	})

	t.Run("deactivate", func(t *testing.T) {
		pid, _ := x.newUser()
		captured := x.cookieAt(pid, time.Now().Add(-5*time.Second))
		require.Equal(t, http.StatusOK, x.status(captured))
		upd := x.bearer(admin, "platform:iam:user:update")
		w := x.send(http.MethodPost, "/api/principals/"+pid+"/deactivate", "", "", upd)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.NotNil(t, x.validAfter(pid), "deactivation stamps sessions_valid_after")
		require.Equal(t, http.StatusUnauthorized, x.status(captured))

		// reactivated: the session captured before the deactivation stays dead
		w = x.send(http.MethodPost, "/api/principals/"+pid+"/activate", "", "", upd)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Equal(t, http.StatusUnauthorized, x.status(captured))
		require.Equal(t, http.StatusOK, x.status(x.mintNow(pid)))
	})
}

// TB2: the comparison is at one-second resolution: a cookie issued in the same
// second as the stamp counts, one issued in an earlier second does not.
func TestSessionsValidAfterBoundary(t *testing.T) {
	x := newSAHarness(t, 0)
	pid, _ := x.newUser()
	stamp := time.Now().Truncate(time.Second).Add(-time.Hour)
	// stored with a sub-second part: the second it falls in is what counts
	_, err := testpg.Pool(t).Exec(context.Background(),
		`UPDATE iam_principals SET sessions_valid_after = $2 WHERE id = $1`, pid, stamp.Add(900*time.Millisecond))
	require.NoError(t, err)

	require.Equal(t, http.StatusUnauthorized, x.status(x.cookieAt(pid, stamp.Add(-time.Second))), "one second before the stamp")
	require.Equal(t, http.StatusOK, x.status(x.cookieAt(pid, stamp)), "the same second as the stamp")
	require.Equal(t, http.StatusOK, x.status(x.cookieAt(pid, stamp.Add(time.Second))), "after the stamp")

	// and the cookie a change response itself mints, in the stamp's own second
	pid2, _ := x.newUser()
	w := x.send(http.MethodPost, "/auth/change-password",
		`{"currentPassword":"`+saPassword+`","newPassword":"`+saNewPassword+`"}`, x.cookieAt(pid2, time.Now().Add(-3*time.Second)), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	stored := x.validAfter(pid2)
	require.NotNil(t, stored)
	require.Zero(t, stored.Nanosecond(), "the stamp is truncated to the second")
	require.Equal(t, http.StatusOK, x.status(setCookieValue(t, w)))
}

// TB4: a principal whose column is NULL behaves as before the column existed, and
// the migration left it nullable with no default (a catalogue-only change).
func TestNullSessionsValidAfterIsUnaffected(t *testing.T) {
	x := newSAHarness(t, 0)
	pid, _ := x.newUser()
	require.Nil(t, x.validAfter(pid))
	require.Equal(t, http.StatusOK, x.status(x.cookieAt(pid, time.Now().Add(-23*time.Hour))))

	var nullable string
	var def *string
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT is_nullable, column_default FROM information_schema.columns
		  WHERE table_name = 'iam_principals' AND column_name = 'sessions_valid_after'`).Scan(&nullable, &def))
	require.Equal(t, "YES", nullable)
	require.Nil(t, def)
}

// TB3: /oauth/authorize treats a session issued before the stamp as not signed in.
func TestAuthorizeRefusesSessionsEndedByStamp(t *testing.T) {
	x := newSAHarness(t, 0)
	pool := testpg.Pool(t)
	require.NoError(t, seed.NewSeeder(pool).SeedFcdevCLIClient(context.Background()))
	pid, _ := x.newUser()
	q := url.Values{
		"response_type": {"code"}, "client_id": {platformauth.FcdevCLIClientID},
		"redirect_uri": {"http://127.0.0.1:53682/callback"}, "state": {"st4te"},
		"code_challenge": {strings.Repeat("a", 43)}, "code_challenge_method": {"S256"},
	}
	authorize := func(cookie string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "http://platform.example.com/oauth/authorize?"+q.Encode(), nil)
		r.AddCookie(&http.Cookie{Name: platformmw.SessionCookieName, Value: cookie})
		w := httptest.NewRecorder()
		x.h.ServeHTTP(w, r)
		return w
	}
	issuesCode := func(w *httptest.ResponseRecorder) bool {
		loc := w.Header().Get("Location")
		return strings.HasPrefix(loc, "http://127.0.0.1:53682/callback") && strings.Contains(loc, "code=")
	}

	pre := x.cookieAt(pid, time.Now().Add(-5*time.Second))
	require.True(t, issuesCode(authorize(pre)), "control: before the stamp the session signs in")

	_, err := pool.Exec(context.Background(),
		`UPDATE iam_principals SET sessions_valid_after = $2 WHERE id = $1`, pid, time.Now())
	require.NoError(t, err)

	w := authorize(pre)
	require.False(t, issuesCode(w), "a session from before the stamp was given a code: %s", w.Header().Get("Location"))
	require.Contains(t, w.Header().Get("Location"), "login", "treated as not signed in: sent to sign in")
	require.True(t, issuesCode(authorize(x.mintNow(pid))), "a session started after the stamp signs in")
}
