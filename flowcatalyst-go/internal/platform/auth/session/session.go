package session

import (
	"net/http"
	"strings"
	"time"
)

// Config holds session configuration
type Config struct {
	CookieName string
	Secure     bool
	SameSite   http.SameSite
	MaxAge     time.Duration
	Path       string
	Domain     string
}

// DefaultConfig returns default session configuration
func DefaultConfig() Config {
	return Config{
		CookieName: "FLOWCATALYST_SESSION",
		Secure:     true,
		SameSite:   http.SameSiteStrictMode,
		MaxAge:     8 * time.Hour,
		Path:       "/",
	}
}

// Manager handles session cookies
type Manager struct {
	config Config
}

// NewManager creates a new session manager
func NewManager(config Config) *Manager {
	return &Manager{config: config}
}

// SetSession sets the session cookie with the given token
func (m *Manager) SetSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.config.CookieName,
		Value:    token,
		Path:     m.config.Path,
		Domain:   m.config.Domain,
		MaxAge:   int(m.config.MaxAge.Seconds()),
		Secure:   m.config.Secure,
		HttpOnly: true,
		SameSite: m.config.SameSite,
	})
}

// ClearSession clears the session cookie
func (m *Manager) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.config.CookieName,
		Value:    "",
		Path:     m.config.Path,
		Domain:   m.config.Domain,
		MaxAge:   -1,
		Secure:   m.config.Secure,
		HttpOnly: true,
		SameSite: m.config.SameSite,
	})
}

// GetSession retrieves the session token from the request
// It checks both the cookie and the Authorization header
func (m *Manager) GetSession(r *http.Request) string {
	// First, try to get from cookie
	cookie, err := r.Cookie(m.config.CookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	// Fall back to Authorization header
	return m.GetBearerToken(r)
}

// GetBearerToken extracts the bearer token from the Authorization header
func (m *Manager) GetBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	// Check for Bearer scheme
	const prefix = "Bearer "
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return ""
	}

	return auth[len(prefix):]
}

// HasSession checks if a session exists in the request
func (m *Manager) HasSession(r *http.Request) bool {
	return m.GetSession(r) != ""
}

// ParseSameSite converts a string to http.SameSite
func ParseSameSite(s string) http.SameSite {
	switch strings.ToLower(s) {
	case "lax":
		return http.SameSiteLaxMode
	case "none":
		return http.SameSiteNoneMode
	case "strict":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteStrictMode
	}
}
