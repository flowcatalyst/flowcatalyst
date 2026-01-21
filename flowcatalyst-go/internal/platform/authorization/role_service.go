package authorization

import (
	"context"
	"errors"
	"time"

	"go.flowcatalyst.tech/internal/common/tsid"
	"go.flowcatalyst.tech/internal/platform/principal"
	"go.flowcatalyst.tech/internal/platform/role"
)

// Super Admin role name constant (duplicated here to avoid import cycle)
const superAdminRoleName = "platform:super-admin"

// isSuperAdminRole checks if a role name is the Super Admin role.
func isSuperAdminRole(roleName string) bool {
	return roleName == superAdminRoleName
}

var (
	// ErrRoleNotFound indicates the role was not found
	ErrRoleNotFound = errors.New("role not found")

	// ErrRoleAlreadyAssigned indicates the role is already assigned to the principal
	ErrRoleAlreadyAssigned = errors.New("role already assigned to principal")

	// ErrRoleNotDefined indicates the role is not defined in the registry or database
	ErrRoleNotDefined = errors.New("role not defined")

	// ErrSuperAdminRestriction indicates super admin assignment restrictions
	ErrSuperAdminRestriction = errors.New("super admin role can only be assigned to anchor domain users")

	// ErrServiceAccountRestriction indicates the role cannot be assigned to service accounts
	ErrServiceAccountRestriction = errors.New("this role cannot be assigned to service accounts")
)

// AssignmentSource indicates how a role was assigned
type AssignmentSource string

const (
	AssignmentSourceManual  AssignmentSource = "MANUAL"
	AssignmentSourceIDPSync AssignmentSource = "IDP_SYNC"
	AssignmentSourceSystem  AssignmentSource = "SYSTEM"
)

// RoleService provides role assignment operations.
//
// Roles are stored embedded in the Principal document (MongoDB denormalized pattern).
//
// Roles can come from three sources:
//   - CODE: Defined in Go code (synced to auth_roles at startup)
//   - DATABASE: Created by administrators through the UI
//   - SDK: Registered by external applications via the SDK API
//
// Role validation checks both the auth_roles table (primary) and PermissionRegistry (fallback).
//
// Role format: {application}:{role-name}
// Example: "platform:tenant-admin", "logistics:dispatcher"
type RoleService struct {
	principalRepo principal.Repository
	roleRepo      role.Repository
	registry      *PermissionRegistry
}

// NewRoleService creates a new role service.
func NewRoleService(principalRepo principal.Repository, roleRepo role.Repository, registry *PermissionRegistry) *RoleService {
	if registry == nil {
		registry = DefaultRegistry
	}
	return &RoleService{
		principalRepo: principalRepo,
		roleRepo:      roleRepo,
		registry:      registry,
	}
}

// PrincipalRole represents a role assignment for API responses.
type PrincipalRole struct {
	PrincipalID      string    `json:"principalId"`
	RoleName         string    `json:"roleName"`
	AssignmentSource string    `json:"assignmentSource"`
	AssignedAt       time.Time `json:"assignedAt"`
}

// AssignRole assigns a role to a principal.
//
// Role must exist in either:
//   - auth_roles table (primary source - includes CODE, DATABASE, and SDK roles)
//   - PermissionRegistry (fallback for backwards compatibility)
//
// Returns the created role assignment.
func (s *RoleService) AssignRole(ctx context.Context, principalID, roleName string, source AssignmentSource) (*PrincipalRole, error) {
	// Find principal
	p, err := s.principalRepo.FindByID(ctx, principalID)
	if err != nil {
		if errors.Is(err, principal.ErrNotFound) {
			return nil, principal.ErrNotFound
		}
		return nil, err
	}

	// Validate role exists in database or registry
	if !s.IsValidRole(ctx, roleName) {
		return nil, ErrRoleNotDefined
	}

	// SECURITY: Super Admin role can only be assigned to anchor domain users
	if isSuperAdminRole(roleName) {
		if p.Type != principal.PrincipalTypeUser {
			return nil, ErrServiceAccountRestriction
		}
		if p.Scope != principal.UserScopeAnchor {
			return nil, ErrSuperAdminRestriction
		}
	}

	// Check if assignment already exists
	if p.HasRole(roleName) {
		return nil, ErrRoleAlreadyAssigned
	}

	// Add role to principal's embedded list
	assignment := principal.RoleAssignment{
		RoleID:           tsid.Generate(),
		RoleName:         roleName,
		AssignmentSource: string(source),
		AssignedAt:       time.Now(),
	}
	p.Roles = append(p.Roles, assignment)

	if err := s.principalRepo.Update(ctx, p); err != nil {
		return nil, err
	}

	return &PrincipalRole{
		PrincipalID:      principalID,
		RoleName:         roleName,
		AssignmentSource: assignment.AssignmentSource,
		AssignedAt:       assignment.AssignedAt,
	}, nil
}

// IsValidRole checks if a role name is valid (exists in DB or registry).
func (s *RoleService) IsValidRole(ctx context.Context, roleName string) bool {
	// Check database first (primary source after sync)
	if s.roleRepo != nil {
		r, err := s.roleRepo.FindByCode(ctx, roleName)
		if err == nil && r != nil {
			return true
		}
	}
	// Fallback to registry (for backwards compatibility during transition)
	return s.registry.HasRole(roleName)
}

