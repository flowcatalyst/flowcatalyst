package middleware

import (
	"net/http"
	"testing"
	"time"
)

func TestSessionCookieAttributes(t *testing.T) {
	t.Cleanup(func() { _ = SetSessionSameSite("lax") })
	for _, secure := range []bool{true, false} {
		for _, expires := range []bool{true, false} {
			c := SessionCookie("jwt", secure, time.Hour, expires)
			if c.Name != "fc_session" || c.Value != "jwt" || c.Path != "/" || !c.HttpOnly ||
				c.Secure != secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 3600 ||
				c.Expires.IsZero() == expires {
				t.Fatalf("secure=%v expires=%v cookie=%+v", secure, expires, c)
			}
		}
		c := ClearSessionCookie(secure)
		if c.Name != "fc_session" || c.Value != "" || c.Path != "/" || !c.HttpOnly ||
			c.Secure != secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge != -1 {
			t.Fatalf("clear secure=%v cookie=%+v", secure, c)
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
		if got := ClearSessionCookie(true).SameSite; got != want {
			t.Fatalf("%q clear: got %v want %v", in, got, want)
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
