package oidc

import (
	"time"
)

// OAuthClientType defines the type of OAuth client
type OAuthClientType string

const (
	OAuthClientTypePublic       OAuthClientType = "PUBLIC"       // SPA, mobile apps
	OAuthClientTypeConfidential OAuthClientType = "CONFIDENTIAL" // Backend services
)

// OAuthClient represents an OAuth2 client registration
// Collection: oauth_clients
type OAuthClient struct {
	ID                        string          `bson:"_id" json:"id"`
	ClientID                  string          `bson:"clientId" json:"clientId"` // Public client identifier
	ClientName                string          `bson:"clientName" json:"clientName"`
	ClientType                OAuthClientType `bson:"clientType" json:"clientType"`
	ClientSecretRef           string          `bson:"clientSecretRef,omitempty" json:"-"` // Secret reference, never expose
	RedirectURIs              []string        `bson:"redirectUris" json:"redirectUris"`
	GrantTypes                []string        `bson:"grantTypes" json:"grantTypes"`
	DefaultScopes             []string        `bson:"defaultScopes,omitempty" json:"defaultScopes,omitempty"`
	PKCERequired              bool            `bson:"pkceRequired" json:"pkceRequired"`
	ApplicationIDs            []string        `bson:"applicationIds,omitempty" json:"applicationIds,omitempty"`
	ServiceAccountPrincipalID string          `bson:"serviceAccountPrincipalId,omitempty" json:"serviceAccountPrincipalId,omitempty"`
	Active                    bool            `bson:"active" json:"active"`
	CreatedAt                 time.Time       `bson:"createdAt" json:"createdAt"`
	UpdatedAt                 time.Time       `bson:"updatedAt" json:"updatedAt"`
}

// IsPublic returns true if this is a public client
func (c *OAuthClient) IsPublic() bool {
	return c.ClientType == OAuthClientTypePublic
}

// IsConfidential returns true if this is a confidential client
func (c *OAuthClient) IsConfidential() bool {
	return c.ClientType == OAuthClientTypeConfidential
}

// HasGrantType checks if the client supports a grant type
func (c *OAuthClient) HasGrantType(grantType string) bool {
	for _, gt := range c.GrantTypes {
		if gt == grantType {
			return true
		}
	}
	return false
}

// HasRedirectURI checks if a redirect URI is allowed
func (c *OAuthClient) HasRedirectURI(uri string) bool {
	for _, ru := range c.RedirectURIs {
		if ru == uri {
			return true
		}
	}
	return false
}

// HasApplicationRestrictions returns true if this client is restricted to specific applications
func (c *OAuthClient) HasApplicationRestrictions() bool {
	return len(c.ApplicationIDs) > 0
}

// HasApplicationAccess checks if access to a specific application is allowed
func (c *OAuthClient) HasApplicationAccess(applicationID string) bool {
	if !c.HasApplicationRestrictions() {
		return true // No restrictions means access to all
	}
	for _, id := range c.ApplicationIDs {
		if id == applicationID {
			return true
		}
	}
	return false
}

// AuthorizationCode represents an OAuth2 authorization code
// Collection: authorization_codes
type AuthorizationCode struct {
	Code                string    `bson:"_id" json:"code"` // Use code as ID for fast lookup
	ClientID            string    `bson:"clientId" json:"clientId"`
	PrincipalID         string    `bson:"principalId" json:"principalId"`
	RedirectURI         string    `bson:"redirectUri" json:"redirectUri"`
	Scope               string    `bson:"scope,omitempty" json:"scope,omitempty"`
	CodeChallenge       string    `bson:"codeChallenge,omitempty" json:"codeChallenge,omitempty"`
	CodeChallengeMethod string    `bson:"codeChallengeMethod,omitempty" json:"codeChallengeMethod,omitempty"`
	Nonce               string    `bson:"nonce,omitempty" json:"nonce,omitempty"`
	State               string    `bson:"state,omitempty" json:"state,omitempty"`
	ContextClientID     string    `bson:"contextClientId,omitempty" json:"contextClientId,omitempty"`
	CreatedAt           time.Time `bson:"createdAt" json:"createdAt"`
	ExpiresAt           time.Time `bson:"expiresAt" json:"expiresAt"`
	Used                bool      `bson:"used" json:"used"`
}

