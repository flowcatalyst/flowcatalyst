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
		signOut(secure, nil)(rec, httptest.NewRequest(http.MethodPost, "/ui/logout", nil))
		cs := rec.Result().Cookies()
		if len(cs) != 2 {
			t.Fatalf("want both session cookie names expired, got %d cookies", len(cs))
		}
		c := cs[0]
		if c.Name != "fc_session" || c.Value != "" || c.Path != "/" || !c.HttpOnly ||
			c.Secure != secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge >= 0 {
			t.Fatalf("secure=%v cookie = %+v", secure, c)
		}
		h := cs[1]
		if h.Name != "__Host-fc_session" || h.Value != "" || h.Path != "/" || h.Domain != "" || !h.HttpOnly ||
			!h.Secure || h.SameSite != http.SameSiteLaxMode || h.MaxAge >= 0 {
			t.Fatalf("secure=%v host cookie = %+v", secure, h)
		}
	}
}
