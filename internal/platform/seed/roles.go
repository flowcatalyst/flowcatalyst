package seed

import "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"

// PlatformRoles returns the 12 built-in roles in the same order as
// fc-platform/src/role/entity.rs::roles::all(). Each role uses
// Source=CODE so the role-sync logic can identify them. Names match
// {applicationCode}:{roleName} (e.g. "platform:admin") to match the
// existing rows installed by the Rust impl.
//
// Exported so the role/operations/sync use case can diff against the
// catalogue without an inter-package indirection.
func PlatformRoles() []role.Role {
	mk := func(roleName, displayName, description string, perms []string) role.Role {
		r := role.New("platform", roleName, displayName)
		r.Description = ptr(description)
		r.Source = role.SourceCode
		r.Permissions = perms
		return *r
	}

	return []role.Role{
		// platform:super-admin
		mk("super-admin", "Platform Super Admin",
			"Full access to all platform operations",
			[]string{permAdminAll}),

		// platform:admin
		mk("admin", "Platform Admin",
			"Manages clients, applications, and platform configuration",
			[]string{
				permAdminClientRead, permAdminClientCreate, permAdminClientUpdate,
				permAdminClientActivate, permAdminClientSuspend, permAdminClientDeactivate,
				permAdminAnchorDomainRead, permAdminAnchorDomainCreate,
				permAdminAnchorDomainUpdate, permAdminAnchorDomainDelete,
				permAdminApplicationRead, permAdminApplicationCreate,
				permAdminApplicationUpdate, permAdminApplicationDelete,
				permAdminApplicationEnableClient, permAdminApplicationDisableClient,
				permAdminAuditLogRead, permAdminAuditLogExport,
				permAdminLoginAttemptRead,
				permDeveloperApplicationOpenAPIManage,
			}),

		// platform:admin-readonly
		mk("admin-readonly", "Platform Admin Read-Only",
			"View-only access to clients, applications, and platform configuration",
			[]string{
				permAdminClientRead,
				permAdminAnchorDomainRead,
				permAdminApplicationRead,
				permAdminAuditLogRead,
				permAdminLoginAttemptRead,
				permDeveloperApplicationOpenAPIView,
			}),

		// platform:iam-admin
		mk("iam-admin", "Platform IAM Admin",
			"Manages users, roles, and access control",
			[]string{
				permIAMUserRead, permIAMUserCreate, permIAMUserUpdate, permIAMUserDelete,
				permIAMUserActivate, permIAMUserDeactivate, permIAMUserAssignRoles,
				permIAMRoleRead, permIAMRoleCreate, permIAMRoleUpdate, permIAMRoleDelete,
				permIAMClientAccessGrant, permIAMClientAccessRevoke, permIAMClientAccessRead,
			}),

		// platform:iam-readonly
		mk("iam-readonly", "Platform IAM Read-Only",
			"View-only access to users and roles",
			[]string{
				permIAMUserRead,
				permIAMRoleRead,
				permIAMClientAccessRead,
			}),

		// platform:client-admin — delegated user management scoped to the
		// administrator's own client(s). Same user permissions as iam-admin
		// MINUS client-access grant/revoke and role authoring. Every action is
		// additionally scope-gated to the client(s) the admin can access (via
		// auth.RequireUserAdmin), and role assignment is bounded to the client's
		// own application roles — never platform roles. See
		// docs/auth-hardening-plan.md.
		mk("client-admin", "Client Administrator",
			"Manages users within the administrator's own client",
			[]string{
				permIAMUserRead, permIAMUserCreate, permIAMUserUpdate, permIAMUserDelete,
				permIAMUserActivate, permIAMUserDeactivate, permIAMUserAssignRoles,
				permIAMRoleRead,
			}),

		// platform:auth-admin
		mk("auth-admin", "Platform Auth Admin",
			"Manages authentication configuration",
			[]string{
				permAuthClientAuthConfigRead, permAuthClientAuthConfigCreate,
				permAuthClientAuthConfigUpdate, permAuthClientAuthConfigDelete,
				permAuthOAuthClientRead, permAuthOAuthClientCreate,
				permAuthOAuthClientUpdate, permAuthOAuthClientDelete,
				permAuthOAuthClientRegenerateSecret,
			}),

		// platform:auth-readonly
		mk("auth-readonly", "Platform Auth Read-Only",
			"View-only access to authentication configuration",
			[]string{
				permAuthClientAuthConfigRead,
				permAuthOAuthClientRead,
			}),

		// platform:ai-agent-readonly
		mk("ai-agent-readonly", "AI Agent Read-Only",
			"Read-only access to event types and subscriptions for AI agent integrations",
			[]string{
				permAdminEventTypeRead,
				permAdminSubscriptionRead,
			}),

		// platform:messaging-admin
		mk("messaging-admin", "Messaging Administrator",
			"Manages event types, subscriptions, dispatch jobs, and scheduled jobs",
			[]string{
				permAdminEventTypeRead, permAdminEventTypeCreate, permAdminEventTypeUpdate,
				permAdminEventTypeDelete, permAdminEventTypeArchive,
				permAdminEventTypeManageSchema, permAdminEventTypeSync,
				permAdminSubscriptionRead, permAdminSubscriptionCreate,
				permAdminSubscriptionUpdate, permAdminSubscriptionDelete, permAdminSubscriptionSync,
				permAdminDispatchPoolRead, permAdminDispatchPoolCreate,
				permAdminDispatchPoolUpdate, permAdminDispatchPoolDelete, permAdminDispatchPoolSync,
				permAdminConnectionRead, permAdminConnectionCreate,
				permAdminConnectionUpdate, permAdminConnectionDelete,
				permAdminEventRead, permAdminEventViewRaw,
				permAdminDispatchJobRead, permAdminDispatchJobViewRaw,
				permAdminScheduledJobRead, permAdminScheduledJobCreate,
				permAdminScheduledJobUpdate, permAdminScheduledJobDelete,
				permAdminScheduledJobPause, permAdminScheduledJobFire, permAdminScheduledJobSync,
				permAdminScheduledJobInstanceRead,
				permAdminProcessRead, permAdminProcessCreate, permAdminProcessUpdate,
				permAdminProcessDelete, permAdminProcessArchive, permAdminProcessSync,
			}),

		// platform:viewer
		mk("viewer", "Platform Viewer",
			"Read-only access across IAM, admin, and messaging",
			[]string{
				permIAMUserRead,
				permIAMRoleRead,
				permIAMClientAccessRead,
				permAdminClientRead,
				permAdminApplicationRead,
				permAdminEventRead,
				permAdminEventTypeRead,
				permAdminSubscriptionRead,
				permAdminDispatchJobRead,
				permAdminDispatchPoolRead,
				permAdminScheduledJobRead,
				permAdminScheduledJobInstanceRead,
				permAdminProcessRead,
				permAdminAuditLogRead,
				permAdminLoginAttemptRead,
			}),

		// platform:developer
		mk("developer", "Developer",
			"Developer portal: API documentation, accessible event types, and a self-service API credential for local testing",
			[]string{
				permDeveloperApplicationOpenAPIView,
				permDeveloperAPICredentialManage,
				permAdminEventTypeRead,
				permAdminProcessRead, permAdminProcessCreate,
				permAdminProcessUpdate, permAdminProcessDelete, permAdminProcessArchive,
			}),

		// platform:application-service
		mk("application-service", "Application Service Account",
			"Permissions for application service accounts (scoped to own application)",
			append([]string(nil), permsApplicationService...)),
	}
}

