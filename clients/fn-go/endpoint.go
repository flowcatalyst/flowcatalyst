package fn

import (
	"net/http"
	"time"
)

// endpointOptions holds an endpoint's optional describe fields (plan
// §5.4). A nil pointer means "not declared" (omitted from describe), which
// is distinct from an explicit zero value (e.g. MaxBody(0)).
type endpointOptions struct {
	maxBodyBytes *int64
	timeoutMs    *int64
	cors         *CORSConfig
}

// EndpointOption configures an endpoint declared with Webhook, Platform or
// Open.
type EndpointOption func(*endpointOptions)

// Timeout overrides the endpoint's deadline (must be <= the function's own
// default per plan §7.1; the runner enforces the ceiling).
func Timeout(d time.Duration) EndpointOption {
	return func(o *endpointOptions) {
		ms := d.Milliseconds()
		o.timeoutMs = &ms
	}
}

// MaxBody overrides the endpoint's maximum request body size in bytes.
func MaxBody(n int64) EndpointOption {
	return func(o *endpointOptions) {
		o.maxBodyBytes = &n
	}
}

// CORSConfig declares an endpoint's CORS policy (plan §5.4).
type CORSConfig struct {
	Origins          []string
	Methods          []string
	Headers          []string
	AllowCredentials bool
}

// CORS declares the endpoint's CORS policy.
func CORS(cfg CORSConfig) EndpointOption {
	return func(o *endpointOptions) {
		c := cfg
		o.cors = &c
	}
}

// Webhook declares a webhook-authenticated endpoint (plan §5.4: signature
// verified by the runner). It must be POST-only; registering any other
// method panics.
func Webhook(pattern string, h http.HandlerFunc, opts ...EndpointOption) {
	registerEndpoint(authWebhook, pattern, h, opts)
}

// Platform declares a platform-authenticated endpoint: the runner validates
// a bearer token against the platform's JWKS before invoking the handler.
func Platform(pattern string, h http.HandlerFunc, opts ...EndpointOption) {
	registerEndpoint(authPlatform, pattern, h, opts)
}

// Open declares an endpoint with no authentication.
func Open(pattern string, h http.HandlerFunc, opts ...EndpointOption) {
	registerEndpoint(authNone, pattern, h, opts)
}
