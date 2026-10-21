//go:build integration

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Login CSRF (docs/platform-bug-fixes.md, F16 follow-up): the public routes
// that sign a browser in, or set or change its credentials, take only a JSON
// body and refuse a request a browser marks as coming from another origin.
// Unlike the cookie-session rule, a request with neither Sec-Fetch-Site nor
// Origin passes (a non-browser caller: login CSRF needs the victim's browser,
// and every browser sends Origin on a cross-origin POST).

// credentialRoutes are the public routes the credential guard covers.
var credentialRoutes = []string{
	"/auth/login",
	"/auth/2fa/verify",
	"/auth/2fa/challenge/email",
	"/auth/2fa/enroll/totp/begin",
	"/auth/2fa/enroll/totp/confirm",
	"/auth/2fa/enroll/email/begin",
	"/auth/2fa/enroll/email/confirm",
	"/auth/password-reset/request",
	"/auth/password-reset/confirm",
	"/auth/password-setup/request",
	"/auth/webauthn/authenticate/begin",
	"/auth/webauthn/authenticate/complete",
	"/portal/auth/check-domain",
	"/portal/auth/login",
	"/portal/auth/password-reset",
}

// notCredentialGuarded: every other unsafe route outside /api, /bff and /ui,
// with the reason it is left to other defences. A new public route fails
// TestEveryPublicUnsafeRouteIsDecided until it is placed in one list or the
// other.
var notCredentialGuarded = map[string]string{
	"POST /auth/check-domain":                    "read-only lookup",
	"POST /auth/logout":                          "issues nothing; GET /auth/oidc/session/end (RP-initiated logout) clears the cookie by design",
	"POST /auth/refresh":                         "refresh token in the body, tokens in the JSON body, no cookie",
	"POST /auth/change-password":                 "authenticated (SessionCSRF for the cookie)",
	"POST /auth/change-password/send-email-code": "authenticated (SessionCSRF for the cookie)",
	"POST /auth/client/switch":                   "authenticated (SessionCSRF for the cookie)",
	"POST /auth/2fa/methods/totp/begin":          "authenticated (SessionCSRF for the cookie)",
	"POST /auth/2fa/methods/totp/confirm":        "authenticated (SessionCSRF for the cookie)",
	"POST /auth/2fa/methods/email/begin":         "authenticated (SessionCSRF for the cookie)",
	"POST /auth/2fa/methods/email/confirm":       "authenticated (SessionCSRF for the cookie)",
	"POST /auth/2fa/recovery-codes/regenerate":   "authenticated (SessionCSRF for the cookie)",
	"DELETE /auth/2fa/methods/{method}":          "authenticated (SessionCSRF for the cookie)",
	"DELETE /auth/2fa/trusted-devices/{id}":      "authenticated (SessionCSRF for the cookie)",
	"POST /auth/webauthn/register/begin":         "authenticated (SessionCSRF for the cookie)",
	"POST /auth/webauthn/register/complete":      "authenticated (SessionCSRF for the cookie)",
	"DELETE /auth/webauthn/credentials/{id}":     "authenticated (SessionCSRF for the cookie)",
	"POST /oauth/token":                          "OAuth: form-encoded by spec, client-authenticated, PKCE; no cookie",
	"POST /oauth/introspect":                     "OAuth: form-encoded by spec, client-authenticated",
	"POST /oauth/revoke":                         "OAuth: form-encoded by spec, client-authenticated",
	"POST /oauth/userinfo":                       "OIDC: bearer access token",
	"POST /control/functions/events":             "function runner bearer token",
	"POST /control/functions/dispatch-jobs":      "function runner bearer token",
	"POST /control/functions/heartbeat":          "function runner bearer token",
	"POST /control/functions/trace-log":          "function runner bearer token",
}

func TestEveryPublicUnsafeRouteIsDecided(t *testing.T) {
	x := newXOHarness(t)
	guarded := map[string]bool{}
	for _, p := range credentialRoutes {
		guarded["POST "+p] = true
	}
	var undecided []string
	seen := map[string]bool{}
	require.NoError(t, chi.Walk(x.h.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			return nil
		}
		for _, p := range []string{"/api/", "/bff/", "/ui/"} {
			if strings.HasPrefix(route, p) {
				return nil
			}
		}
		k := method + " " + route
		seen[k] = true
		if !guarded[k] && notCredentialGuarded[k] == "" {
			undecided = append(undecided, k)
		}
		return nil
	}))
	sort.Strings(undecided)
	assert.Empty(t, undecided, "public unsafe routes with no decision: add each to credentialRoutes or notCredentialGuarded")
	for k := range guarded {
		assert.True(t, seen[k], "credentialRoutes names a route that is not mounted: %s", k)
	}
}

