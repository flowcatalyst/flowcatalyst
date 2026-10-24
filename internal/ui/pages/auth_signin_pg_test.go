//go:build integration

package pages_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/server"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The new sign-in pages (/ui/auth/*) on the whole platform router over
// Postgres: the pages dispatch to the real /auth/login, /auth/check-domain,
// /auth/password-reset/* and the login theme, so the lockout, the CSRF guard,
// the cookie and the answers are the platform's own.

const siBase = "https://platform.example.com"

type siPlatform struct {
	t *testing.T
	h http.Handler
}

func newSignInPlatform(t *testing.T) *siPlatform {
	t.Helper()
	pool := testpg.Pool(t)
	keyPath, err := server.EnsureSigningKeyFile(filepath.Join(t.TempDir(), "jwt.pem"))
	require.NoError(t, err)
	r := chi.NewRouter()
	_, err = server.WirePlatform(r, pool, server.EnvCfg{
		JWTIssuer: siBase, JWTSigningKeyPath: keyPath,
		SessionTTLSecs: 3600, AccessTokenTTLSecs: 3600, RefreshTokenTTLSecs: 3600,
		FunctionsArtifactStore: "file://" + t.TempDir(),
	}, dispatch.Settings{})
	require.NoError(t, err)
	return &siPlatform{t: t, h: r}
}

// siUser seeds an active password user and returns its email and password.
func siUser(t *testing.T) (string, string) {
	t.Helper()
	pid := ids.NewPrincipalID()
	email := strings.ToLower(string(pid)) + "@signin-ui.example.org"
	const password = "correct horse battery staple 42"
	hash, err := passwordhash.Hash(password)
	require.NoError(t, err)
	_, err = testpg.Pool(t).Exec(context.Background(),
		`INSERT INTO iam_principals (id, type, scope, name, active, email, password_hash)
		 VALUES ($1, 'USER', 'CLIENT', 'Sign In UI', TRUE, $2, $3)`, pid, email, hash)
	require.NoError(t, err)
	return email, password
}

// do sends the request as a browser on the platform's own origin would.
func (p *siPlatform) do(method, target string, form url.Values, headers map[string]string) *httptest.ResponseRecorder {
	p.t.Helper()
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	r := httptest.NewRequest(method, "http://platform.example.com"+target, body)
	r.RemoteAddr = "198.51.100.7:5555"
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if method == http.MethodPost {
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.Header.Set("Origin", siBase)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	p.h.ServeHTTP(w, r)
	return w
}

func sessionCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == platformmw.SessionCookieName && c.Value != "" {
			return c
		}
	}
	return nil
}

