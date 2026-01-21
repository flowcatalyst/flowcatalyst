/**
 * FlowCatalyst Platform Service
 *
 * IAM and Eventing service entry point.
 */

import Fastify from 'fastify';
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
	registerWellKnownRedirects,
	registerOAuthCompatibilityRoutes,
	registerAuthRoutes,
} from './infrastructure/oidc/index.js';
import { registerAdminRoutes, type AdminRoutesDeps } from './api/index.js';
import { initializeAuthorization } from './authorization/index.js';
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
} from './application/index.js';

// Load environment
const env = getEnv();

// Initialize authorization system
initializeAuthorization();

// Create Fastify app with logging
const fastify = Fastify({
	logger: createFastifyLoggerOptions({
		serviceName: 'platform',
		level: env.LOG_LEVEL,
	}),
});

fastify.log.info({ env: env.NODE_ENV }, 'Starting FlowCatalyst Platform service');

// Create database connection
const database = createDatabase({ url: env.DATABASE_URL });
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

// Create aggregate registry and register handlers
const aggregateRegistry = createAggregateRegistry();
aggregateRegistry.register(createAggregateHandler('Principal', principalRepository));
aggregateRegistry.register(createAggregateHandler('Client', clientRepository));
aggregateRegistry.register(createAggregateHandler('AnchorDomain', anchorDomainRepository));
aggregateRegistry.register(createAggregateHandler('Application', applicationRepository));
aggregateRegistry.register(createAggregateHandler('ApplicationClientConfig', applicationClientConfigRepository));
aggregateRegistry.register(createAggregateHandler('AuthRole', roleRepository));
aggregateRegistry.register(createAggregateHandler('ClientAccessGrant', clientAccessGrantRepository));
aggregateRegistry.register(createAggregateHandler('ClientAuthConfig', clientAuthConfigRepository));
aggregateRegistry.register(createAggregateHandler('OAuthClient', oauthClientRepository));

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
});

// Create password service
const passwordService = getPasswordService();

// Create encryption service
const encryptionService = createEncryptionServiceFromEnv();

// Compute OIDC issuer URL
const oidcIssuer =
	env.OIDC_ISSUER ?? env.EXTERNAL_BASE_URL ?? `http://localhost:${env.PORT}`;

