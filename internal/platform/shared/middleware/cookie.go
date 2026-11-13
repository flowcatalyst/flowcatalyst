package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/sessiontoken"
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

// sessionSecure records whether this process issues Secure cookies (set once at
// start-up from the same value that feeds every CookieSecure field). Default false.
var sessionSecure atomic.Bool

// SetSessionCookieSecure tells the process whether it issues Secure session
// cookies. On a Secure process the old cookie name (fc_session) is accepted only
// for tokens minted before the __Host- name existed; see [SessionCookieAccepted].
func SetSessionCookieSecure(secure bool) { sessionSecure.Store(secure) }

// SessionCookie builds the browser session cookie carrying a minted session JWT.
// It is named __Host-fc_session when it carries Secure (the effective value: a
// SameSite=None cookie is forced Secure) and fc_session otherwise (plain-HTTP
// development, where a browser refuses a __Host- cookie). A __Host- cookie must
// have Path=/, no Domain and Secure; NewCookie guarantees all three.
func SessionCookie(token string, secure bool, ttl time.Duration, setExpires bool) *http.Cookie {
	c := NewCookie(CookieSpec{
		Name: SessionCookieName, Value: token, Secure: secure,
		SameSite: http.SameSite(sessionSameSite.Load()), TTL: ttl, SetExpires: setExpires,
	})
	if c.Secure {
		c.Name = HostSessionCookieName
	}
	return c
}

// ExpireSessionCookies writes the expiry of BOTH session cookie names, so a
// sign-out cannot leave one behind. The old-name expiry carries the same
// attributes as the cookie SessionCookie builds on a non-Secure process (or the
// browser may not match the original); the __Host- expiry always carries Secure
// (a browser on plain HTTP ignores it, which is harmless).
func ExpireSessionCookies(w http.ResponseWriter, secure bool) {
	ss := http.SameSite(sessionSameSite.Load())
	http.SetCookie(w, NewCookie(CookieSpec{Name: SessionCookieName, Secure: secure, SameSite: ss, TTL: -1}))
	http.SetCookie(w, NewCookie(CookieSpec{Name: HostSessionCookieName, Secure: true, SameSite: ss, TTL: -1}))
}

// ReadSessionCookie is the only place that reads the session cookie from a
// request. __Host-fc_session, when present and non-empty, is used and the old
// fc_session is ignored entirely; otherwise fc_session is used and oldName is
// true. The value is trimmed; ("", false) when neither is present.
func ReadSessionCookie(r *http.Request) (token string, oldName bool) {
	if c, err := r.Cookie(HostSessionCookieName); err == nil {
		if v := strings.TrimSpace(c.Value); v != "" {
			return v, false
		}
	}
	if c, err := r.Cookie(SessionCookieName); err == nil {
		return strings.TrimSpace(c.Value), true
	}
	return "", false
}

// SessionCookieAccepted is the one rule for a validated session token that came
// from the cookie. A token that arrived under the old name (fc_session) on a
// Secure process is refused when it carries the host-cookie marker: a token
// minted after the upgrade never legitimately travels as fc_session on HTTPS,
// so it is a planted cookie. A token without the marker predates the upgrade and
// is accepted until it expires. A non-Secure process accepts either.
func SessionCookieAccepted(oldName bool, c *sessiontoken.Claims) bool {
	if c == nil {
		return false
	}
	if oldName && sessionSecure.Load() && c.HostCookie {
		return false
	}
	return true
}
