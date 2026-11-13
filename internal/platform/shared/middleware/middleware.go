// Package middleware provides the platform's chi/net/http middleware stack.
package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/flowcatalyst/flowcatalyst-go/internal/logging"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/provider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/sessiontoken"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// errIdentityTokenNotAPICredential rejects an interactive-login (identity)
// access token (or any other non-API class) presented as an API bearer. Such tokens carry no authority and
// exist only to satisfy OAuth2/OIDC (the id_token is the app's authentication
// artifact); accepting one here — even as an empty-authority context — would
// let an authentication-only handler treat a login token as an API credential.
var errIdentityTokenNotAPICredential = errors.New("this token cannot authorize API requests (an interactive-login or session token is not an API credential); obtain an API token via the client_credentials grant")

// errNotSessionToken: the fc_session cookie carried something other than a session
// token. The Authenticator treats it like a stale cookie (proceeds unauthenticated).
var errNotSessionToken = errors.New("the session cookie does not carry a session token")

// errSessionCookieOldName: a token carrying the host-cookie marker arrived under
// the old cookie name on a Secure process (a planted cookie). Treated like a
// stale cookie.
var errSessionCookieOldName = errors.New("the session token arrived under the old cookie name")

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
	// Provider validates bearer/session JWTs and projects principal
	// claims. Required.
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

	// StepUpMaxAge is how recent a cookie session's sign-in must be to change
	// sign-in methods (see RequireRecentSignIn). 0 disables the rule.
	StepUpMaxAge time.Duration
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

			token, fromCookie, oldName := extractToken(r)
			switch {
			case token != "":
				ac, issuedAt, err := introspect(ctx, cfg.Provider, token, fromCookie, oldName)
				if err != nil {
					// A stale / invalid fc_session cookie must not hard-fail the
					// request: the browser replays it on every call — including
					// the public login routes (/auth/login, /auth/oidc/login) —
					// so treat it as logged-out and proceed unauthenticated. An
					// explicit Authorization: Bearer carrying a bad token is
					// still rejected (unless IgnoreInvalidTokens) so API callers
					// get a clear error rather than a silent downgrade.
					if !fromCookie && !cfg.IgnoreInvalidTokens {
						writeInvalidTokenError(w, err)
						return
					}
					// Strip the token and proceed unauthenticated.
				} else if ac != nil {
					ctx = auth.WithContext(ctx, ac)
					ctx = logging.WithPrincipalID(ctx, ac.PrincipalID)
					if fromCookie {
						ctx = WithSessionCookieAuth(ctx)
						ctx = WithSessionInfo(ctx, issuedAt, cfg.StepUpMaxAge)
					} else {
						ctx = WithBearerAuth(ctx)
					}
				}

			case cfg.AllowTestHeaders && r.Header.Get("X-FC-Test-Principal") != "" && testHeadersAllowedFrom(r):
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
//
// The cookie has two names. HostSessionCookieName is written on HTTPS (a
// __Host- cookie cannot be planted from a sibling subdomain); SessionCookieName
// is written on plain HTTP and is the old name read during the transition. See
// [ReadSessionCookie] and [SessionCookieAccepted].
const SessionCookieName = "fc_session"

// HostSessionCookieName is the session cookie's name on a Secure process.
const HostSessionCookieName = "__Host-fc_session"

// extractToken pulls the JWT out of the Authorization header (fromCookie
// false), falling back to the fc_session cookie (fromCookie true). Returns
// ("", false) when neither is present (or the header is malformed). The source
// matters: a bad bearer token is an explicit error, a stale cookie is a
// graceful logout.
func extractToken(r *http.Request) (token string, fromCookie, oldName bool) {
	h := r.Header.Get("Authorization")
	if h != "" {
		const prefix = "Bearer "
		if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
			return strings.TrimSpace(h[len(prefix):]), false, false
		}
		// Header present but not a Bearer scheme — don't fall through
		// to the cookie. A request that declares Basic / Digest /
		// anything-else is signalling its intent explicitly.
		return "", false, false
	}
	if tok, old := ReadSessionCookie(r); tok != "" {
		return tok, true, old
	}
	return "", false, false
}

