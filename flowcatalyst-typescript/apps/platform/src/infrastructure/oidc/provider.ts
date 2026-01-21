/**
 * OIDC Provider Configuration
 *
 * Sets up and configures oidc-provider for the FlowCatalyst platform.
 * This is the main entry point for OIDC/OAuth2 functionality.
 */

import Provider, {
	type Configuration,
	type KoaContextWithOIDC,
	type ResourceServer,
	type Client,
	type AccessToken,
	type ClientCredentials,
	type UnknownObject,
	type Interaction,
} from 'oidc-provider';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { PrincipalRepository } from '../persistence/repositories/principal-repository.js';
import type { OAuthClientRepository } from '../persistence/repositories/oauth-client-repository.js';
import type { EncryptionService } from '@flowcatalyst/platform-crypto';
import { createDrizzleAdapterFactory } from './drizzle-adapter.js';
import { createFindAccount } from './account-adapter.js';
import { createClientLoader } from './client-adapter.js';

/**
 * Configuration for creating the OIDC provider.
 */
export interface OidcProviderConfig {
	/** The issuer URL (e.g., https://auth.example.com) */
	issuer: string;

	/** Database instance for the adapter */
	db: PostgresJsDatabase;

	/** Principal repository for account lookup */
	principalRepository: PrincipalRepository;

	/** OAuth client repository for client lookup */
	oauthClientRepository: OAuthClientRepository;

	/** Encryption service for decrypting client secrets */
	encryptionService: EncryptionService;

	/** Cookie signing keys (at least one required in production) */
	cookieKeys?: string[] | undefined;

	/** Access token TTL in seconds (default: 3600 = 1 hour) */
	accessTokenTtl?: number | undefined;

	/** ID token TTL in seconds (default: 3600 = 1 hour) */
	idTokenTtl?: number | undefined;

	/** Refresh token TTL in seconds (default: 2592000 = 30 days) */
	refreshTokenTtl?: number | undefined;

	/** Session TTL in seconds (default: 86400 = 24 hours) */
	sessionTtl?: number | undefined;

	/** Authorization code TTL in seconds (default: 600 = 10 minutes) */
	authCodeTtl?: number | undefined;

	/** Whether to enable dev interactions (login/consent pages) */
	devInteractions?: boolean | undefined;

	/** Base path for interactions (login/consent) */
	interactionsPath?: string | undefined;
}

/**
 * Custom claims to add to tokens.
 */
const CUSTOM_CLAIMS = [
	'flowcatalyst:type',
	'flowcatalyst:scope',
	'flowcatalyst:client_id',
	'flowcatalyst:roles',
	'flowcatalyst:clients',
];

/**
 * Creates and configures the OIDC provider.
 */
