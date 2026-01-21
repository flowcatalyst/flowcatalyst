package platform

import (
	"go.flowcatalyst.tech/internal/platform/authorization"
)

// Platform administration permissions.
// Controls access to clients, applications, and platform configuration.
var (
	// Client Management
	ClientView = authorization.MustPermission(
		"platform", "admin", "client", "view",
		"View client details and list clients",
	)
	ClientCreate = authorization.MustPermission(
		"platform", "admin", "client", "create",
		"Create new clients",
	)
	ClientUpdate = authorization.MustPermission(
		"platform", "admin", "client", "update",
		"Update client details and settings",
	)
	ClientDelete = authorization.MustPermission(
		"platform", "admin", "client", "delete",
		"Delete or suspend clients",
	)

	// Application Management
	ApplicationView = authorization.MustPermission(
		"platform", "admin", "application", "view",
		"View application details and list applications",
	)
	ApplicationCreate = authorization.MustPermission(
		"platform", "admin", "application", "create",
		"Create new applications",
	)
	ApplicationUpdate = authorization.MustPermission(
		"platform", "admin", "application", "update",
		"Update application details and settings",
	)
	ApplicationDelete = authorization.MustPermission(
		"platform", "admin", "application", "delete",
		"Delete or deactivate applications",
	)

	// Platform Configuration
	ConfigView = authorization.MustPermission(
		"platform", "admin", "config", "view",
		"View platform configuration",
	)
	ConfigUpdate = authorization.MustPermission(
		"platform", "admin", "config", "update",
		"Update platform configuration",
	)
)

// AllAdminPermissions returns all admin permissions for registration.
func AllAdminPermissions() []*authorization.PermissionRecord {
	return []*authorization.PermissionRecord{
		ClientView, ClientCreate, ClientUpdate, ClientDelete,
		ApplicationView, ApplicationCreate, ApplicationUpdate, ApplicationDelete,
		ConfigView, ConfigUpdate,
	}
}
