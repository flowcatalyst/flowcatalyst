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
// (CanAccessScope). The coarse "may write/delete principals" permission is
// enforced at the controller BEFORE the target is loaded (docs/owner-rulings-todo.md
// #3, PR-3) — this function is purely the per-resource scope gate that runs
// after.
//
// A target outside the caller's client scope answers the SAME not-found error
// a genuinely missing id would — never 403 — so an unauthorized caller can't
// use the response to learn whether an id is real (PR-3(b)). blockNonClientTarget
// stays a 403: a client-admin CAN see the target exists (same or no client
// reach issue) but is the wrong KIND of administrator for it — a distinct,
// intentional authorization decision, not a tenancy boundary.
//
// Anchors pass both checks unconditionally (CanAccessScope is true for an
// anchor; the kind-of-user block only applies to non-anchors).
func requireUserResourceAccess(ctx context.Context, p *principal.Principal) error {
	ac := auth.FromContext(ctx)
	if err := blockNonClientTarget(ac, p); err != nil {
		return err
	}
	if !auth.CanAccessScope(ac, p.ClientID) {
		return httperror.NotFound("Principal", p.ID)
	}
	return nil
}

// requireUserAdmin is the post-load resource-level authorization shared by the
// user-administration operations (assign_roles, assign_application_access,
// developer_credential's admin branch). The coarse "may administer users"
// permission is enforced at the controller BEFORE the target is loaded (PR-3);
// this is purely the per-resource gate that runs after: the target must be a
// CLIENT-scope user (blockNonClientTarget) homed at a client the caller can
// access (CanAccessScope). Out-of-scope answers the same not-found error a
// missing id would (PR-3(b)) — see requireUserResourceAccess for the same
// blockNonClientTarget-stays-403 rationale.
func requireUserAdmin(ctx context.Context, p *principal.Principal) error {
	ac := auth.FromContext(ctx)
	if err := blockNonClientTarget(ac, p); err != nil {
		return err
	}
	if !auth.CanAccessScope(ac, p.ClientID) {
		return httperror.NotFound("User", p.ID)
	}
	return nil
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
