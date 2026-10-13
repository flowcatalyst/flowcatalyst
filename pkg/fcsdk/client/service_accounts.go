package client

import "context"

// ServiceAccountResponse is the platform's service-account record.
type ServiceAccountResponse struct {
	ID            string `json:"id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Active        bool   `json:"active"`
	ApplicationID string `json:"applicationId,omitempty"`
	// AuthType is the credential scheme, e.g. BEARER_TOKEN.
	AuthType      string   `json:"authType"`
	ClientIDs     []string `json:"clientIds"`
	OAuthClientID string   `json:"oauthClientId,omitempty"`
	PrincipalID   string   `json:"principalId,omitempty"`
	Roles         []string `json:"roles"`
	Scope         string   `json:"scope,omitempty"`
	LastUsedAt    string   `json:"lastUsedAt,omitempty"`
	CreatedAt     string   `json:"createdAt"`
	UpdatedAt     string   `json:"updatedAt"`
}

// ServiceAccountsResource — /api/service-accounts/*. Only the read is
// exposed; an application's account is linked with
// ApplicationsResource.LinkServiceAccount.
type ServiceAccountsResource struct {
	c *FlowCatalystClient
}

// ServiceAccounts returns the service-accounts accessor.
func (c *FlowCatalystClient) ServiceAccounts() *ServiceAccountsResource {
	return &ServiceAccountsResource{c: c}
}

// Get — GET /api/service-accounts/{id}.
func (r *ServiceAccountsResource) Get(ctx context.Context, id string) (*ServiceAccountResponse, error) {
	var out ServiceAccountResponse
	if err := r.c.Get(ctx, "/api/service-accounts/"+id, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
