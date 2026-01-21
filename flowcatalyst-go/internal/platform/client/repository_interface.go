package client

import "context"

// Repository defines the interface for client data access.
// All implementations must be wrapped with instrumentation.
type Repository interface {
	// Client operations
	FindByID(ctx context.Context, id string) (*Client, error)
	FindByIdentifier(ctx context.Context, identifier string) (*Client, error)
	FindAll(ctx context.Context, skip, limit int64) ([]*Client, error)
	FindByStatus(ctx context.Context, status ClientStatus) ([]*Client, error)
	Search(ctx context.Context, query string) ([]*Client, error)
	Insert(ctx context.Context, client *Client) error
	Update(ctx context.Context, client *Client) error
	UpdateStatus(ctx context.Context, id string, status ClientStatus, reason string) error
	AddNote(ctx context.Context, id string, note ClientNote) error
	Delete(ctx context.Context, id string) error

	// Access Grant operations
	FindAccessGrantsByPrincipal(ctx context.Context, principalID string) ([]*ClientAccessGrant, error)
	FindAccessGrantsByClient(ctx context.Context, clientID string) ([]*ClientAccessGrant, error)
	GrantAccess(ctx context.Context, grant *ClientAccessGrant) error
	RevokeAccess(ctx context.Context, principalID, clientID string) error
	HasAccess(ctx context.Context, principalID, clientID string) (bool, error)

	// Anchor Domain operations
	FindAnchorDomains(ctx context.Context) ([]*AnchorDomain, error)
	IsAnchorDomain(ctx context.Context, domain string) (bool, error)
	AddAnchorDomain(ctx context.Context, domain *AnchorDomain) error
	RemoveAnchorDomain(ctx context.Context, domain string) error

	// Auth Config operations
	FindAuthConfigByDomain(ctx context.Context, emailDomain string) (*ClientAuthConfig, error)
	FindAllAuthConfigs(ctx context.Context) ([]*ClientAuthConfig, error)
	InsertAuthConfig(ctx context.Context, config *ClientAuthConfig) error
	UpdateAuthConfig(ctx context.Context, config *ClientAuthConfig) error
	DeleteAuthConfig(ctx context.Context, id string) error

	// IDP Role Mapping operations
	FindIdpRoleMappingsByDomain(ctx context.Context, emailDomain string) ([]*IdpRoleMapping, error)
	FindAllIdpRoleMappings(ctx context.Context) ([]*IdpRoleMapping, error)
	InsertIdpRoleMapping(ctx context.Context, mapping *IdpRoleMapping) error
	DeleteIdpRoleMapping(ctx context.Context, id string) error
	DeleteIdpRoleMappingsByDomain(ctx context.Context, emailDomain string) error
}
