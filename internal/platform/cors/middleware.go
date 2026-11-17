package cors

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OriginCacheTTL is how long [Origins] keeps the allowed-origins table: an origin added or removed
// in the console applies within this.
const OriginCacheTTL = 15 * time.Second

// PreflightMaxAge is how long a browser may keep a preflight answer (Access-Control-Max-Age). A
// removed origin can keep sending the requests a cached preflight allowed for this long, but every
// actual answer is checked against the table again, so its page cannot read them after
// OriginCacheTTL.
const PreflightMaxAge = 10 * time.Minute

// OriginSource lists the allowed CORS origins: the platform's tnt_cors_allowed_origins
// ([Repository.GetAllowedOrigins]).
type OriginSource interface {
	GetAllowedOrigins(ctx context.Context) ([]string, error)
}

// Origins is the allowed-origins table as the CORS middleware reads it: cached for
// [OriginCacheTTL]; a failed read keeps the last answer (and is not retried before the TTL), and
// before any read has succeeded no origin is allowed.
type Origins struct {
	src OriginSource
	now func() time.Time

	mu      sync.Mutex
	at      time.Time
	allowed map[string]bool
}

// NewOrigins returns the cached table over src. now is the clock (nil: the system clock).
func NewOrigins(src OriginSource, now func() time.Time) *Origins {
	if now == nil {
		now = time.Now
	}
	return &Origins{src: src, now: now}
}

// Allows reports whether origin is in the table. Origins are compared without a trailing slash.
func (o *Origins) Allows(ctx context.Context, origin string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.allowed == nil || o.now().Sub(o.at) > OriginCacheTTL {
		if list, err := o.src.GetAllowedOrigins(ctx); err == nil {
			m := make(map[string]bool, len(list))
			for _, s := range list {
				m[strings.TrimRight(strings.TrimSpace(s), "/")] = true
			}
			o.allowed, o.at = m, o.now()
		} else {
			o.at = o.now() // do not read again for every request while the table cannot be read
			if o.allowed == nil {
				return false
			}
		}
	}
	return o.allowed[strings.TrimRight(origin, "/")]
}

// Rule is what one set of routes lets a page on an allowed origin do. Each list is the header
// value as sent ("GET, POST").
type Rule struct {
	// Methods is Access-Control-Allow-Methods: the methods the routes answer.
	Methods string
	// Headers is Access-Control-Allow-Headers: the request headers the routes' clients send that
	// a browser does not allow cross-origin by itself.
	Headers string
	// Expose is Access-Control-Expose-Headers: the response headers the clients read.
	Expose string
}

// Middleware answers cross-origin requests for one set of routes from the platform's
// allowed-origins table. It is the one CORS rule of the platform's own endpoints (the push gateway
// and the field-worker protocol use it, each with its Rule):
//
//   - a request without an Origin header is passed on untouched;
//   - an origin in the table gets Access-Control-Allow-Origin naming it (never "*"), the rule's
//     exposed headers, and Vary: Origin; a preflight (OPTIONS with
//     Access-Control-Request-Method) is answered here, 204 with the rule's methods and headers
//     and a max age of [PreflightMaxAge];
//   - an origin not in the table gets no CORS headers (only Vary: Origin): its preflight is
//     refused 403 here, and any other request is passed on, so the browser keeps the answer
//     from its page;
//   - Access-Control-Allow-Credentials is never sent, so a browser never lets a page on another
//     origin send or use the platform's session cookie through these routes.
func Middleware(origins *Origins, rule Rule) func(http.Handler) http.Handler {
	maxAge := strconv.Itoa(int(PreflightMaxAge / time.Second))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			h := w.Header()
			h.Add("Vary", "Origin")
			preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
			if !origins.Allows(r.Context(), origin) {
				if preflight {
					http.Error(w, "origin not allowed", http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Expose-Headers", rule.Expose)
			if preflight {
				h.Add("Vary", "Access-Control-Request-Method")
				h.Add("Vary", "Access-Control-Request-Headers")
				h.Set("Access-Control-Allow-Methods", rule.Methods)
				h.Set("Access-Control-Allow-Headers", rule.Headers)
				h.Set("Access-Control-Max-Age", maxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