func TestSignInPagesAgainstPlatform(t *testing.T) {
	p := newSignInPlatform(t)
	email, password := siUser(t)

	t.Run("the login page is public, standalone and hardened", func(t *testing.T) {
		w := p.do(http.MethodGet, "/ui/auth/login?redirect=/ui/roles", nil, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "Sign in to your account")
		assert.Contains(t, w.Body.String(), "FlowCatalyst", "the default theme")
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		assert.Contains(t, w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
		assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	})

	t.Run("a password user: email step, password step, session", func(t *testing.T) {
		w := p.do(http.MethodPost, "/ui/auth/login/email", url.Values{"email": {email}}, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "Enter your password")

		w = p.do(http.MethodPost, "/ui/auth/login/password", url.Values{"email": {email}, "password": {password}, "redirect": {"/ui/roles"}}, nil)
		require.Equal(t, http.StatusSeeOther, w.Code, w.Body.String())
		assert.Equal(t, "/ui/roles", w.Header().Get("Location"))
		c := sessionCookie(w)
		require.NotNil(t, c, "the platform's session cookie")
		assert.True(t, c.HttpOnly)
		assert.NotContains(t, w.Body.String(), password)

		// The cookie is the platform's: it authenticates /auth/me.
		me := p.do(http.MethodGet, "/auth/me", nil, map[string]string{"Cookie": c.Name + "=" + c.Value})
		assert.Equal(t, http.StatusOK, me.Code, me.Body.String())
		assert.Contains(t, me.Body.String(), email)
	})

	t.Run("an unknown user and a wrong password get the same answer", func(t *testing.T) {
		const ghost = "prn_nobody-here@signin-ui.example.org" // same initial as the seeded user: the avatar shows it
		known := p.do(http.MethodPost, "/ui/auth/login/password", url.Values{"email": {email}, "password": {"nope"}}, nil)
		unknown := p.do(http.MethodPost, "/ui/auth/login/password", url.Values{"email": {ghost}, "password": {"nope"}}, nil)
		assert.Equal(t, http.StatusUnauthorized, known.Code)
		assert.Equal(t, known.Code, unknown.Code)
		assert.Contains(t, known.Body.String(), "Invalid credentials")
		strip := func(w *httptest.ResponseRecorder, em string) string {
			return strings.NewReplacer(em, "EMAIL", url.QueryEscape(em), "EMAIL").Replace(w.Body.String())
		}
		assert.Equal(t, strip(known, email), strip(unknown, ghost), "nothing but the typed address differs")
		assert.Nil(t, sessionCookie(known))
		assert.Nil(t, sessionCookie(unknown))
	})

	t.Run("repeated failures lock the address out as /auth/login does", func(t *testing.T) {
		victim, vpw := siUser(t)
		var last *httptest.ResponseRecorder
		for range 8 {
			last = p.do(http.MethodPost, "/ui/auth/login/password", url.Values{"email": {victim}, "password": {"wrong"}}, nil)
			if last.Code == http.StatusTooManyRequests {
				break
			}
		}
		require.Equal(t, http.StatusTooManyRequests, last.Code, "the backoff engages within a few attempts")
		assert.NotEmpty(t, last.Header().Get("Retry-After"))
		assert.Contains(t, last.Body.String(), "Try again in")
		// Even the right password is refused while the address waits, on the
		// platform's own endpoint as on the page.
		w := p.do(http.MethodPost, "/ui/auth/login/password", url.Values{"email": {victim}, "password": {vpw}}, nil)
		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Nil(t, sessionCookie(w))
	})

	t.Run("a post from another site is refused before anything runs", func(t *testing.T) {
		for _, h := range []map[string]string{
			{"Sec-Fetch-Site": "cross-site", "Origin": "https://attacker.test"},
			{"Sec-Fetch-Site": "same-site", "Origin": "https://evil.example.com"},
		} {
			w := p.do(http.MethodPost, "/ui/auth/login/password", url.Values{"email": {email}, "password": {password}}, h)
			assert.Equal(t, http.StatusForbidden, w.Code)
			assert.Nil(t, sessionCookie(w), "login CSRF: no session for an attacker's form")
		}
	})

	t.Run("forgot password answers alike for a known and an unknown address", func(t *testing.T) {
		a := p.do(http.MethodPost, "/ui/auth/forgot-password", url.Values{"email": {email}}, nil)
		b := p.do(http.MethodPost, "/ui/auth/forgot-password", url.Values{"email": {"ghost@signin-ui.example.org"}}, nil)
		assert.Equal(t, http.StatusOK, a.Code, a.Body.String())
		assert.Equal(t, http.StatusOK, b.Code)
		assert.Contains(t, a.Body.String(), "If an account exists for")
		assert.Equal(t, strings.ReplaceAll(a.Body.String(), email, "E"), strings.ReplaceAll(b.Body.String(), "ghost@signin-ui.example.org", "E"))
	})

	t.Run("a bad reset token is the invalid page and the token is not echoed", func(t *testing.T) {
		w := p.do(http.MethodGet, "/ui/auth/reset-password?token=not-a-real-token", nil, nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "invalid or has already been used")
		assert.NotContains(t, w.Body.String(), "not-a-real-token")
		w = p.do(http.MethodPost, "/ui/auth/reset-password", url.Values{"token": {"not-a-real-token"}, "password": {"Correct-Horse-9!x"}, "confirmPassword": {"Correct-Horse-9!x"}}, nil)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
		assert.NotContains(t, w.Body.String(), "Correct-Horse-9!x")
		assert.Nil(t, sessionCookie(w))
	})

	t.Run("sign-out clears the cookie the platform set", func(t *testing.T) {
		w := p.do(http.MethodPost, "/ui/auth/logout", url.Values{}, nil)
		assert.Equal(t, http.StatusSeeOther, w.Code)
		assert.Equal(t, "/ui/auth/login", w.Header().Get("Location"))
		var cleared bool
		for _, c := range w.Result().Cookies() {
			if c.Name == platformmw.SessionCookieName && c.MaxAge < 0 {
				cleared = true
			}
		}
		assert.True(t, cleared, "Set-Cookie: %v", w.Header().Values("Set-Cookie"))
	})
}