// introspect validates the session-cookie JWT via the sessiontoken
// package (signature + standard claim checks) and projects the parsed
// claims onto an AuthContext. Returns (nil, error) for malformed or
// expired tokens, (ctx, nil) on success.
//
// Cookie + Bearer transports share this path. The line between this
// local validation path and the `/oauth/introspect` endpoint is
// deliberate — see ADR-0001.
func introspect(ctx context.Context, p *provider.Provider, token string, fromCookie, oldName bool) (*auth.AuthContext, time.Time, error) {
	c, err := p.ValidateSessionToken(ctx, token)
	if err != nil {
		return nil, time.Time{}, err
	}
	if fromCookie && !SessionCookieAccepted(oldName, c) {
		return nil, time.Time{}, errSessionCookieOldName
	}
	ac, err := authContextFromClaims(ctx, p, c, fromCookie)
	return ac, c.IssuedAt, err
}

// Credential is the platform credential a request carried, as [AuthenticateRequest]
// resolved it.
type Credential struct {
	// Auth is the caller's AuthContext.
	Auth *auth.AuthContext
	// FromCookie is true when the credential was the fc_session cookie, false for an
	// Authorization: Bearer token.
	FromCookie bool
	// ExpiresAt is the token's exp (the session's or the access token's); zero if none.
	ExpiresAt time.Time
}

// AuthenticateRequest resolves the platform credential of a request exactly as the
// [Authenticator] middleware does, for a caller that is not a chi middleware (the push
// gateway). present is false for a request with no bearer token and no session cookie, and
// for a stale or invalid session cookie (the browser replays it everywhere, so it counts as
// logged out). A bearer token that does not validate, or that is not an API credential, is
// an error: the caller answers 401.
func AuthenticateRequest(ctx context.Context, p *provider.Provider, r *http.Request) (cred Credential, present bool, err error) {
	token, fromCookie, oldName := extractToken(r)
	if token == "" {
		return Credential{}, false, nil
	}
	c, err := p.ValidateSessionToken(ctx, token)
	var ac *auth.AuthContext
	if err == nil && fromCookie && !SessionCookieAccepted(oldName, c) {
		err = errSessionCookieOldName
	}
	if err == nil {
		ac, err = authContextFromClaims(ctx, p, c, fromCookie)
	}
	if err != nil {
		if fromCookie {
			return Credential{}, false, nil
		}
		return Credential{}, false, err
	}
	if ac == nil {
		return Credential{}, false, nil
	}
	return Credential{Auth: ac, FromCookie: fromCookie, ExpiresAt: c.ExpiresAt}, true, nil
}

