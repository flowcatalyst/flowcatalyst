package platform

import (
	"go.flowcatalyst.tech/internal/platform/authorization"
)

// Permissions for application service accounts.
//
// These permissions allow applications/integrations to manage their own
// resources (event types, subscriptions, roles) programmatically.
//
// Note: Resource scoping is enforced at runtime - service accounts can only
// manage resources prefixed with their application's code.
var (
	// Event Type Management (scoped to application's prefix)
	AppEventTypeView = authorization.MustPermission(
		"platform", "application-service", "event-type", "view",
		"View event types for own application",
	)
	AppEventTypeCreate = authorization.MustPermission(
		"platform", "application-service", "event-type", "create",
		"Create event types for own application",
	)
	AppEventTypeUpdate = authorization.MustPermission(
		"platform", "application-service", "event-type", "update",
		"Update event types for own application",
	)
	AppEventTypeDelete = authorization.MustPermission(
		"platform", "application-service", "event-type", "delete",
		"Delete event types for own application",
	)

	// Subscription Management (scoped to application's prefix)
	AppSubscriptionView = authorization.MustPermission(
		"platform", "application-service", "subscription", "view",
		"View subscriptions for own application",
	)
	AppSubscriptionCreate = authorization.MustPermission(
		"platform", "application-service", "subscription", "create",
		"Create subscriptions for own application",
	)
	AppSubscriptionUpdate = authorization.MustPermission(
		"platform", "application-service", "subscription", "update",
		"Update subscriptions for own application",
	)
	AppSubscriptionDelete = authorization.MustPermission(
		"platform", "application-service", "subscription", "delete",
		"Delete subscriptions for own application",
	)

	// Role Management (scoped to application's prefix)
	AppRoleView = authorization.MustPermission(
		"platform", "application-service", "role", "view",
		"View roles for own application",
	)
	AppRoleCreate = authorization.MustPermission(
		"platform", "application-service", "role", "create",
		"Create roles for own application",
	)
	AppRoleUpdate = authorization.MustPermission(
		"platform", "application-service", "role", "update",
		"Update roles for own application",
	)
	AppRoleDelete = authorization.MustPermission(
		"platform", "application-service", "role", "delete",
		"Delete roles for own application",
	)

	// Permission Management (scoped to application's prefix)
	AppPermissionView = authorization.MustPermission(
		"platform", "application-service", "permission", "view",
		"View permissions for own application",
	)
	AppPermissionSync = authorization.MustPermission(
		"platform", "application-service", "permission", "sync",
		"Sync/register permissions for own application",
	)
)

// AllApplicationServicePermissions returns all application service permissions for registration.
func AllApplicationServicePermissions() []*authorization.PermissionRecord {
	return []*authorization.PermissionRecord{
		AppEventTypeView, AppEventTypeCreate, AppEventTypeUpdate, AppEventTypeDelete,
		AppSubscriptionView, AppSubscriptionCreate, AppSubscriptionUpdate, AppSubscriptionDelete,
		AppRoleView, AppRoleCreate, AppRoleUpdate, AppRoleDelete,
		AppPermissionView, AppPermissionSync,
	}
}