// IsExpired returns true if the authorization code has expired
func (c *AuthorizationCode) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// HasPKCE returns true if PKCE was used
func (c *AuthorizationCode) HasPKCE() bool {
	return c.CodeChallenge != ""
}

// RefreshToken represents an OAuth2 refresh token
// Collection: refresh_tokens
type RefreshToken struct {
	TokenHash       string    `bson:"_id" json:"-"`    // SHA-256 hash of token, used as ID
	PrincipalID     string    `bson:"principalId" json:"principalId"`
	ClientID        string    `bson:"clientId" json:"clientId"`
	ContextClientID string    `bson:"contextClientId,omitempty" json:"contextClientId,omitempty"`
	Scope           string    `bson:"scope,omitempty" json:"scope,omitempty"`
	TokenFamily     string    `bson:"tokenFamily" json:"tokenFamily"` // For rotation tracking
	CreatedAt       time.Time `bson:"createdAt" json:"createdAt"`
	ExpiresAt       time.Time `bson:"expiresAt" json:"expiresAt"`
	Revoked         bool      `bson:"revoked" json:"revoked"`
	RevokedAt       time.Time `bson:"revokedAt,omitempty" json:"revokedAt,omitempty"`
	ReplacedBy      string    `bson:"replacedBy,omitempty" json:"replacedBy,omitempty"` // Hash of replacement token
}

// IsExpired returns true if the refresh token has expired
func (t *RefreshToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// IsValid returns true if the token is not expired and not revoked
func (t *RefreshToken) IsValid() bool {
	return !t.Revoked && !t.IsExpired()
}

// OIDCLoginState stores state for OIDC login flow
// Collection: oidc_login_state
type OIDCLoginState struct {
	State                    string    `bson:"_id" json:"state"` // Random state value, used as ID
	Domain                   string    `bson:"domain" json:"domain"` // Email domain for IDP lookup
	ClientID                 string    `bson:"clientId" json:"clientId"` // FlowCatalyst client ID
	EmailDomain              string    `bson:"emailDomain" json:"emailDomain"`
	AuthConfigID             string    `bson:"authConfigId" json:"authConfigId"`
	Nonce                    string    `bson:"nonce" json:"nonce"`
	CodeVerifier             string    `bson:"codeVerifier" json:"codeVerifier"` // PKCE verifier
	ReturnURL                string    `bson:"returnUrl,omitempty" json:"returnUrl,omitempty"`
	OAuthClientID            string    `bson:"oauthClientId,omitempty" json:"oauthClientId,omitempty"`
	OAuthRedirectURI         string    `bson:"oauthRedirectUri,omitempty" json:"oauthRedirectUri,omitempty"`
	OAuthScope               string    `bson:"oauthScope,omitempty" json:"oauthScope,omitempty"`
	OAuthState               string    `bson:"oauthState,omitempty" json:"oauthState,omitempty"`
	OAuthCodeChallenge       string    `bson:"oauthCodeChallenge,omitempty" json:"oauthCodeChallenge,omitempty"`
	OAuthCodeChallengeMethod string    `bson:"oauthCodeChallengeMethod,omitempty" json:"oauthCodeChallengeMethod,omitempty"`
	OAuthNonce               string    `bson:"oauthNonce,omitempty" json:"oauthNonce,omitempty"`
	CreatedAt                time.Time `bson:"createdAt" json:"createdAt"`
	ExpiresAt                time.Time `bson:"expiresAt" json:"expiresAt"`
}

// IsExpired returns true if the login state has expired
func (s *OIDCLoginState) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// IsPartOfOAuthFlow returns true if this is part of an OAuth authorization flow
func (s *OIDCLoginState) IsPartOfOAuthFlow() bool {
	return s.OAuthClientID != ""
}
