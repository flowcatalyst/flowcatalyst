package authorization

import (
	"fmt"
	"log/slog"
	"sync"
)

// PermissionRegistry is an in-memory registry of all permission and role definitions.
//
// Permissions and roles are registered at startup from code-first definitions.
// The registry provides fast lookup of permissions and roles by their
// string representation, and validates that all role permissions reference
// valid permission definitions.
//
// This is the source of truth for all permissions and roles in the system.
type PermissionRegistry struct {
	mu          sync.RWMutex
	permissions map[string]*PermissionRecord // Permission string -> PermissionRecord
	roles       map[string]*RoleRecord       // Role string -> RoleRecord
	logger      *slog.Logger
}

// NewPermissionRegistry creates a new permission registry.
func NewPermissionRegistry(logger *slog.Logger) *PermissionRegistry {
	if logger == nil {
		logger = slog.Default()
	}
	return &PermissionRegistry{
		permissions: make(map[string]*PermissionRecord),
		roles:       make(map[string]*RoleRecord),
		logger:      logger,
	}
}

// DefaultRegistry is the default global permission registry.
// It should be initialized at application startup with RegisterAll().
var DefaultRegistry = NewPermissionRegistry(nil)

// RegisterPermission registers a permission definition.
// If a permission with the same string already exists, it is silently skipped.
func (r *PermissionRegistry) RegisterPermission(permission *PermissionRecord) {
	if permission == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := permission.ToPermissionString()
	if _, exists := r.permissions[key]; exists {
		r.logger.Debug("permission already registered, skipping", "permission", key)
		return
	}
	r.permissions[key] = permission
	r.logger.Debug("registered permission", "permission", key)
}

// RegisterRole registers a role definition.
// Validates that all role permissions reference existing permissions.
// If a role with the same string already exists, it is silently skipped.
func (r *PermissionRegistry) RegisterRole(role *RoleRecord) error {
	if role == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := role.ToRoleString()
	if _, exists := r.roles[key]; exists {
		r.logger.Debug("role already registered, skipping", "role", key)
		return nil
	}

	// Validate that all role permissions reference existing permissions
	for _, perm := range role.Permissions() {
		permString := perm.ToPermissionString()
		if _, exists := r.permissions[permString]; !exists {
			return fmt.Errorf("role %s references unknown permission: %s", key, permString)
		}
	}

	r.roles[key] = role
	r.logger.Debug("registered role", "role", key, "permissions", len(role.Permissions()))
	return nil
}

// RegisterRoleDynamic dynamically registers a role from the database or SDK.
// Unlike RegisterRole(), this does not validate permissions against the registry,
// as external applications may define their own permission schemes.
//
// If a role with the same name already exists, it will be updated.
func (r *PermissionRegistry) RegisterRoleDynamic(roleName string, permissionStrings []string, description string) error {
	if roleName == "" {
		return fmt.Errorf("cannot register role with empty name")
	}

	// Parse role name into application and role parts
	application, roleNamePart, err := ParseRoleString(roleName)
	if err != nil {
		return fmt.Errorf("invalid role name format: %w", err)
	}

	// Create permission records from strings (without validation)
	perms := make([]*PermissionRecord, 0, len(permissionStrings))
	for _, ps := range permissionStrings {
		pr, err := ParsePermissionString(ps)
		if err != nil {
			r.logger.Warn("invalid permission string in dynamic role", "role", roleName, "permission", ps)
			continue // Skip invalid permissions rather than failing
		}
		perms = append(perms, pr)
	}

	role := &RoleRecord{
		application: application,
		roleName:    roleNamePart,
		permissions: perms,
		description: description,
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.roles[roleName] = role
	r.logger.Debug("dynamically registered role", "role", roleName, "permissions", len(perms))
	return nil
}

// UnregisterRole unregisters a role. Used when roles are deleted from the database.
func (r *PermissionRegistry) UnregisterRole(roleName string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.roles[roleName]; exists {
		delete(r.roles, roleName)
		r.logger.Debug("unregistered role", "role", roleName)
		return true
	}
	return false
}

// GetPermission returns a permission definition by its string representation.
func (r *PermissionRegistry) GetPermission(permissionString string) (*PermissionRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	perm, exists := r.permissions[permissionString]
	return perm, exists
}

// GetRole returns a role definition by its string representation.
func (r *PermissionRegistry) GetRole(roleString string) (*RoleRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	role, exists := r.roles[roleString]
	return role, exists
}

// HasPermission checks if a permission exists.
func (r *PermissionRegistry) HasPermission(permissionString string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.permissions[permissionString]
	return exists
}

// HasRole checks if a role exists.
func (r *PermissionRegistry) HasRole(roleString string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.roles[roleString]
	return exists
}

// GetAllPermissions returns all registered permissions.
func (r *PermissionRegistry) GetAllPermissions() []*PermissionRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	perms := make([]*PermissionRecord, 0, len(r.permissions))
	for _, p := range r.permissions {
		perms = append(perms, p)
	}
	return perms
}

// GetAllRoles returns all registered roles.
func (r *PermissionRegistry) GetAllRoles() []*RoleRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	roles := make([]*RoleRecord, 0, len(r.roles))
	for _, role := range r.roles {
		roles = append(roles, role)
	}
	return roles
}

// GetPermissionsForRole returns all permissions granted by a role.
func (r *PermissionRegistry) GetPermissionsForRole(roleString string) []*PermissionRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	role, exists := r.roles[roleString]
	if !exists {
		return nil
	}
	return role.Permissions()
}

// GetPermissionsForRoles returns all unique permission strings granted by multiple roles.
func (r *PermissionRegistry) GetPermissionsForRoles(roleStrings []string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]bool)
	var result []string

	for _, roleString := range roleStrings {
		role, exists := r.roles[roleString]
		if !exists {
			continue
		}
		for _, perm := range role.Permissions() {
			ps := perm.ToPermissionString()
			if !seen[ps] {
				seen[ps] = true
				result = append(result, ps)
			}
		}
	}
	return result
}

// GetPermissionStringsForRoles returns all unique permission strings granted by multiple roles as a set.
func (r *PermissionRegistry) GetPermissionStringsForRolesSet(roleStrings []string) map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]bool)
	for _, roleString := range roleStrings {
		role, exists := r.roles[roleString]
		if !exists {
			continue
		}
		for _, perm := range role.Permissions() {
			result[perm.ToPermissionString()] = true
		}
	}
	return result
}

// GetRolesForApplication returns all roles for a specific application.
func (r *PermissionRegistry) GetRolesForApplication(applicationCode string) []*RoleRecord {
	if applicationCode == "" {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	prefix := applicationCode + ":"
	var result []*RoleRecord
	for roleString, role := range r.roles {
		if len(roleString) > len(prefix) && roleString[:len(prefix)] == prefix {
			result = append(result, role)
		}
	}
	return result
}

// PermissionCount returns the number of registered permissions.
func (r *PermissionRegistry) PermissionCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.permissions)
}

// RoleCount returns the number of registered roles.
func (r *PermissionRegistry) RoleCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.roles)
}

// Clear removes all registered permissions and roles.
// Used mainly for testing.
func (r *PermissionRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.permissions = make(map[string]*PermissionRecord)
	r.roles = make(map[string]*RoleRecord)
}
