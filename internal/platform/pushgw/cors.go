package pushgw

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

// originCacheTTL is how long the allowed-origins table is cached; an origin added in the console
// applies within this.
const originCacheTTL = 15 * time.Second

// OriginSource lists the allowed CORS origins (the platform's tnt_cors_allowed_origins:
// cors.Repository.GetAllowedOrigins).
type OriginSource interface {
	GetAllowedOrigins(ctx context.Context) ([]string, error)
}

// CORS answers cross-origin requests for the push routes only (spec 4): a page on an origin of
// the platform's allowed-origins table may call the gateway with a bearer token or tickets. It
// never answers Access-Control-Allow-Credentials, so a browser never lets such a page use the
// session cookie; a page on another origin cannot read an answer. A request without an Origin
// header, or from an origin not in the table, gets no CORS headers (a preflight from one is 403).
type CORS struct {
	src OriginSource

	mu      sync.Mutex
	at      time.Time
	allowed map[string]bool
	now     func() time.Time
}

// NewCORS returns the push CORS middleware over src.
func NewCORS(src OriginSource) *CORS { return &CORS{src: src, now: time.Now} }

// Methods and headers a push client sends: the three routes are POST and GET; Authorization and
// DPoP carry the credentials, Last-Event-ID the resume point of a fetch-based stream.
const (
	corsMethods = "GET, POST, OPTIONS"
	corsHeaders = "Authorization, Content-Type, DPoP, Last-Event-ID, X-Correlation-ID"
	corsExpose  = "Retry-After, X-Correlation-ID"
)

// Wrap puts next behind the CORS rules.
func (c *CORS) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		h.Add("Vary", "Origin")
		preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
		if !c.allows(r.Context(), origin) {
			if preflight {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Expose-Headers", corsExpose)
		if preflight {
			h.Add("Vary", "Access-Control-Request-Method")
			h.Add("Vary", "Access-Control-Request-Headers")
			h.Set("Access-Control-Allow-Methods", corsMethods)
			h.Set("Access-Control-Allow-Headers", corsHeaders)
			h.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allows reports whether origin is in the table; a failed read keeps the last answer.
func (c *CORS) allows(ctx context.Context, origin string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.allowed == nil || c.now().Sub(c.at) > originCacheTTL {
		if list, err := c.src.GetAllowedOrigins(ctx); err == nil {
			m := make(map[string]bool, len(list))
			for _, o := range list {
				m[strings.TrimRight(strings.TrimSpace(o), "/")] = true
			}
			c.allowed, c.at = m, c.now()
		} else {
			c.at = c.now() // do not read again for every request while the table cannot be read
			if c.allowed == nil {
				return false
			}
		}
	}
	return c.allowed[strings.TrimRight(origin, "/")]
}
