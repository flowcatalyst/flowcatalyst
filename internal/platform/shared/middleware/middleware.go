// Package middleware provides the platform's chi/net/http middleware stack.
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/ory/fosite"
	"github.com/ory/fosite/token/jwt"

	"github.com/flowcatalyst/flowcatalyst-go/internal/logging"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/provider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// CorrelationID extracts X-Correlation-ID from the inbound request or
// generates a fresh one, attaches it to the context for log enrichment,
// and echoes it back on the response.
func CorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Correlation-ID")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Correlation-ID", id)
		ctx := logging.WithCorrelationID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AuthConfig governs how the Authenticator middleware behaves.
type AuthConfig struct {
	// Provider hosts the fosite OAuth2Provider used to introspect bearer
	// tokens. Required.
	Provider *provider.Provider

	// AllowTestHeaders enables the X-FC-Test-Principal dev fallback.
	// Defaults to false — set true only in dev/test environments. When
	// false the test headers are ignored entirely regardless of value.
	AllowTestHeaders bool

	// IgnoreInvalidTokens flips the default reject-on-invalid behavior.
	// Zero value (false) means: on a malformed/expired token, respond
	// 401 immediately. Set true to strip the token and proceed without
	// an AuthContext (per-handler permission checks will then reject).
	IgnoreInvalidTokens bool
}

