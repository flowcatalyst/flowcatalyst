// Package auth provides the authenticated-request context and the
// permission-check helpers used by every write handler. Mirrors the
// Rust shared::authorization_service::checks namespace.
//
// Conventions (see docs/conventions.md §1):
//   - CanRead<Resource>(ctx)   for GET
//   - CanCreate/Update/Delete  for the specific verbs
//   - CanWrite<Resource>       for any of create/update/delete
//   - RequireAnchor(ctx)       for anchor-only endpoints
//   - IsAdmin(ctx)             anchor OR ADMIN_ALL permission
//
// The check functions return a usecase.Error (Kind=Authorization) on
// failure so handlers can httperror.Write(err) without branching.
package auth

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// Scope is the principal's top-level scope. Matches Rust UserScope.
type Scope string

const (
	ScopeAnchor  Scope = "ANCHOR"
	ScopePartner Scope = "PARTNER"
	ScopeClient  Scope = "CLIENT"
)

// AuthContext carries the authenticated principal across the request.
// Attached to the request context by the auth middleware; handlers and
// use cases retrieve it via FromContext.
type AuthContext struct {
	PrincipalID string
	Email       string
	Scope       Scope
	// Clients is the set of tenant IDs this principal can access.
	Clients []string
	// Roles is the set of role codes assigned to this principal.
	Roles []string
	// Applications is the set of application IDs in scope.
	Applications []string
	// Permissions is the flattened set of permission codes from all roles.
	Permissions []string
}

// IsAnchor reports whether the principal has anchor scope.
func (a *AuthContext) IsAnchor() bool { return a.Scope == ScopeAnchor }

// CanAccessClient reports whether the principal has access to a specific tenant.
func (a *AuthContext) CanAccessClient(clientID string) bool {
	if a.IsAnchor() {
		return true
	}
	for _, c := range a.Clients {
		if c == clientID {
			return true
		}
	}
	return false
}

// HasPermission reports whether the principal carries the named permission.
func (a *AuthContext) HasPermission(code string) bool {
	for _, p := range a.Permissions {
		if p == code {
			return true
		}
	}
	return false
}

// ctxKey is the private context key for AuthContext.
type ctxKey struct{}

// WithContext attaches an AuthContext to ctx. The auth middleware calls this.
func WithContext(ctx context.Context, a *AuthContext) context.Context {
	return context.WithValue(ctx, ctxKey{}, a)
}

// FromContext retrieves the AuthContext, or nil if none is attached.
func FromContext(ctx context.Context) *AuthContext {
	v, _ := ctx.Value(ctxKey{}).(*AuthContext)
	return v
}

// ── Check helpers ──────────────────────────────────────────────────────────

// RequireAnchor errors if the principal is not anchor-scoped.
func RequireAnchor(a *AuthContext) error {
	if a == nil {
		return usecase.Authorization("UNAUTHENTICATED", "authentication required")
	}
	if !a.IsAnchor() {
		return usecase.Authorization("ANCHOR_REQUIRED", "anchor scope required")
	}
	return nil
}

// IsAdmin returns nil if the principal is anchor-scoped or carries ADMIN_ALL.
func IsAdmin(a *AuthContext) error {
	if a == nil {
		return usecase.Authorization("UNAUTHENTICATED", "authentication required")
	}
	if a.IsAnchor() || a.HasPermission("ADMIN_ALL") {
		return nil
	}
	return usecase.Authorization("ADMIN_REQUIRED", "admin permission required")
}

// requirePermission is the generic helper.
func requirePermission(a *AuthContext, perm string) error {
	if a == nil {
		return usecase.Authorization("UNAUTHENTICATED", "authentication required")
	}
	if a.IsAnchor() || a.HasPermission(perm) {
		return nil
	}
	return usecase.Authorization("PERMISSION_REQUIRED", "permission required: "+perm)
}

// CanWritePermission is the public alias used by ad-hoc handlers (e.g.
// SDK batch ingest) where the permission name isn't covered by a typed
// Can* helper.
func CanWritePermission(a *AuthContext, perm string) error { return requirePermission(a, perm) }

// requireAny returns nil if the principal has ANY of perms.
func requireAny(a *AuthContext, perms ...string) error {
	if a == nil {
		return usecase.Authorization("UNAUTHENTICATED", "authentication required")
	}
	if a.IsAnchor() {
		return nil
	}
	for _, p := range perms {
		if a.HasPermission(p) {
			return nil
		}
	}
	return usecase.Authorization("PERMISSION_REQUIRED", "one of: "+joinPerms(perms))
}

