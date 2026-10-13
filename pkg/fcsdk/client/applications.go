package client

import (
	"context"
	"encoding/json"
	"fmt"
)

// ─── Request DTOs ────────────────────────────────────────────────────

// CreateApplicationRequest — POST /api/applications.
type CreateApplicationRequest struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	ApplicationType string `json:"type,omitempty"`
	DefaultBaseURL  string `json:"defaultBaseUrl,omitempty"`
	IconURL         string `json:"iconUrl,omitempty"`
	// Logo is inline logo markup (e.g. SVG); LogoMimeType names its media type.
	Logo         string `json:"logo,omitempty"`
	LogoMimeType string `json:"logoMimeType,omitempty"`
	Website      string `json:"website,omitempty"`
}

// UpdateApplicationRequest — PUT /api/applications/{id}.
type UpdateApplicationRequest struct {
	Name           *string `json:"name,omitempty"`
	Description    *string `json:"description,omitempty"`
	DefaultBaseURL *string `json:"defaultBaseUrl,omitempty"`
	IconURL        *string `json:"iconUrl,omitempty"`
	Logo           *string `json:"logo,omitempty"`
	LogoMimeType   *string `json:"logoMimeType,omitempty"`
	Website        *string `json:"website,omitempty"`
}

// ClientConfigRequest — body for PUT /api/applications/{id}/clients/{clientId}.
type ClientConfigRequest struct {
	Enabled         *bool           `json:"enabled,omitempty"`
	BaseURLOverride *string         `json:"baseUrlOverride,omitempty"`
	Config          json.RawMessage `json:"config,omitempty"`
}

// ─── Response DTOs ───────────────────────────────────────────────────

