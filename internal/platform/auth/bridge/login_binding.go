package bridge

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"time"
)

// Browser binding of an OIDC login state (SSO login CSRF).
//
// The login state lives in the database, so on its own a callback URL
// (state + code) completes in any browser: someone with an account at any
// mapped identity provider could start a login, sign in at the IdP, stop
// before the callback and send the link to a victim, whose browser would be
// signed in to the attacker's account. The start therefore sets a cookie in
// the browser that started it, and the callback requires it.
//
// One cookie per login (the name carries a digest of the state), so two
// sign-ins in two tabs do not overwrite each other's binding. With HTTPS the
// name takes the __Host- prefix: the browser then accepts it only with Secure,
// Path=/ and no Domain, so a sibling subdomain cannot plant one for the
// platform's host (cookie tossing). SameSite=Lax: the IdP's redirect back is a
// top-level GET navigation, on which Lax cookies are sent.

// loginBindingTTL matches the login state's lifetime (NewLoginState).
const loginBindingTTL = 10 * time.Minute

// loginBindingCookieName is the binding cookie's name for state.
func (e *LoginEndpoint) loginBindingCookieName(state string) string {
	sum := sha256.Sum256([]byte(state))
	name := "fc_sso_" + hex.EncodeToString(sum[:8])
	if e.CookieSecure {
		return "__Host-" + name
	}
	return name
}

// loginBindingCookie is the cookie the login start sets for state.
func (e *LoginEndpoint) loginBindingCookie(state string) *http.Cookie {
	return &http.Cookie{
		Name:     e.loginBindingCookieName(state),
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   e.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(loginBindingTTL.Seconds()),
	}
}

// loginBoundToBrowser reports whether r carries the binding cookie for state.
func (e *LoginEndpoint) loginBoundToBrowser(r *http.Request, state string) bool {
	c, err := r.Cookie(e.loginBindingCookieName(state))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(state)) == 1
}

// clearLoginBinding expires the binding cookie for state.
func (e *LoginEndpoint) clearLoginBinding(w http.ResponseWriter, state string) {
	c := e.loginBindingCookie(state)
	c.Value = ""
	c.MaxAge = -1
	http.SetCookie(w, c)
}
