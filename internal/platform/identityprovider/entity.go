// Package identityprovider stores OIDC IDP configuration
// (Entra, Keycloak, Google, ...) and the
// platform-internal "INTERNAL" identity provider for password auth.
package identityprovider

import (
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Type is the IDP kind.
type Type string

const (
	TypeInternal Type = "INTERNAL"
	TypeOIDC     Type = "OIDC"
)

// ParseType parses a stored/wire type value. Returns ok=false for anything
// other than INTERNAL or OIDC — callers MUST reject on ok=false rather
// than coerce an unrecognised value to INTERNAL (X-06: a loud read error,
// never a silent default). Follows the (T, bool) shape of
// common.ParseOutboxItemType.
func ParseType(s string) (Type, bool) {
	switch Type(s) {
	case TypeInternal, TypeOIDC:
		return Type(s), true
	default:
		return "", false
	}
}

// IdentityProvider is the aggregate root.
type IdentityProvider struct {
	ID                  string  `json:"id"`
	Code                string  `json:"code"`
	Name                string  `json:"name"`
	Type                Type    `json:"type"`
	OIDCIssuerURL       *string `json:"oidcIssuerUrl,omitempty"`
	OIDCClientID        *string `json:"oidcClientId,omitempty"`
	OIDCClientSecretRef *string `json:"oidcClientSecretRef,omitempty"`
	OIDCMultiTenant     bool    `json:"oidcMultiTenant"`
	OIDCIssuerPattern   *string `json:"oidcIssuerPattern,omitempty"`
	// AllowedEmailDomains is DERIVED from tnt_email_domain_mappings (the
	// domains currently routed to this IdP) — the mapping table is the single
	// source of truth for domain → IdP routing. The repo hydrates it on read;
	// Persist ignores it. Authoring goes through the IdP orchestration ops,
	// which create / re-point mappings.
	AllowedEmailDomains []string `json:"allowedEmailDomains"`
	// ServesNonAnchorDomain is DERIVED like AllowedEmailDomains: at least one
	// of those domains' mappings has a scope other than ANCHOR (PARTNER or
	// CLIENT), i.e. the provider authenticates a tenant, not the platform
	// operator. Hydrated on read, never persisted, not on the wire.
	ServesNonAnchorDomain bool `json:"-"`
	// SyncRolesFromIDP gates whether OIDC logins through this IdP reconcile
	// the user's IDP_SYNC-sourced platform roles from the token's `roles`
	// claim.
	SyncRolesFromIDP bool `json:"syncRolesFromIdp"`
	// AllowedRoleIDs restricts which platform roles (by role TSID) this IdP
	// may confer via role sync. Empty = no restriction, which is only
	// acceptable for a provider that serves ANCHOR domains alone
	// ([IdentityProvider.RoleSyncNeedsAllowList]).
	AllowedRoleIDs []string `json:"allowedRoleIds"`
	// NOTE: an IdP is deliberately NOT bound to any client or plane. Its job
	// is authentication only — any login attempt (employee or any client's
	// portal) for a domain it owns routes to it. Which client's portal
	// identity a portal login yields is the OAuth client's portal linkage
	// (oauth_clients.portal_client_id), never an IdP property. A per-IdP
	// portal binding existed briefly and was removed (owner decision
	// 2026-08-20): it wrongly limited an IdP to a single portal.
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
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
		AllowedRoleIDs:      []string{},
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}

// HasClientSecret reports whether a secret reference is configured.
func (i *IdentityProvider) HasClientSecret() bool { return i.OIDCClientSecretRef != nil }

// RoleSyncUnrestricted reports role sync on with no allow-list: the provider
// may grant any platform role that a role mapping names.
func (i *IdentityProvider) RoleSyncUnrestricted() bool {
	return i != nil && i.SyncRolesFromIDP && len(i.AllowedRoleIDs) == 0
}

// RoleSyncNeedsAllowList reports whether the provider is in the state the
// platform no longer lets anyone save: unrestricted role sync
// ([IdentityProvider.RoleSyncUnrestricted]) on a provider that serves at least
// one email domain whose mapping is not ANCHOR. Such a provider speaks for a
// tenant, and without an allow-list it could grant any platform role that any
// provider's role mapping names. Uses the derived ServesNonAnchorDomain, so it
// is meaningful on a repository-hydrated row.
func (i *IdentityProvider) RoleSyncNeedsAllowList() bool {
	return i.RoleSyncUnrestricted() && i.ServesNonAnchorDomain
}

// ErrRoleAllowListRequired is the save-time refusal of a provider that
// RoleSyncNeedsAllowList.
func ErrRoleAllowListRequired() error {
	return usecase.Validation("ROLE_ALLOWLIST_REQUIRED",
		"Role sync on an identity provider that serves a client or partner domain needs an allow-list of the roles it may grant")
}
