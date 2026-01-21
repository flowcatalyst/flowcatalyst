package client

import (
	"time"
)

// ClientStatus defines the status of a client
type ClientStatus string

const (
	ClientStatusActive    ClientStatus = "ACTIVE"
	ClientStatusInactive  ClientStatus = "INACTIVE"
	ClientStatusSuspended ClientStatus = "SUSPENDED"
)

// Client represents a tenant/organization in the platform
// Collection: auth_clients
type Client struct {
	ID              string       `bson:"_id" json:"id"`
	Name            string       `bson:"name" json:"name"`
	Identifier      string       `bson:"identifier" json:"identifier"` // Unique human-readable identifier
	Status          ClientStatus `bson:"status" json:"status"`
	StatusReason    string       `bson:"statusReason,omitempty" json:"statusReason,omitempty"`
	StatusChangedAt time.Time    `bson:"statusChangedAt,omitempty" json:"statusChangedAt,omitempty"`
	Notes           []ClientNote `bson:"notes,omitempty" json:"notes,omitempty"`
	CreatedAt       time.Time    `bson:"createdAt" json:"createdAt"`
	UpdatedAt       time.Time    `bson:"updatedAt" json:"updatedAt"`
}

// ClientNote represents an administrative note on a client
type ClientNote struct {
	Text      string    `bson:"text" json:"text"`
	Timestamp time.Time `bson:"timestamp" json:"timestamp"`
	AddedBy   string    `bson:"addedBy" json:"addedBy"` // Principal ID who added the note
	Category  string    `bson:"category,omitempty" json:"category,omitempty"`
}

// IsActive returns true if the client is active
func (c *Client) IsActive() bool {
	return c.Status == ClientStatusActive
}

// IsSuspended returns true if the client is suspended
func (c *Client) IsSuspended() bool {
	return c.Status == ClientStatusSuspended
}

// ClientAccessGrant represents access granted to a principal for a client
// Collection: client_access_grants
type ClientAccessGrant struct {
	ID          string    `bson:"_id" json:"id"`
	PrincipalID string    `bson:"principalId" json:"principalId"`
	ClientID    string    `bson:"clientId" json:"clientId"`
	GrantedAt   time.Time `bson:"grantedAt" json:"grantedAt"`
	ExpiresAt   time.Time `bson:"expiresAt,omitempty" json:"expiresAt,omitempty"`
}

// IsExpired returns true if the grant has expired
func (g *ClientAccessGrant) IsExpired() bool {
	if g.ExpiresAt.IsZero() {
		return false // No expiry set
	}
	return time.Now().After(g.ExpiresAt)
}

// AnchorDomain represents a domain that grants anchor (platform admin) scope
// Collection: anchor_domains
type AnchorDomain struct {
	ID        string    `bson:"_id" json:"id"`
	Domain    string    `bson:"domain" json:"domain"` // e.g., "flowcatalyst.tech"
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
}

// AuthProvider defines the authentication provider type
type AuthProvider string

const (
	AuthProviderInternal AuthProvider = "INTERNAL" // Local password auth
	AuthProviderOIDC     AuthProvider = "OIDC"     // External OIDC provider
)

// AuthConfigType defines the type of auth configuration
type AuthConfigType string

const (
	AuthConfigTypeAnchor  AuthConfigType = "ANCHOR"  // Platform-wide access
	AuthConfigTypePartner AuthConfigType = "PARTNER" // Partner access
	AuthConfigTypeClient  AuthConfigType = "CLIENT"  // Client-specific
)

// ClientAuthConfig maps email domains to authentication configuration
// Collection: client_auth_config
type ClientAuthConfig struct {
	ID                  string         `bson:"_id" json:"id"`
	EmailDomain         string         `bson:"emailDomain" json:"emailDomain"` // e.g., "acme.com"
	ConfigType          AuthConfigType `bson:"configType" json:"configType"`
	ClientID            string         `bson:"clientId,omitempty" json:"clientId,omitempty"` // Associated FlowCatalyst client
	PrimaryClientID     string         `bson:"primaryClientId,omitempty" json:"primaryClientId,omitempty"`
	AdditionalClientIDs []string       `bson:"additionalClientIds,omitempty" json:"additionalClientIds,omitempty"`
	GrantedClientIDs    []string       `bson:"grantedClientIds,omitempty" json:"grantedClientIds,omitempty"` // For PARTNER type
	AuthProvider        AuthProvider   `bson:"authProvider" json:"authProvider"`

	// OIDC configuration
	IdpType             string `bson:"idpType,omitempty" json:"idpType,omitempty"` // KEYCLOAK, ENTRA, OIDC
	OIDCIssuerURL       string `bson:"oidcIssuerUrl,omitempty" json:"oidcIssuerUrl,omitempty"`
	OIDCClientID        string `bson:"oidcClientId,omitempty" json:"oidcClientId,omitempty"`
	OIDCClientSecret    string `bson:"-" json:"-"`                                   // Resolved secret value (not stored)
	OIDCClientSecretRef string `bson:"oidcClientSecretRef,omitempty" json:"-"`       // Secret reference, never expose
	OIDCMultiTenant     bool   `bson:"oidcMultiTenant,omitempty" json:"oidcMultiTenant,omitempty"`
	OIDCIssuerPattern   string `bson:"oidcIssuerPattern,omitempty" json:"oidcIssuerPattern,omitempty"`

	// Provider-specific configuration
	EntraTenantID string `bson:"entraTenantId,omitempty" json:"entraTenantId,omitempty"` // Azure Entra tenant ID
	GroupsClaim   string `bson:"groupsClaim,omitempty" json:"groupsClaim,omitempty"`     // Custom groups claim name
	RolesClaim    string `bson:"rolesClaim,omitempty" json:"rolesClaim,omitempty"`       // Custom roles claim name

	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

// IdpRoleMapping maps IDP groups/roles to FlowCatalyst roles
// Collection: idp_role_mappings
type IdpRoleMapping struct {
	ID           string    `bson:"_id" json:"id"`
	EmailDomain  string    `bson:"emailDomain" json:"emailDomain"` // Domain this mapping applies to
	IdpGroupName string    `bson:"idpGroupName" json:"idpGroupName"` // Group or role name from IDP
	RoleID       string    `bson:"roleId" json:"roleId"` // FlowCatalyst role ID
	RoleName     string    `bson:"roleName,omitempty" json:"roleName,omitempty"` // For display
	CreatedAt    time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt    time.Time `bson:"updatedAt" json:"updatedAt"`
}

// IsOIDC returns true if this config uses OIDC authentication
func (c *ClientAuthConfig) IsOIDC() bool {
	return c.AuthProvider == AuthProviderOIDC
}

// GetAllClientIDs returns all client IDs this config grants access to
func (c *ClientAuthConfig) GetAllClientIDs() []string {
	switch c.ConfigType {
	case AuthConfigTypeAnchor:
		return nil // Anchor has access to all
	case AuthConfigTypePartner:
		return c.GrantedClientIDs
	case AuthConfigTypeClient:
		ids := []string{c.PrimaryClientID}
		ids = append(ids, c.AdditionalClientIDs...)
		return ids
	default:
		return nil
	}
}
