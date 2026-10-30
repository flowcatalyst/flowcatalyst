package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// ClientApplicationIDs returns the applications a client is entitled to (an
// enabled client-config): the bound a non-anchor administrator is held to when
// assigning roles. Empty for no client, or with no repository wired.
func ClientApplicationIDs(ctx context.Context, configs *application.ClientConfigRepo, clientID string) (map[string]bool, error) {
	allowed := map[string]bool{}
	if clientID == "" || configs == nil {
		return allowed, nil
	}
	cfgs, err := configs.FindByClient(ctx, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_client_configs failed", err)
	}
	for _, c := range cfgs {
		if c.Enabled {
			allowed[string(c.ApplicationID)] = true
		}
	}
	return allowed, nil
}

// RequireAssignableRoles bounds a non-anchor (client administrator) role
// assignment: every role must exist (400 UNKNOWN_ROLE), be an application role
// (ApplicationID set, so not a platform role: 403 PLATFORM_ROLE_FORBIDDEN) and
// belong to an application in allowed, the target client's entitlements (403
// ROLE_APP_FORBIDDEN). A client administrator can therefore grant any role of
// any application the client is entitled to, not only the applications it
// holds itself. Anchors are not subject to it (callers skip it). The first
// offending role answers, as Java's Access.assignableRolesProblem.
func RequireAssignableRoles(ctx context.Context, roles *role.Repository, roleNames []string, allowed map[string]bool) error {
	for _, name := range roleNames {
		r, err := findRole(ctx, roles, name)
		if err != nil {
			return err
		}
		if r == nil {
			return usecase.Validation("UNKNOWN_ROLE", "role not found: "+name)
		}
		if r.ApplicationID == nil || r.IsPlatform() {
			return usecase.Authorization("PLATFORM_ROLE_FORBIDDEN",
				"client administrators cannot assign platform roles")
		}
		if !allowed[string(*r.ApplicationID)] {
			return usecase.Authorization("ROLE_APP_FORBIDDEN",
				"role belongs to an application the client cannot access")
		}
	}
	return nil
}

// applicationRoles confines an application's principal sync to that
// application's roles, whoever the caller (403 ROLE_APP_FORBIDDEN, Java's and
// Rust's message; F12): a role belongs to its application
// (role.OwningApplicationCode). A name that is not a role is not refused and
// not assigned (F15, owner ruling 2026-10-13): it is returned in skipped,
// whatever its prefix, and left out of assign. Both keep the order given.
func applicationRoles(ctx context.Context, roles *role.Repository, applicationCode string, roleNames []string) (assign []role.Name, skipped []string, err error) {
	for _, name := range roleNames {
		r, err := findRole(ctx, roles, name)
		if err != nil {
			return nil, nil, err
		}
		if r == nil {
			skipped = append(skipped, name)
			continue
		}
		if r.OwningApplicationCode() != applicationCode {
			return nil, nil, usecase.Authorization("ROLE_APP_FORBIDDEN",
				"role '"+name+"' does not belong to application '"+applicationCode+"'")
		}
		assign = append(assign, r.CanonicalName())
	}
	return assign, skipped, nil
}

// findRole looks a role up by name; an unwired repository refuses rather than
// admits.
func findRole(ctx context.Context, roles *role.Repository, name string) (*role.Role, error) {
	if roles == nil {
		return nil, usecase.Internal("REPO", "role repository not wired", nil)
	}
	if role.ValidateName(name) != nil {
		return nil, nil // a malformed name is not a role (F18)
	}
	r, err := roles.FindByName(ctx, name)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_role failed", err)
	}
	return r, nil
}
