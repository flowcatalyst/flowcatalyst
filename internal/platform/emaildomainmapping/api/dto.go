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
	// 2FA enforcement for internal-auth users of this domain. All optional
	// (default false / empty) — OIDC-domain creates omit them entirely.
	Require2FA            bool     `json:"require2fa,omitempty"`
	Allowed2FAMethods     []string `json:"allowed2faMethods,omitempty" doc:"Permitted 2FA methods (TOTP, EMAIL_PIN). ≥1 required when require2fa is set."`
	RememberDeviceEnabled bool     `json:"rememberDeviceEnabled,omitempty"`
	RememberDeviceDays    int      `json:"rememberDeviceDays,omitempty"`
}

func (r CreateMappingRequest) toCommand() operations.CreateCommand {
	return operations.CreateCommand{
		EmailDomain:           r.EmailDomain,
		IdentityProviderID:    r.IdentityProviderID,
		ScopeType:             r.ScopeType,
		PrimaryClientID:       r.PrimaryClientID,
		AdditionalClientIDs:   r.AdditionalClientIDs,
		GrantedClientIDs:      r.GrantedClientIDs,
		RequiredOIDCTenantID:  r.RequiredOIDCTenantID,
		Require2FA:            r.Require2FA,
		Allowed2FAMethods:     r.Allowed2FAMethods,
		RememberDeviceEnabled: r.RememberDeviceEnabled,
		RememberDeviceDays:    r.RememberDeviceDays,
	}
}

// UpdateMappingRequest is the wire body for PUT /api/email-domain-mappings/{id}.
// The identity provider is not updatable here — re-pointing a domain goes
// through POST /api/email-domain-mappings/{id}/move-provider, which applies
// the direction-specific side effects.
type UpdateMappingRequest struct {
	PrimaryClientID       *string  `json:"primaryClientId,omitempty"`
	AdditionalClientIDs   []string `json:"additionalClientIds,omitempty"`
	GrantedClientIDs      []string `json:"grantedClientIds,omitempty"`
	RequiredOIDCTenantID  *string  `json:"requiredOidcTenantId,omitempty"`
	Require2FA            *bool    `json:"require2fa,omitempty"`
	Allowed2FAMethods     []string `json:"allowed2faMethods,omitempty"`
	RememberDeviceEnabled *bool    `json:"rememberDeviceEnabled,omitempty"`
	RememberDeviceDays    *int     `json:"rememberDeviceDays,omitempty"`
}

func (r UpdateMappingRequest) toCommand(id string) operations.UpdateCommand {
	return operations.UpdateCommand{
		ID:                    id,
		PrimaryClientID:       r.PrimaryClientID,
		AdditionalClientIDs:   r.AdditionalClientIDs,
		GrantedClientIDs:      r.GrantedClientIDs,
		RequiredOIDCTenantID:  r.RequiredOIDCTenantID,
		Require2FA:            r.Require2FA,
		Allowed2FAMethods:     r.Allowed2FAMethods,
		RememberDeviceEnabled: r.RememberDeviceEnabled,
		RememberDeviceDays:    r.RememberDeviceDays,
	}
}

// MappingResponse mirrors emaildomainmapping.EmailDomainMapping.
type MappingResponse struct {
	ID                    string          `json:"id"`
	EmailDomain           string          `json:"emailDomain"`
	IdentityProviderID    string          `json:"identityProviderId"`
	IdentityProviderName  *string         `json:"identityProviderName,omitempty"`
	ScopeType             string          `json:"scopeType"`
	PrimaryClientID       *string         `json:"primaryClientId,omitempty"`
	AdditionalClientIDs   []string        `json:"additionalClientIds"`
	GrantedClientIDs      []string        `json:"grantedClientIds"`
	RequiredOIDCTenantID  *string         `json:"requiredOidcTenantId,omitempty"`
	Require2FA            bool            `json:"require2fa"`
	Allowed2FAMethods     []string        `json:"allowed2faMethods"`
	RememberDeviceEnabled bool            `json:"rememberDeviceEnabled"`
	RememberDeviceDays    int             `json:"rememberDeviceDays"`
	CreatedAt             httpcompat.Time `json:"createdAt"`
	UpdatedAt             httpcompat.Time `json:"updatedAt"`
}

// fromEntity builds the wire shape. idpName is the resolved identity-provider
// display name (nil when it could not be looked up).
func fromEntity(e *emaildomainmapping.EmailDomainMapping, idpName *string) MappingResponse {
	addl := e.AdditionalClientIDs
	if addl == nil {
		addl = []string{}
	}
	granted := e.GrantedClientIDs
	if granted == nil {
		granted = []string{}
	}
	methods := e.Allowed2FAMethods
	if methods == nil {
		methods = []string{}
	}
	return MappingResponse{
		ID:                    e.ID,
		EmailDomain:           e.EmailDomain,
		IdentityProviderID:    e.IdentityProviderID,
		IdentityProviderName:  idpName,
		ScopeType:             string(e.ScopeType),
		PrimaryClientID:       e.PrimaryClientID,
		AdditionalClientIDs:   addl,
		GrantedClientIDs:      granted,
		RequiredOIDCTenantID:  e.RequiredOIDCTenantID,
		Require2FA:            e.Require2FA,
		Allowed2FAMethods:     methods,
		RememberDeviceEnabled: e.RememberDeviceEnabled,
		RememberDeviceDays:    e.RememberDeviceDays,
		CreatedAt:             jsontime.New(e.CreatedAt),
		UpdatedAt:             jsontime.New(e.UpdatedAt),
	}
}

// MappingListResponse is the wire shape for GET /api/email-domain-mappings.
// SPA's EmailDomainMappingListPage reads `response.mappings`.
type MappingListResponse struct {
	Mappings []MappingResponse `json:"mappings"`
	Total    int               `json:"total"`
}

// LookupNotFoundResponse is returned by the lookup endpoint when no
// mapping exists for the supplied domain (200 with {found:false}).
type LookupNotFoundResponse struct {
	Found bool `json:"found"`
}

// MoveProviderRequest is the wire body for
// POST /api/email-domain-mappings/{id}/move-provider.
type MoveProviderRequest struct {
	IdentityProviderID string `json:"identityProviderId" doc:"Target identity provider id"`
}

// MoveProviderResponse mirrors operations.MoveProviderResult.
type MoveProviderResponse struct {
	MappingID              string `json:"mappingId"`
	EmailDomain            string `json:"emailDomain"`
	FromIdentityProviderID string `json:"fromIdentityProviderId"`
	ToIdentityProviderID   string `json:"toIdentityProviderId"`
	UsersReset             int    `json:"usersReset" doc:"OIDC-provisioned users converted back to internal auth (0 when moving to an OIDC provider)"`
}