// Authenticator validates the inbound Authorization: Bearer <jwt>,
// builds an AuthContext from the token's claims, and attaches it to the
// request context. Requests without a bearer token proceed without an
// AuthContext — per-handler Can*/Require* helpers reject them with
// UNAUTHENTICATED.
//
// When AllowTestHeaders is true, X-FC-Test-Principal (and the matching
// X-FC-Test-Scope/Clients/Permissions/Email/Roles headers) provide a
// dev-only bypass. Production deployments leave this false.
func Authenticator(cfg AuthConfig) func(http.Handler) http.Handler {
	if cfg.Provider == nil {
		panic("middleware.Authenticator: AuthConfig.Provider must not be nil")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			token := extractBearerToken(r)
			switch {
			case token != "":
				ac, err := introspect(ctx, cfg.Provider, token)
				if err != nil {
					if !cfg.IgnoreInvalidTokens {
						writeInvalidTokenError(w, err)
						return
					}
					// Strip the token and proceed.
				} else if ac != nil {
					ctx = auth.WithContext(ctx, ac)
					ctx = logging.WithPrincipalID(ctx, ac.PrincipalID)
				}

			case cfg.AllowTestHeaders && r.Header.Get("X-FC-Test-Principal") != "":
				ac := buildTestAuthContext(r)
				ctx = auth.WithContext(ctx, ac)
				ctx = logging.WithPrincipalID(ctx, ac.PrincipalID)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SessionCookieName is the cookie carrying the platform's JWT for
// browser sessions. The OIDC bridge / interactive authorize flow sets
// this on success; the Vue frontend round-trips it transparently. Same
// token type as the Authorization: Bearer transport — just a different
// carrier.
const SessionCookieName = "fc_session"

// extractBearerToken pulls the JWT out of the Authorization header,
// falling back to the fc_session cookie. Returns "" when neither is
// present (or the header is malformed).
func extractBearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h != "" {
		const prefix = "Bearer "
		if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
			return strings.TrimSpace(h[len(prefix):])
		}
		// Header present but not a Bearer scheme — don't fall through
		// to the cookie. A request that declares Basic / Digest /
		// anything-else is signalling its intent explicitly.
		return ""
	}
	if c, err := r.Cookie(SessionCookieName); err == nil {
		return strings.TrimSpace(c.Value)
	}
	return ""
}

// introspect runs the token through fosite and projects the resulting
// session onto an AuthContext. Returns (nil, nil) for tokens that are
// well-formed but inactive (revoked or expired-and-introspection-says-so),
// (nil, error) for malformed or rejected tokens, and (ctx, nil) on success.
func introspect(ctx context.Context, p *provider.Provider, token string) (*auth.AuthContext, error) {
	session := provider.NewSession()
	_, ar, err := p.OAuth2.IntrospectToken(ctx, token, fosite.AccessToken, session)
	if err != nil {
		return nil, err
	}
	if ar == nil {
		return nil, nil
	}

	// fosite populates the supplied session with the token's claims on
	// successful introspection. GetJWTClaims returns the abstract
	// container interface — cast to the concrete *jwt.JWTClaims to read
	// the fields the platform stamps on at mint time.
	container := session.GetJWTClaims()
	jc, _ := container.(*jwt.JWTClaims)
	if jc == nil {
		return nil, nil
	}

	ac := &auth.AuthContext{
		PrincipalID: jc.Subject,
	}
	// JWTClaims.Scope is the granted-scope slice; we stored the
	// principal's UserScope ("ANCHOR"/"PARTNER"/"CLIENT") as the
	// scope claim at mint time.
	if len(jc.Scope) > 0 {
		ac.Scope = auth.Scope(jc.Scope[0])
	}
	if jc.Extra != nil {
		if v, ok := jc.Extra["email"].(string); ok {
			ac.Email = v
		}
		ac.Clients = stringSlice(jc.Extra["clients"])
		ac.Roles = stringSlice(jc.Extra["roles"])
		ac.Applications = stringSlice(jc.Extra["applications"])
		ac.Permissions = stringSlice(jc.Extra["permissions"])
	}
	return ac, nil
}

// stringSlice coerces a JWT Extra claim into []string. Tokens minted by
// this server emit []string directly, but tokens that round-trip through
// JSON serialization arrive as []interface{} with string elements.
func stringSlice(v any) []string {
	switch x := v.(type) {
	case []string:
		return append([]string(nil), x...)
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// writeInvalidTokenError emits a RFC 6750-flavoured 401 with the
// platform's standard error envelope so SDK consumers can branch on it.
func writeInvalidTokenError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
	w.WriteHeader(http.StatusUnauthorized)
	desc := "invalid bearer token"
	if rfcErr := fosite.ErrorToRFC6749Error(err); rfcErr != nil {
		desc = rfcErr.ErrorField
		if rfcErr.DescriptionField != "" {
			desc = rfcErr.DescriptionField
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":             "invalid_token",
		"error_description": desc,
	})
}

// buildTestAuthContext is the dev/test path. Only reachable when
// AllowTestHeaders is true. Headers:
//
//	X-FC-Test-Principal:    principal ID
//	X-FC-Test-Scope:        ANCHOR | PARTNER | CLIENT (default CLIENT)
//	X-FC-Test-Clients:      comma-separated client IDs
//	X-FC-Test-Permissions:  comma-separated permission codes
//	X-FC-Test-Roles:        comma-separated role names
//	X-FC-Test-Email:        principal email
func buildTestAuthContext(r *http.Request) *auth.AuthContext {
	scope := auth.Scope(r.Header.Get("X-FC-Test-Scope"))
	if scope == "" {
		scope = auth.ScopeClient
	}
	return &auth.AuthContext{
		PrincipalID: r.Header.Get("X-FC-Test-Principal"),
		Email:       r.Header.Get("X-FC-Test-Email"),
		Scope:       scope,
		Clients:     splitCSV(r.Header.Get("X-FC-Test-Clients")),
		Roles:       splitCSV(r.Header.Get("X-FC-Test-Roles")),
		Permissions: splitCSV(r.Header.Get("X-FC-Test-Permissions")),
	}
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if start < i {
				out = append(out, strings.TrimSpace(s[start:i]))
			}
			start = i + 1
		}
	}
	return out
}

// WithAuth is a context helper for tests that bypasses the HTTP layer.
func WithAuth(ctx context.Context, ac *auth.AuthContext) context.Context {
	return auth.WithContext(ctx, ac)
}
