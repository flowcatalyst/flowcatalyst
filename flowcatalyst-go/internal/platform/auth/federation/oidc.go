package federation

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"log/slog"
)

var (
	ErrInvalidToken      = errors.New("invalid token")
	ErrTokenExpired      = errors.New("token expired")
	ErrInvalidIssuer     = errors.New("invalid issuer")
	ErrInvalidAudience   = errors.New("invalid audience")
	ErrInvalidNonce      = errors.New("nonce mismatch")
	ErrDiscoveryFailed   = errors.New("failed to fetch discovery document")
	ErrExchangeFailed    = errors.New("token exchange failed")
	ErrUserInfoFailed    = errors.New("failed to fetch user info")
	ErrJWKSFetchFailed   = errors.New("failed to fetch JWKS")
)

// OIDCAdapter provides a base implementation for OIDC-compliant IDPs
type OIDCAdapter struct {
	config    *Config
	discovery *DiscoveryDocument
	jwks      map[string]interface{} // kid -> public key
	mu        sync.RWMutex
	client    *http.Client

	// Discovery cache
	discoveryExpiresAt time.Time
	jwksExpiresAt      time.Time
}

// NewOIDCAdapter creates a new generic OIDC adapter
func NewOIDCAdapter(config *Config) (*OIDCAdapter, error) {
	adapter := &OIDCAdapter{
		config: config,
		jwks:   make(map[string]interface{}),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Fetch discovery document
	if err := adapter.refreshDiscovery(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize OIDC adapter: %w", err)
	}

	return adapter, nil
}

// Type returns the IDP type
func (a *OIDCAdapter) Type() IdpType {
	return IdpTypeOIDC
}

// GetAuthorizationURL returns the URL to redirect the user for authentication
func (a *OIDCAdapter) GetAuthorizationURL(ctx context.Context, req *AuthRequest) (string, error) {
	if err := a.ensureDiscovery(ctx); err != nil {
		return "", err
	}

	u, err := url.Parse(a.discovery.AuthorizationEndpoint)
	if err != nil {
		return "", fmt.Errorf("invalid authorization endpoint: %w", err)
	}

	q := u.Query()
	q.Set("client_id", a.config.ClientID)
	q.Set("redirect_uri", req.RedirectURI)
	q.Set("response_type", "code")
	q.Set("state", req.State)

	// Build scope
	scopes := a.config.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}
	q.Set("scope", strings.Join(scopes, " "))

	// Add nonce for OIDC
	if req.Nonce != "" {
		q.Set("nonce", req.Nonce)
	}

	// Add PKCE if code verifier is provided
	if req.CodeVerifier != "" {
		challenge := generateCodeChallenge(req.CodeVerifier)
		q.Set("code_challenge", challenge)
		q.Set("code_challenge_method", "S256")
	}

	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ExchangeCode exchanges an authorization code for tokens
func (a *OIDCAdapter) ExchangeCode(ctx context.Context, code, redirectURI, codeVerifier string) (*TokenSet, error) {
	if err := a.ensureDiscovery(ctx); err != nil {
		return nil, err
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", a.config.ClientID)

	if a.config.ClientSecret != "" {
		data.Set("client_secret", a.config.ClientSecret)
	}

	if codeVerifier != "" {
		data.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", a.discovery.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrExchangeFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("Token exchange failed", "status", resp.StatusCode, "body", string(body))
		return nil, fmt.Errorf("%w: status %d", ErrExchangeFailed, resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &TokenSet{
		AccessToken:  tokenResp.AccessToken,
		IDToken:      tokenResp.IDToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scope:        tokenResp.Scope,
	}, nil
}

// ValidateIDToken validates an ID token and returns user info
func (a *OIDCAdapter) ValidateIDToken(ctx context.Context, idToken, nonce string) (*UserInfo, error) {
	if err := a.ensureJWKS(ctx); err != nil {
		return nil, err
	}

	// Parse token without validation first to get the key ID
	token, _, err := jwt.NewParser().ParseUnverified(idToken, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	// Get the key ID from the token header
	kid, ok := token.Header["kid"].(string)
	if !ok {
		return nil, fmt.Errorf("%w: missing kid in token header", ErrInvalidToken)
	}

	// Get the public key
	a.mu.RLock()
	key, exists := a.jwks[kid]
	a.mu.RUnlock()

	if !exists {
		// Try refreshing JWKS
		if err := a.refreshJWKS(ctx); err != nil {
			return nil, err
		}
		a.mu.RLock()
		key, exists = a.jwks[kid]
		a.mu.RUnlock()
		if !exists {
			return nil, fmt.Errorf("%w: unknown key id: %s", ErrInvalidToken, kid)
		}
	}

	// Parse and validate the token
	claims := jwt.MapClaims{}
	token, err = jwt.ParseWithClaims(idToken, claims, func(t *jwt.Token) (interface{}, error) {
		return key, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	// Validate issuer
	iss, _ := claims["iss"].(string)
	if !a.isValidIssuer(iss) {
		return nil, ErrInvalidIssuer
	}

	// Validate audience
	aud := getAudience(claims)
	if !contains(aud, a.config.ClientID) {
		return nil, ErrInvalidAudience
	}

	// Validate nonce if provided
	if nonce != "" && !a.config.SkipNonceCheck {
		tokenNonce, _ := claims["nonce"].(string)
		if tokenNonce != nonce {
			return nil, ErrInvalidNonce
		}
	}

	// Extract user info from claims
	return a.extractUserInfo(claims), nil
}

// GetUserInfo fetches user info from the userinfo endpoint
func (a *OIDCAdapter) GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	if err := a.ensureDiscovery(ctx); err != nil {
		return nil, err
	}

	if a.discovery.UserInfoEndpoint == "" {
		return nil, errors.New("userinfo endpoint not available")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", a.discovery.UserInfoEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create userinfo request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUserInfoFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d, body: %s", ErrUserInfoFailed, resp.StatusCode, string(body))
	}

	var claims map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return nil, fmt.Errorf("failed to decode userinfo response: %w", err)
	}

	return a.extractUserInfoFromMap(claims), nil
}

// RefreshTokens refreshes an access token using a refresh token
func (a *OIDCAdapter) RefreshTokens(ctx context.Context, refreshToken string) (*TokenSet, error) {
	if err := a.ensureDiscovery(ctx); err != nil {
		return nil, err
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", a.config.ClientID)

	if a.config.ClientSecret != "" {
		data.Set("client_secret", a.config.ClientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", a.discovery.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("refresh failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode refresh response: %w", err)
	}

	return &TokenSet{
		AccessToken:  tokenResp.AccessToken,
		IDToken:      tokenResp.IDToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scope:        tokenResp.Scope,
	}, nil
}

// === Internal methods ===

func (a *OIDCAdapter) ensureDiscovery(ctx context.Context) error {
	a.mu.RLock()
	valid := a.discovery != nil && time.Now().Before(a.discoveryExpiresAt)
	a.mu.RUnlock()

	if valid {
		return nil
	}

	return a.refreshDiscovery(ctx)
}

func (a *OIDCAdapter) refreshDiscovery(ctx context.Context) error {
	discoveryURL := strings.TrimSuffix(a.config.IssuerURL, "/") + "/.well-known/openid-configuration"

	req, err := http.NewRequestWithContext(ctx, "GET", discoveryURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create discovery request: %w", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDiscoveryFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: status %d, body: %s", ErrDiscoveryFailed, resp.StatusCode, string(body))
	}

	var doc DiscoveryDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("failed to decode discovery document: %w", err)
	}

	a.mu.Lock()
	a.discovery = &doc
	a.discoveryExpiresAt = time.Now().Add(1 * time.Hour)
	a.mu.Unlock()

	return nil
}

func (a *OIDCAdapter) ensureJWKS(ctx context.Context) error {
	a.mu.RLock()
	valid := len(a.jwks) > 0 && time.Now().Before(a.jwksExpiresAt)
	a.mu.RUnlock()

	if valid {
		return nil
	}

	return a.refreshJWKS(ctx)
}

func (a *OIDCAdapter) refreshJWKS(ctx context.Context) error {
	if err := a.ensureDiscovery(ctx); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", a.discovery.JwksURI, nil)
	if err != nil {
		return fmt.Errorf("failed to create JWKS request: %w", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrJWKSFetchFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrJWKSFetchFailed, resp.StatusCode)
	}

	var jwks struct {
		Keys []map[string]interface{} `json:"keys"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("failed to decode JWKS: %w", err)
	}

	newJWKS := make(map[string]interface{})
	for _, key := range jwks.Keys {
		kid, _ := key["kid"].(string)
		if kid == "" {
			continue
		}

		// Parse the JWK into a public key
		pubKey, err := parseJWK(key)
		if err != nil {
			slog.Warn("Failed to parse JWK", "kid", kid, "error", err)
			continue
		}
		newJWKS[kid] = pubKey
	}

	a.mu.Lock()
	a.jwks = newJWKS
	a.jwksExpiresAt = time.Now().Add(1 * time.Hour)
	a.mu.Unlock()

	return nil
}

func (a *OIDCAdapter) isValidIssuer(iss string) bool {
	// Normalize and compare
	expected := strings.TrimSuffix(a.config.IssuerURL, "/")
	actual := strings.TrimSuffix(iss, "/")
	return actual == expected
}

func (a *OIDCAdapter) extractUserInfo(claims jwt.MapClaims) *UserInfo {
	return a.extractUserInfoFromMap(map[string]interface{}(claims))
}

func (a *OIDCAdapter) extractUserInfoFromMap(claims map[string]interface{}) *UserInfo {
	info := &UserInfo{
		Claims: make(map[string]string),
	}

	// Standard claims
	info.Subject, _ = claims["sub"].(string)
	info.Email, _ = claims["email"].(string)
	info.EmailVerified, _ = claims["email_verified"].(bool)
	info.Name, _ = claims["name"].(string)
	info.GivenName, _ = claims["given_name"].(string)
	info.FamilyName, _ = claims["family_name"].(string)
	info.Picture, _ = claims["picture"].(string)

	// Extract groups from configured claim
	groupsClaim := a.config.GroupsClaim
	if groupsClaim == "" {
		groupsClaim = "groups"
	}
	if groups, ok := claims[groupsClaim].([]interface{}); ok {
		for _, g := range groups {
			if s, ok := g.(string); ok {
				info.Groups = append(info.Groups, s)
			}
		}
	}

	// Extract roles from configured claim
	rolesClaim := a.config.RolesClaim
	if rolesClaim == "" {
		rolesClaim = "roles"
	}
	if roles, ok := claims[rolesClaim].([]interface{}); ok {
		for _, r := range roles {
			if s, ok := r.(string); ok {
				info.Roles = append(info.Roles, s)
			}
		}
	}

	return info
}

// === Utility functions ===

func generateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func getAudience(claims jwt.MapClaims) []string {
	switch aud := claims["aud"].(type) {
	case string:
		return []string{aud}
	case []interface{}:
		result := make([]string, 0, len(aud))
		for _, a := range aud {
			if s, ok := a.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
