package idp

import (
	"context"
	"errors"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
)

// EntraProvider integrates with Microsoft Entra ID (formerly Azure AD)
// for app-role sync via Microsoft Graph. Mirrors fc-platform/src/idp/entra.rs.
//
// TODO(wave-3d-follow-up): wire against Microsoft Graph.
// The Rust impl uses an MSAL-style client_credentials grant + GET
// /users/{id}/appRoleAssignments. The Go port uses
// github.com/coreos/go-oidc/v3 for the OIDC client and net/http for Graph.
type EntraProvider struct {
	cfg *identityprovider.IdentityProvider
}

// NewEntraProvider wires an Entra integration.
func NewEntraProvider(cfg *identityprovider.IdentityProvider) *EntraProvider {
	return &EntraProvider{cfg: cfg}
}

// Name returns the provider type.
func (*EntraProvider) Name() string { return "entra" }

// SyncRoles fetches the supplied user's app-role assignments from Graph.
//
// TODO(wave-3d-follow-up): implement.
func (*EntraProvider) SyncRoles(_ context.Context, _ string) ([]string, error) {
	return nil, errors.Join(ErrRoleSyncNotSupported, errors.New("entra.SyncRoles: wired in wave 3d follow-up"))
}
