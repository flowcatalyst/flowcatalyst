package bridge

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestBindingCookieAttributes pins the SSO login-binding cookie.
func TestBindingCookieAttributes(t *testing.T) {
	for _, secure := range []bool{true, false} {
		e := &LoginEndpoint{CookieSecure: secure}
		c := e.loginBindingCookie("state-123")
		if c.Value != "state-123" || c.Path != "/" || !c.HttpOnly || c.Secure != secure ||
			c.SameSite != http.SameSiteLaxMode || c.MaxAge != int(loginBindingTTL.Seconds()) ||
			!strings.HasPrefix(c.Name, map[bool]string{true: "__Host-fc_sso_", false: "fc_sso_"}[secure]) {
			t.Fatalf("secure=%v cookie = %+v", secure, c)
		}
	}
}

// TestSessionEndClearsSessionCookie pins the fc_session clear on OIDC logout.
func TestSessionEndClearsSessionCookie(t *testing.T) {
	for _, secure := range []bool{true, false} {
		e := &LoginEndpoint{CookieSecure: secure}
		rec := httptest.NewRecorder()
		e.handleSessionEnd(rec, httptest.NewRequest(http.MethodGet, "/auth/oidc/session/end", nil))
		cs := rec.Result().Cookies()
		if len(cs) != 1 {
			t.Fatalf("want 1 cookie, got %d", len(cs))
		}
		c := cs[0]
		if c.Name != "fc_session" || c.Value != "" || c.Path != "/" || !c.HttpOnly ||
			c.Secure != secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge >= 0 {
			t.Fatalf("secure=%v cookie = %+v", secure, c)
		}
	}
}
