package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSignOutCookieAttributes pins the fc_session clear written by the UI
// sign-out: it must carry the attributes of the cookie it clears.
func TestSignOutCookieAttributes(t *testing.T) {
	for _, secure := range []bool{true, false} {
		rec := httptest.NewRecorder()
		signOut(secure)(rec, httptest.NewRequest(http.MethodPost, "/ui/logout", nil))
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