// authContextFromClaims projects validated token claims onto an AuthContext.
func authContextFromClaims(ctx context.Context, p *provider.Provider, c *sessiontoken.Claims, fromCookie bool) (*auth.AuthContext, error) {
	if c == nil {
		return nil, nil
	}

	// Each token class works only on its own transport, an ALLOWLIST per
	// transport. The cookie takes a session token only: its authority is
	// resolved from the database, so an API token (possibly narrowed to a scope
	// or an application) planted in the cookie must not inherit the principal's
	// full authority. The Bearer header takes an API access token only: a session
	// token there would skip the database resolution (and the Active check), and
	// identity / field / any future class carries no API authority at all. Reject
	// rather than build an empty-authority context, which an
	// authentication-only handler would still accept.
	if fromCookie {
		if !sessiontoken.IsSessionToken(c) {
			return nil, errNotSessionToken
		}
	} else if !sessiontoken.IsAPIBearer(c) {
		return nil, errIdentityTokenNotAPICredential
	}

	// Session cookies (the SPA) carry only identity (subject). Resolve the
	// mutable authorization data — scope, roles, clients, applications,
	// permissions — FRESH from the DB on every request, so a role/permission/
	// scope change takes effect immediately and the cookie stays tiny. A
	// deactivated or deleted principal resolves to an error → the session is
	// rejected.
	if fromCookie {
		rc, rerr := p.ResolveSessionClaims(ctx, c.Subject, c.IssuedAt)
		if rerr != nil {
			return nil, rerr
		}
		// The claim carries "id:code" pairs (or "*"); the internal model is
		// bare ids plus a flag. Split once, here at the boundary.
		apps, allApps := auth.ParseApplicationsClaim(rc.Applications)
		return &auth.AuthContext{
			PrincipalID:     rc.Subject,
			Scope:           auth.Scope(rc.Scope),
			Email:           rc.Email,
			Clients:         rc.Clients,
			Roles:           rc.Roles,
			Applications:    apps,
			AllApplications: allApps || rc.AllApplications,
			Permissions:     rc.Permissions,
			// Session cookies are minted only by interactive logins.
			PrincipalType: auth.PrincipalTypeUser,
		}, nil
	}

	// Bearer transport (OAuth access tokens minted by authservice) is
	// self-contained for stateless validation. Tokens minted with a requested
	// or full scope carry their granted permissions on the "scope" claim
	// (parsed into c.Permissions); tokens minted without one carry only roles,
	// so we derive permissions from the roles here. Either way permission-gated
	// handlers see the same set regardless of token source.
	perms := c.Permissions
	if len(perms) == 0 && len(c.Roles) > 0 {
		if derived, derr := p.FlattenPermissions(ctx, c.Roles); derr == nil {
			perms = derived
		}
	}
	// Like applications, the clients claim carries "id:identifier" pairs (or
	// "*" for an anchor); the internal model is bare ids, as on the cookie path.
	apps, allApps := auth.ParseApplicationsClaim(c.Applications)
	return &auth.AuthContext{
		PrincipalID:  c.Subject,
		Scope:        auth.Scope(c.Scope),
		Email:        c.Email,
		Clients:      auth.ParseClientsClaim(c.Clients),
		Roles:        c.Roles,
		Applications: apps,
		// The deprecated all_applications boolean is still honoured alongside
		// the "*" sentinel until consumers have moved off it.
		AllApplications: allApps || c.AllApplications,
		Permissions:     perms,
		PrincipalType:   c.PrincipalType,
	}, nil
}

// stringSlice coerces a claim into []string — kept here for any future
// adapter that needs it. Tokens we mint already arrive as []string.
func stringSlice(v any) []string {
	switch x := v.(type) {
	case []string:
		return append([]string(nil), x...)
	case []any:
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
	if err != nil && err.Error() != "" {
		desc = err.Error()
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
//	X-FC-Test-Principal-Type: USER | SERVICE (default unset)
func buildTestAuthContext(r *http.Request) *auth.AuthContext {
	scope := auth.Scope(r.Header.Get("X-FC-Test-Scope"))
	if scope == "" {
		scope = auth.ScopeClient
	}
	// Application scope. Default all-applications=true so existing tests keep
	// full app reach; X-FC-Test-All-Applications overrides explicitly, and
	// supplying X-FC-Test-Applications (an id list) flips to scoped access.
	apps := splitCSV(r.Header.Get("X-FC-Test-Applications"))
	allApps := true
	if v := r.Header.Get("X-FC-Test-All-Applications"); v != "" {
		allApps = v == "true"
	} else if len(apps) > 0 {
		allApps = false
	}
	return &auth.AuthContext{
		PrincipalID:     r.Header.Get("X-FC-Test-Principal"),
		Email:           r.Header.Get("X-FC-Test-Email"),
		Scope:           scope,
		Clients:         splitCSV(r.Header.Get("X-FC-Test-Clients")),
		Roles:           splitCSV(r.Header.Get("X-FC-Test-Roles")),
		Applications:    apps,
		AllApplications: allApps,
		Permissions:     splitCSV(r.Header.Get("X-FC-Test-Permissions")),
		// Unset (the default) = unknown type, exempt from the profile-only
		// gate; tests of that gate send X-FC-Test-Principal-Type: USER.
		PrincipalType: r.Header.Get("X-FC-Test-Principal-Type"),
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
