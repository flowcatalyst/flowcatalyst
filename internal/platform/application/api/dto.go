// dto.go contains the wire-format types for the application API.
package api

import (
	"encoding/json"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// CreateApplicationRequest is the wire body for POST /api/applications.
type CreateApplicationRequest struct {
	Code           string  `json:"code" doc:"Application code (lowercase, alphanumeric, hyphens)"`
	Name           string  `json:"name"`
	Type           string  `json:"type,omitempty" doc:"APPLICATION or INTEGRATION"`
	Description    *string `json:"description,omitempty"`
	IconURL        *string `json:"iconUrl,omitempty"`
	Website        *string `json:"website,omitempty"`
	DefaultBaseURL *string `json:"defaultBaseUrl,omitempty"`
}

func (r CreateApplicationRequest) toCommand() operations.CreateCommand {
	return operations.CreateCommand{
		Code:           r.Code,
		Name:           r.Name,
		Type:           r.Type,
		Description:    r.Description,
		IconURL:        r.IconURL,
		Website:        r.Website,
		DefaultBaseURL: r.DefaultBaseURL,
	}
}

// UpdateApplicationRequest is the wire body for PUT /api/applications/{id}.
type UpdateApplicationRequest struct {
	Name           *string `json:"name,omitempty"`
	Description    *string `json:"description,omitempty"`
	IconURL        *string `json:"iconUrl,omitempty"`
	Website        *string `json:"website,omitempty"`
	DefaultBaseURL *string `json:"defaultBaseUrl,omitempty"`
}

func (r UpdateApplicationRequest) toCommand(id string) operations.UpdateCommand {
	return operations.UpdateCommand{
		ID:             id,
		Name:           r.Name,
		Description:    r.Description,
		IconURL:        r.IconURL,
		Website:        r.Website,
		DefaultBaseURL: r.DefaultBaseURL,
	}
}

// AttachServiceAccountRequest is the wire body for
// POST /api/applications/{id}/service-account.
type AttachServiceAccountRequest struct {
	ServiceAccountID   string `json:"serviceAccountId"`
	ServiceAccountCode string `json:"serviceAccountCode"`
}

// ApplicationResponse mirrors application.Application.
type ApplicationResponse struct {
	ID               string  `json:"id"`
	Type             string  `json:"type"`
	Code             string  `json:"code"`
	Name             string  `json:"name"`
	Description      *string `json:"description,omitempty"`
	IconURL          *string `json:"iconUrl,omitempty"`
	Website          *string `json:"website,omitempty"`
	Logo             *string `json:"logo,omitempty"`
	LogoMimeType     *string `json:"logoMimeType,omitempty"`
	DefaultBaseURL   *string `json:"defaultBaseUrl,omitempty"`
	ServiceAccountID *string `json:"serviceAccountId,omitempty"`
	Active           bool    `json:"active"`
	// HasLoginClient is true iff this application has a login-type OAuth
	// client provisioned. Populated only by the detail endpoint
	// (GET /applications/{id}); list responses leave it false. SPA gates
	// the "Provision Login Client" form on this flag.
	HasLoginClient bool            `json:"hasLoginClient"`
	CreatedAt      httpcompat.Time `json:"createdAt"`
	UpdatedAt      httpcompat.Time `json:"updatedAt"`
}

func fromEntity(a *application.Application) ApplicationResponse {
	return ApplicationResponse{
		ID:               a.ID,
		Type:             string(a.Type),
		Code:             a.Code,
		Name:             a.Name,
		Description:      a.Description,
		IconURL:          a.IconURL,
		Website:          a.Website,
		Logo:             a.Logo,
		LogoMimeType:     a.LogoMimeType,
		DefaultBaseURL:   a.DefaultBaseURL,
		ServiceAccountID: a.ServiceAccountID,
		Active:           a.Active,
		CreatedAt:        jsontime.New(a.CreatedAt),
		UpdatedAt:        jsontime.New(a.UpdatedAt),
	}
}

// ApplicationListResponse is the wire shape for GET /api/applications.
// Matches the Rust shape (`applications` + `total`); SPA's
// ApplicationListPage reads `response.applications` directly.
type ApplicationListResponse struct {
	Applications []ApplicationResponse `json:"applications"`
	Total        int                   `json:"total"`
}

// ClientConfigResponse mirrors application.ClientConfig.
type ClientConfigResponse struct {
	ID              string          `json:"id"`
	ApplicationID   string          `json:"applicationId"`
	ClientID        string          `json:"clientId"`
	Enabled         bool            `json:"enabled"`
	BaseURLOverride *string         `json:"baseUrlOverride,omitempty"`
	ConfigJSON      json.RawMessage `json:"configJson,omitempty"`
	CreatedAt       httpcompat.Time `json:"createdAt"`
	UpdatedAt       httpcompat.Time `json:"updatedAt"`
}

func clientConfigFromEntity(c *application.ClientConfig) ClientConfigResponse {
	return ClientConfigResponse{
		ID:              c.ID,
		ApplicationID:   c.ApplicationID,
		ClientID:        c.ClientID,
		Enabled:         c.Enabled,
		BaseURLOverride: c.BaseURLOverride,
		ConfigJSON:      c.ConfigJSON,
		CreatedAt:       jsontime.New(c.CreatedAt),
		UpdatedAt:       jsontime.New(c.UpdatedAt),
	}
}

// ClientConfigListResponse is the wire shape for
// GET /api/applications/{id}/clients.
type ClientConfigListResponse struct {
	Items []ClientConfigResponse `json:"items"`
}

// ApplicationOAuthClientCredentials is the OAuth-client credentials block
// nested in provisioning responses. ClientSecret is plaintext and only
// emitted at creation time for CONFIDENTIAL clients.
type ApplicationOAuthClientCredentials struct {
	ID           string `json:"id"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret,omitempty"`
}

// ApplicationServiceAccountCredentials is the service-account block nested
// in the provision-service-account response.
type ApplicationServiceAccountCredentials struct {
	PrincipalID string                            `json:"principalId"`
	Name        string                            `json:"name"`
	OAuthClient ApplicationOAuthClientCredentials `json:"oauthClient"`
}

// ApplicationProvisionServiceAccountResponse is the body for
// POST /api/applications/{id}/provision-service-account. Matches the
// SPA's ServiceAccountCredentials shape. The client secret is plaintext
// and only emitted once.
type ApplicationProvisionServiceAccountResponse struct {
	Message        string                               `json:"message"`
	ServiceAccount ApplicationServiceAccountCredentials `json:"serviceAccount"`
}

// ApplicationLoginClientCredentials is the login-client block nested in
// the provision-login-client response.
type ApplicationLoginClientCredentials struct {
	ClientType   string                            `json:"clientType"`
	RedirectURIs []string                          `json:"redirectUris"`
	OAuthClient  ApplicationOAuthClientCredentials `json:"oauthClient"`
}

// ApplicationProvisionLoginClientResponse is the body for
// POST /api/applications/{id}/provision-login-client. Matches the SPA's
// LoginClientCredentials shape. PUBLIC clients have no secret.
type ApplicationProvisionLoginClientResponse struct {
	Message     string                            `json:"message"`
	LoginClient ApplicationLoginClientCredentials `json:"loginClient"`
}

// ProvisionLoginClientRequest is the wire body for
// POST /api/applications/{id}/provision-login-client.
type ProvisionLoginClientRequest struct {
	ClientType     string   `json:"clientType,omitempty" doc:"PUBLIC (default) or CONFIDENTIAL"`
	RedirectURIs   []string `json:"redirectUris"`
	AllowedOrigins []string `json:"allowedOrigins,omitempty"`
}

// ApplicationRolesResponse is the body for
// GET /api/applications/by-id/{id}/roles.
type ApplicationRolesResponse struct {
	Roles []string `json:"roles"`
}
