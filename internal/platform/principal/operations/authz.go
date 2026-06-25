package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
)

// requireUserResourceAccess is the post-load resource-level authorization shared
// by the by-id user-management operations (update, activate, deactivate,
// delete). It is the use-case-side move of the controller's old
// requireScopeByID: a non-anchor administrator may only act on CLIENT-scope
// users (blockNonClientTarget) homed at a client they can access
// (CheckScopeAccess). The coarse "may write/delete principals" permission stays
// at the controller — this is purely the per-resource scope gate.
//
// Anchors pass both checks unconditionally (CheckScopeAccess short-circuits on
// anchor; the kind-of-user block only applies to non-anchors).
func requireUserResourceAccess(ctx context.Context, p *principal.Principal) error {
	ac := auth.FromContext(ctx)
	if err := blockNonClientTarget(ac, p); err != nil {
		return err
	}
	return auth.CheckScopeAccess(ac, p.ClientID)
}

// requireUserAdmin is the post-load resource-level authorization shared by the
// user-administration operations that the controller gated with
// auth.RequireUserAdmin + blockNonClientTarget (assign_roles,
// assign_application_access). It is the use-case-side move of those controller
// checks: anchors manage any user; a non-anchor administrator must hold a
// user-write permission AND be able to access the target user's client
// (RequireUserAdmin) AND the target must be a CLIENT-scope user
// (blockNonClientTarget). The coarse permission stays implied by RequireUserAdmin
// itself (it re-checks CanWritePrincipals for non-anchors); no separate coarse
// check is left in these controllers because RequireUserAdmin subsumes it.
func requireUserAdmin(ctx context.Context, p *principal.Principal) error {
	ac := auth.FromContext(ctx)
	if err := auth.RequireUserAdmin(ac, clientIDPtr(p)); err != nil {
		return err
	}
	return blockNonClientTarget(ac, p)
}

// blockNonClientTarget stops a non-anchor administrator (client-admin) from
// acting on an ANCHOR- or PARTNER-scoped principal. Anchors are unrestricted.
// Mirrors principal/api.blockNonClientTarget — duplicated here so the use case
// can enforce the same "which kind of user" bound post-load.
func blockNonClientTarget(ac *auth.AuthContext, p *principal.Principal) error {
	if ac != nil && !ac.IsAnchor() && p != nil && p.Scope != principal.ScopeClient {
		return httperror.Forbidden("Client administrators can only manage client-scope users")
	}
	return nil
}

// clientIDPtr returns the principal's home client id pointer (nil for a
// clientless / platform principal).
func clientIDPtr(p *principal.Principal) *string {
	if p == nil {
		return nil
	}
	return p.ClientID
}
