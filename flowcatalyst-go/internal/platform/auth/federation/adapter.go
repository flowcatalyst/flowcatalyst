package federation

import (
	"context"
	"time"
)

// IdpType represents the type of identity provider
type IdpType string

const (
	IdpTypeKeycloak IdpType = "KEYCLOAK"
	IdpTypeEntra    IdpType = "ENTRA"
	IdpTypeOIDC     IdpType = "OIDC" // Generic OIDC
)

// UserInfo represents user information from an upstream IDP
type UserInfo struct {
	Subject       string            // "sub" claim - unique identifier from IDP
	Email         string            // User's email
	EmailVerified bool              // Whether email is verified
	Name          string            // Full name
	GivenName     string            // First name
	FamilyName    string            // Last name
	Picture       string            // Profile picture URL
	Groups        []string          // Group memberships (for role mapping)
	Roles         []string          // Role claims (if provided by IDP)
	TenantID      string            // For multi-tenant IDPs like Entra
	Claims        map[string]string // Additional claims
}

// TokenSet represents tokens received from an upstream IDP
type TokenSet struct {
	AccessToken  string
	IDToken      string
	RefreshToken string
	TokenType    string
	ExpiresIn    int64
	ExpiresAt    time.Time
	Scope        string
}

// AuthRequest represents an authorization request to send to an upstream IDP
type AuthRequest struct {
	ClientID     string
	RedirectURI  string
	State        string
	Nonce        string
	Scope        string
	CodeVerifier string // For PKCE
}

// AuthResponse represents the response from an authorization callback
type AuthResponse struct {
	Code  string
	State string
	Error string
}

// Adapter defines the interface for upstream identity provider adapters
type Adapter interface {
	// Type returns the IDP type
	Type() IdpType

	// GetAuthorizationURL returns the URL to redirect the user for authentication
	GetAuthorizationURL(ctx context.Context, req *AuthRequest) (string, error)

	// ExchangeCode exchanges an authorization code for tokens
	ExchangeCode(ctx context.Context, code, redirectURI, codeVerifier string) (*TokenSet, error)

	// ValidateIDToken validates an ID token and returns user info
	ValidateIDToken(ctx context.Context, idToken, nonce string) (*UserInfo, error)

	// GetUserInfo fetches user info from the userinfo endpoint
	GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error)

	// RefreshTokens refreshes an access token using a refresh token
	RefreshTokens(ctx context.Context, refreshToken string) (*TokenSet, error)
}

// Config holds common configuration for IDP adapters
type Config struct {
	IssuerURL     string
	ClientID      string
	ClientSecret  string // Reference to secret provider (e.g., "env://MY_SECRET")
	Scopes        []string
	RedirectURI   string
	TenantID      string // For Entra
	CustomClaims  map[string]string
	GroupsClaim   string // Claim containing group memberships
	RolesClaim    string // Claim containing role assignments
	SkipNonceCheck bool  // For development only
}

// DiscoveryDocument represents the OIDC discovery document
type DiscoveryDocument struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	UserInfoEndpoint                  string   `json:"userinfo_endpoint"`
	JwksURI                           string   `json:"jwks_uri"`
	EndSessionEndpoint                string   `json:"end_session_endpoint,omitempty"`
	ScopesSupported                   []string `json:"scopes_supported"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	ClaimsSupported                   []string `json:"claims_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported,omitempty"`
}
