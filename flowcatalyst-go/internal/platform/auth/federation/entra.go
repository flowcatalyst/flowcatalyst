package federation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// EntraAdapter is an adapter for Microsoft Entra ID (Azure AD)
type EntraAdapter struct {
	*OIDCAdapter
	tenantID          string
	allowedTenants    []string // For multi-tenant apps, list of allowed tenant IDs
	allowAnyTenant    bool     // For multi-tenant apps, allow any tenant
}

// EntraConfig extends Config with Entra-specific options
type EntraConfig struct {
	*Config
	TenantID       string   // Single tenant ID or "common"/"organizations"/"consumers"
	AllowedTenants []string // For multi-tenant, specific allowed tenants
	AllowAnyTenant bool     // Allow any tenant (use with caution)
}

// NewEntraAdapter creates a new Azure Entra adapter
func NewEntraAdapter(config *EntraConfig) (*EntraAdapter, error) {
	if config.TenantID == "" {
		return nil, fmt.Errorf("tenant ID is required for Entra adapter")
	}

	// Build issuer URL if not provided
	if config.Config.IssuerURL == "" {
		config.Config.IssuerURL = fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0", config.TenantID)
	}

	// Entra uses 'groups' claim for group memberships
	if config.Config.GroupsClaim == "" {
		config.Config.GroupsClaim = "groups"
	}

	// Entra uses 'roles' claim for app roles
	if config.Config.RolesClaim == "" {
		config.Config.RolesClaim = "roles"
	}

	// Default scopes for Entra
	if len(config.Config.Scopes) == 0 {
		config.Config.Scopes = []string{"openid", "profile", "email"}
	}

	baseAdapter, err := NewOIDCAdapter(config.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Entra adapter: %w", err)
	}

	return &EntraAdapter{
		OIDCAdapter:    baseAdapter,
		tenantID:       config.TenantID,
		allowedTenants: config.AllowedTenants,
		allowAnyTenant: config.AllowAnyTenant,
	}, nil
}

// Type returns the IDP type
func (a *EntraAdapter) Type() IdpType {
	return IdpTypeEntra
}