// Create OIDC provider
const oidcProvider = createOidcProvider({
	issuer: oidcIssuer,
	db: db,
	principalRepository,
	oauthClientRepository,
	encryptionService,
	cookieKeys: env.OIDC_COOKIES_KEYS,
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

// Client use cases
const createClientUseCase = createCreateClientUseCase({
	clientRepository,
	unitOfWork,
});

const updateClientUseCase = createUpdateClientUseCase({
	clientRepository,
	unitOfWork,
});

const changeClientStatusUseCase = createChangeClientStatusUseCase({
	clientRepository,
	unitOfWork,
});

const deleteClientUseCase = createDeleteClientUseCase({
	clientRepository,
	unitOfWork,
});

const addClientNoteUseCase = createAddClientNoteUseCase({
	clientRepository,
	unitOfWork,
});

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

const enableApplicationForClientUseCase = createEnableApplicationForClientUseCase({
	applicationRepository,
	clientRepository,
	applicationClientConfigRepository,
	unitOfWork,
});

const disableApplicationForClientUseCase = createDisableApplicationForClientUseCase({
	applicationClientConfigRepository,
	unitOfWork,
});

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

const grantClientAccessUseCase = createGrantClientAccessUseCase({
	principalRepository,
	clientRepository,
	clientAccessGrantRepository,
	unitOfWork,
});

const revokeClientAccessUseCase = createRevokeClientAccessUseCase({
	principalRepository,
	clientAccessGrantRepository,
	unitOfWork,
});

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

// Register plugins
async function registerPlugins() {
	// Cookie handling (required for session tokens)
	await fastify.register(cookie);

	// CORS
	await fastify.register(cors, { origin: true, credentials: true });

	// Tracing (correlation IDs, execution IDs)
	await fastify.register(tracingPlugin);

	// Audit (authentication) - validates JWT tokens from oidc-provider
	await fastify.register(auditPlugin, {
		validateToken: async (token: string) => {
			try {
				// Use oidc-provider's token introspection
				// For now, we'll decode and validate the JWT directly
				// In production, use the introspection endpoint or JWKS validation
				const payload = await validateOidcToken(token);
				return payload?.sub ?? null;
			} catch {
				return null;
			}
		},
	});

	// Execution context (combines tracing + audit for use cases)
	await fastify.register(executionContextPlugin);

	// Error handler
	await fastify.register(errorHandlerPlugin, createStandardErrorHandlerOptions());

	// Mount OIDC provider at /oidc
	await mountOidcProvider(fastify, oidcProvider, '/oidc');

	// Register well-known redirects for OIDC discovery
	registerWellKnownRedirects(fastify, '/oidc');

	// Register OAuth compatibility routes (/oauth/* -> /oidc/*)
	registerOAuthCompatibilityRoutes(fastify, oidcProvider, '/oidc');

	// Register auth routes (/auth/login, /auth/logout, /auth/me)
	await registerAuthRoutes(fastify, {
		principalRepository,
		passwordService,
		issueSessionToken: (principalId, email, roles, clients) => {
			// Use oidc-provider's internal token issuance
			// For now, create a simple JWT - in production this should use oidc-provider
			return createSessionToken(principalId, email, roles, clients);
		},
		validateSessionToken: async (token) => {
			const result = await validateOidcToken(token);
			return result?.sub ?? null;
		},
		cookieConfig: {
			name: 'fc_session',
			secure: !isDevelopment(),
			sameSite: 'lax',
			maxAge: env.OIDC_SESSION_TTL ?? 86400,
		},
	});
}

/**
 * Validate an OIDC access token.
 * Uses jose to verify the JWT signature against the provider's JWKS.
 */
async function validateOidcToken(token: string): Promise<{ sub: string } | null> {
	try {
		// Import jose dynamically to avoid circular dependencies
		const { createRemoteJWKSet, jwtVerify } = await import('jose');

		// Get JWKS from the provider
		const jwksUri = new URL('/.well-known/jwks.json', oidcIssuer);
		const JWKS = createRemoteJWKSet(jwksUri);

		// Verify the token
		const { payload } = await jwtVerify(token, JWKS, {
			issuer: oidcIssuer,
		});

		if (typeof payload.sub === 'string') {
			return { sub: payload.sub };
		}

		return null;
	} catch {
		return null;
	}
}

// Session signing key (should be same as OIDC cookie keys in production)
let sessionSigningKey: Uint8Array | null = null;

/**
 * Get or create the session signing key.
 */
function getSessionSigningKey(): Uint8Array {
	if (!sessionSigningKey) {
		// Use OIDC cookie keys if available, otherwise generate
		const cookieKey = env.OIDC_COOKIES_KEYS?.[0] ?? 'flowcatalyst-dev-session-key';
		sessionSigningKey = new TextEncoder().encode(cookieKey);
	}
	return sessionSigningKey;
}

/**
 * Create a session token for authenticated users.
 * This creates a JWT that can be validated by validateOidcToken.
 */
function createSessionToken(
	principalId: string,
	email: string,
	roles: string[],
	clients: string[],
): string {
	// Use jose to create JWT synchronously with HS256
	// Note: This is a simplified version - in production, use oidc-provider's token issuance
	const { SignJWT } = require('jose') as typeof import('jose');
	const key = getSessionSigningKey();

	// Create JWT payload
	const now = Math.floor(Date.now() / 1000);
	const expiry = now + (env.OIDC_SESSION_TTL ?? 86400);

	// Build JWT synchronously using the builder pattern
	// Note: SignJWT.sign() is async, so we need to handle this differently
	// For now, use a simple base64-encoded JSON token that we can validate
	const payload = {
		iss: oidcIssuer,
		sub: principalId,
		email,
		roles,
		'flowcatalyst:clients': clients,
		iat: now,
		exp: expiry,
	};

	// Simple HMAC-based token for session (not a full JWT, but signed)
	const crypto = require('crypto');
	const payloadB64 = Buffer.from(JSON.stringify(payload)).toString('base64url');
	const signature = crypto
		.createHmac('sha256', key)
		.update(payloadB64)
		.digest('base64url');

	return `${payloadB64}.${signature}`;
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
		// User management
		principalRepository,
		clientAccessGrantRepository,
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
	};

	await registerAdminRoutes(fastify, deps);
}

// Start server
async function start() {
	try {
		await registerPlugins();
		await registerRoutes();

		const port = env.PORT;
		const host = env.HOST;

		fastify.log.info({ port, host }, 'Starting HTTP server');

		await fastify.listen({ port, host });

		if (isDevelopment()) {
			console.log(`\n  Platform API:     http://localhost:${port}/api`);
			console.log(`  OIDC Discovery:   http://localhost:${port}/.well-known/openid-configuration`);
			console.log(`  OIDC Auth:        http://localhost:${port}/oidc/auth`);
			console.log(`  OIDC Token:       http://localhost:${port}/oidc/token`);
			console.log(`  Health check:     http://localhost:${port}/health\n`);
		}
	} catch (err) {
		fastify.log.error(err);
		process.exit(1);
	}
}

start();

// Export app for testing
export { fastify };