export function createOidcProvider(config: OidcProviderConfig): Provider {
	const {
		issuer,
		db,
		principalRepository,
		oauthClientRepository,
		encryptionService,
		cookieKeys,
		accessTokenTtl = 3600,
		idTokenTtl = 3600,
		refreshTokenTtl = 2592000,
		sessionTtl = 86400,
		authCodeTtl = 600,
		devInteractions = false,
		interactionsPath = '/oidc/interaction',
	} = config;

	// Create adapter factory
	const AdapterFactory = createDrizzleAdapterFactory(db as PostgresJsDatabase);

	// Create client loader for dynamic client registration
	const loadClient = createClientLoader(oauthClientRepository, encryptionService);

	// Create find account function
	const findAccount = createFindAccount(principalRepository);

	// Build configuration
	const providerConfig: Configuration = {
		// Adapter for persistent storage
		adapter: AdapterFactory,

		// Account lookup function
		findAccount,

		// Client loading
		// Note: oidc-provider doesn't have a direct "loadClient" option
		// We'll use the clients array or use extraClientMetadata
		clients: [], // Empty - we'll load dynamically

		// Cookie configuration
		cookies: {
			keys: cookieKeys ?? ['flowcatalyst-dev-key-change-in-production'],
			long: {
				signed: true,
				httpOnly: true,
				overwrite: true,
				sameSite: 'lax',
			},
			short: {
				signed: true,
				httpOnly: true,
				overwrite: true,
				sameSite: 'lax',
			},
		},

		// Token TTLs
		ttl: {
			AccessToken: accessTokenTtl,
			AuthorizationCode: authCodeTtl,
			IdToken: idTokenTtl,
			RefreshToken: refreshTokenTtl,
			Session: sessionTtl,
			Interaction: 3600, // 1 hour for login/consent interactions
			Grant: refreshTokenTtl, // Same as refresh token
			DeviceCode: 600, // 10 minutes for device flow
		},

		// Features
		features: {
			// Disable dev interactions in production
			devInteractions: {
				enabled: devInteractions,
			},

			// Enable refresh token rotation for security
			revocation: { enabled: true },

			// Resource indicators for audience restriction
			resourceIndicators: {
				enabled: true,
				getResourceServerInfo: async (
					_ctx: KoaContextWithOIDC,
					resourceIndicator: string,
				): Promise<ResourceServer> => {
					// Allow any resource indicator for now
					// In production, validate against known APIs
					return {
						scope: 'openid profile email',
						audience: resourceIndicator,
						accessTokenTTL: accessTokenTtl,
						accessTokenFormat: 'jwt',
					};
				},
			},

			// Client credentials grant
			clientCredentials: { enabled: true },

			// Introspection and revocation
			introspection: { enabled: true },
		},

		// Claims configuration
		claims: {
			openid: ['sub'],
			profile: ['name', 'updated_at'],
			email: ['email', 'email_verified'],
			// Custom FlowCatalyst claims
			flowcatalyst: CUSTOM_CLAIMS,
		},

		// Scopes configuration
		scopes: ['openid', 'profile', 'email', 'offline_access', 'flowcatalyst'],

		// Interaction URLs
		interactions: {
			url: (_ctx: KoaContextWithOIDC, interaction: Interaction): string => {
				return `${interactionsPath}/${interaction.uid}`;
			},
		},

		// PKCE configuration - require for public clients
		pkce: {
			required: (_ctx: KoaContextWithOIDC, client: Client): boolean => {
				// Require PKCE for all public clients
				return client.tokenEndpointAuthMethod === 'none';
			},
		},

		// Rotate refresh tokens on use
		rotateRefreshToken: (_ctx: KoaContextWithOIDC): boolean => {
			// Always rotate for better security
			return true;
		},

		// Extra access token claims
		extraTokenClaims: async (
			_ctx: KoaContextWithOIDC,
			token: AccessToken | ClientCredentials,
		): Promise<UnknownObject | undefined> => {
			const accountId = 'accountId' in token ? token.accountId : undefined;
			if (accountId) {
				const principal = await principalRepository.findById(accountId);
				if (principal) {
					return {
						'flowcatalyst:type': principal.type,
						'flowcatalyst:scope': principal.scope,
						'flowcatalyst:client_id': principal.clientId,
						'flowcatalyst:roles': principal.roles.map((r) => r.roleName),
						'flowcatalyst:clients':
							principal.scope === 'ANCHOR'
								? ['*']
								: principal.clientId
									? [principal.clientId]
									: [],
					};
				}
			}
			return undefined;
		},

		// Response types
		responseTypes: ['code'],

		// Subject types
		subjectTypes: ['public'],

		// Enable CORS
		clientBasedCORS: (
			_ctx: KoaContextWithOIDC,
			_origin: string,
			_client: Client,
		): boolean => {
			// We handle CORS separately in Fastify middleware
			// Allow all origins here, actual validation happens in our CORS handler
			return true;
		},
	};

	// Create provider
	const provider = new Provider(issuer, providerConfig);

	// Add dynamic client loading via middleware
	provider.use(async (ctx: KoaContextWithOIDC, next: () => Promise<void>) => {
		// Intercept client lookup - load client from our repository if not found
		const existingClient = ctx.oidc?.client;

		if (!existingClient && ctx.oidc?.params) {
			const clientId = ctx.oidc.params['client_id'] as string | undefined;
			if (clientId) {
				const clientMetadata = await loadClient(clientId);

				if (clientMetadata) {
					// Provider.Client.find will use the adapter to find/create the client
					// Note: oidc-provider will handle setting the client on the context
					await provider.Client.find(clientId);
				}
			}
		}

		await next();
	});

	return provider;
}

/**
 * Get the OIDC provider callback handler for use with Fastify.
 */
export function getProviderCallback(provider: Provider) {
	return provider.callback();
}

/**
 * Type for the OIDC provider instance.
 */
export type OidcProvider = Provider;
