package shell

import (
	"net/http"
	"testing"
)

func TestFlashCookieAttributes(t *testing.T) {
	for _, secure := range []bool{true, false} {
		s := &Shell{CookieSecure: secure}
		set := s.flashCookie("pool-created", flashMaxAge)
		if set.Name != FlashCookie || set.Value != "pool-created" || set.Path != "/ui" || !set.HttpOnly ||
			set.Secure != secure || set.SameSite != http.SameSiteLaxMode || set.MaxAge != flashMaxAge {
			t.Fatalf("set cookie = %+v", set)
		}
		clr := s.flashCookie("", -1)
		if clr.Name != FlashCookie || clr.Value != "" || clr.Path != "/ui" || !clr.HttpOnly ||
			clr.Secure != secure || clr.SameSite != http.SameSiteLaxMode || clr.MaxAge >= 0 {
			t.Fatalf("clear cookie = %+v", clr)
		}
	}
}