// ApplicationResponse is the platform's application aggregate.
type ApplicationResponse struct {
	ID               string `json:"id"`
	Code             string `json:"code"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	ApplicationType  string `json:"type"`
	DefaultBaseURL   string `json:"defaultBaseUrl,omitempty"`
	IconURL          string `json:"iconUrl,omitempty"`
	Logo             string `json:"logo,omitempty"`
	LogoMimeType     string `json:"logoMimeType,omitempty"`
	Website          string `json:"website,omitempty"`
	ServiceAccountID string `json:"serviceAccountId,omitempty"`
	// HasLoginClient reports whether a login OAuth client is provisioned
	// for the application.
	HasLoginClient bool   `json:"hasLoginClient"`
	Active         bool   `json:"active"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

// ApplicationListResponse — GET /api/applications.
type ApplicationListResponse struct {
	Applications []ApplicationResponse `json:"applications"`
	Total        uint64                `json:"total,omitempty"`
}

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

// ServiceAccountCredentials is the service account created by
// ProvisionServiceAccount, with the OAuth client that authenticates it.
type ServiceAccountCredentials struct {
	Name        string                 `json:"name"`
	PrincipalID string                 `json:"principalId"`
	OAuthClient OAuthClientCredentials `json:"oauthClient"`
}

// OAuthClientCredentials identifies an OAuth client. ClientSecret is
// present only in the response that created it.
type OAuthClientCredentials struct {
	ID           string `json:"id"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret,omitempty"`
}

// ProvisionServiceAccountResponse — POST /api/applications/{id}/provision-service-account.
type ProvisionServiceAccountResponse struct {
	Message        string                    `json:"message"`
	ServiceAccount ServiceAccountCredentials `json:"serviceAccount"`
}

// ClientConfigResponse — one application-to-client configuration, as
// returned by GET /api/applications/{id}/clients.
type ClientConfigResponse struct {
	ID              string `json:"id"`
	ApplicationID   string `json:"applicationId"`
	ClientID        string `json:"clientId"`
	Enabled         bool   `json:"enabled"`
	BaseURLOverride string `json:"baseUrlOverride,omitempty"`
	// ConfigJSON is the free-form per-client configuration document.
	ConfigJSON json.RawMessage `json:"configJson,omitempty"`
	CreatedAt  string          `json:"createdAt"`
	UpdatedAt  string          `json:"updatedAt"`
}

// ClientConfigsResponse — GET /api/applications/{id}/clients, which wraps
// the entries in `items`.
type ClientConfigsResponse struct {
	ClientConfigs []ClientConfigResponse `json:"items"`
}

// CreatedResponse — returned by create endpoints that emit only an id.
type CreatedResponse struct {
	ID string `json:"id"`
}

// SuccessResponse — generic { success, message } envelope.
type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// ─── Resource ────────────────────────────────────────────────────────

// ApplicationsResource — /api/applications/*.
type ApplicationsResource struct {
	c *FlowCatalystClient
}

// Create — POST /api/applications. Returns the new application's id.
func (r *ApplicationsResource) Create(ctx context.Context, req *CreateApplicationRequest) (*CreatedResponse, error) {
	var out CreatedResponse
	if err := r.c.Post(ctx, "/api/applications", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List — GET /api/applications with optional active/page/pageSize filters.
// Pass nil for each parameter you want omitted.
func (r *ApplicationsResource) List(ctx context.Context, active *bool, page, pageSize *uint32) (*ApplicationListResponse, error) {
	q := NewQuery().Bool("active", active).Uint32("page", page).Uint32("pageSize", pageSize).Encode()
	var out ApplicationListResponse
	if err := r.c.Get(ctx, "/api/applications"+q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get — GET /api/applications/{id}.
func (r *ApplicationsResource) Get(ctx context.Context, id string) (*ApplicationResponse, error) {
	var out ApplicationResponse
	if err := r.c.Get(ctx, "/api/applications/"+id, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetByCode — GET /api/applications/by-code/{code}.
func (r *ApplicationsResource) GetByCode(ctx context.Context, code string) (*ApplicationResponse, error) {
	var out ApplicationResponse
	if err := r.c.Get(ctx, "/api/applications/by-code/"+code, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update — PUT /api/applications/{id}.
func (r *ApplicationsResource) Update(ctx context.Context, id string, req *UpdateApplicationRequest) (*ApplicationResponse, error) {
	var out ApplicationResponse
	if err := r.c.Put(ctx, "/api/applications/"+id, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete — DELETE /api/applications/{id}.
func (r *ApplicationsResource) Delete(ctx context.Context, id string) error {
	return r.c.Delete(ctx, "/api/applications/"+id, nil)
}

// Activate — POST /api/applications/{id}/activate.
func (r *ApplicationsResource) Activate(ctx context.Context, id string) (*ApplicationResponse, error) {
	var out ApplicationResponse
	if err := r.c.Post(ctx, "/api/applications/"+id+"/activate", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Deactivate — POST /api/applications/{id}/deactivate.
func (r *ApplicationsResource) Deactivate(ctx context.Context, id string) (*ApplicationResponse, error) {
	var out ApplicationResponse
	if err := r.c.Post(ctx, "/api/applications/"+id+"/deactivate", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ProvisionServiceAccount — POST /api/applications/{id}/provision-service-account.
// The response carries the new client secret exactly once.
func (r *ApplicationsResource) ProvisionServiceAccount(ctx context.Context, id string) (*ProvisionServiceAccountResponse, error) {
	var out ProvisionServiceAccountResponse
	if err := r.c.Post(ctx, "/api/applications/"+id+"/provision-service-account", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetServiceAccount — GET /api/applications/{id}/service-account.
func (r *ApplicationsResource) GetServiceAccount(ctx context.Context, id string) (*ServiceAccountResponse, error) {
	var out ServiceAccountResponse
	if err := r.c.Get(ctx, "/api/applications/"+id+"/service-account", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListRoles — GET /api/applications/by-id/{id}/roles. The platform
// answers `{ roles: [name, ...] }`; this returns the role names.
//
// The platform mounts the admin TSID lookup under /by-id so it doesn't
// collide with the SDK's /{appCode}/roles/sync route.
func (r *ApplicationsResource) ListRoles(ctx context.Context, id string) ([]string, error) {
	var out struct {
		Roles []string `json:"roles"`
	}
	if err := r.c.Get(ctx, "/api/applications/by-id/"+id+"/roles", &out); err != nil {
		return nil, err
	}
	return out.Roles, nil
}

// ListClients — GET /api/applications/{id}/clients.
func (r *ApplicationsResource) ListClients(ctx context.Context, id string) (*ClientConfigsResponse, error) {
	var out ClientConfigsResponse
	if err := r.c.Get(ctx, "/api/applications/"+id+"/clients", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateClientConfig — PUT /api/applications/{id}/clients/{clientId}.
func (r *ApplicationsResource) UpdateClientConfig(ctx context.Context, id, clientID string, req *ClientConfigRequest) (*ClientConfigResponse, error) {
	var out ClientConfigResponse
	if err := r.c.Put(ctx, fmt.Sprintf("/api/applications/%s/clients/%s", id, clientID), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EnableForClient — POST /api/applications/{id}/clients/{clientId}/enable.
func (r *ApplicationsResource) EnableForClient(ctx context.Context, id, clientID string) (*ClientConfigResponse, error) {
	var out ClientConfigResponse
	if err := r.c.Post(ctx, fmt.Sprintf("/api/applications/%s/clients/%s/enable", id, clientID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DisableForClient — POST /api/applications/{id}/clients/{clientId}/disable.
func (r *ApplicationsResource) DisableForClient(ctx context.Context, id, clientID string) (*ClientConfigResponse, error) {
	var out ClientConfigResponse
	if err := r.c.Post(ctx, fmt.Sprintf("/api/applications/%s/clients/%s/disable", id, clientID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
