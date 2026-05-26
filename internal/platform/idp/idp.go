// Package idp holds provider-specific OIDC integration glue (Keycloak,
// Entra, Google, generic). Mirrors fc-platform/src/idp/.
//
// Phase 3d scope: the abstract Provider interface + a generic OIDC
// implementation that delegates to go-oidc/v3. Provider-specific
// integrations (Keycloak admin API, Entra Graph API for role sync)
// are scaffolded but their bodies are TODOs — they're not on the
// critical path for cutover, and they're well-isolated when the team
// gets to them.
package idp

import (
	"context"
	"errors"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
)

// Provider is the per-IDP integration surface. Each concrete impl
// (KeycloakProvider, EntraProvider, GenericOIDCProvider) implements
// this interface against the upstream IDP's API.
type Provider interface {
	// Name is the provider type identifier (e.g. "keycloak", "entra").
	Name() string

	// SyncRoles fetches the user's roles from the IDP and returns the
	// platform-side role names they should be granted.
	//
	// Optional — providers that don't support role sync return
	// ErrRoleSyncNotSupported. The platform falls back to the role
	// assignments configured locally on the EmailDomainMapping.
	SyncRoles(ctx context.Context, externalUserID string) ([]string, error)
}

// ErrRoleSyncNotSupported is returned by providers that don't expose
// a role-sync API.
var ErrRoleSyncNotSupported = errors.New("idp: role sync not supported by this provider")

// Resolve returns the right Provider for the supplied IdentityProvider
// configuration. Caller passes the persisted IDP config; this function
// inspects the type/code and returns the matching integration.
//
// TODO(wave-3d-follow-up): wire concrete providers.
func Resolve(ip *identityprovider.IdentityProvider) Provider {
	// For Phase 3d: every IDP returns the generic OIDC provider that
	// only does login. Keycloak/Entra-specific integrations (role sync,
	// user provisioning) land as separate Provider impls once the
	// underlying admin APIs are wired.
	return &genericOIDCProvider{cfg: ip}
}

// genericOIDCProvider handles login-only flows for any RFC-compliant
// OIDC provider. Role sync is unsupported here; subclass providers
// (Keycloak, Entra) layer on top.
type genericOIDCProvider struct {
	cfg *identityprovider.IdentityProvider
}

func (*genericOIDCProvider) Name() string { return "oidc" }

func (*genericOIDCProvider) SyncRoles(_ context.Context, _ string) ([]string, error) {
	return nil, ErrRoleSyncNotSupported
}
