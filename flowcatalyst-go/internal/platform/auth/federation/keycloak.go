package federation

import (
	"context"
	"fmt"
	"strings"
)

// KeycloakAdapter is an adapter for Keycloak identity provider
type KeycloakAdapter struct {
	*OIDCAdapter
	realm string
}

// NewKeycloakAdapter creates a new Keycloak adapter
func NewKeycloakAdapter(config *Config) (*KeycloakAdapter, error) {
	// Extract realm from issuer URL if not explicitly set
	// Keycloak issuer URLs are typically: https://keycloak.example.com/realms/my-realm
	realm := extractKeycloakRealm(config.IssuerURL)

	// Ensure we have the groups claim set for Keycloak
	if config.GroupsClaim == "" {
		config.GroupsClaim = "groups"
	}

	// Keycloak often puts roles in realm_access.roles or resource_access.<client>.roles
	// We'll handle this in the user info extraction

	baseAdapter, err := NewOIDCAdapter(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Keycloak adapter: %w", err)
	}

	return &KeycloakAdapter{
		OIDCAdapter: baseAdapter,
		realm:       realm,
	}, nil
}

// Type returns the IDP type
func (a *KeycloakAdapter) Type() IdpType {
	return IdpTypeKeycloak
}

// ValidateIDToken validates an ID token with Keycloak-specific handling
func (a *KeycloakAdapter) ValidateIDToken(ctx context.Context, idToken, nonce string) (*UserInfo, error) {
	// Use base validation
	userInfo, err := a.OIDCAdapter.ValidateIDToken(ctx, idToken, nonce)
	if err != nil {
		return nil, err
	}

	// Keycloak-specific: extract roles from realm_access and resource_access
	// We need to re-parse the token to get these nested claims
	userInfo = a.enrichWithKeycloakRoles(userInfo, idToken)

	return userInfo, nil
}

// GetUserInfo fetches user info with Keycloak-specific handling
func (a *KeycloakAdapter) GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	userInfo, err := a.OIDCAdapter.GetUserInfo(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	// Enrich with roles from access token if needed
	return userInfo, nil
}

// enrichWithKeycloakRoles extracts roles from Keycloak-specific claims
func (a *KeycloakAdapter) enrichWithKeycloakRoles(userInfo *UserInfo, idToken string) *UserInfo {
	// Parse the token to get Keycloak-specific claims
	// Note: Token is already validated, so we just parse for claims
	claims, err := parseTokenClaims(idToken)
	if err != nil {
		return userInfo
	}

	// Extract realm roles from realm_access.roles
	if realmAccess, ok := claims["realm_access"].(map[string]interface{}); ok {
		if roles, ok := realmAccess["roles"].([]interface{}); ok {
			for _, r := range roles {
				if role, ok := r.(string); ok {
					userInfo.Roles = appendUnique(userInfo.Roles, role)
				}
			}
		}
	}

	// Extract resource/client roles from resource_access.<client_id>.roles
	if resourceAccess, ok := claims["resource_access"].(map[string]interface{}); ok {
		for clientID, access := range resourceAccess {
			if accessMap, ok := access.(map[string]interface{}); ok {
				if roles, ok := accessMap["roles"].([]interface{}); ok {
					for _, r := range roles {
						if role, ok := r.(string); ok {
							// Prefix with client ID for clarity
							prefixedRole := clientID + ":" + role
							userInfo.Roles = appendUnique(userInfo.Roles, prefixedRole)
						}
					}
				}
			}
		}
	}

	// Extract preferred_username if name is empty
	if userInfo.Name == "" {
		if preferredUsername, ok := claims["preferred_username"].(string); ok {
			userInfo.Name = preferredUsername
		}
	}

	return userInfo
}

// extractKeycloakRealm extracts the realm from a Keycloak issuer URL
func extractKeycloakRealm(issuerURL string) string {
	// Keycloak issuer URLs are: https://host/realms/realm-name
	parts := strings.Split(issuerURL, "/realms/")
	if len(parts) == 2 {
		// Remove any trailing path
		realm := strings.Split(parts[1], "/")[0]
		return realm
	}
	return ""
}

// parseTokenClaims parses JWT claims without validation (for already-validated tokens)
func parseTokenClaims(tokenString string) (map[string]interface{}, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Decode payload (second part)
	payload, err := decodeBase64URL(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	var claims map[string]interface{}
	if err := jsonUnmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	return claims, nil
}

// appendUnique appends a string to a slice if not already present
func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}
