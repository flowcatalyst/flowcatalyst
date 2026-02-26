/**
 * Repository creation — instantiates all platform repositories.
 */

import {
	createPrincipalRepository,
	createAnchorDomainRepository,
	createClientRepository,
	createApplicationRepository,
	createApplicationClientConfigRepository,
	createRoleRepository,
	createPermissionRepository,
	createClientAccessGrantRepository,
	createClientAuthConfigRepository,
	createOAuthClientRepository,
	createAuditLogRepository,
	createEventTypeRepository,
	createDispatchPoolRepository,
	createSubscriptionRepository,
	createEventReadRepository,
	createDispatchJobReadRepository,
	createIdentityProviderRepository,
	createEmailDomainMappingRepository,
	createIdpRoleMappingRepository,
	createOidcLoginStateRepository,
	createCorsAllowedOriginRepository,
	createPlatformConfigRepository,
	createPlatformConfigAccessRepository,
	createLoginAttemptRepository,
} from "../infrastructure/persistence/index.js";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function createRepositories(db: any, schemaDb: any) {
	return {
		principalRepository: createPrincipalRepository(db),
		anchorDomainRepository: createAnchorDomainRepository(db),
		clientRepository: createClientRepository(db),
		applicationRepository: createApplicationRepository(db),
		applicationClientConfigRepository:
			createApplicationClientConfigRepository(db),
		roleRepository: createRoleRepository(db),
		permissionRepository: createPermissionRepository(db),
		clientAccessGrantRepository: createClientAccessGrantRepository(db),
		clientAuthConfigRepository: createClientAuthConfigRepository(db),
		oauthClientRepository: createOAuthClientRepository(schemaDb),
		auditLogRepository: createAuditLogRepository(db),
		eventTypeRepository: createEventTypeRepository(schemaDb),
		dispatchPoolRepository: createDispatchPoolRepository(db),
		subscriptionRepository: createSubscriptionRepository(schemaDb),
		eventReadRepository: createEventReadRepository(db),
		dispatchJobReadRepository: createDispatchJobReadRepository(db),
		identityProviderRepository: createIdentityProviderRepository(schemaDb),
		emailDomainMappingRepository: createEmailDomainMappingRepository(schemaDb),
		idpRoleMappingRepository: createIdpRoleMappingRepository(db),
		oidcLoginStateRepository: createOidcLoginStateRepository(db),
		corsAllowedOriginRepository: createCorsAllowedOriginRepository(db),
		platformConfigRepository: createPlatformConfigRepository(db),
		platformConfigAccessRepository: createPlatformConfigAccessRepository(db),
		loginAttemptRepository: createLoginAttemptRepository(db),
	};
}

export type Repositories = ReturnType<typeof createRepositories>;
