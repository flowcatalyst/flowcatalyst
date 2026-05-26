package idp

import (
	"context"
	"errors"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
)

// KeycloakProvider integrates with the Keycloak admin REST API for
// per-realm role sync. Mirrors fc-platform/src/idp/keycloak.rs.
//
// TODO(wave-3d-follow-up): wire against the Keycloak admin REST API.
// The Rust impl uses reqwest + admin token; the Go port uses net/http
// + go-oidc client_credentials grant against {realm}/protocol/openid-connect/token.
type KeycloakProvider struct {
	cfg *identityprovider.IdentityProvider
}

// NewKeycloakProvider wires a Keycloak integration.
func NewKeycloakProvider(cfg *identityprovider.IdentityProvider) *KeycloakProvider {
	return &KeycloakProvider{cfg: cfg}
}

// Name returns the provider type.
func (*KeycloakProvider) Name() string { return "keycloak" }

// SyncRoles fetches realm-roles for the supplied user via the Keycloak
// admin REST API: GET /admin/realms/{realm}/users/{id}/role-mappings/realm.
//
// TODO(wave-3d-follow-up): implement.
func (*KeycloakProvider) SyncRoles(_ context.Context, _ string) ([]string, error) {
	return nil, errors.Join(ErrRoleSyncNotSupported, errors.New("keycloak.SyncRoles: wired in wave 3d follow-up"))
}