// What a page on another origin can make a browser send to these routes is
// refused before the handler runs.
func TestCredentialRoutesRefuseCrossOrigin(t *testing.T) {
	x := newXOHarness(t)
	for _, path := range credentialRoutes {
		for _, tc := range []struct {
			name string
			q    xoReq
		}{
			{"cross-site form-shaped text/plain", xoReq{
				contentType: "text/plain", body: `{"email":"a@b.c","password":"x"}`,
				secFetchSite: "cross-site", origin: "https://attacker.test",
			}},
			{"cross-site JSON", xoReq{
				contentType: "application/json", body: `{}`,
				secFetchSite: "cross-site", origin: "https://attacker.test",
			}},
			{"same-site sibling, text/plain", xoReq{
				contentType: "text/plain", body: `{}`,
				secFetchSite: "same-site", origin: "https://evil.example.com",
			}},
			{"no Sec-Fetch-Site, foreign Origin", xoReq{
				contentType: "application/json", body: `{}`,
				origin: "https://evil.example.com",
			}},
			{"Origin null", xoReq{contentType: "application/json", body: `{}`, origin: "null"}},
		} {
			t.Run(path+" "+tc.name, func(t *testing.T) {
				q := tc.q
				q.method, q.path = http.MethodPost, path
				got := x.do(q)
				require.Equal(t, http.StatusForbidden, got.status, got.body)
				require.Equal(t, "CROSS_ORIGIN_REQUEST", got.code, got.body)
			})
		}
	}
}

// From this origin, or from a non-browser caller, the body must be JSON.
func TestCredentialRoutesRequireJSON(t *testing.T) {
	x := newXOHarness(t)
	for _, path := range credentialRoutes {
		for _, tc := range []struct {
			name string
			q    xoReq
		}{
			{"same-origin text/plain", xoReq{
				contentType: "text/plain;charset=UTF-8", body: `{}`,
				secFetchSite: "same-origin", origin: xoPublicBase,
			}},
			{"same-origin form", xoReq{
				contentType: "application/x-www-form-urlencoded", body: `email=a%40b.c`,
				secFetchSite: "same-origin", origin: xoPublicBase,
			}},
			{"same-origin multipart", xoReq{
				contentType: "multipart/form-data; boundary=x", body: "--x--",
				secFetchSite: "same-origin", origin: xoPublicBase,
			}},
			{"same-origin body without Content-Type", xoReq{
				body:         `{}`,
				secFetchSite: "same-origin", origin: xoPublicBase,
			}},
			{"non-browser form", xoReq{contentType: "application/x-www-form-urlencoded", body: `email=a%40b.c`}},
		} {
			t.Run(path+" "+tc.name, func(t *testing.T) {
				q := tc.q
				q.method, q.path = http.MethodPost, path
				got := x.do(q)
				require.Equal(t, http.StatusUnsupportedMediaType, got.status, got.body)
				require.Equal(t, "UNSUPPORTED_MEDIA_TYPE", got.code, got.body)
			})
		}
	}
}

// The SPA (same-origin JSON), an address-bar/user-initiated request, a proxy
// that rewrites Host, and a non-browser JSON caller reach the handler. What the
// handler answers to the empty body does not matter here; only that the guard
// let it through.
func TestCredentialRoutesAllowSameOriginAndNonBrowserJSON(t *testing.T) {
	x := newXOHarness(t)
	for _, path := range credentialRoutes {
		for _, tc := range []struct {
			name string
			q    xoReq
		}{
			{"same-origin JSON (the SPA)", xoReq{secFetchSite: "same-origin", origin: xoPublicBase}},
			{"Sec-Fetch-Site none", xoReq{secFetchSite: "none"}},
			{"no Sec-Fetch-Site, Origin is the host", xoReq{origin: xoPublicBase}},
			{"proxy rewrote Host, Origin is the public base URL", xoReq{host: "platform-internal:8080", origin: xoPublicBase}},
			{"non-browser, no browser headers", xoReq{}},
			{"+json media type", xoReq{secFetchSite: "same-origin", contentType: "application/vnd.fc+json"}},
		} {
			t.Run(path+" "+tc.name, func(t *testing.T) {
				q := tc.q
				q.method, q.path, q.body = http.MethodPost, path, `{}`
				if q.contentType == "" {
					q.contentType = "application/json"
				}
				got := x.do(q)
				require.NotEqual(t, "CROSS_ORIGIN_REQUEST", got.code, got.body)
				require.NotEqual(t, "UNSUPPORTED_MEDIA_TYPE", got.code, got.body)
				require.NotEqual(t, http.StatusUnsupportedMediaType, got.status, got.body)
			})
		}
	}
}

