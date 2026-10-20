package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// CookieSpec describes one cookie. HttpOnly is always set: every cookie the
// platform issues is server-only.
type CookieSpec struct {
	Name  string
	Value string
	// Path defaults to "/".
	Path   string
	Secure bool
	// SameSite zero value means Lax.
	SameSite http.SameSite
	// TTL > 0 sets MaxAge; TTL < 0 builds an expiring clear (MaxAge -1);
	// TTL == 0 builds a session cookie (no MaxAge).
	TTL time.Duration
	// SetExpires also stamps the Expires attribute (now+TTL) on a set cookie,
	// for clients that ignore Max-Age.
	SetExpires bool
}

// NewCookie is the single constructor for every cookie the platform sets, so
// attribute changes (SameSite, Secure, Path) are made in one place.
func NewCookie(s CookieSpec) *http.Cookie {
	path := s.Path
	if path == "" {
		path = "/"
	}
	ss := s.SameSite
	if ss == 0 {
		ss = http.SameSiteLaxMode
	}
	// Browsers reject SameSite=None without Secure, so the pair is never built
	// apart: the cookie would silently never be stored.
	secure := s.Secure || ss == http.SameSiteNoneMode
	c := &http.Cookie{
		Name:     s.Name,
		Value:    s.Value,
		Path:     path,
		HttpOnly: true,
		Secure:   secure,
		SameSite: ss,
	}
	switch {
	case s.TTL < 0:
		c.Value = ""
		c.MaxAge = -1
	case s.TTL > 0:
		c.MaxAge = int(s.TTL.Seconds())
		if s.SetExpires {
			c.Expires = time.Now().Add(s.TTL)
		}
	}
	return c
}

// sessionSameSite is the SameSite mode of the fc_session cookie (Lax unless
// FC_SESSION_COOKIE_SAME_SITE says otherwise).
var sessionSameSite atomic.Int32

func init() { sessionSameSite.Store(int32(http.SameSiteLaxMode)) }

// SetSessionSameSite configures the fc_session SameSite mode: "" or "lax"
// (default), "strict" or "none" ("none" forces Secure on, which browsers require).
func SetSessionSameSite(mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "lax":
		sessionSameSite.Store(int32(http.SameSiteLaxMode))
	case "strict":
		sessionSameSite.Store(int32(http.SameSiteStrictMode))
	case "none":
		sessionSameSite.Store(int32(http.SameSiteNoneMode))
	default:
		return fmt.Errorf("FC_SESSION_COOKIE_SAME_SITE: %q is not one of lax, strict, none", mode)
	}
	return nil
}

// SessionCookie builds the fc_session cookie carrying a minted session JWT.
func SessionCookie(token string, secure bool, ttl time.Duration, setExpires bool) *http.Cookie {
	return NewCookie(CookieSpec{
		Name: SessionCookieName, Value: token, Secure: secure,
		SameSite: http.SameSite(sessionSameSite.Load()), TTL: ttl, SetExpires: setExpires,
	})
}

// ClearSessionCookie builds the fc_session expiry. It carries the same
// attributes as SessionCookie, or the browser may not match the original.
func ClearSessionCookie(secure bool) *http.Cookie {
	return NewCookie(CookieSpec{
		Name: SessionCookieName, Secure: secure,
		SameSite: http.SameSite(sessionSameSite.Load()), TTL: -1,
	})
}
