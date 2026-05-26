// Package identityprovider is the port of fc-platform/src/identity_provider.
// Stores OIDC IDP configuration (Entra, Keycloak, Google, ...) and the
// platform-internal "INTERNAL" identity provider for password auth.
package identityprovider

import (
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Type is the IDP kind.
type Type string

const (
	TypeInternal Type = "INTERNAL"
	TypeOIDC     Type = "OIDC"
)

// ParseType is the lenient parser. Unknown → INTERNAL.
func ParseType(s string) Type {
	if s == string(TypeOIDC) {
		return TypeOIDC
	}
	return TypeInternal
}

// IdentityProvider is the aggregate root.
type IdentityProvider struct {
	ID                  string    `json:"id"`
	Code                string    `json:"code"`
	Name                string    `json:"name"`
	Type                Type      `json:"type"`
	OIDCIssuerURL       *string   `json:"oidcIssuerUrl,omitempty"`
	OIDCClientID        *string   `json:"oidcClientId,omitempty"`
	OIDCClientSecretRef *string   `json:"oidcClientSecretRef,omitempty"`
	OIDCMultiTenant     bool      `json:"oidcMultiTenant"`
	OIDCIssuerPattern   *string   `json:"oidcIssuerPattern,omitempty"`
	AllowedEmailDomains []string  `json:"allowedEmailDomains"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

// IDStr satisfies usecase.HasID.
func (i IdentityProvider) IDStr() string { return i.ID }

// New constructs an IdentityProvider.
func New(code, name string, t Type) *IdentityProvider {
	now := time.Now().UTC()
	return &IdentityProvider{
		ID:                  tsid.Generate(tsid.IdentityProvider),
		Code:                code,
		Name:                name,
		Type:                t,
		AllowedEmailDomains: []string{},
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}

// HasClientSecret reports whether a secret reference is configured.
func (i *IdentityProvider) HasClientSecret() bool { return i.OIDCClientSecretRef != nil }
