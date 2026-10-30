package seed

import "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"

// PlatformRoles returns the built-in roles. Each role uses
// Source=CODE so the role-sync logic can identify them. Names match
// {applicationCode}:{roleName} (e.g. "platform:admin") to match the
// existing rows installed by earlier deployments.
//
// Exported so the role/operations/sync use case can diff against the
// catalogue without an inter-package indirection.
func PlatformRoles() []role.Role {
	mk := func(roleName, displayName, description string, perms []string) role.Role {
		r := role.New("platform", roleName, displayName)
		r.Description = new(description)
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
				permAdminDocsRead,
				permDeveloperApplicationOpenAPIManage,
				// These families existed in the catalogue but were held by no
				// role — harmless while anchor scope satisfied every gate,
				// locked out the moment it stopped.
				permAdminConfigRead, permAdminConfigUpdate,
				permAdminCorsOriginRead, permAdminCorsOriginCreate, permAdminCorsOriginDelete,
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
				permAdminDocsRead,
				permDeveloperApplicationOpenAPIView,
				permAdminConfigRead,
				permAdminCorsOriginRead,
			}),

		// platform:iam-admin
		mk("iam-admin", "Platform IAM Admin",
			"Manages users, roles, and access control",
			[]string{
				permIAMUserRead, permIAMUserCreate, permIAMUserUpdate, permIAMUserDelete,
				permIAMUserActivate, permIAMUserDeactivate, permIAMUserAssignRoles,
				permIAMUserManageAccess,
				permIAMRoleRead, permIAMRoleCreate, permIAMRoleUpdate, permIAMRoleDelete,
				permIAMClientAccessGrant, permIAMClientAccessRevoke, permIAMClientAccessRead,
				permAdminIdentityProviderRead, permAdminIdentityProviderCreate,
				permAdminIdentityProviderUpdate, permAdminIdentityProviderDelete,
				permAdminEmailDomainMappingRead, permAdminEmailDomainMappingCreate,
				permAdminEmailDomainMappingUpdate, permAdminEmailDomainMappingDelete,
			}),

		// platform:iam-readonly
		mk("iam-readonly", "Platform IAM Read-Only",
			"View-only access to users and roles",
			[]string{
				permIAMUserRead,
				permIAMRoleRead,
				permIAMClientAccessRead,
				permAdminIdentityProviderRead,
				permAdminEmailDomainMappingRead,
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
				permAdminConnectionUpdate, permAdminConnectionDelete, permAdminConnectionSync,
				permAdminEventRead, permAdminEventViewRaw,
				permAdminDispatchJobRead, permAdminDispatchJobViewRaw,
				permAdminDispatchJobRequeue, permAdminDispatchJobCancel,
				permAdminDispatchJobComplete, permAdminDispatchJobEditPayload,
				permAdminScheduledJobRead, permAdminScheduledJobCreate,
				permAdminScheduledJobUpdate, permAdminScheduledJobDelete,
				permAdminScheduledJobPause, permAdminScheduledJobFire, permAdminScheduledJobSync,
				permAdminScheduledJobInstanceRead,
				permAdminProcessRead, permAdminProcessCreate, permAdminProcessUpdate,
				permAdminProcessDelete, permAdminProcessArchive, permAdminProcessSync,
				permFunctionView, permFunctionManage,
				permFunctionDomainManage, permFunctionRouteManage,
				permFunctionTraceView, permFunctionTraceManage, permFunctionTraceViewBodies,
				permFunctionTraceIndefinite,
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
				// Not platform:function:trace:view: trace records hold what a
				// function printed and its business keys; the roles that
				// operate functions read them (docs/function-trace-design.md).
				permFunctionView,
				permAdminAuditLogRead,
				permAdminLoginAttemptRead,
				permAdminIdentityProviderRead,
				permAdminEmailDomainMappingRead,
				permAdminConfigRead,
				permAdminCorsOriginRead,
			}),

		// platform:router — the deployed message router's own role. Exactly
		// one permission: it fetches its configuration document and does
		// nothing else on the platform. Anchor-only (the document spans every
		// client's queues) and not client-delegable.
		mk("router", "Router",
			"Fetches the dispatch router configuration",
			[]string{permAdminDispatchPoolRead}),

		// platform:portal-administrator — CLIENT-delegable: assign to a
		// client administrator (manage their client's portal users in the
		// platform UI) or to the portal application's service account (the
		// /api/portal-users surface). Client confinement comes from the
		// holder's own client scope, not from the role.
		mk("portal-administrator", "Portal Administrator",
			"Manage the client's portal users: invite, suspend, and remove portal identities",
			[]string{
				permPortalUserView, permPortalUserManage,
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

		// platform:config-secrets-admin — the deliberate grant for reading
		// platform-config secrets in plaintext. platform:admin edits config but
		// cannot reveal a secret; super-admin's wildcard can. Give this role to
		// the few people (or break-glass accounts) who may, instead of widening
		// platform:admin.
		mk("config-secrets-admin", "Platform Config Secrets Administrator",
			"View and edit platform configuration and reveal its secrets (every reveal is audited)",
			[]string{permAdminConfigRead, permAdminConfigUpdate, permAdminConfigReveal}),

		// platform:function-publisher — CI / developer tooling (`fcdev fn
		// publish|promote`, docs/function-runner-plan.md §10). View, publish,
		// and promote; NOT manage (create/update/delete the function
		// resource itself is a separate, more privileged grant) and NOT
		// secret:manage.
		mk("function-publisher", "Function Publisher",
			"Publish and promote function versions",
			[]string{permFunctionView, permFunctionTraceView, permFunctionPublish, permFunctionPromote}),

		// platform:function-deployer — a CI account running `fcdev fn deploy
		// <addr> . --create --client <code>` (F26): resolve a client code
		// (client view), create the function under its application when absent
		// (function manage + application view), publish, wait READY, promote
		// `live`. Nothing else. Function manage also permits deleting a
		// function; the description says so.
		mk("function-deployer", "Function Deployer",
			"Deploy functions from CI: create a function if absent, publish a version and promote it. "+
				"Includes function manage, which also permits DELETING functions; "+
				"use function-publisher to deploy functions that already exist.",
			[]string{
				permFunctionView, permFunctionManage, permFunctionPublish, permFunctionPromote,
				permAdminClientRead, permAdminApplicationRead,
			}),

		// platform:function-runner — the deployed function runner's own
		// role (docs/function-runner-plan.md §3, §6.5). One permission,
		// same posture as platform:router: it polls its own control plane
		// (/control/functions/desired, /heartbeat, /artifacts/{digest},
		// /events — plan §8.3) and does nothing else on the platform.
		mk("function-runner", "Function Runner",
			"Fetches function runner desired state and reports heartbeats",
			[]string{permFunctionRunnerControl}),

		// The ingestion roles (docs/ingestion-design.md section 4). Admin holds
		// every human grant but the runtime's, route included (owner ruling
		// 2026-10-25: camel routes are written by a pipeline or by an
		// ingestion administrator, who pushes them with fcdev ingest apply).
		// runtime-identity and bind-secrets (what the runtime reads with its
		// own identity, where named secrets are sent) are held by the admin
		// and, for the routes a pipeline writes, the route author.
		mk("ingestion-admin", "Ingestion Administrator",
			"Manages ingestions: create, change, delete, make versions live, test connections, apply sets and write camel routes",
			[]string{
				permIngestionView, permIngestionCreate, permIngestionUpdate, permIngestionDelete,
				permIngestionPromote, permIngestionTest, permIngestionSetApply, permIngestionRoute,
				permIngestionRuntimeIdentity, permIngestionBindSecrets,
				// Switches tracing on and reads what it captured, bodies
				// included (docs/ingestion-trace-design.md decision 8).
				permIngestionTraceManage, permIngestionTraceView, permIngestionTraceViewBodies,
			}),

		// The operator reads trace records with headers, bodies and error
		// text withheld; the viewer, and the pipeline's route author below,
		// read none.
		mk("ingestion-operator", "Ingestion Operator",
			"Views ingestions, changes them, pauses and resumes them, and tests their connections",
			[]string{permIngestionView, permIngestionUpdate, permIngestionTest, permIngestionTraceView}),

		mk("ingestion-viewer", "Ingestion Viewer",
			"View-only access to ingestions, their versions, history and failures",
			[]string{permIngestionView}),

		// The pipeline role (owner ruling 2026-10-25): route authoring runs
		// code on the runtime's network and identity, so it is held by a
		// pipeline's service account. The route permission counts only for an
		// anchor-tier SERVICE principal (or an ingestion administrator, who
		// holds delete): a user given this role cannot write a route
		// (ingestion.RequireKindPermission). It writes drafts: without promote
		// a create or an update is saved as a version that is not live, and
		// someone who holds promote (the admin) makes it live; it cannot
		// delete. bind-secrets lets it save a route, every text setting of
		// which counts as a secret name.
		mk("ingestion-route-author", "Ingestion Route Author (pipeline)",
			"For a pipeline's service account: writes ingestions of source kinds that run a route (camel-route) as drafts an administrator makes live; refused to users",
			[]string{permIngestionView, permIngestionCreate, permIngestionUpdate, permIngestionRoute, permIngestionBindSecrets}),

		// The ingestion runtime's own role: same posture as platform:router
		// and platform:function-runner, one permission and nothing else.
		mk("ingestion-runtime", "Ingestion Runtime",
			"Polls for ingestions to run and reports their state",
			[]string{permIngestionRuntimeOperate}),
	}
}

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
	permAdminConfigRead, permAdminConfigUpdate, permAdminConfigReveal,
	permAdminBatchEventsWrite, permAdminBatchDispatchJobsWrite, permAdminBatchAuditLogsWrite,
	permAdminDispatchJobRedriveComplete,
	permIAMUserManage, permIAMRoleManage, permIAMPermissionRead,
	permAuthClientAuthConfigManage, permAuthOAuthClientManage,
	permDeveloperApplicationOpenAPISync,
	permFunctionSecretManage, permFunctionVersionInvoke,
}
