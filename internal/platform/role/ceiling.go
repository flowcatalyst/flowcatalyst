package role

import (
	"context"
	"errors"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The role ceiling: nobody hands out platform authority they do not hold
// (docs/platform-bug-fixes.md F8 and F10; the Java RoleCeiling and the Rust
// role::ceiling make the same rule, with the same codes and wording).
//
//   - Roles (F8). A caller may add or remove a role only when it holds every
//     one of that role's platform permissions. The rule applies to the change:
//     roles the target keeps are not checked, so an administrator can still
//     edit the other roles of someone who holds more than they do. Removal
//     counts, so a lesser administrator cannot strip a super-admin.
//   - Permissions (F10). A caller may add a permission to a role, or remove
//     one from it (deleting the role removes them all), only when it holds
//     that permission. Otherwise the role rule is one edit away: add
//     `platform:*:*:*` to a role you already hold. Again only the change is
//     checked.
//   - Platform permissions only: those whose first segment is `platform`, the
//     authority an escalation reaches for and what `platform:*:*:*` covers. An
//     application's own permissions (`orders:order:create`) are held by nobody
//     outside that application's roles, so counting them would stop every
//     administrator assigning application roles; those stay bounded by the
//     application rules the callers already enforce.
//   - Matching is the enforcement rule (auth.Grants, segment wildcards). A
//     super-admin (`platform:*:*:*`) may assign anything.
//   - A role name that does not exist grants nothing, so it is never above
//     anyone; the callers refuse unknown names where they must.
//
// The system's own role writers (seeding, the bootstrap admin, fcdev init,
// application provisioning, the IdP role sync at login) do not run through
// this rule: they act for no caller.

// CeilingCode is the 403 code of a role refusal (Java and Rust use the same
// code).
const CeilingCode = "ROLE_ABOVE_CALLER"

// PermissionCeilingCode is the 403 code of a permission refusal (Java and Rust
// use the same code).
const PermissionCeilingCode = "PERMISSION_ABOVE_CALLER"

const platformPermissionPrefix = "platform:"

// PermissionsOf resolves a role name to its permissions; nil for a role that
// does not exist.
type PermissionsOf func(ctx context.Context, roleName string) ([]string, error)

// PermissionsFrom is the PermissionsOf of a role repository. A nil repository
// fails every lookup, so an unwired ceiling refuses rather than admits.
func PermissionsFrom(repo *Repository) PermissionsOf {
	return func(ctx context.Context, name string) ([]string, error) {
		if repo == nil {
			return nil, errors.New("role ceiling: no role repository wired")
		}
		r, err := repo.FindByName(ctx, name)
		if err != nil || r == nil {
			return nil, err
		}
		return r.Permissions, nil
	}
}

// PermissionsAboveCaller returns the platform permissions in perms that caller
// does not hold, in order and without repeats. A nil caller holds nothing.
func PermissionsAboveCaller(caller *auth.AuthContext, perms []string) []string {
	if caller.IsSuperAdmin() {
		return nil
	}
	var held []string
	if caller != nil {
		held = caller.Permissions
	}
	var above []string
	seen := map[string]struct{}{}
	for _, p := range perms {
		if !strings.HasPrefix(p, platformPermissionPrefix) || auth.Grants(held, p) {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		above = append(above, p)
	}
	return above
}

// RolesAboveCaller returns the roles in names whose platform permissions
// caller does not all hold, in the order given and without repeats.
func RolesAboveCaller(ctx context.Context, caller *auth.AuthContext, names []string, permissionsOf PermissionsOf) ([]string, error) {
	if caller.IsSuperAdmin() {
		return nil, nil
	}
	var above []string
	seen := map[string]struct{}{}
	for _, name := range names {
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		perms, err := permissionsOf(ctx, name)
		if err != nil {
			return nil, usecase.Internal("REPO", "role lookup failed", err)
		}
		if len(PermissionsAboveCaller(caller, perms)) > 0 {
			above = append(above, name)
		}
	}
	return above, nil
}

// RequireRolesWithinCaller refuses, 403 ROLE_ABOVE_CALLER naming the roles,
// when any role in changed is above caller. Pass ChangedRoles(before, after)
// for a role change.
func RequireRolesWithinCaller(ctx context.Context, caller *auth.AuthContext, changed []string, permissionsOf PermissionsOf) error {
	above, err := RolesAboveCaller(ctx, caller, changed, permissionsOf)
	if err != nil {
		return err
	}
	if len(above) == 0 {
		return nil
	}
	return usecase.Authorization(CeilingCode,
		"You may only assign or remove roles whose permissions you hold yourself: "+strings.Join(above, ", "))
}

// RequirePermissionsWithinCaller refuses, 403 PERMISSION_ABOVE_CALLER naming
// the permissions, when any platform permission in changed is one caller does
// not hold. Pass ChangedPermissions(before, after) for an edit, a new role's
// permissions for a create, a deleted role's for a delete.
func RequirePermissionsWithinCaller(caller *auth.AuthContext, changed []string) error {
	above := PermissionsAboveCaller(caller, changed)
	if len(above) == 0 {
		return nil
	}
	return usecase.Authorization(PermissionCeilingCode,
		"You may only add or remove permissions you hold yourself: "+strings.Join(above, ", "))
}

// ChangedPermissions is what an edit of a role's permissions from before to
// after adds or removes (the same set difference as ChangedRoles).
func ChangedPermissions(before, after []string) []string { return changed(before, after) }

// ChangedRoles is what a change from before to after adds or removes: after
// minus before, then before minus after, each in order and without repeats.
func ChangedRoles(before, after []string) []string { return changed(before, after) }

func changed(before, after []string) []string {
	inBefore := make(map[string]struct{}, len(before))
	for _, b := range before {
		inBefore[b] = struct{}{}
	}
	inAfter := make(map[string]struct{}, len(after))
	for _, a := range after {
		inAfter[a] = struct{}{}
	}
	var out []string
	seen := map[string]struct{}{}
	add := func(s string) {
		if _, dup := seen[s]; !dup {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	for _, a := range after {
		if _, ok := inBefore[a]; !ok {
			add(a)
		}
	}
	for _, b := range before {
		if _, ok := inAfter[b]; !ok {
			add(b)
		}
	}
	return out
}
