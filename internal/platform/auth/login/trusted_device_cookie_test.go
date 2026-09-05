package login

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClearTrustedDeviceCookie pins that the clear targets the same cookie
// rememberDevice sets (name + attributes) and expires it, so a password
// change leaves no dead trusted-device token on the browser.
func TestClearTrustedDeviceCookie(t *testing.T) {
	for _, secure := range []bool{true, false} {
		e := New(Config{CookieSecure: secure})
		rec := httptest.NewRecorder()
		e.clearTrustedDeviceCookie(rec)

		cookies := rec.Result().Cookies()
		require.Len(t, cookies, 1)
		c := cookies[0]
		require.Equal(t, e.trustedDeviceCookieName(), c.Name)
		require.Empty(t, c.Value)
		require.Equal(t, "/", c.Path)
		require.True(t, c.HttpOnly)
		require.Equal(t, secure, c.Secure)
		require.Equal(t, http.SameSiteStrictMode, c.SameSite)
		require.Less(t, c.MaxAge, 0, "must be an expiry, not a set")
	}
}
