/**
 * Repositories
 *
 * Data access layer for domain entities.
 */

export {
	type PrincipalRepository,
	type PrincipalFilters,
	createPrincipalRepository,
} from "./principal-repository.js";

export {
	type ClientRepository,
	createClientRepository,
} from "./client-repository.js";

export {
	type AnchorDomainRepository,
	createAnchorDomainRepository,
} from "./anchor-domain-repository.js";

export {
	type ApplicationRepository,
	createApplicationRepository,
	type ApplicationClientConfigRepository,
	createApplicationClientConfigRepository,
} from "./application-repository.js";

export {
	type RoleRepository,
	createRoleRepository,
	type PermissionRepository,
	createPermissionRepository,
	type NewAuthRole,
	type NewAuthPermission,
} from "./role-repository.js";

export {
	type ClientAccessGrantRepository,
	createClientAccessGrantRepository,
} from "./client-access-grant-repository.js";

export {
	type ClientAuthConfigRepository,
	createClientAuthConfigRepository,
} from "./client-auth-config-repository.js";

export {
	type OAuthClientRepository,
	createOAuthClientRepository,
} from "./oauth-client-repository.js";

export {
	type EventTypeRepository,
	type EventTypeFilters,
	createEventTypeRepository,
} from "./event-type-repository.js";

export {
	type DispatchPoolRepository,
	type DispatchPoolFilters,
	createDispatchPoolRepository,
} from "./dispatch-pool-repository.js";

export {
	type SubscriptionRepository,
	type SubscriptionFilters,
	createSubscriptionRepository,
} from "./subscription-repository.js";

export {
	type AuditLogRepository,
	type AuditLogFilters,
	type PaginatedAuditLogs,
	type PaginationOptions,
	createAuditLogRepository,
} from "./audit-log-repository.js";

export {
	type EventReadRepository,
	type EventReadFilters,
	type EventReadPagination,
	type PagedEventReadResult,
	type EventFilterOptionsRequest,
	type EventFilterOptions,
	createEventReadRepository,
} from "./event-read-repository.js";

export {
	type IdentityProviderRepository,
	createIdentityProviderRepository,
} from "./identity-provider-repository.js";

export {
	type EmailDomainMappingRepository,
	createEmailDomainMappingRepository,
} from "./email-domain-mapping-repository.js";

export {
	type IdpRoleMappingRepository,
	createIdpRoleMappingRepository,
} from "./idp-role-mapping-repository.js";

export {
	type OidcLoginStateRepository,
	type OidcLoginState,
	createOidcLoginStateRepository,
} from "./oidc-login-state-repository.js";

export {
	type ServiceAccountRepository,
	createServiceAccountRepository,
} from "./service-account-repository.js";

export {
	type PlatformConfigRepository,
	createPlatformConfigRepository,
} from "./platform-config-repository.js";

export {
	type PlatformConfigAccessRepository,
	createPlatformConfigAccessRepository,
} from "./platform-config-access-repository.js";

export {
	type CorsAllowedOriginRepository,
	createCorsAllowedOriginRepository,
} from "./cors-allowed-origin-repository.js";

export {
	type DispatchJobReadRepository,
	type DispatchJobReadFilters,
	type DispatchJobReadPagination,
	type PagedDispatchJobReadResult,
	type DispatchJobFilterOptionsRequest,
	type DispatchJobFilterOptions,
	createDispatchJobReadRepository,
} from "./dispatch-job-read-repository.js";

export {
	type LoginAttemptRepository,
	type LoginAttemptFilters,
	type PaginatedLoginAttempts,
	type LoginAttemptPaginationOptions,
	createLoginAttemptRepository,
} from "./login-attempt-repository.js";
