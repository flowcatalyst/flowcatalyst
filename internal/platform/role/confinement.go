package role

import (
	"log/slog"
	"strings"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// Confinement: a role may hold only its own application's permissions
// (docs/platform-bug-fixes.md F12; Java Role.confined, security-fixes S1.5,
// and Rust role::operations::require_confined, same code and wording). A
// permission's first segment names its application, so a `platform:`
// permission, the super-admin wildcard included, can only sit on a `platform:`
// role, and an application's SDK credential cannot mint platform authority by
// syncing `myapp:admin` with `platform:*:*:*`.
//
//   - Only what a write adds is checked, never what a role already holds
//     (Rust's rule): a legacy role holding another application's permissions
//     still loads, keeps them through edits that do not add them, and can have
//     them removed. Removal is never refused.
//   - The admin API (create, update, grant) lets a super-admin cross
//     applications; the SDK role sync and the platform role sync never do.
//   - It runs before the permission ceiling (Java's order), so for anyone but a
//     super-admin the ceiling is left bounding platform permissions on
//     `platform:` roles.

// OutsideApplicationCode is the 400 code of a confinement refusal (Java and
// Rust use the same code).
const OutsideApplicationCode = "PERMISSION_OUTSIDE_APPLICATION"

// OwningApplicationCode is the application the role belongs to: its
// ApplicationCode, or on a legacy row without one the first segment of its
// name.
func (r Role) OwningApplicationCode() string {
	if strings.TrimSpace(r.ApplicationCode) != "" {
		return r.ApplicationCode
	}
	app, _, _ := strings.Cut(r.Name, ":")
	return app
}

// PermissionsOutsideApplication returns the permissions in perms whose first
// segment is not applicationCode, in order and without repeats.
func PermissionsOutsideApplication(applicationCode string, perms []string) []string {
	var outside []string
	seen := map[string]struct{}{}
	for _, p := range perms {
		first, _, _ := strings.Cut(p, ":")
		if first == applicationCode {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		outside = append(outside, p)
	}
	return outside
}

// RequirePermissionsInApplication refuses, 400 PERMISSION_OUTSIDE_APPLICATION
// naming the first offender, when a permission in added does not belong to
// applicationCode. Pass AddedPermissions(before, after) for an edit and a new
// role's permissions for a create. crossApplication lifts the rule (a
// super-admin through the admin API); the crossing is logged.
func RequirePermissionsInApplication(applicationCode string, added []string, crossApplication bool) error {
	outside := PermissionsOutsideApplication(applicationCode, added)
	if len(outside) == 0 {
		return nil
	}
	if crossApplication {
		slog.Info("super-admin put another application's permissions on a role",
			"application", applicationCode, "permissions", outside)
		return nil
	}
	return usecase.Validation(OutsideApplicationCode,
		"Permission '"+outside[0]+"' does not belong to application '"+applicationCode+
			"'; a role may only hold its own application's permissions")
}

// AddedPermissions is what an edit of a role's permissions from before to
// after adds: after minus before, in order and without repeats.
func AddedPermissions(before, after []string) []string {
	inBefore := make(map[string]struct{}, len(before))
	for _, b := range before {
		inBefore[b] = struct{}{}
	}
	var out []string
	seen := map[string]struct{}{}
	for _, a := range after {
		if _, held := inBefore[a]; held {
			continue
		}
		if _, dup := seen[a]; dup {
			continue
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}
	return out
}
