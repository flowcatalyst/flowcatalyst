// dto.go contains the wire-format types for the identity_provider API.
package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// CreateIdentityProviderRequest is the wire body for POST /api/identity-providers.
type CreateIdentityProviderRequest struct {
	Code                string   `json:"code" doc:"IDP code (e.g. internal, entra)"`
	Name                string   `json:"name" doc:"Display name"`
	Type                string   `json:"type" doc:"IDP type (INTERNAL or OIDC)"`
	OIDCIssuerURL       *string  `json:"oidcIssuerUrl,omitempty"`
	OIDCClientID        *string  `json:"oidcClientId,omitempty"`
	OIDCClientSecretRef *string  `json:"oidcClientSecretRef,omitempty"`
	OIDCMultiTenant     bool     `json:"oidcMultiTenant"`
	OIDCIssuerPattern   *string  `json:"oidcIssuerPattern,omitempty"`
	AllowedEmailDomains []string `json:"allowedEmailDomains,omitempty"`
}

func (r CreateIdentityProviderRequest) toCommand() operations.CreateCommand {
	return operations.CreateCommand{
		Code:                r.Code,
		Name:                r.Name,
		Type:                r.Type,
		OIDCIssuerURL:       r.OIDCIssuerURL,
		OIDCClientID:        r.OIDCClientID,
		OIDCClientSecretRef: r.OIDCClientSecretRef,
		OIDCMultiTenant:     r.OIDCMultiTenant,
		OIDCIssuerPattern:   r.OIDCIssuerPattern,
		AllowedEmailDomains: r.AllowedEmailDomains,
	}
}

// UpdateIdentityProviderRequest is the wire body for PUT /api/identity-providers/{id}.
type UpdateIdentityProviderRequest struct {
	Name                *string  `json:"name,omitempty"`
	OIDCIssuerURL       *string  `json:"oidcIssuerUrl,omitempty"`
	OIDCClientID        *string  `json:"oidcClientId,omitempty"`
	OIDCClientSecretRef *string  `json:"oidcClientSecretRef,omitempty"`
	OIDCMultiTenant     *bool    `json:"oidcMultiTenant,omitempty"`
	OIDCIssuerPattern   *string  `json:"oidcIssuerPattern,omitempty"`
	AllowedEmailDomains []string `json:"allowedEmailDomains,omitempty"`
}

func (r UpdateIdentityProviderRequest) toCommand(id string) operations.UpdateCommand {
	return operations.UpdateCommand{
		ID:                  id,
		Name:                r.Name,
		OIDCIssuerURL:       r.OIDCIssuerURL,
		OIDCClientID:        r.OIDCClientID,
		OIDCClientSecretRef: r.OIDCClientSecretRef,
		OIDCMultiTenant:     r.OIDCMultiTenant,
		OIDCIssuerPattern:   r.OIDCIssuerPattern,
		AllowedEmailDomains: r.AllowedEmailDomains,
	}
}

// IdentityProviderResponse mirrors identityprovider.IdentityProvider.
// The OIDC client secret reference is intentionally NOT serialized; the SPA
// only needs to know whether a secret is configured via hasClientSecret.
type IdentityProviderResponse struct {
	ID                  string          `json:"id"`
	Code                string          `json:"code"`
	Name                string          `json:"name"`
	Type                string          `json:"type"`
	OIDCIssuerURL       *string         `json:"oidcIssuerUrl,omitempty"`
	OIDCClientID        *string         `json:"oidcClientId,omitempty"`
	HasClientSecret     bool            `json:"hasClientSecret"`
	OIDCMultiTenant     bool            `json:"oidcMultiTenant"`
	OIDCIssuerPattern   *string         `json:"oidcIssuerPattern,omitempty"`
	AllowedEmailDomains []string        `json:"allowedEmailDomains"`
	CreatedAt           httpcompat.Time `json:"createdAt"`
	UpdatedAt           httpcompat.Time `json:"updatedAt"`
}

func fromEntity(ip *identityprovider.IdentityProvider) IdentityProviderResponse {
	domains := ip.AllowedEmailDomains
	if domains == nil {
		domains = []string{}
	}
	return IdentityProviderResponse{
		ID:                  ip.ID,
		Code:                ip.Code,
		Name:                ip.Name,
		Type:                string(ip.Type),
		OIDCIssuerURL:       ip.OIDCIssuerURL,
		OIDCClientID:        ip.OIDCClientID,
		HasClientSecret:     ip.HasClientSecret(),
		OIDCMultiTenant:     ip.OIDCMultiTenant,
		OIDCIssuerPattern:   ip.OIDCIssuerPattern,
		AllowedEmailDomains: domains,
		CreatedAt:           jsontime.New(ip.CreatedAt),
		UpdatedAt:           jsontime.New(ip.UpdatedAt),
	}
}

// IdentityProviderListResponse is the wire shape for GET /api/identity-providers.
// SPA's IdentityProviderListPage reads `response.identityProviders`.
type IdentityProviderListResponse struct {
	IdentityProviders []IdentityProviderResponse `json:"identityProviders"`
	Total             int                        `json:"total"`
}
