/**
 * Repositories
 *
 * Data access layer for domain entities.
 */

export {
	type PrincipalRepository,
	createPrincipalRepository,
} from './principal-repository.js';

export {
	type ClientRepository,
	createClientRepository,
} from './client-repository.js';

export {
	type AnchorDomainRepository,
	createAnchorDomainRepository,
} from './anchor-domain-repository.js';

export {
	type ApplicationRepository,
	createApplicationRepository,
	type ApplicationClientConfigRepository,
	createApplicationClientConfigRepository,
} from './application-repository.js';

export {
	type RoleRepository,
	createRoleRepository,
	type PermissionRepository,
	createPermissionRepository,
	type NewAuthRole,
	type NewAuthPermission,
} from './role-repository.js';

export {
	type ClientAccessGrantRepository,
	createClientAccessGrantRepository,
} from './client-access-grant-repository.js';

export {
	type ClientAuthConfigRepository,
	createClientAuthConfigRepository,
} from './client-auth-config-repository.js';

export {
	type OAuthClientRepository,
	createOAuthClientRepository,
} from './oauth-client-repository.js';

export {
	type AuditLogRepository,
	type AuditLogFilters,
	type PaginatedAuditLogs,
	type PaginationOptions,
	createAuditLogRepository,
} from './audit-log-repository.js';
