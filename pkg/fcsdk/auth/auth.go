// Package auth provides authentication primitives for FlowCatalyst SDK
// applications: JWT validation (RS256 via JWKS, or HS256 via a shared
// secret), the OAuth2 authorization-code flow with PKCE, and a
// client_credentials TokenProvider that plugs into client.WithTokenProvider.
//
// Mirrors crates/fc-sdk/src/auth/ in shape and wire format. A token
// minted by FlowCatalyst validates identically from any of the four
// SDKs (Rust, TypeScript, Laravel, Go).
//
// # Resource server (token validation)
//
//	v := auth.NewTokenValidator(auth.TokenValidatorConfig{
//	    IssuerURL: "https://auth.flowcatalyst.io",
//	    Audience:  "my-app",
//	})
//	ctx, err := v.ValidateBearer(r.Context(), r.Header.Get("Authorization"))
//	if err != nil { /* 401 */ }
//	if ctx.HasRole("admin") && ctx.HasClientAccess("clt_123") { /* allow */ }
//
// # OAuth2 authorization code flow (web app)
//
//	oauth := auth.NewOAuthClient(auth.OAuthConfig{
//	    IssuerURL:    "https://auth.flowcatalyst.io",
//	    ClientID:     "my-app",
//	    ClientSecret: "secret",
//	    RedirectURI:  "https://myapp.example.com/callback",
//	})
//	url, params := oauth.AuthorizeURL()
//	// ... redirect, store params in session, then on callback:
//	tokens, err := oauth.ExchangeCode(ctx, code, params.PKCE.CodeVerifier)
//
// # Service-to-service (client_credentials)
//
//	tp := auth.NewClientCredentialsProvider(auth.ClientCredentialsConfig{
//	    IssuerURL:    "https://auth.flowcatalyst.io",
//	    ClientID:     "svc-app",
//	    ClientSecret: "...",
//	})
//	c := client.New("https://api.flowcatalyst.io", client.WithTokenProvider(tp.Token))
package auth

import "errors"

// Error categorises auth failures. Use errors.Is on the sentinels below.
type Error struct {
	Kind    ErrorKind
	Message string
}

// ErrorKind enumerates auth error categories. Mirrors the Rust SDK's
// AuthError enum variants.
type ErrorKind int

const (
	// KindTokenExpired — JWT exp has passed.
	KindTokenExpired ErrorKind = iota
	// KindInvalidToken — bad signature, wrong issuer/audience, malformed.
	KindInvalidToken
	// KindDiscovery — OIDC discovery or JWKS fetch failed.
	KindDiscovery
	// KindTokenExchange — /oauth/token or related endpoint failed.
	KindTokenExchange
	// KindConfig — bad configuration (missing required field, bad key length).
	KindConfig
	// KindCrypto — cryptographic operation failed.
	KindCrypto
)

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	switch e.Kind {
	case KindTokenExpired:
		return "token has expired"
	case KindInvalidToken:
		return "invalid token: " + e.Message
	case KindDiscovery:
		return "discovery error: " + e.Message
	case KindTokenExchange:
		return "token exchange error: " + e.Message
	case KindConfig:
		return "config error: " + e.Message
	case KindCrypto:
		return "crypto error: " + e.Message
	default:
		return "auth error: " + e.Message
	}
}

// Sentinel errors. Use errors.Is to branch on them.
var (
	ErrTokenExpired  = &Error{Kind: KindTokenExpired}
	ErrInvalidToken  = &Error{Kind: KindInvalidToken}
	ErrDiscovery     = &Error{Kind: KindDiscovery}
	ErrTokenExchange = &Error{Kind: KindTokenExchange}
	ErrConfig        = &Error{Kind: KindConfig}
	ErrCrypto        = &Error{Kind: KindCrypto}
)

// Is matches by Kind so errors.Is(err, ErrTokenExpired) works regardless
// of the wrapped message.
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) {
		return false
	}
	return e.Kind == t.Kind
}

func newErr(kind ErrorKind, msg string) *Error {
	return &Error{Kind: kind, Message: msg}
}