func ptr[T any](v T) *T { return &v }

// Reference permission constants that don't land in any built-in role
// today (CLIENT_MANAGE, EVENT_TYPE_MANAGE, etc.) so the unused-symbol
// linter leaves them — they're still part of the catalog, referenced by
// SDK consumers and future role definitions. The blank-identifier var is
// itself exempt from the linter, so listing them here keeps the catalog
// honest without forcing fake usage elsewhere.
var _ = []string{
	permAdminClientDelete, permAdminClientManage,
	permAdminAnchorDomainManage,
	permAdminApplicationManage, permAdminApplicationActivate, permAdminApplicationDeactivate,
	permAdminEventTypeManage,
	permAdminProcessManage,
	permAdminDispatchPoolManage,
	permAdminConnectionManage,
	permAdminSubscriptionManage,
	permAdminScheduledJobManage,
	permAdminIdentityProviderRead, permAdminIdentityProviderCreate,
	permAdminIdentityProviderUpdate, permAdminIdentityProviderDelete,
	permAdminIdentityProviderManage,
	permAdminEmailDomainMappingRead, permAdminEmailDomainMappingCreate,
	permAdminEmailDomainMappingUpdate, permAdminEmailDomainMappingDelete,
	permAdminEmailDomainMappingManage,
	permAdminServiceAccountRead, permAdminServiceAccountCreate,
	permAdminServiceAccountUpdate, permAdminServiceAccountDelete,
	permAdminServiceAccountManage,
	permAdminCorsOriginRead, permAdminCorsOriginCreate,
	permAdminCorsOriginDelete, permAdminCorsOriginManage,
	permAdminConfigRead, permAdminConfigUpdate,
	permAdminBatchEventsWrite, permAdminBatchDispatchJobsWrite, permAdminBatchAuditLogsWrite,
	permIAMUserManage, permIAMRoleManage, permIAMPermissionRead,
	permAuthClientAuthConfigManage, permAuthOAuthClientManage,
	permDeveloperApplicationOpenAPISync,
}