// The routes left alone keep answering as before, whatever the provenance.
func TestNonCredentialPublicRoutesNotGuarded(t *testing.T) {
	x := newXOHarness(t)
	got := x.do(xoReq{
		method: http.MethodPost, path: "/auth/check-domain", contentType: "text/plain",
		body: `{"email":"someone@example.org"}`, secFetchSite: "cross-site", origin: "https://attacker.test",
	})
	require.Equal(t, http.StatusOK, got.status, got.body)

	got = x.do(xoReq{method: http.MethodPost, path: "/auth/logout", secFetchSite: "cross-site", origin: "https://attacker.test"})
	require.Equal(t, http.StatusNoContent, got.status, got.body)
}

// lcsrfUser seeds an active password user (no 2FA: no domain mapping, no
// factor) and returns its email and password.
func lcsrfUser(t *testing.T) (string, string) {
	t.Helper()
	pid := ids.NewPrincipalID()
	email := strings.ToLower(string(pid)) + "@lcsrf.example.org"
	const password = "correct horse battery staple 42"
	hash, err := passwordhash.Hash(password)
	require.NoError(t, err)
	_, err = testpg.Pool(t).Exec(context.Background(),
		`INSERT INTO iam_principals (id, type, scope, name, active, email, password_hash)
		 VALUES ($1, 'USER', 'CLIENT', 'Login CSRF', TRUE, $2, $3)`, pid, email, hash)
	require.NoError(t, err)
	return email, password
}

func (x *xoHarness) login(q xoReq) (int, string, *http.Cookie) {
	x.t.Helper()
	r := httptest.NewRequest(http.MethodPost, "http://platform.example.com/auth/login", strings.NewReader(q.body))
	if q.contentType != "" {
		r.Header.Set("Content-Type", q.contentType)
	}
	if q.secFetchSite != "" {
		r.Header.Set("Sec-Fetch-Site", q.secFetchSite)
	}
	if q.origin != "" {
		r.Header.Set("Origin", q.origin)
	}
	w := httptest.NewRecorder()
	x.h.ServeHTTP(w, r)
	for _, c := range w.Result().Cookies() {
		if c.Name == platformmw.SessionCookieName && c.Value != "" {
			return w.Code, w.Body.String(), c
		}
	}
	return w.Code, w.Body.String(), nil
}

// The attack: a page on another site posts the attacker's credentials to
// /auth/login as a form (enctype=text/plain makes the body valid JSON), and the
// response signs the victim's browser in to the attacker's account.
func TestLoginCSRFRefusedAndRealLoginsStillWork(t *testing.T) {
	x := newXOHarness(t)
	email, password := lcsrfUser(t)
	body := `{"email":"` + email + `","password":"` + password + `"}`

	for _, tc := range []struct {
		name string
		q    xoReq
		want int
	}{
		{"cross-site text/plain form post", xoReq{
			contentType: "text/plain", body: body,
			secFetchSite: "cross-site", origin: "https://attacker.test",
		}, http.StatusForbidden},
		{"cross-site, browser without Sec-Fetch-Site", xoReq{
			contentType: "text/plain", body: body,
			origin: "https://attacker.test",
		}, http.StatusForbidden},
		{"same-site sibling, text/plain", xoReq{
			contentType: "text/plain", body: body,
			secFetchSite: "same-site", origin: "https://evil.example.com",
		}, http.StatusForbidden},
		{"same-origin but text/plain", xoReq{
			contentType: "text/plain", body: body,
			secFetchSite: "same-origin", origin: xoPublicBase,
		}, http.StatusUnsupportedMediaType},
	} {
		t.Run("refused: "+tc.name, func(t *testing.T) {
			status, respBody, cookie := x.login(tc.q)
			require.Equal(t, tc.want, status, respBody)
			require.Nil(t, cookie, "no session cookie on a refused login")
		})
	}

	for _, tc := range []struct {
		name string
		q    xoReq
	}{
		{"the SPA: same-origin JSON", xoReq{
			contentType: "application/json", body: body,
			secFetchSite: "same-origin", origin: xoPublicBase,
		}},
		{"non-browser JSON (curl, a script): no Origin, no Sec-Fetch-Site", xoReq{contentType: "application/json", body: body}},
		{"browser over plain HTTP: Origin only", xoReq{contentType: "application/json", body: body, origin: xoPublicBase}},
	} {
		t.Run("allowed: "+tc.name, func(t *testing.T) {
			status, respBody, cookie := x.login(tc.q)
			require.Equal(t, http.StatusOK, status, respBody)
			require.NotNil(t, cookie, "a session cookie: %s", respBody)
		})
	}
}