// ValidateIDToken validates an ID token with Entra-specific handling
func (a *EntraAdapter) ValidateIDToken(ctx context.Context, idToken, nonce string) (*UserInfo, error) {
	// For multi-tenant scenarios, we need custom issuer validation
	// Parse token first to get tenant ID
	claims, err := parseTokenClaims(idToken)
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	// Extract tenant ID from token
	tokenTenantID := extractEntraTenantID(claims)
	if tokenTenantID == "" {
		return nil, fmt.Errorf("tenant ID not found in token")
	}

	// Validate tenant if we're in multi-tenant mode
	if !a.isAllowedTenant(tokenTenantID) {
		return nil, fmt.Errorf("tenant %s is not allowed", tokenTenantID)
	}

	// Use base validation - it handles issuer validation
	userInfo, err := a.OIDCAdapter.ValidateIDToken(ctx, idToken, nonce)
	if err != nil {
		// For multi-tenant apps using "common" or "organizations",
		// the issuer in the token will be tenant-specific
		if a.isMultiTenant() && isIssuerMismatchError(err) {
			// Re-validate with tenant-specific issuer
			userInfo, err = a.validateMultiTenantToken(ctx, idToken, nonce, tokenTenantID)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	// Enrich with Entra-specific claims
	userInfo = a.enrichWithEntraClaims(userInfo, claims)
	userInfo.TenantID = tokenTenantID

	return userInfo, nil
}

// isAllowedTenant checks if a tenant is allowed
func (a *EntraAdapter) isAllowedTenant(tenantID string) bool {
	// If we're in single-tenant mode
	if !a.isMultiTenant() {
		return tenantID == a.tenantID
	}

	// If any tenant is allowed
	if a.allowAnyTenant {
		return true
	}

	// Check allowed list
	for _, allowed := range a.allowedTenants {
		if allowed == tenantID {
			return true
		}
	}

	return false
}

// isMultiTenant returns true if this is a multi-tenant configuration
func (a *EntraAdapter) isMultiTenant() bool {
	return a.tenantID == "common" || a.tenantID == "organizations" || a.tenantID == "consumers"
}

// validateMultiTenantToken validates a token from a multi-tenant app
func (a *EntraAdapter) validateMultiTenantToken(ctx context.Context, idToken, nonce, tenantID string) (*UserInfo, error) {
	// Create a temporary adapter with the correct tenant-specific issuer
	tempConfig := *a.config
	tempConfig.IssuerURL = fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0", tenantID)

	tempAdapter, err := NewOIDCAdapter(&tempConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create tenant-specific adapter: %w", err)
	}

	return tempAdapter.ValidateIDToken(ctx, idToken, nonce)
}

// enrichWithEntraClaims adds Entra-specific claims to user info
func (a *EntraAdapter) enrichWithEntraClaims(userInfo *UserInfo, claims map[string]interface{}) *UserInfo {
	// Extract UPN (User Principal Name) - often more useful than email
	if upn, ok := claims["upn"].(string); ok {
		if userInfo.Email == "" {
			userInfo.Email = upn
		}
		userInfo.Claims["upn"] = upn
	}

	// Preferred username
	if preferredUsername, ok := claims["preferred_username"].(string); ok {
		if userInfo.Email == "" {
			userInfo.Email = preferredUsername
		}
		userInfo.Claims["preferred_username"] = preferredUsername
	}

	// Object ID (unique identifier in Entra)
	if oid, ok := claims["oid"].(string); ok {
		userInfo.Claims["oid"] = oid
	}

	// App roles (from 'roles' claim)
	if roles, ok := claims["roles"].([]interface{}); ok {
		for _, r := range roles {
			if role, ok := r.(string); ok {
				userInfo.Roles = appendUnique(userInfo.Roles, role)
			}
		}
	}

	// Security groups (from 'groups' claim)
	// Note: For large numbers of groups, Entra uses a groups overage claim
	if groups, ok := claims["groups"].([]interface{}); ok {
		for _, g := range groups {
			if group, ok := g.(string); ok {
				userInfo.Groups = appendUnique(userInfo.Groups, group)
			}
		}
	}

	// Handle groups overage - when there are too many groups, Entra provides a link
	if hasGroupsOverage, ok := claims["_claim_names"].(map[string]interface{}); ok {
		if _, hasGroups := hasGroupsOverage["groups"]; hasGroups {
			// Groups overage: need to call Graph API to get groups
			// This would require additional implementation
			userInfo.Claims["groups_overage"] = "true"
		}
	}

	// wids claim contains directory roles
	if wids, ok := claims["wids"].([]interface{}); ok {
		for _, w := range wids {
			if wid, ok := w.(string); ok {
				userInfo.Roles = appendUnique(userInfo.Roles, "directory:"+wid)
			}
		}
	}

	return userInfo
}

// extractEntraTenantID extracts the tenant ID from token claims
func extractEntraTenantID(claims map[string]interface{}) string {
	// Try 'tid' claim first (tenant ID)
	if tid, ok := claims["tid"].(string); ok {
		return tid
	}

	// Try extracting from issuer
	if iss, ok := claims["iss"].(string); ok {
		// Issuer format: https://login.microsoftonline.com/{tenant}/v2.0
		parts := strings.Split(iss, "/")
		for i, part := range parts {
			if part == "login.microsoftonline.com" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}

	return ""
}

// isIssuerMismatchError checks if an error is due to issuer mismatch
func isIssuerMismatchError(err error) bool {
	return err == ErrInvalidIssuer || strings.Contains(err.Error(), "issuer")
}

// decodeBase64URL decodes a base64url encoded string
func decodeBase64URL(s string) ([]byte, error) {
	// Add padding if needed
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

// jsonUnmarshal is a wrapper for json.Unmarshal
func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
