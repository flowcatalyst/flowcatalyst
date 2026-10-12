package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
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
		if r.ApplicationID == nil {
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

// requireApplicationRoles confines an application's principal sync to that
// application's roles, whoever the caller (403 ROLE_APP_FORBIDDEN, Java's and
// Rust's message). A role belongs to its application (role.OwningApplicationCode);
// a name that is not a role belongs to the application its prefix names. A
// name that is not a role and has no prefix is accepted, as before (it grants
// nothing).
func requireApplicationRoles(ctx context.Context, roles *role.Repository, applicationCode string, roleNames []string) error {
	for _, name := range roleNames {
		r, err := findRole(ctx, roles, name)
		if err != nil {
			return err
		}
		owner := ""
		if r != nil {
			owner = r.OwningApplicationCode()
		} else if prefix, _, found := strings.Cut(name, ":"); found {
			owner = prefix
		}
		if owner != "" && owner != applicationCode {
			return usecase.Authorization("ROLE_APP_FORBIDDEN",
				"role '"+name+"' does not belong to application '"+applicationCode+"'")
		}
	}
	return nil
}

// findRole looks a role up by name; an unwired repository refuses rather than
// admits.
func findRole(ctx context.Context, roles *role.Repository, name string) (*role.Role, error) {
	if roles == nil {
		return nil, usecase.Internal("REPO", "role repository not wired", nil)
	}
	r, err := roles.FindByName(ctx, name)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_role failed", err)
	}
	return r, nil
}
