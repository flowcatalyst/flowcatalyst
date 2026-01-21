// Package platform contains platform-specific permission and role definitions.
package platform

import (
	"go.flowcatalyst.tech/internal/platform/authorization"
)

// IAM (Identity and Access Management) permissions.
// Controls access to users, roles, permissions, and service accounts.
var (
	// User Management
	UserView = authorization.MustPermission(
		"platform", "iam", "user", "view",
		"View user details and list users",
	)
	UserCreate = authorization.MustPermission(
		"platform", "iam", "user", "create",
		"Create new users",
	)
	UserUpdate = authorization.MustPermission(
		"platform", "iam", "user", "update",
		"Update user details and settings",
	)
	UserDelete = authorization.MustPermission(
		"platform", "iam", "user", "delete",
		"Delete or deactivate users",
	)

	// Role Management
	RoleView = authorization.MustPermission(
		"platform", "iam", "role", "view",
		"View role definitions and assignments",
	)
	RoleCreate = authorization.MustPermission(
		"platform", "iam", "role", "create",
		"Create new roles",
	)
	RoleUpdate = authorization.MustPermission(
		"platform", "iam", "role", "update",
		"Update role definitions and permissions",
	)
	RoleDelete = authorization.MustPermission(
		"platform", "iam", "role", "delete",
		"Delete roles",
	)

	// Permission Management (read-only for most, code-defined)
	PermissionView = authorization.MustPermission(
		"platform", "iam", "permission", "view",
		"View permission definitions",
	)

	// Service Account Management
	ServiceAccountView = authorization.MustPermission(
		"platform", "iam", "service-account", "view",
		"View service accounts",
	)
	ServiceAccountCreate = authorization.MustPermission(
		"platform", "iam", "service-account", "create",
		"Create service accounts",
	)
	ServiceAccountUpdate = authorization.MustPermission(
		"platform", "iam", "service-account", "update",
		"Update service accounts",
	)
	ServiceAccountDelete = authorization.MustPermission(
		"platform", "iam", "service-account", "delete",
		"Delete service accounts",
	)

	// Identity Provider (IDP) Management
	IdpManage = authorization.MustPermission(
		"platform", "iam", "idp", "manage",
		"Manage identity provider configurations (create, update, delete domain IDPs)",
	)
)

// AllIamPermissions returns all IAM permissions for registration.
func AllIamPermissions() []*authorization.PermissionRecord {
	return []*authorization.PermissionRecord{
		UserView, UserCreate, UserUpdate, UserDelete,
		RoleView, RoleCreate, RoleUpdate, RoleDelete,
		PermissionView,
		ServiceAccountView, ServiceAccountCreate, ServiceAccountUpdate, ServiceAccountDelete,
		IdpManage,
	}
}
