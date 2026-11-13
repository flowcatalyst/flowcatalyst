package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionCookieAttributes(t *testing.T) {
	t.Cleanup(func() { _ = SetSessionSameSite("lax") })
	for _, secure := range []bool{true, false} {
		for _, expires := range []bool{true, false} {
			c := SessionCookie("jwt", secure, time.Hour, expires)
			wantName := "fc_session"
			if secure {
				wantName = "__Host-fc_session"
			}
			if c.Name != wantName || c.Value != "jwt" || c.Path != "/" || !c.HttpOnly ||
				c.Secure != secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 3600 ||
				c.Expires.IsZero() == expires {
				t.Fatalf("secure=%v expires=%v cookie=%+v", secure, expires, c)
			}
		}
		rec := httptest.NewRecorder()
		ExpireSessionCookies(rec, secure)
		cs := rec.Result().Cookies()
		if len(cs) != 2 {
			t.Fatalf("clear secure=%v: want 2 cookies, got %+v", secure, cs)
		}
		if c := cs[0]; c.Name != "fc_session" || c.Value != "" || c.Path != "/" || !c.HttpOnly ||
			c.Secure != secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge != -1 {
			t.Fatalf("clear secure=%v old-name cookie=%+v", secure, c)
		}
		if c := cs[1]; c.Name != "__Host-fc_session" || c.Value != "" || c.Path != "/" || c.Domain != "" ||
			!c.HttpOnly || !c.Secure || c.MaxAge != -1 {
			t.Fatalf("clear secure=%v host cookie=%+v", secure, c)
		}
	}
}

func TestSessionSameSiteKnob(t *testing.T) {
	t.Cleanup(func() { _ = SetSessionSameSite("lax") })
	for in, want := range map[string]http.SameSite{
		"": http.SameSiteLaxMode, "lax": http.SameSiteLaxMode, "Strict": http.SameSiteStrictMode, "\tnone": http.SameSiteNoneMode,
	} {
		if err := SetSessionSameSite(in); err != nil {
			t.Fatal(err)
		}
		if got := SessionCookie("x", true, time.Minute, false).SameSite; got != want {
			t.Fatalf("%q: got %v want %v", in, got, want)
		}
		rec := httptest.NewRecorder()
		ExpireSessionCookies(rec, true)
		for _, c := range rec.Result().Cookies() {
			if c.SameSite != want {
				t.Fatalf("%q clear %s: got %v want %v", in, c.Name, c.SameSite, want)
			}
		}
	}
	if err := SetSessionSameSite("bogus"); err == nil {
		t.Fatal("bogus mode must be refused")
	}
}

func TestNewCookieDefaultsAndSessionCookie(t *testing.T) {
	c := NewCookie(CookieSpec{Name: "x", Value: "v", Secure: true, SameSite: http.SameSiteStrictMode, Path: "/ui", TTL: 5 * time.Second})
	if c.Path != "/ui" || c.SameSite != http.SameSiteStrictMode || c.MaxAge != 5 || !c.HttpOnly {
		t.Fatalf("%+v", c)
	}
	d := NewCookie(CookieSpec{Name: "x", Value: "v"})
	if d.Path != "/" || d.SameSite != http.SameSiteLaxMode || d.MaxAge != 0 {
		t.Fatalf("%+v", d)
	}
}

func TestSameSiteNoneAlwaysSecure(t *testing.T) {
	c := NewCookie(CookieSpec{Name: "x", Value: "v", SameSite: http.SameSiteNoneMode, Secure: false})
	if !c.Secure {
		t.Error("SameSite=None built without Secure: browsers would reject the cookie")
	}
	if c := NewCookie(CookieSpec{Name: "x", Value: "v", SameSite: http.SameSiteLaxMode}); c.Secure {
		t.Error("Secure was forced on for Lax")
	}
}