// IsSuperAdminRole checks if a role is the Super Admin role.
// Super Admin has special restrictions - can only be assigned to anchor domain users.
func (s *RoleService) IsSuperAdminRole(roleName string) bool {
	return isSuperAdminRole(roleName)
}

// IsSuperAdmin checks if a principal has the Super Admin role.
func (s *RoleService) IsSuperAdmin(ctx context.Context, principalID string) (bool, error) {
	return s.HasRole(ctx, principalID, superAdminRoleName)
}

// RemoveRole removes a role from a principal.
func (s *RoleService) RemoveRole(ctx context.Context, principalID, roleName string) error {
	p, err := s.principalRepo.FindByID(ctx, principalID)
	if err != nil {
		if errors.Is(err, principal.ErrNotFound) {
			return principal.ErrNotFound
		}
		return err
	}

	// Find and remove the role
	found := false
	newRoles := make([]principal.RoleAssignment, 0, len(p.Roles))
	for _, r := range p.Roles {
		if r.RoleName == roleName {
			found = true
		} else {
			newRoles = append(newRoles, r)
		}
	}

	if !found {
		return ErrRoleNotFound
	}

	p.Roles = newRoles
	return s.principalRepo.Update(ctx, p)
}

// RemoveRolesBySource removes all roles from a principal that have a specific assignment source.
// Used for IDP sync to remove old IDP-assigned roles before adding new ones.
func (s *RoleService) RemoveRolesBySource(ctx context.Context, principalID string, source AssignmentSource) (int, error) {
	p, err := s.principalRepo.FindByID(ctx, principalID)
	if err != nil {
		if errors.Is(err, principal.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}

	sourceBefore := len(p.Roles)
	newRoles := make([]principal.RoleAssignment, 0, len(p.Roles))
	for _, r := range p.Roles {
		if r.AssignmentSource != string(source) {
			newRoles = append(newRoles, r)
		}
	}

	removed := sourceBefore - len(newRoles)
	if removed > 0 {
		p.Roles = newRoles
		if err := s.principalRepo.Update(ctx, p); err != nil {
			return 0, err
		}
	}

	return removed, nil
}

// FindRoleNamesByPrincipal returns all role names assigned to a principal.
func (s *RoleService) FindRoleNamesByPrincipal(ctx context.Context, principalID string) ([]string, error) {
	p, err := s.principalRepo.FindByID(ctx, principalID)
	if err != nil {
		if errors.Is(err, principal.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return p.GetRoleNames(), nil
}

// FindRoleDefinitionsByPrincipal returns all role definitions assigned to a principal.
// Includes full role metadata from PermissionRegistry.
func (s *RoleService) FindRoleDefinitionsByPrincipal(ctx context.Context, principalID string) ([]*RoleRecord, error) {
	roleNames, err := s.FindRoleNamesByPrincipal(ctx, principalID)
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

// FindAssignmentsByPrincipal returns all role assignments for a principal.
// Returns embedded roles converted to PrincipalRole for API compatibility.
func (s *RoleService) FindAssignmentsByPrincipal(ctx context.Context, principalID string) ([]*PrincipalRole, error) {
	p, err := s.principalRepo.FindByID(ctx, principalID)
	if err != nil {
		if errors.Is(err, principal.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	assignments := make([]*PrincipalRole, len(p.Roles))
	for i, r := range p.Roles {
		assignments[i] = &PrincipalRole{
			PrincipalID:      principalID,
			RoleName:         r.RoleName,
			AssignmentSource: r.AssignmentSource,
			AssignedAt:       r.AssignedAt,
		}
	}
	return assignments, nil
}

// HasRole checks if a principal has a specific role.
func (s *RoleService) HasRole(ctx context.Context, principalID, roleName string) (bool, error) {
	p, err := s.principalRepo.FindByID(ctx, principalID)
	if err != nil {
		if errors.Is(err, principal.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return p.HasRole(roleName), nil
}

// GetPermissionsForPrincipal returns all permission strings granted to a principal via their roles.
func (s *RoleService) GetPermissionsForPrincipal(ctx context.Context, principalID string) ([]string, error) {
	roleNames, err := s.FindRoleNamesByPrincipal(ctx, principalID)
	if err != nil {
		return nil, err
	}
	return s.registry.GetPermissionsForRoles(roleNames), nil
}

// GetAllRoles returns all available roles from the database.
func (s *RoleService) GetAllRoles(ctx context.Context) ([]*role.Role, error) {
	if s.roleRepo == nil {
		return nil, nil
	}
	return s.roleRepo.FindAll(ctx)
}

// GetRolesForApplication returns all roles for a specific application.
func (s *RoleService) GetRolesForApplication(applicationCode string) []*RoleRecord {
	return s.registry.GetRolesForApplication(applicationCode)
}

// GetRoleByName returns a role by name from the database.
func (s *RoleService) GetRoleByName(ctx context.Context, roleName string) (*role.Role, error) {
	if s.roleRepo == nil {
		return nil, nil
	}
	return s.roleRepo.FindByCode(ctx, roleName)
}
