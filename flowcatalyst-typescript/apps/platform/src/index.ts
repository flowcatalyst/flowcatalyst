/**
 * FlowCatalyst Platform Service
 *
 * IAM and Eventing service entry point.
 */

import { existsSync } from 'node:fs';
import Fastify, { type FastifyInstance } from 'fastify';
import swagger from '@fastify/swagger';
import swaggerUi from '@fastify/swagger-ui';
import cookie from '@fastify/cookie';
import cors from '@fastify/cors';
import {
	tracingPlugin,
	auditPlugin,
	executionContextPlugin,
	errorHandlerPlugin,
	createStandardErrorHandlerOptions,
	createFastifyLoggerOptions,
	jsonSuccess,
} from '@flowcatalyst/http';
import {
	createDatabase,
	createTransactionManager,
	createAggregateRegistry,
	createAggregateHandler,
	createDrizzleUnitOfWork,
} from '@flowcatalyst/persistence';
import { getPasswordService, createEncryptionServiceFromEnv } from '@flowcatalyst/platform-crypto';

import { getEnv, isDevelopment } from './env.js';
import {
	createOidcProvider,
	mountOidcProvider,
	registerWellKnownRoutes,
	registerOAuthCompatibilityRoutes,
	registerAuthRoutes,
	registerOidcFederationRoutes,
	registerClientSelectionRoutes,
	createJwtKeyService,
} from './infrastructure/oidc/index.js';
import { registerAdminRoutes, type AdminRoutesDeps, registerBffRoutes, type BffRoutesDeps, registerSdkRoutes, type SdkRoutesDeps, registerMeApiRoutes, type MeRoutesDeps, registerPublicApiRoutes, registerPlatformConfigApiRoutes, registerDebugBffRoutes } from './api/index.js';
import { createPlatformConfigService } from './domain/index.js';
import { createEventDispatchService } from './infrastructure/dispatch/event-dispatch-service.js';
import { initializeAuthorization, createGuardedUseCase, clientScopedGuard, clientAccessGuard } from './authorization/index.js';
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
} from './infrastructure/persistence/index.js';
import {
	createCreateUserUseCase,
	createUpdateUserUseCase,
	createActivateUserUseCase,
	createDeactivateUserUseCase,
	createDeleteUserUseCase,
	createCreateClientUseCase,
	createUpdateClientUseCase,
	createChangeClientStatusUseCase,
	createDeleteClientUseCase,
	createAddClientNoteUseCase,
	createCreateAnchorDomainUseCase,
	createUpdateAnchorDomainUseCase,
	createDeleteAnchorDomainUseCase,
	createCreateApplicationUseCase,
	createUpdateApplicationUseCase,
	createDeleteApplicationUseCase,
	createActivateApplicationUseCase,
	createDeactivateApplicationUseCase,
	createEnableApplicationForClientUseCase,
	createDisableApplicationForClientUseCase,
	createCreateRoleUseCase,
	createUpdateRoleUseCase,
	createDeleteRoleUseCase,
	createAssignRolesUseCase,
	createGrantClientAccessUseCase,
	createRevokeClientAccessUseCase,
	createCreateInternalAuthConfigUseCase,
	createCreateOidcAuthConfigUseCase,
	createUpdateOidcSettingsUseCase,
	createUpdateConfigTypeUseCase,
	createUpdateAdditionalClientsUseCase,
	createUpdateGrantedClientsUseCase,
	createDeleteAuthConfigUseCase,
	createCreateOAuthClientUseCase,
	createUpdateOAuthClientUseCase,
	createRegenerateOAuthClientSecretUseCase,
	createDeleteOAuthClientUseCase,
	createCreateEventTypeUseCase,
	createUpdateEventTypeUseCase,
	createArchiveEventTypeUseCase,
	createDeleteEventTypeUseCase,
	createAddSchemaUseCase,
	createFinaliseSchemaUseCase,
	createDeprecateSchemaUseCase,
	createSyncEventTypesUseCase,
	createCreateDispatchPoolUseCase,
	createUpdateDispatchPoolUseCase,
	createDeleteDispatchPoolUseCase,
	createSyncDispatchPoolsUseCase,
	createCreateSubscriptionUseCase,
	createUpdateSubscriptionUseCase,
	createDeleteSubscriptionUseCase,
	createSyncSubscriptionsUseCase,
	createCreateIdentityProviderUseCase,
	createUpdateIdentityProviderUseCase,
	createDeleteIdentityProviderUseCase,
	createCreateEmailDomainMappingUseCase,
	createUpdateEmailDomainMappingUseCase,
	createDeleteEmailDomainMappingUseCase,
	createCreateServiceAccountUseCase,
	createUpdateServiceAccountUseCase,
	createDeleteServiceAccountUseCase,
	createRegenerateAuthTokenUseCase,
	createRegenerateSigningSecretUseCase,
	createAssignServiceAccountRolesUseCase,
	createAssignApplicationAccessUseCase,
	createAddCorsOriginUseCase,
	createDeleteCorsOriginUseCase,
} from './application/index.js';

