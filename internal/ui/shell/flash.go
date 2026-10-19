package shell

import (
	"time"

	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"net/http"
	"regexp"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// FlashCookie carries the flash of a native post/redirect/get to the next
// page: a fixed key only (never text from the request), mapped to its
// message by Shell.Flashes when that page renders, then cleared. Datastar
// actions do not use it: they patch the flash stack directly (Flash).
const FlashCookie = "fc_ui_flash"

// flashKeyPattern is the shape of a flash key; anything else in the cookie
// is ignored without a lookup.
var flashKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

// flashMaxAge bounds how long a key waits for the page after the redirect.
const flashMaxAge = 60

// SetFlash queues the message with this key for the next page this browser
// renders (call it before the 303). The key must be one of Shell.Flashes;
// it is a programming constant, never request data.
func (s *Shell) SetFlash(w http.ResponseWriter, key string) {
	if !flashKeyPattern.MatchString(key) {
		panic("shell: invalid flash key " + key)
	}
	http.SetCookie(w, s.flashCookie(key, flashMaxAge))
}

// takeFlash reads and clears the flash cookie and returns the message its key
// names (none for an unknown or malformed key).
func (s *Shell) takeFlash(w http.ResponseWriter, r *http.Request) []kit.FlashMessageProps {
	c, err := r.Cookie(FlashCookie)
	if err != nil {
		return nil
	}
	http.SetCookie(w, s.flashCookie("", -1))
	if !flashKeyPattern.MatchString(c.Value) {
		return nil
	}
	msg, ok := s.Flashes[c.Value]
	if !ok {
		return nil
	}
	return []kit.FlashMessageProps{msg}
}

func (s *Shell) flashCookie(value string, maxAge int) *http.Cookie {
	return platformmw.NewCookie(platformmw.CookieSpec{
		Name: FlashCookie, Value: value, Path: "/ui", Secure: s.CookieSecure,
		TTL: time.Duration(maxAge) * time.Second,
	})
}
