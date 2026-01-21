package platform

import (
	"go.flowcatalyst.tech/internal/platform/authorization"
)

// Role name constants
const (
	SuperAdminRoleName         = "platform:super-admin"
	IamAdminRoleName           = "platform:iam-admin"
	PlatformAdminRoleName      = "platform:platform-admin"
	MessagingAdminRoleName     = "platform:messaging-admin"
	ApplicationServiceRoleName = "platform:application-service"
)

// SuperAdminRole is the platform super administrator role - full access to everything.
// This role grants all permissions and bypasses most access checks.
// Should only be assigned to platform operators.
var SuperAdminRole = authorization.MustRole(
	"platform",
	"super-admin",
	[]authorization.PermissionDefinition{
		// IAM permissions
		UserView, UserCreate, UserUpdate, UserDelete,
		RoleView, RoleCreate, RoleUpdate, RoleDelete,
		PermissionView,
		ServiceAccountView, ServiceAccountCreate, ServiceAccountUpdate, ServiceAccountDelete,
		IdpManage,
		// Admin permissions
		ClientView, ClientCreate, ClientUpdate, ClientDelete,
		ApplicationView, ApplicationCreate, ApplicationUpdate, ApplicationDelete,
		ConfigView, ConfigUpdate,
		// Messaging permissions
		EventView, EventViewRaw,
		EventTypeView, EventTypeCreate, EventTypeUpdate, EventTypeDelete,
		SubscriptionView, SubscriptionCreate, SubscriptionUpdate, SubscriptionDelete,
		DispatchJobView, DispatchJobViewRaw, DispatchJobCreate, DispatchJobRetry,
		DispatchPoolView, DispatchPoolCreate, DispatchPoolUpdate, DispatchPoolDelete,
		// Application service permissions (super admin can manage all apps)
		AppEventTypeView, AppEventTypeCreate, AppEventTypeUpdate, AppEventTypeDelete,
		AppSubscriptionView, AppSubscriptionCreate, AppSubscriptionUpdate, AppSubscriptionDelete,
		AppRoleView, AppRoleCreate, AppRoleUpdate, AppRoleDelete,
		AppPermissionView, AppPermissionSync,
	},
	"Platform super administrator - full access to everything",
)

// IamAdminRole manages users, roles, permissions, and service accounts.
var IamAdminRole = authorization.MustRole(
	"platform",
	"iam-admin",
	[]authorization.PermissionDefinition{
		UserView, UserCreate, UserUpdate, UserDelete,
		RoleView, RoleCreate, RoleUpdate, RoleDelete,
		PermissionView,
		ServiceAccountView, ServiceAccountCreate, ServiceAccountUpdate, ServiceAccountDelete,
	},
	"IAM administrator - manages users, roles, and service accounts",
)

// PlatformAdminRole manages clients, applications, and platform configuration.
var PlatformAdminRole = authorization.MustRole(
	"platform",
	"platform-admin",
	[]authorization.PermissionDefinition{
		ClientView, ClientCreate, ClientUpdate, ClientDelete,
		ApplicationView, ApplicationCreate, ApplicationUpdate, ApplicationDelete,
		ConfigView, ConfigUpdate,
		IdpManage, // IDP management - configure authentication for domains
	},
	"Platform administrator - manages clients, applications, and identity providers",
)

// MessagingAdminRole manages event types, subscriptions, and dispatch jobs.
var MessagingAdminRole = authorization.MustRole(
	"platform",
	"messaging-admin",
	[]authorization.PermissionDefinition{
		EventTypeView, EventTypeCreate, EventTypeUpdate, EventTypeDelete,
		SubscriptionView, SubscriptionCreate, SubscriptionUpdate, SubscriptionDelete,
		DispatchJobView, DispatchJobCreate, DispatchJobRetry,
	},
	"Messaging administrator - manages event types and subscriptions",
)

// ApplicationServiceRole is automatically assigned to service accounts created for
// applications and integrations. It grants permissions to manage resources
// prefixed with the application's code (event types, subscriptions, roles).
//
// Resource scoping is enforced at runtime - even with these permissions,
// service accounts can only access/modify resources belonging to their
// linked application.
var ApplicationServiceRole = authorization.MustRole(
	"platform",
	"application-service",
	[]authorization.PermissionDefinition{
		// Event type management for own application
		AppEventTypeView, AppEventTypeCreate, AppEventTypeUpdate, AppEventTypeDelete,
		// Subscription management for own application
		AppSubscriptionView, AppSubscriptionCreate, AppSubscriptionUpdate, AppSubscriptionDelete,
		// Role management for own application
		AppRoleView, AppRoleCreate, AppRoleUpdate, AppRoleDelete,
		// Permission management for own application
		AppPermissionView, AppPermissionSync,
	},
	"Application service account - manages own application's resources",
)

// AllRoles returns all platform roles for registration.
func AllRoles() []*authorization.RoleRecord {
	return []*authorization.RoleRecord{
		SuperAdminRole,
		IamAdminRole,
		PlatformAdminRole,
		MessagingAdminRole,
		ApplicationServiceRole,
	}
}

// IsSuperAdminRole checks if a role name is the Super Admin role.
func IsSuperAdminRole(roleName string) bool {
	return roleName == SuperAdminRoleName
}
