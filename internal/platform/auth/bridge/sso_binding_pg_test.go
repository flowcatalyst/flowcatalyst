//go:build integration

package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalauth"
)

// SSO login CSRF (docs/platform-bug-fixes.md, F16 follow-up): the login state
// lived only in the database, so a callback URL worked in any browser. An
// attacker with an account at any mapped identity provider could start a
// login, sign in at the IdP, stop before the callback, and send the victim the
// callback link: the victim's browser got an fc_session for the attacker's
// account. The callback now needs the cookie the start set in the same browser.

func envelopeCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error != "" {
		return env.Error
	}
	return env.Code
}

// The attacker's callback link, opened in the victim's browser (no binding
// cookie): refused, nothing signed in, and the state is spent.
func TestSSOCallbackWithoutBrowserBindingRefused(t *testing.T) {
	f := newOidcTestFixture(t)
	email := "mallory@" + f.domain

	state := randString(16)
	require.NoError(t, f.states.Insert(context.Background(),
		NewLoginState(state, f.domain, f.identityProviderID, f.mappingID, "nonce-bind-1", "")))
	f.idp.idTokenClaims = map[string]any{"nonce": "nonce-bind-1", "email": email}

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state="+state+"&code=fake-code-"+state, nil)
	rec := httptest.NewRecorder()
	f.endpoint.handleCallback(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, "LOGIN_NOT_STARTED_HERE", envelopeCode(t, rec), rec.Body.String())
	assert.False(t, f.lastSessionCalled, "no session for a callback this browser did not start")

	// The link is spent: the attacker cannot hand it to someone else.
	req = httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state="+state+"&code=fake-code-"+state, nil)
	req.AddCookie(f.endpoint.loginBindingCookie(state))
	rec = httptest.NewRecorder()
	f.endpoint.handleCallback(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, "INVALID_STATE", envelopeCode(t, rec), rec.Body.String())
	assert.False(t, f.lastSessionCalled)
}

// A binding cookie for another login (the attacker's own browser holds the
// cookie for the state in the link, the victim's at most one for a login of
// their own) does not satisfy the callback.
func TestSSOCallbackWithAnotherLoginsBindingRefused(t *testing.T) {
	f := newOidcTestFixture(t)
	for name, cookieFor := range map[string]func(state string) *http.Cookie{
		"another login's cookie": func(string) *http.Cookie { return f.endpoint.loginBindingCookie(randString(16)) },
		"this login's name, another value": func(state string) *http.Cookie {
			c := f.endpoint.loginBindingCookie(state)
			c.Value = "not-the-state"
			return c
		},
	} {
		t.Run(name, func(t *testing.T) {
			// A fresh state per case: a refused callback spends its state.
			state := randString(16)
			require.NoError(t, f.states.Insert(context.Background(),
				NewLoginState(state, f.domain, f.identityProviderID, f.mappingID, "nonce-bind-2", "")))
			f.idp.idTokenClaims = map[string]any{"nonce": "nonce-bind-2", "email": "mallory@" + f.domain}
			f.lastSessionCalled = false

			req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state="+state+"&code=fake-code-"+state, nil)
			req.AddCookie(cookieFor(state))
			rec := httptest.NewRecorder()
			f.endpoint.handleCallback(rec, req)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Equal(t, "LOGIN_NOT_STARTED_HERE", envelopeCode(t, rec), rec.Body.String())
			assert.False(t, f.lastSessionCalled)
		})
	}
}

// The whole round trip in one browser: the start sets the binding cookie
// (HttpOnly, SameSite=Lax, Path=/, the state's lifetime; __Host- and Secure
// when the platform runs on HTTPS), and the callback carrying it signs in and
// clears it.
func TestSSOLoginBindsTheStateToTheBrowser(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(map[bool]string{false: "http", true: "https"}[secure], func(t *testing.T) {
			f := newOidcTestFixture(t)
			f.endpoint.CookieSecure = secure

			rec := httptest.NewRecorder()
			f.endpoint.handleLogin(rec, httptest.NewRequest(http.MethodGet, "/auth/oidc/login?domain="+f.domain, nil))
			require.Equal(t, http.StatusFound, rec.Code, rec.Body.String())
			loc, err := url.Parse(rec.Header().Get("Location"))
			require.NoError(t, err)
			state, nonce := loc.Query().Get("state"), loc.Query().Get("nonce")
			require.NotEmpty(t, state)

			var binding *http.Cookie
			for _, c := range rec.Result().Cookies() {
				if c.Name == f.endpoint.loginBindingCookie(state).Name {
					binding = c
				}
			}
			require.NotNil(t, binding, "the start sets the binding cookie")
			assert.Equal(t, state, binding.Value)
			assert.True(t, binding.HttpOnly)
			assert.Equal(t, http.SameSiteLaxMode, binding.SameSite)
			assert.Equal(t, "/", binding.Path)
			assert.Empty(t, binding.Domain)
			assert.Equal(t, 600, binding.MaxAge)
			assert.Equal(t, secure, binding.Secure)
			if secure {
				assert.Regexp(t, `^__Host-`, binding.Name)
			}

			f.idp.idTokenClaims = map[string]any{"nonce": nonce, "email": "alice@" + f.domain}
			req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state="+url.QueryEscape(state)+"&code=fake-code", nil)
			req.AddCookie(&http.Cookie{Name: binding.Name, Value: binding.Value})
			rec = httptest.NewRecorder()
			f.endpoint.handleCallback(rec, req)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.True(t, f.lastSessionCalled, "signed in")

			cleared := false
			for _, c := range rec.Result().Cookies() {
				if c.Name == binding.Name && c.MaxAge < 0 {
					cleared = true
				}
			}
			assert.True(t, cleared, "the callback clears the binding cookie")
		})
	}
}

// The portal plane's SSO start shares the callback, so it binds its state too.
func TestPortalSSOStartBindsTheStateToTheBrowser(t *testing.T) {
	f := newOidcTestFixture(t)
	flows := portalauth.NewFlowRepo(f.pool)
	f.endpoint.Portal = &PortalBridge{Flows: flows}
	flow, err := portalauth.NewLoginFlow("oc-"+randString(8), "clt_"+randString(8), "https://app.example.org/cb", "rp-state")
	require.NoError(t, err)
	require.NoError(t, flows.Insert(context.Background(), flow))

	rec := httptest.NewRecorder()
	f.endpoint.handlePortalOIDCLogin(rec, httptest.NewRequest(http.MethodGet,
		"/portal/auth/oidc/login?flow="+url.QueryEscape(flow.ID)+"&provider_id="+url.QueryEscape(f.identityProviderID), nil))
	require.Equal(t, http.StatusFound, rec.Code, rec.Body.String())
	loc, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	state := loc.Query().Get("state")
	require.NotEmpty(t, state)

	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == f.endpoint.loginBindingCookieName(state) && c.Value == state {
			found = true
		}
	}
	assert.True(t, found, "the portal SSO start sets the binding cookie")
}
