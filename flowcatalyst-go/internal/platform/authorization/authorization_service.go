package authorization

import (
	"context"
	"errors"
	"fmt"

	"go.flowcatalyst.tech/internal/platform/principal"
)

var (
	// ErrForbidden indicates the principal does not have the required permission
	ErrForbidden = errors.New("forbidden: missing required permission")
)

// AuthorizationService provides authorization operations using code-first permissions and roles.
// It provides RBAC permission checks against PermissionRegistry.
//
// IMPORTANT: This service ONLY validates RBAC permissions.
// Tenant isolation and business rules MUST be enforced in application logic.
//
// Permission format: {application}:{context}:{aggregate}:{action}
// Example: "platform:tenant:user:create"
//
// Role format: {application}:{role-name}
// Example: "platform:tenant-admin"
type AuthorizationService struct {
	principalRepo principal.Repository
	registry      *PermissionRegistry
}

// NewAuthorizationService creates a new authorization service.
func NewAuthorizationService(principalRepo principal.Repository, registry *PermissionRegistry) *AuthorizationService {
	if registry == nil {
		registry = DefaultRegistry
	}
	return &AuthorizationService{
		principalRepo: principalRepo,
		registry:      registry,
	}
}

// HasPermission checks if a principal has a specific permission.
//
// Permission string format: {application}:{context}:{aggregate}:{action}
// Example: "platform:tenant:user:create"
func (s *AuthorizationService) HasPermission(ctx context.Context, principalID, permissionString string) (bool, error) {
	// Get all role names for this principal
	roleNames, err := s.GetRoleNames(ctx, principalID)
	if err != nil {
		return false, err
	}

	// Get all permissions granted by these roles
	grantedPermissions := s.registry.GetPermissionStringsForRolesSet(roleNames)

	return grantedPermissions[permissionString], nil
}

// HasPermissionParts checks if a principal has a specific permission using semantic parts.
// Builds the permission string from parts and checks it.
func (s *AuthorizationService) HasPermissionParts(ctx context.Context, principalID, application, context_, aggregate, action string) (bool, error) {
	permissionString := fmt.Sprintf("%s:%s:%s:%s", application, context_, aggregate, action)
	return s.HasPermission(ctx, principalID, permissionString)
}

// RequirePermission checks that a principal has a specific permission.
// Returns ErrForbidden if permission is not granted.
func (s *AuthorizationService) RequirePermission(ctx context.Context, principalID, permissionString string) error {
	has, err := s.HasPermission(ctx, principalID, permissionString)
	if err != nil {
		return err
	}
	if !has {
		return fmt.Errorf("%w: %s", ErrForbidden, permissionString)
	}
	return nil
}

// RequirePermissionDef checks that a principal has a specific permission.
// Returns ErrForbidden if permission is not granted.
func (s *AuthorizationService) RequirePermissionDef(ctx context.Context, principalID string, permission PermissionDefinition) error {
	return s.RequirePermission(ctx, principalID, permission.ToPermissionString())
}

// RequirePermissionParts checks that a principal has a specific permission using semantic parts.
// Returns ErrForbidden if permission is not granted.
func (s *AuthorizationService) RequirePermissionParts(ctx context.Context, principalID, application, context_, aggregate, action string) error {
	permissionString := fmt.Sprintf("%s:%s:%s:%s", application, context_, aggregate, action)
	return s.RequirePermission(ctx, principalID, permissionString)
}

// GetRoleNames returns all role names assigned to a principal.
// Reads from the embedded roles array on Principal.
//
// Role string format: {application}:{role-name}
// Example: "platform:tenant-admin"
func (s *AuthorizationService) GetRoleNames(ctx context.Context, principalID string) ([]string, error) {
	p, err := s.principalRepo.FindByID(ctx, principalID)
	if err != nil {
		if errors.Is(err, principal.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return p.GetRoleNames(), nil
}

// GetPermissions returns all permission strings granted to a principal (from all their roles).
//
// Permission string format: {application}:{context}:{aggregate}:{action}
// Example: "platform:tenant:user:create"
func (s *AuthorizationService) GetPermissions(ctx context.Context, principalID string) ([]string, error) {
	roleNames, err := s.GetRoleNames(ctx, principalID)
	if err != nil {
		return nil, err
	}
	return s.registry.GetPermissionsForRoles(roleNames), nil
}

// GetRoleDefinitions returns all role definitions assigned to a principal.
// Includes full role metadata (permissions, descriptions).
func (s *AuthorizationService) GetRoleDefinitions(ctx context.Context, principalID string) ([]*RoleRecord, error) {
	roleNames, err := s.GetRoleNames(ctx, principalID)
	if err != nil {
		return nil, err
	}

	var roles []*RoleRecord
	for _, roleName := range roleNames {
		if role, exists := s.registry.GetRole(roleName); exists {
			roles = append(roles, role)
		}
	}
	return roles, nil
}

// GetPermissionDefinitions returns all permission definitions granted to a principal.
// Includes full permission metadata (descriptions, parts).
func (s *AuthorizationService) GetPermissionDefinitions(ctx context.Context, principalID string) ([]*PermissionRecord, error) {
	permissions, err := s.GetPermissions(ctx, principalID)
	if err != nil {
		return nil, err
	}

	var permDefs []*PermissionRecord
	for _, permString := range permissions {
		if perm, exists := s.registry.GetPermission(permString); exists {
			permDefs = append(permDefs, perm)
		}
	}
	return permDefs, nil
}

// HasRole checks if a principal has a specific role.
func (s *AuthorizationService) HasRole(ctx context.Context, principalID, roleName string) (bool, error) {
	roleNames, err := s.GetRoleNames(ctx, principalID)
	if err != nil {
		return false, err
	}
	for _, r := range roleNames {
		if r == roleName {
			return true, nil
		}
	}
	return false, nil
}

// HasAnyRole checks if a principal has ANY of the specified roles.
func (s *AuthorizationService) HasAnyRole(ctx context.Context, principalID string, roleNames ...string) (bool, error) {
	principalRoles, err := s.GetRoleNames(ctx, principalID)
	if err != nil {
		return false, err
	}

	principalRolesSet := make(map[string]bool)
	for _, r := range principalRoles {
		principalRolesSet[r] = true
	}

	for _, roleName := range roleNames {
		if principalRolesSet[roleName] {
			return true, nil
		}
	}
	return false, nil
}

// HasAllRoles checks if a principal has ALL of the specified roles.
func (s *AuthorizationService) HasAllRoles(ctx context.Context, principalID string, roleNames ...string) (bool, error) {
	principalRoles, err := s.GetRoleNames(ctx, principalID)
	if err != nil {
		return false, err
	}

	principalRolesSet := make(map[string]bool)
	for _, r := range principalRoles {
		principalRolesSet[r] = true
	}

	for _, roleName := range roleNames {
		if !principalRolesSet[roleName] {
			return false, nil
		}
	}
	return true, nil
}
