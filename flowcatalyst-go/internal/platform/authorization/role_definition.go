package authorization

import (
	"fmt"
)

// RoleDefinition defines a role in the FlowCatalyst system.
//
// Roles follow the structure: {application}:{role-name}
//
// Examples:
//   - platform:admin
//   - platform:iam-admin
//   - platform:messaging-admin
//   - tms:dispatcher
//   - tms:warehouse-manager
//
// Each role maps to a set of permission strings.
// All parts must be lowercase alphanumeric with hyphens allowed.
type RoleDefinition interface {
	// Application returns the application code (e.g., "platform", "tms")
	Application() string

	// RoleName returns the role name within app (e.g., "admin", "dispatcher")
	RoleName() string

	// Permissions returns the permissions this role grants
	Permissions() []*PermissionRecord

	// Description returns a human-readable description
	Description() string

	// ToRoleString generates the string representation of this role.
	// Format: {application}:{role-name}
	ToRoleString() string

	// PermissionStrings returns the permission strings for this role.
	PermissionStrings() []string
}

// RoleRecord is a concrete implementation of RoleDefinition.
type RoleRecord struct {
	application string
	roleName    string
	permissions []*PermissionRecord
	description string
}

// NewRole creates a new role definition.
// All parts must be lowercase alphanumeric with hyphens allowed (not at start/end).
func NewRole(application, roleName string, permissions []PermissionDefinition, description string) (*RoleRecord, error) {
	if err := validatePart(application, "application"); err != nil {
		return nil, err
	}
	if err := validatePart(roleName, "roleName"); err != nil {
		return nil, err
	}
	if description == "" {
		return nil, fmt.Errorf("description cannot be empty")
	}

	// Convert PermissionDefinition to PermissionRecord
	perms := make([]*PermissionRecord, 0, len(permissions))
	for _, p := range permissions {
		if p == nil {
			return nil, fmt.Errorf("permission cannot be nil")
		}
		// Type assert to PermissionRecord if possible, otherwise create new
		if pr, ok := p.(*PermissionRecord); ok {
			perms = append(perms, pr)
		} else {
			pr, err := NewPermission(p.Application(), p.Context(), p.Aggregate(), p.Action(), p.Description())
			if err != nil {
				return nil, fmt.Errorf("invalid permission in role: %w", err)
			}
			perms = append(perms, pr)
		}
	}

	return &RoleRecord{
		application: application,
		roleName:    roleName,
		permissions: perms,
		description: description,
	}, nil
}

// MustRole creates a new role definition, panicking on error.
// Use this for compile-time defined roles where validation errors indicate a bug.
func MustRole(application, roleName string, permissions []PermissionDefinition, description string) *RoleRecord {
	r, err := NewRole(application, roleName, permissions, description)
	if err != nil {
		panic(fmt.Sprintf("invalid role definition: %v", err))
	}
	return r
}

// NewRoleFromStrings creates a role from permission strings.
// Use this only when you don't have PermissionDefinition instances available.
func NewRoleFromStrings(application, roleName string, permissionStrings []string, description string) (*RoleRecord, error) {
	if err := validatePart(application, "application"); err != nil {
		return nil, err
	}
	if err := validatePart(roleName, "roleName"); err != nil {
		return nil, err
	}
	if description == "" {
		return nil, fmt.Errorf("description cannot be empty")
	}

	perms := make([]*PermissionRecord, 0, len(permissionStrings))
	for _, ps := range permissionStrings {
		pr, err := ParsePermissionString(ps)
		if err != nil {
			return nil, fmt.Errorf("invalid permission string %q: %w", ps, err)
		}
		perms = append(perms, pr)
	}

	return &RoleRecord{
		application: application,
		roleName:    roleName,
		permissions: perms,
		description: description,
	}, nil
}

// Application returns the application code
func (r *RoleRecord) Application() string {
	return r.application
}

// RoleName returns the role name within app
func (r *RoleRecord) RoleName() string {
	return r.roleName
}

// Permissions returns the permissions this role grants
func (r *RoleRecord) Permissions() []*PermissionRecord {
	return r.permissions
}

// Description returns the human-readable description
func (r *RoleRecord) Description() string {
	return r.description
}

// ToRoleString generates the string representation.
// Format: {application}:{role-name}
func (r *RoleRecord) ToRoleString() string {
	return fmt.Sprintf("%s:%s", r.application, r.roleName)
}

// PermissionStrings returns the permission strings for this role.
func (r *RoleRecord) PermissionStrings() []string {
	strs := make([]string, len(r.permissions))
	for i, p := range r.permissions {
		strs[i] = p.ToPermissionString()
	}
	return strs
}

// String returns a human-readable representation
func (r *RoleRecord) String() string {
	return fmt.Sprintf("%s (%d permissions: %s)", r.ToRoleString(), len(r.permissions), r.description)
}

// ParseRoleString parses a role string to extract application and role name.
// Format: application:role-name
func ParseRoleString(roleString string) (application, roleName string, err error) {
	parts := splitPermissionString(roleString)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid role string format (expected at least 2 parts): %s", roleString)
	}
	// First part is application, rest is role name (may contain colons)
	application = parts[0]
	if len(parts) == 2 {
		roleName = parts[1]
	} else {
		// Join remaining parts with colon (e.g., "dispatch:admin" -> "dispatch:admin")
		roleName = parts[1]
		for i := 2; i < len(parts); i++ {
			roleName += ":" + parts[i]
		}
	}
	return application, roleName, nil
}

// ExtractApplicationCode extracts the application code from a role string.
// Role format: {application}:{subdomain}:{role-name} or {application}:{role-name}
//
// Examples:
//   - "operant:dispatch:admin" → "operant"
//   - "platform:admin" → "platform"
func ExtractApplicationCode(roleString string) string {
	if roleString == "" {
		return ""
	}
	for i := 0; i < len(roleString); i++ {
		if roleString[i] == ':' {
			return roleString[:i]
		}
	}
	return roleString
}

// GetDisplayName returns the display name for a role (without application prefix).
// Role format: {application}:{display-name}
//
// Examples:
//   - "operant:dispatch:admin" → "dispatch:admin"
//   - "platform:admin" → "admin"
func GetDisplayName(roleString string) string {
	if roleString == "" {
		return ""
	}
	for i := 0; i < len(roleString); i++ {
		if roleString[i] == ':' {
			if i+1 < len(roleString) {
				return roleString[i+1:]
			}
			return ""
		}
	}
	return roleString
}

// ExtractApplicationCodes extracts unique application codes from a collection of role strings.
func ExtractApplicationCodes(roleStrings []string) []string {
	if len(roleStrings) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var result []string
	for _, role := range roleStrings {
		code := ExtractApplicationCode(role)
		if code != "" && !seen[code] {
			seen[code] = true
			result = append(result, code)
		}
	}
	return result
}

// FilterRolesForApplication filters roles to only those belonging to an application.
func FilterRolesForApplication(roleStrings []string, applicationCode string) []string {
	if len(roleStrings) == 0 || applicationCode == "" {
		return nil
	}
	prefix := applicationCode + ":"
	var result []string
	for _, role := range roleStrings {
		if len(role) > len(prefix) && role[:len(prefix)] == prefix {
			result = append(result, role)
		}
	}
	return result
}
