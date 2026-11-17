package pushgw

import (
	"net/http"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
)

// originCacheTTL is how long the allowed-origins table is cached; an origin added in the console
// applies within this.
const originCacheTTL = cors.OriginCacheTTL

// OriginSource lists the allowed CORS origins (the platform's tnt_cors_allowed_origins:
// cors.Repository.GetAllowedOrigins).
type OriginSource = cors.OriginSource

// CORS answers cross-origin requests for the push routes only (spec 4): a page on an origin of
// the platform's allowed-origins table may call the gateway with a bearer token or tickets. The
// rule is the platform's one CORS middleware (cors.Middleware) with the push routes' methods and
// headers: it never answers Access-Control-Allow-Credentials, so a browser never lets such a page
// use the session cookie; a page on another origin cannot read an answer. A request without an
// Origin header, or from an origin not in the table, gets no CORS headers (a preflight from one is
// 403).
type CORS struct {
	now  func() time.Time
	wrap func(http.Handler) http.Handler
}

// NewCORS returns the push CORS middleware over src.
func NewCORS(src OriginSource) *CORS {
	c := &CORS{now: time.Now}
	c.wrap = cors.Middleware(cors.NewOrigins(src, func() time.Time { return c.now() }), corsRule)
	return c
}

// corsRule is what a push client sends and reads: the three routes are POST and GET;
// Authorization and DPoP carry the credentials, Last-Event-ID the resume point of a fetch-based
// stream; Retry-After comes with a 429 or 503.
var corsRule = cors.Rule{
	Methods: "GET, POST, OPTIONS",
	Headers: "Authorization, Content-Type, DPoP, Last-Event-ID, X-Correlation-ID",
	Expose:  "Retry-After, X-Correlation-ID",
}

// Wrap puts next behind the CORS rules.
func (c *CORS) Wrap(next http.Handler) http.Handler { return c.wrap(next) }