/**
 * Platform configuration options for in-process embedding.
 */
export interface PlatformConfig {
	port?: number;
	host?: string;
	databaseUrl?: string;
	logLevel?: 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal';
	frontendDir?: string | undefined;
}

/**
 * Start the FlowCatalyst Platform service.
 *
 * @param config - Optional overrides for port, host, database, log level
 * @returns The Fastify instance (ready and listening)
 */
export async function startPlatform(config?: PlatformConfig): Promise<FastifyInstance> {
// Load environment
const env = getEnv();

const PORT = config?.port ?? env.PORT;
const HOST = config?.host ?? env.HOST;
const DATABASE_URL = config?.databaseUrl ?? env.DATABASE_URL;
const LOG_LEVEL = config?.logLevel ?? env.LOG_LEVEL;

// Initialize authorization system
initializeAuthorization();

// Create Fastify app with logging
const fastify = Fastify({
	logger: createFastifyLoggerOptions({
		serviceName: 'platform',
		level: LOG_LEVEL,
	}),
});

fastify.log.info({ env: env.NODE_ENV }, 'Starting FlowCatalyst Platform service');

// Create database connection
const database = createDatabase({ url: DATABASE_URL });
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const db = database.db as any;
const transactionManager = createTransactionManager(db);

// Create repositories
const principalRepository = createPrincipalRepository(db);
const anchorDomainRepository = createAnchorDomainRepository(db);
const clientRepository = createClientRepository(db);
const applicationRepository = createApplicationRepository(db);
const applicationClientConfigRepository = createApplicationClientConfigRepository(db);
const roleRepository = createRoleRepository(db);
const permissionRepository = createPermissionRepository(db);
const clientAccessGrantRepository = createClientAccessGrantRepository(db);
const clientAuthConfigRepository = createClientAuthConfigRepository(db);
const oauthClientRepository = createOAuthClientRepository(db);
const auditLogRepository = createAuditLogRepository(db);
const eventTypeRepository = createEventTypeRepository(db);
const dispatchPoolRepository = createDispatchPoolRepository(db);
const subscriptionRepository = createSubscriptionRepository(db);
const eventReadRepository = createEventReadRepository(db);
const dispatchJobReadRepository = createDispatchJobReadRepository(db);
const identityProviderRepository = createIdentityProviderRepository(db);
const emailDomainMappingRepository = createEmailDomainMappingRepository(db);
const idpRoleMappingRepository = createIdpRoleMappingRepository(db);
const oidcLoginStateRepository = createOidcLoginStateRepository(db);
const corsAllowedOriginRepository = createCorsAllowedOriginRepository(db);
const platformConfigRepository = createPlatformConfigRepository(db);
const platformConfigAccessRepository = createPlatformConfigAccessRepository(db);

// Create platform config service
const platformConfigService = createPlatformConfigService({
	configRepository: platformConfigRepository,
	accessRepository: platformConfigAccessRepository,
});

// Create aggregate registry and register handlers
// Prefix map allows the registry to resolve plain-object aggregates by ID prefix
const aggregateRegistry = createAggregateRegistry({
	prn: 'Principal',
	clt: 'Client',
	anc: 'AnchorDomain',
	app: 'Application',
	apc: 'ApplicationClientConfig',
	rol: 'AuthRole',
	gnt: 'ClientAccessGrant',
	cac: 'ClientAuthConfig',
	oac: 'OAuthClient',
	evt: 'EventType',
	dpl: 'DispatchPool',
	sub: 'Subscription',
	idp: 'IdentityProvider',
	edm: 'EmailDomainMapping',
	cor: 'CorsAllowedOrigin',
});
aggregateRegistry.register(createAggregateHandler('Principal', principalRepository));
aggregateRegistry.register(createAggregateHandler('Client', clientRepository));
aggregateRegistry.register(createAggregateHandler('AnchorDomain', anchorDomainRepository));
aggregateRegistry.register(createAggregateHandler('Application', applicationRepository));
aggregateRegistry.register(createAggregateHandler('ApplicationClientConfig', applicationClientConfigRepository));
aggregateRegistry.register(createAggregateHandler('AuthRole', roleRepository));
aggregateRegistry.register(createAggregateHandler('ClientAccessGrant', clientAccessGrantRepository));
aggregateRegistry.register(createAggregateHandler('ClientAuthConfig', clientAuthConfigRepository));
aggregateRegistry.register(createAggregateHandler('OAuthClient', oauthClientRepository));
aggregateRegistry.register(createAggregateHandler('EventType', eventTypeRepository));
aggregateRegistry.register(createAggregateHandler('DispatchPool', dispatchPoolRepository));
aggregateRegistry.register(createAggregateHandler('Subscription', subscriptionRepository));
aggregateRegistry.register(createAggregateHandler('IdentityProvider', identityProviderRepository));
aggregateRegistry.register(createAggregateHandler('EmailDomainMapping', emailDomainMappingRepository));
aggregateRegistry.register(createAggregateHandler('CorsAllowedOrigin', corsAllowedOriginRepository));

// Create event dispatch service (builds dispatch jobs for events inside UoW transaction)
const eventDispatchService = createEventDispatchService({
	subscriptionRepository,
});

// Create unit of work
const unitOfWork = createDrizzleUnitOfWork({
	transactionManager,
	aggregateRegistry,
	extractClientId: (aggregate) => {
		if ('clientId' in aggregate && typeof aggregate.clientId === 'string') {
			return aggregate.clientId;
		}
		return null;
	},
	eventDispatchService,
});

// Create password service
const passwordService = getPasswordService();

// Bootstrap: sync permissions/roles to DB + create admin user
const { runBootstrap } = await import('./bootstrap/index.js');
await runBootstrap({
	roleRepository,
	permissionRepository,
	principalRepository,
	applicationRepository,
	identityProviderRepository,
	emailDomainMappingRepository,
	passwordService,
	logger: fastify.log,
});

// Create encryption service
const encryptionService = createEncryptionServiceFromEnv();

// Compute OIDC issuer URL
const oidcIssuer =
	env.OIDC_ISSUER ?? env.EXTERNAL_BASE_URL ?? `http://localhost:${PORT}`;

// Initialize JWT key service (RS256 key pair)
const jwtKeyService = await createJwtKeyService({
	issuer: env.JWT_ISSUER,
	keyDir: env.JWT_KEY_DIR,
	privateKeyPath: env.JWT_PRIVATE_KEY_PATH,
	publicKeyPath: env.JWT_PUBLIC_KEY_PATH,
	devKeyDir: env.JWT_DEV_KEY_DIR,
	sessionTokenTtl: env.OIDC_SESSION_TTL,
	accessTokenTtl: env.OIDC_ACCESS_TOKEN_TTL,
});

fastify.log.info({ keyId: jwtKeyService.getKeyId() }, 'JWT key service initialized');

// Create OIDC provider
const oidcProvider = createOidcProvider({
	issuer: oidcIssuer,
	db: db,
	principalRepository,
	oauthClientRepository,
	encryptionService,
	cookieKeys: env.OIDC_COOKIES_KEYS,
	jwks: jwtKeyService.getSigningJwks(),
	accessTokenTtl: env.OIDC_ACCESS_TOKEN_TTL,
	idTokenTtl: env.OIDC_ID_TOKEN_TTL,
	refreshTokenTtl: env.OIDC_REFRESH_TOKEN_TTL,
	sessionTtl: env.OIDC_SESSION_TTL,
	authCodeTtl: env.OIDC_AUTH_CODE_TTL,
	devInteractions: isDevelopment(), // Enable dev login pages in development
});

fastify.log.info({ issuer: oidcIssuer }, 'OIDC provider created');

// Create use cases
const createUserUseCase = createCreateUserUseCase({
	principalRepository,
	anchorDomainRepository,
	emailDomainMappingRepository,
	identityProviderRepository,
	passwordService,
	unitOfWork,
});

const updateUserUseCase = createUpdateUserUseCase({
	principalRepository,
	unitOfWork,
});

const activateUserUseCase = createActivateUserUseCase({
	principalRepository,
	unitOfWork,
});

const deactivateUserUseCase = createDeactivateUserUseCase({
	principalRepository,
	unitOfWork,
});

const deleteUserUseCase = createDeleteUserUseCase({
	principalRepository,
	unitOfWork,
});

// Client use cases (with resource-level guards)
const createClientUseCase = createCreateClientUseCase({
	clientRepository,
	unitOfWork,
});

const updateClientUseCase = createGuardedUseCase(
	createUpdateClientUseCase({ clientRepository, unitOfWork }),
	clientAccessGuard((cmd) => cmd.clientId),
);

const changeClientStatusUseCase = createGuardedUseCase(
	createChangeClientStatusUseCase({ clientRepository, unitOfWork }),
	clientAccessGuard((cmd) => cmd.clientId),
);

const deleteClientUseCase = createGuardedUseCase(
	createDeleteClientUseCase({ clientRepository, unitOfWork }),
	clientAccessGuard((cmd) => cmd.clientId),
);

const addClientNoteUseCase = createGuardedUseCase(
	createAddClientNoteUseCase({ clientRepository, unitOfWork }),
	clientAccessGuard((cmd) => cmd.clientId),
);

// Anchor domain use cases
const createAnchorDomainUseCase = createCreateAnchorDomainUseCase({
	anchorDomainRepository,
	unitOfWork,
});

const updateAnchorDomainUseCase = createUpdateAnchorDomainUseCase({
	anchorDomainRepository,
	unitOfWork,
});

const deleteAnchorDomainUseCase = createDeleteAnchorDomainUseCase({
	anchorDomainRepository,
	unitOfWork,
});

// Application use cases
const createApplicationUseCase = createCreateApplicationUseCase({
	applicationRepository,
	unitOfWork,
});

const updateApplicationUseCase = createUpdateApplicationUseCase({
	applicationRepository,
	unitOfWork,
});

const deleteApplicationUseCase = createDeleteApplicationUseCase({
	applicationRepository,
	unitOfWork,
});

const enableApplicationForClientUseCase = createGuardedUseCase(
	createEnableApplicationForClientUseCase({
		applicationRepository,
		clientRepository,
		applicationClientConfigRepository,
		unitOfWork,
	}),
	clientAccessGuard((cmd) => cmd.clientId),
);

const disableApplicationForClientUseCase = createGuardedUseCase(
	createDisableApplicationForClientUseCase({
		applicationClientConfigRepository,
		unitOfWork,
	}),
	clientAccessGuard((cmd) => cmd.clientId),
);

const activateApplicationUseCase = createActivateApplicationUseCase({
	applicationRepository,
	unitOfWork,
});

const deactivateApplicationUseCase = createDeactivateApplicationUseCase({
	applicationRepository,
	unitOfWork,
});

// Role use cases
const createRoleUseCase = createCreateRoleUseCase({
	roleRepository,
	unitOfWork,
});

const updateRoleUseCase = createUpdateRoleUseCase({
	roleRepository,
	unitOfWork,
});

const deleteRoleUseCase = createDeleteRoleUseCase({
	roleRepository,
	unitOfWork,
});

// User role and client access use cases
const assignRolesUseCase = createAssignRolesUseCase({
	principalRepository,
	roleRepository,
	unitOfWork,
});

const grantClientAccessUseCase = createGuardedUseCase(
	createGrantClientAccessUseCase({
		principalRepository,
		clientRepository,
		clientAccessGrantRepository,
		unitOfWork,
	}),
	clientAccessGuard((cmd) => cmd.clientId),
);

const revokeClientAccessUseCase = createGuardedUseCase(
	createRevokeClientAccessUseCase({
		principalRepository,
		clientAccessGrantRepository,
		unitOfWork,
	}),
	clientAccessGuard((cmd) => cmd.clientId),
);

// Auth config use cases
const createInternalAuthConfigUseCase = createCreateInternalAuthConfigUseCase({
	clientAuthConfigRepository,
	unitOfWork,
});

const createOidcAuthConfigUseCase = createCreateOidcAuthConfigUseCase({
	clientAuthConfigRepository,
	unitOfWork,
});

const updateOidcSettingsUseCase = createUpdateOidcSettingsUseCase({
	clientAuthConfigRepository,
	unitOfWork,
});

const updateConfigTypeUseCase = createUpdateConfigTypeUseCase({
	clientAuthConfigRepository,
	unitOfWork,
});

const updateAdditionalClientsUseCase = createUpdateAdditionalClientsUseCase({
	clientAuthConfigRepository,
	unitOfWork,
});

const updateGrantedClientsUseCase = createUpdateGrantedClientsUseCase({
	clientAuthConfigRepository,
	unitOfWork,
});

const deleteAuthConfigUseCase = createDeleteAuthConfigUseCase({
	clientAuthConfigRepository,
	unitOfWork,
});

// OAuth client use cases
const createOAuthClientUseCase = createCreateOAuthClientUseCase({
	oauthClientRepository,
	unitOfWork,
});

const updateOAuthClientUseCase = createUpdateOAuthClientUseCase({
	oauthClientRepository,
	unitOfWork,
});

const regenerateOAuthClientSecretUseCase = createRegenerateOAuthClientSecretUseCase({
	oauthClientRepository,
	unitOfWork,
});

const deleteOAuthClientUseCase = createDeleteOAuthClientUseCase({
	oauthClientRepository,
	unitOfWork,
});

// EventType use cases
const createEventTypeUseCase = createCreateEventTypeUseCase({
	eventTypeRepository,
	unitOfWork,
});

const updateEventTypeUseCase = createUpdateEventTypeUseCase({
	eventTypeRepository,
	unitOfWork,
});

const archiveEventTypeUseCase = createArchiveEventTypeUseCase({
	eventTypeRepository,
	unitOfWork,
});

const deleteEventTypeUseCase = createDeleteEventTypeUseCase({
	eventTypeRepository,
	unitOfWork,
});

const addSchemaUseCase = createAddSchemaUseCase({
	eventTypeRepository,
	unitOfWork,
});

const finaliseSchemaUseCase = createFinaliseSchemaUseCase({
	eventTypeRepository,
	unitOfWork,
});

const deprecateSchemaUseCase = createDeprecateSchemaUseCase({
	eventTypeRepository,
	unitOfWork,
});

const syncEventTypesUseCase = createSyncEventTypesUseCase({
	eventTypeRepository,
	unitOfWork,
});

// Dispatch Pool use cases (with client-scope guard for client-scoped pools)
const createDispatchPoolUseCase = createGuardedUseCase(
	createCreateDispatchPoolUseCase({
		dispatchPoolRepository,
		clientRepository,
		unitOfWork,
	}),
	clientScopedGuard(),
);

const updateDispatchPoolUseCase = createUpdateDispatchPoolUseCase({
	dispatchPoolRepository,
	unitOfWork,
});

const deleteDispatchPoolUseCase = createDeleteDispatchPoolUseCase({
	dispatchPoolRepository,
	unitOfWork,
});

const syncDispatchPoolsUseCase = createSyncDispatchPoolsUseCase({
	dispatchPoolRepository,
	unitOfWork,
});

// Subscription use cases (with client-scope guard for client-scoped subs)
const createSubscriptionUseCase = createGuardedUseCase(
	createCreateSubscriptionUseCase({
		subscriptionRepository,
		dispatchPoolRepository,
		unitOfWork,
	}),
	clientScopedGuard(),
);

const updateSubscriptionUseCase = createUpdateSubscriptionUseCase({
	subscriptionRepository,
	dispatchPoolRepository,
	unitOfWork,
});

const deleteSubscriptionUseCase = createDeleteSubscriptionUseCase({
	subscriptionRepository,
	unitOfWork,
});

const syncSubscriptionsUseCase = createSyncSubscriptionsUseCase({
	subscriptionRepository,
	dispatchPoolRepository,
	unitOfWork,
});

// Identity Provider use cases
const createIdentityProviderUseCase = createCreateIdentityProviderUseCase({
	identityProviderRepository,
	unitOfWork,
});

const updateIdentityProviderUseCase = createUpdateIdentityProviderUseCase({
	identityProviderRepository,
	unitOfWork,
});

const deleteIdentityProviderUseCase = createDeleteIdentityProviderUseCase({
	identityProviderRepository,
	unitOfWork,
});

// Email Domain Mapping use cases
const createEmailDomainMappingUseCase = createCreateEmailDomainMappingUseCase({
	emailDomainMappingRepository,
	identityProviderRepository,
	unitOfWork,
});

const updateEmailDomainMappingUseCase = createUpdateEmailDomainMappingUseCase({
	emailDomainMappingRepository,
	identityProviderRepository,
	unitOfWork,
});

const deleteEmailDomainMappingUseCase = createDeleteEmailDomainMappingUseCase({
	emailDomainMappingRepository,
	unitOfWork,
});

// Service Account use cases
const createServiceAccountUseCase = createCreateServiceAccountUseCase({
	principalRepository,
	oauthClientRepository,
	encryptionService,
	unitOfWork,
});

const updateServiceAccountUseCase = createUpdateServiceAccountUseCase({
	principalRepository,
	unitOfWork,
});

const deleteServiceAccountUseCase = createDeleteServiceAccountUseCase({
	principalRepository,
	oauthClientRepository,
	unitOfWork,
});

const regenerateAuthTokenUseCase = createRegenerateAuthTokenUseCase({
	principalRepository,
	encryptionService,
	unitOfWork,
});

const regenerateSigningSecretUseCase = createRegenerateSigningSecretUseCase({
	principalRepository,
	encryptionService,
	unitOfWork,
});

const assignServiceAccountRolesUseCase = createAssignServiceAccountRolesUseCase({
	principalRepository,
	roleRepository,
	unitOfWork,
});

// CORS use cases
const addCorsOriginUseCase = createAddCorsOriginUseCase({
	corsAllowedOriginRepository,
	unitOfWork,
});

const deleteCorsOriginUseCase = createDeleteCorsOriginUseCase({
	corsAllowedOriginRepository,
	unitOfWork,
});

// Application access use case
const assignApplicationAccessUseCase = createAssignApplicationAccessUseCase({
	principalRepository,
	applicationRepository,
	applicationClientConfigRepository,
	clientAccessGrantRepository,
	unitOfWork,
});

// Register plugins
async function registerPlugins() {
	// OpenAPI / Swagger
	await fastify.register(swagger, {
		openapi: {
			openapi: '3.1.0',
			info: {
				title: 'FlowCatalyst Platform API',
				version: '1.0.0',
				description: 'IAM, Eventing, and Administration API for the FlowCatalyst platform.',
			},
			servers: [{ url: '/' }],
			components: {
				securitySchemes: {
					bearerAuth: {
						type: 'http',
						scheme: 'bearer',
						bearerFormat: 'JWT',
					},
					cookieAuth: {
						type: 'apiKey',
						in: 'cookie',
						name: 'fc_session',
					},
				},
			},
			security: [{ bearerAuth: [] }],
		},
	});

	await fastify.register(swaggerUi, {
		routePrefix: '/docs',
		uiConfig: {
			docExpansion: 'list',
			deepLinking: true,
		},
	});

	// Cookie handling (required for session tokens)
	await fastify.register(cookie);

	// CORS
	await fastify.register(cors, { origin: true, credentials: true });

	// Tracing (correlation IDs, execution IDs)
	await fastify.register(tracingPlugin);

	// Audit (authentication) - validates JWT tokens using RS256 key service
	await fastify.register(auditPlugin, {
		sessionCookieName: 'fc_session',
		validateToken: async (token: string) => {
			return jwtKeyService.validateAndGetPrincipalId(token);
		},
		loadPrincipal: async (principalId: string) => {
			const principal = await principalRepository.findById(principalId);
			if (!principal || !principal.active) return null;
			return {
				id: principal.id,
				type: principal.type,
				scope: principal.scope ?? 'CLIENT',
				clientId: principal.clientId,
				roles: new Set(principal.roles.map((r) => r.roleName)),
			};
		},
	});

	// Execution context (combines tracing + audit for use cases)
	await fastify.register(executionContextPlugin);

	// Error handler
	await fastify.register(errorHandlerPlugin, createStandardErrorHandlerOptions());

	// Mount OIDC provider at /oidc
	await mountOidcProvider(fastify, oidcProvider, '/oidc');

	// Register well-known routes (JWKS served directly, openid-configuration redirected)
	registerWellKnownRoutes(fastify, '/oidc', jwtKeyService);

	// Register OAuth compatibility routes (/oauth/* -> /oidc/*)
	registerOAuthCompatibilityRoutes(fastify, oidcProvider, '/oidc');

	// Register auth routes (/auth/login, /auth/logout, /auth/me, /auth/check-domain)
	await registerAuthRoutes(fastify, {
		principalRepository,
		emailDomainMappingRepository,
		identityProviderRepository,
		clientRepository,
		passwordService,
		issueSessionToken: (principalId, email, roles, clients) => {
			return jwtKeyService.issueSessionToken(principalId, email, roles, clients);
		},
		validateSessionToken: (token) => {
			return jwtKeyService.validateAndGetPrincipalId(token);
		},
		cookieConfig: {
			name: 'fc_session',
			secure: !isDevelopment(),
			sameSite: 'lax',
			maxAge: env.OIDC_SESSION_TTL ?? 86400,
		},
	});

	// Compute external base URL for OIDC federation callbacks
	const externalBaseUrl = env.EXTERNAL_BASE_URL ?? `http://localhost:${PORT}`;

	// Register OIDC federation routes (/auth/oidc/login, /auth/oidc/callback)
	await registerOidcFederationRoutes(fastify, {
		identityProviderRepository,
		emailDomainMappingRepository,
		principalRepository,
		clientRepository,
		roleRepository,
		idpRoleMappingRepository,
		oidcLoginStateRepository,
		unitOfWork,
		resolveClientSecret: async (idp) => {
			if (!idp.oidcClientSecretRef) return undefined;
			const result = encryptionService.decrypt(idp.oidcClientSecretRef);
			return result.isOk() ? result.value : undefined;
		},
		issueSessionToken: (principalId, email, roles, clients) => {
			return jwtKeyService.issueSessionToken(principalId, email, roles, clients);
		},
		cookieConfig: {
			name: 'fc_session',
			secure: !isDevelopment(),
			sameSite: 'lax',
			maxAge: env.OIDC_SESSION_TTL ?? 86400,
		},
		externalBaseUrl,
	});

	// Register client selection routes (/auth/client/accessible, /auth/client/switch, /auth/client/current)
	await registerClientSelectionRoutes(fastify, {
		principalRepository,
		clientRepository,
		emailDomainMappingRepository,
		issueSessionToken: (principalId, email, roles, clients) => {
			return jwtKeyService.issueSessionToken(principalId, email, roles, clients);
		},
		validateSessionToken: (token) => {
			return jwtKeyService.validateAndGetPrincipalId(token);
		},
		cookieConfig: {
			name: 'fc_session',
			secure: !isDevelopment(),
			sameSite: 'lax',
			maxAge: env.OIDC_SESSION_TTL ?? 86400,
		},
	});
}

// Register routes
async function registerRoutes() {
	// Health check
	fastify.get('/health', async (request, reply) => {
		return jsonSuccess(reply, {
			status: 'healthy',
			service: 'platform',
			timestamp: new Date().toISOString(),
		});
	});

	// Admin API routes
	const deps: AdminRoutesDeps = {
		// Principal management
		principalRepository,
		clientAccessGrantRepository,
		passwordService,
		createUserUseCase,
		updateUserUseCase,
		activateUserUseCase,
		deactivateUserUseCase,
		deleteUserUseCase,
		assignRolesUseCase,
		grantClientAccessUseCase,
		revokeClientAccessUseCase,
		// Client management
		clientRepository,
		createClientUseCase,
		updateClientUseCase,
		changeClientStatusUseCase,
		deleteClientUseCase,
		addClientNoteUseCase,
		// Anchor domain management
		anchorDomainRepository,
		createAnchorDomainUseCase,
		updateAnchorDomainUseCase,
		deleteAnchorDomainUseCase,
		// Application management
		applicationRepository,
		applicationClientConfigRepository,
		createApplicationUseCase,
		updateApplicationUseCase,
		deleteApplicationUseCase,
		activateApplicationUseCase,
		deactivateApplicationUseCase,
		enableApplicationForClientUseCase,
		disableApplicationForClientUseCase,
		// Role management
		roleRepository,
		permissionRepository,
		createRoleUseCase,
		updateRoleUseCase,
		deleteRoleUseCase,
		// Auth config management
		clientAuthConfigRepository,
		createInternalAuthConfigUseCase,
		createOidcAuthConfigUseCase,
		updateOidcSettingsUseCase,
		updateConfigTypeUseCase,
		updateAdditionalClientsUseCase,
		updateGrantedClientsUseCase,
		deleteAuthConfigUseCase,
		// OAuth client management
		oauthClientRepository,
		createOAuthClientUseCase,
		updateOAuthClientUseCase,
		regenerateOAuthClientSecretUseCase,
		deleteOAuthClientUseCase,
		// Audit log viewing
		auditLogRepository,
		// EventType management
		eventTypeRepository,
		createEventTypeUseCase,
		updateEventTypeUseCase,
		deleteEventTypeUseCase,
		archiveEventTypeUseCase,
		addSchemaUseCase,
		finaliseSchemaUseCase,
		deprecateSchemaUseCase,
		syncEventTypesUseCase,
		// Dispatch Pool management
		dispatchPoolRepository,
		createDispatchPoolUseCase,
		updateDispatchPoolUseCase,
		deleteDispatchPoolUseCase,
		syncDispatchPoolsUseCase,
		// Subscription management
		subscriptionRepository,
		createSubscriptionUseCase,
		updateSubscriptionUseCase,
		deleteSubscriptionUseCase,
		syncSubscriptionsUseCase,
		// Event & Dispatch Job read models
		eventReadRepository,
		dispatchJobReadRepository,
		// Identity Provider management
		identityProviderRepository,
		createIdentityProviderUseCase,
		updateIdentityProviderUseCase,
		deleteIdentityProviderUseCase,
		// Email Domain Mapping management
		emailDomainMappingRepository,
		createEmailDomainMappingUseCase,
		updateEmailDomainMappingUseCase,
		deleteEmailDomainMappingUseCase,
		// Application access management
		assignApplicationAccessUseCase,
		// CORS origin management
		corsAllowedOriginRepository,
		addCorsOriginUseCase,
		deleteCorsOriginUseCase,
		// Platform config management
		platformConfigService,
		platformConfigAccessRepository,
		// Service Account management
		createServiceAccountUseCase,
		updateServiceAccountUseCase,
		deleteServiceAccountUseCase,
		regenerateAuthTokenUseCase,
		regenerateSigningSecretUseCase,
		assignServiceAccountRolesUseCase,
	};

	await registerAdminRoutes(fastify, deps);

	// BFF routes (frontend-facing)
	const bffDeps: BffRoutesDeps = {
		// Event type BFF
		eventTypeRepository,
		createEventTypeUseCase,
		updateEventTypeUseCase,
		deleteEventTypeUseCase,
		archiveEventTypeUseCase,
		addSchemaUseCase,
		finaliseSchemaUseCase,
		deprecateSchemaUseCase,
		// Role BFF
		roleRepository,
		permissionRepository,
		applicationRepository,
		createRoleUseCase,
		updateRoleUseCase,
		deleteRoleUseCase,
	};

	await registerBffRoutes(fastify, bffDeps);

	// SDK routes (external integrations)
	const sdkDeps: SdkRoutesDeps = {
		// SDK Clients
		clientRepository,
		createClientUseCase,
		updateClientUseCase,
		changeClientStatusUseCase,
		deleteClientUseCase,
		// SDK Roles
		roleRepository,
		applicationRepository,
		createRoleUseCase,
		updateRoleUseCase,
		deleteRoleUseCase,
		// SDK Principals
		principalRepository,
		clientAccessGrantRepository,
		createUserUseCase,
		updateUserUseCase,
		activateUserUseCase,
		deactivateUserUseCase,
		assignRolesUseCase,
		grantClientAccessUseCase,
		revokeClientAccessUseCase,
	};

	await registerSdkRoutes(fastify, sdkDeps);

	// User-facing /api/me routes
	const meDeps: MeRoutesDeps = {
		clientRepository,
		applicationRepository,
		applicationClientConfigRepository,
	};

	await registerMeApiRoutes(fastify, meDeps);

	// Public routes (no auth required)
	const publicDeps = {
		platformConfigService,
	};

	await registerPublicApiRoutes(fastify, publicDeps);
	await registerPlatformConfigApiRoutes(fastify, publicDeps);

	// Debug BFF routes (raw event/dispatch job access)
	const debugBffDeps = {
		db,
	};

	await registerDebugBffRoutes(fastify, debugBffDeps);
}

// Start server
await registerPlugins();
await registerRoutes();

// Serve frontend static files if configured
if (config?.frontendDir && existsSync(config.frontendDir)) {
	const fastifyStatic = (await import('@fastify/static')).default;
	await fastify.register(fastifyStatic, {
		root: config.frontendDir,
		wildcard: false,
	});

	// SPA catch-all: serve index.html for navigation paths not matched by API routes
	fastify.setNotFoundHandler(async (request, reply) => {
		if (request.method === 'GET' && request.url.indexOf('.') === -1) {
			return reply.sendFile('index.html');
		}
		reply.code(404).send({ error: 'Not Found' });
	});

	fastify.log.info({ frontendDir: config.frontendDir }, 'Frontend static serving enabled');
}

fastify.log.info({ port: PORT, host: HOST }, 'Starting HTTP server');

await fastify.listen({ port: PORT, host: HOST });

if (isDevelopment()) {
	console.log(`\n  Platform API:     http://localhost:${PORT}/api`);
	console.log(`  OpenAPI Docs:     http://localhost:${PORT}/docs`);
	console.log(`  OpenAPI JSON:     http://localhost:${PORT}/docs/json`);
	console.log(`  OIDC Discovery:   http://localhost:${PORT}/.well-known/openid-configuration`);
	console.log(`  OIDC Auth:        http://localhost:${PORT}/oidc/auth`);
	console.log(`  OIDC Token:       http://localhost:${PORT}/oidc/token`);
	console.log(`  OIDC Federation:  http://localhost:${PORT}/auth/oidc/login?domain=...`);
	console.log(`  Health check:     http://localhost:${PORT}/health\n`);
}

return fastify;
} // end startPlatform

// Key utilities (for CLI commands like rotate-keys)
export { generateKeyPair, computeKeyId, loadKeyDir, writeKeyPair, removeKeyPair } from './infrastructure/oidc/key-utils.js';

// Run when executed as main module (not when imported by flowcatalyst app)
import { fileURLToPath as _toPath } from 'node:url';
import { resolve as _resolve } from 'node:path';
const _self = _resolve(_toPath(import.meta.url));
const _entry = process.argv[1] ? _resolve(process.argv[1]) : '';
if (_self === _entry) {
	startPlatform().catch((err) => {
		console.error('Failed to start platform:', err);
		process.exit(1);
	});
}
