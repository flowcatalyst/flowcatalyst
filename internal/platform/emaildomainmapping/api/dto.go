// dto.go contains the wire-format types for the email_domain_mapping API.
package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// CreateMappingRequest is the wire body for POST /api/email-domain-mappings.
type CreateMappingRequest struct {
	EmailDomain          string   `json:"emailDomain" doc:"DNS-like email domain (e.g. example.com)"`
	IdentityProviderID   string   `json:"identityProviderId"`
	ScopeType            string   `json:"scopeType" doc:"Scope of mapping (ANCHOR, PARTNER, CLIENT)"`
	PrimaryClientID      *string  `json:"primaryClientId,omitempty"`
	AdditionalClientIDs  []string `json:"additionalClientIds,omitempty"`
	GrantedClientIDs     []string `json:"grantedClientIds,omitempty"`
	RequiredOIDCTenantID *string  `json:"requiredOidcTenantId,omitempty"`
	AllowedRoleIDs       []string `json:"allowedRoleIds,omitempty"`
	SyncRolesFromIDP     bool     `json:"syncRolesFromIdp"`
}

func (r CreateMappingRequest) toCommand() operations.CreateCommand {
	return operations.CreateCommand{
		EmailDomain:          r.EmailDomain,
		IdentityProviderID:   r.IdentityProviderID,
		ScopeType:            r.ScopeType,
		PrimaryClientID:      r.PrimaryClientID,
		AdditionalClientIDs:  r.AdditionalClientIDs,
		GrantedClientIDs:     r.GrantedClientIDs,
		RequiredOIDCTenantID: r.RequiredOIDCTenantID,
		AllowedRoleIDs:       r.AllowedRoleIDs,
		SyncRolesFromIDP:     r.SyncRolesFromIDP,
	}
}

// UpdateMappingRequest is the wire body for PUT /api/email-domain-mappings/{id}.
type UpdateMappingRequest struct {
	IdentityProviderID   *string  `json:"identityProviderId,omitempty"`
	PrimaryClientID      *string  `json:"primaryClientId,omitempty"`
	AdditionalClientIDs  []string `json:"additionalClientIds,omitempty"`
	GrantedClientIDs     []string `json:"grantedClientIds,omitempty"`
	RequiredOIDCTenantID *string  `json:"requiredOidcTenantId,omitempty"`
	AllowedRoleIDs       []string `json:"allowedRoleIds,omitempty"`
	SyncRolesFromIDP     *bool    `json:"syncRolesFromIdp,omitempty"`
}

func (r UpdateMappingRequest) toCommand(id string) operations.UpdateCommand {
	return operations.UpdateCommand{
		ID:                   id,
		IdentityProviderID:   r.IdentityProviderID,
		PrimaryClientID:      r.PrimaryClientID,
		AdditionalClientIDs:  r.AdditionalClientIDs,
		GrantedClientIDs:     r.GrantedClientIDs,
		RequiredOIDCTenantID: r.RequiredOIDCTenantID,
		AllowedRoleIDs:       r.AllowedRoleIDs,
		SyncRolesFromIDP:     r.SyncRolesFromIDP,
	}
}

// MappingResponse mirrors emaildomainmapping.EmailDomainMapping.
type MappingResponse struct {
	ID                   string          `json:"id"`
	EmailDomain          string          `json:"emailDomain"`
	IdentityProviderID   string          `json:"identityProviderId"`
	ScopeType            string          `json:"scopeType"`
	PrimaryClientID      *string         `json:"primaryClientId,omitempty"`
	AdditionalClientIDs  []string        `json:"additionalClientIds"`
	GrantedClientIDs     []string        `json:"grantedClientIds"`
	RequiredOIDCTenantID *string         `json:"requiredOidcTenantId,omitempty"`
	AllowedRoleIDs       []string        `json:"allowedRoleIds"`
	SyncRolesFromIDP     bool            `json:"syncRolesFromIdp"`
	CreatedAt            httpcompat.Time `json:"createdAt"`
	UpdatedAt            httpcompat.Time `json:"updatedAt"`
}

func fromEntity(e *emaildomainmapping.EmailDomainMapping) MappingResponse {
	addl := e.AdditionalClientIDs
	if addl == nil {
		addl = []string{}
	}
	granted := e.GrantedClientIDs
	if granted == nil {
		granted = []string{}
	}
	roles := e.AllowedRoleIDs
	if roles == nil {
		roles = []string{}
	}
	return MappingResponse{
		ID:                   e.ID,
		EmailDomain:          e.EmailDomain,
		IdentityProviderID:   e.IdentityProviderID,
		ScopeType:            string(e.ScopeType),
		PrimaryClientID:      e.PrimaryClientID,
		AdditionalClientIDs:  addl,
		GrantedClientIDs:     granted,
		RequiredOIDCTenantID: e.RequiredOIDCTenantID,
		AllowedRoleIDs:       roles,
		SyncRolesFromIDP:     e.SyncRolesFromIDP,
		CreatedAt:            jsontime.New(e.CreatedAt),
		UpdatedAt:            jsontime.New(e.UpdatedAt),
	}
}

// MappingListResponse is the wire shape for GET /api/email-domain-mappings.
type MappingListResponse struct {
	Items []MappingResponse `json:"items"`
}

// LookupNotFoundResponse is returned by the lookup endpoint when no
// mapping exists for the supplied domain (200 with {found:false}).
type LookupNotFoundResponse struct {
	Found bool `json:"found"`
}