func joinPerms(p []string) string {
	out := ""
	for i, s := range p {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

// ── EventType permission checks (the worked template) ─────────────────────
//
// The pattern is mechanical: each resource gets one set of these
// functions, named exactly as the Rust convention specifies. New
// subdomains follow this template verbatim.

// CanReadEventTypes — GET on event types.
func CanReadEventTypes(a *AuthContext) error {
	return requirePermission(a, "READ_EVENT_TYPES")
}

// CanCreateEventTypes — POST.
func CanCreateEventTypes(a *AuthContext) error {
	return requirePermission(a, "CREATE_EVENT_TYPES")
}

// CanUpdateEventTypes — PUT/PATCH.
func CanUpdateEventTypes(a *AuthContext) error {
	return requirePermission(a, "UPDATE_EVENT_TYPES")
}

// CanDeleteEventTypes — DELETE.
func CanDeleteEventTypes(a *AuthContext) error {
	return requirePermission(a, "DELETE_EVENT_TYPES")
}

// CanWriteEventTypes — POST/PUT/DELETE (any of the three).
func CanWriteEventTypes(a *AuthContext) error {
	return requireAny(a, "CREATE_EVENT_TYPES", "UPDATE_EVENT_TYPES", "DELETE_EVENT_TYPES")
}

// ── Connection permissions ───────────────────────────────────────────────
func CanReadConnections(a *AuthContext) error   { return requirePermission(a, "READ_CONNECTIONS") }
func CanCreateConnections(a *AuthContext) error { return requirePermission(a, "CREATE_CONNECTIONS") }
func CanUpdateConnections(a *AuthContext) error { return requirePermission(a, "UPDATE_CONNECTIONS") }
func CanDeleteConnections(a *AuthContext) error { return requirePermission(a, "DELETE_CONNECTIONS") }
func CanWriteConnections(a *AuthContext) error {
	return requireAny(a, "CREATE_CONNECTIONS", "UPDATE_CONNECTIONS", "DELETE_CONNECTIONS")
}

// ── Subscription permissions ─────────────────────────────────────────────
func CanReadSubscriptions(a *AuthContext) error   { return requirePermission(a, "READ_SUBSCRIPTIONS") }
func CanCreateSubscriptions(a *AuthContext) error { return requirePermission(a, "CREATE_SUBSCRIPTIONS") }
func CanUpdateSubscriptions(a *AuthContext) error { return requirePermission(a, "UPDATE_SUBSCRIPTIONS") }
func CanDeleteSubscriptions(a *AuthContext) error { return requirePermission(a, "DELETE_SUBSCRIPTIONS") }
func CanWriteSubscriptions(a *AuthContext) error {
	return requireAny(a, "CREATE_SUBSCRIPTIONS", "UPDATE_SUBSCRIPTIONS", "DELETE_SUBSCRIPTIONS")
}

// ── Dispatch pool permissions ────────────────────────────────────────────
func CanReadDispatchPools(a *AuthContext) error   { return requirePermission(a, "READ_DISPATCH_POOLS") }
func CanCreateDispatchPools(a *AuthContext) error { return requirePermission(a, "CREATE_DISPATCH_POOLS") }
func CanUpdateDispatchPools(a *AuthContext) error { return requirePermission(a, "UPDATE_DISPATCH_POOLS") }
func CanDeleteDispatchPools(a *AuthContext) error { return requirePermission(a, "DELETE_DISPATCH_POOLS") }
func CanWriteDispatchPools(a *AuthContext) error {
	return requireAny(a, "CREATE_DISPATCH_POOLS", "UPDATE_DISPATCH_POOLS", "DELETE_DISPATCH_POOLS")
}

// ── Process permissions ──────────────────────────────────────────────────
func CanReadProcesses(a *AuthContext) error   { return requirePermission(a, "READ_PROCESSES") }
func CanCreateProcesses(a *AuthContext) error { return requirePermission(a, "CREATE_PROCESSES") }
func CanUpdateProcesses(a *AuthContext) error { return requirePermission(a, "UPDATE_PROCESSES") }
func CanDeleteProcesses(a *AuthContext) error { return requirePermission(a, "DELETE_PROCESSES") }
func CanWriteProcesses(a *AuthContext) error {
	return requireAny(a, "CREATE_PROCESSES", "UPDATE_PROCESSES", "DELETE_PROCESSES")
}

// ── Application permissions ──────────────────────────────────────────────
func CanReadApplications(a *AuthContext) error   { return requirePermission(a, "READ_APPLICATIONS") }
func CanCreateApplications(a *AuthContext) error { return requirePermission(a, "CREATE_APPLICATIONS") }
func CanUpdateApplications(a *AuthContext) error { return requirePermission(a, "UPDATE_APPLICATIONS") }
func CanDeleteApplications(a *AuthContext) error { return requirePermission(a, "DELETE_APPLICATIONS") }
func CanWriteApplications(a *AuthContext) error {
	return requireAny(a, "CREATE_APPLICATIONS", "UPDATE_APPLICATIONS", "DELETE_APPLICATIONS")
}

// ── Role permissions ─────────────────────────────────────────────────────
func CanReadRoles(a *AuthContext) error   { return requirePermission(a, "READ_ROLES") }
func CanCreateRoles(a *AuthContext) error { return requirePermission(a, "CREATE_ROLES") }
func CanUpdateRoles(a *AuthContext) error { return requirePermission(a, "UPDATE_ROLES") }
func CanDeleteRoles(a *AuthContext) error { return requirePermission(a, "DELETE_ROLES") }
func CanWriteRoles(a *AuthContext) error {
	return requireAny(a, "CREATE_ROLES", "UPDATE_ROLES", "DELETE_ROLES")
}

// ── Service account permissions ──────────────────────────────────────────
func CanReadServiceAccounts(a *AuthContext) error {
	return requirePermission(a, "READ_SERVICE_ACCOUNTS")
}
func CanCreateServiceAccounts(a *AuthContext) error {
	return requirePermission(a, "CREATE_SERVICE_ACCOUNTS")
}
func CanUpdateServiceAccounts(a *AuthContext) error {
	return requirePermission(a, "UPDATE_SERVICE_ACCOUNTS")
}
func CanDeleteServiceAccounts(a *AuthContext) error {
	return requirePermission(a, "DELETE_SERVICE_ACCOUNTS")
}
func CanWriteServiceAccounts(a *AuthContext) error {
	return requireAny(a, "CREATE_SERVICE_ACCOUNTS", "UPDATE_SERVICE_ACCOUNTS", "DELETE_SERVICE_ACCOUNTS")
}

// ── Client (tenant) permissions ──────────────────────────────────────────
// Anchor-only by convention — tenant management is platform-owner work.
func CanReadClients(a *AuthContext) error   { return RequireAnchor(a) }
func CanCreateClients(a *AuthContext) error { return RequireAnchor(a) }
func CanUpdateClients(a *AuthContext) error { return RequireAnchor(a) }
func CanDeleteClients(a *AuthContext) error { return RequireAnchor(a) }
func CanWriteClients(a *AuthContext) error  { return RequireAnchor(a) }

// ── Principal (user) permissions ─────────────────────────────────────────
func CanReadPrincipals(a *AuthContext) error   { return requirePermission(a, "READ_PRINCIPALS") }
func CanCreatePrincipals(a *AuthContext) error { return requirePermission(a, "CREATE_PRINCIPALS") }
func CanUpdatePrincipals(a *AuthContext) error { return requirePermission(a, "UPDATE_PRINCIPALS") }
func CanDeletePrincipals(a *AuthContext) error { return requirePermission(a, "DELETE_PRINCIPALS") }
func CanWritePrincipals(a *AuthContext) error {
	return requireAny(a, "CREATE_PRINCIPALS", "UPDATE_PRINCIPALS", "DELETE_PRINCIPALS")
}

// ── Identity provider permissions ────────────────────────────────────────
// Anchor-only — IDPs are platform-level config.
func CanReadIdentityProviders(a *AuthContext) error  { return RequireAnchor(a) }
func CanWriteIdentityProviders(a *AuthContext) error { return RequireAnchor(a) }

// ── Scheduled job permissions ────────────────────────────────────────────
func CanReadScheduledJobs(a *AuthContext) error   { return requirePermission(a, "READ_SCHEDULED_JOBS") }
func CanCreateScheduledJobs(a *AuthContext) error { return requirePermission(a, "CREATE_SCHEDULED_JOBS") }
func CanUpdateScheduledJobs(a *AuthContext) error { return requirePermission(a, "UPDATE_SCHEDULED_JOBS") }
func CanDeleteScheduledJobs(a *AuthContext) error { return requirePermission(a, "DELETE_SCHEDULED_JOBS") }
func CanWriteScheduledJobs(a *AuthContext) error {
	return requireAny(a, "CREATE_SCHEDULED_JOBS", "UPDATE_SCHEDULED_JOBS", "DELETE_SCHEDULED_JOBS")
}
func CanFireScheduledJobs(a *AuthContext) error { return requirePermission(a, "FIRE_SCHEDULED_JOBS") }
