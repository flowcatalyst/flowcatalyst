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

// LinkServiceAccountRequest — body for POST /api/applications/{id}/service-account.
// Both members are required; the platform checks they name the same account.
type LinkServiceAccountRequest struct {
	ServiceAccountID   string `json:"serviceAccountId"`
	ServiceAccountCode string `json:"serviceAccountCode"`
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

// List — GET /api/applications?type=&active=. The platform returns every
// matching application in one response (there is no paging). Pass an empty
// appType and a nil active to list them all.
func (r *ApplicationsResource) List(ctx context.Context, appType string, active *bool) (*ApplicationListResponse, error) {
	q := NewQuery().String("type", appType).Bool("active", active).Encode()
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

// Update — PUT /api/applications/{id}. The platform answers 204 No
// Content; call Get(id) for the updated record.
func (r *ApplicationsResource) Update(ctx context.Context, id string, req *UpdateApplicationRequest) error {
	return r.c.Put(ctx, "/api/applications/"+id, req, nil)
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

// LinkServiceAccount — POST /api/applications/{id}/service-account. Points
// the application at an existing service account (the platform answers 204).
// The application's current account is on ApplicationResponse.ServiceAccountID;
// read the account itself with ServiceAccounts().Get.
func (r *ApplicationsResource) LinkServiceAccount(ctx context.Context, id string, req *LinkServiceAccountRequest) error {
	return r.c.Post(ctx, "/api/applications/"+id+"/service-account", req, nil)
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

// GetClientConfig — GET /api/applications/{id}/clients/{clientId}: the
// application's configuration for one client. The platform has no route
// that edits it; EnableForClient / DisableForClient flip it on and off.
func (r *ApplicationsResource) GetClientConfig(ctx context.Context, id, clientID string) (*ClientConfigResponse, error) {
	var out ClientConfigResponse
	if err := r.c.Get(ctx, fmt.Sprintf("/api/applications/%s/clients/%s", id, clientID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EnableForClient — POST /api/applications/{id}/clients/{clientId}/enable
// (204 No Content).
func (r *ApplicationsResource) EnableForClient(ctx context.Context, id, clientID string) error {
	return r.c.Post(ctx, fmt.Sprintf("/api/applications/%s/clients/%s/enable", id, clientID), nil, nil)
}

// DisableForClient — POST /api/applications/{id}/clients/{clientId}/disable
// (204 No Content).
func (r *ApplicationsResource) DisableForClient(ctx context.Context, id, clientID string) error {
	return r.c.Post(ctx, fmt.Sprintf("/api/applications/%s/clients/%s/disable", id, clientID), nil, nil)
}
