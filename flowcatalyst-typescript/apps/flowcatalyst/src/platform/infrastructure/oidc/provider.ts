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
import type { JSONWebKeySet } from 'jose';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { PrincipalRepository } from '../persistence/repositories/principal-repository.js';
import type { OAuthClientRepository } from '../persistence/repositories/oauth-client-repository.js';
import type { EncryptionService } from '@flowcatalyst/platform-crypto';
import { createDrizzleAdapterFactory } from './drizzle-adapter.js';
import { createFindAccount } from './account-adapter.js';
import { createClientLoader } from './client-adapter.js';
import { extractApplicationCodes } from './jwt-key-service.js';

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

  /** JWKS containing our RSA signing key (from JwtKeyService) */
  jwks?: JSONWebKeySet | undefined;

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
const CUSTOM_CLAIMS = ['type', 'scope', 'client_id', 'roles', 'clients', 'applications'];

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
    jwks: jwksConfig,
    accessTokenTtl = 3600,
    idTokenTtl = 3600,
    refreshTokenTtl = 2592000,
    sessionTtl = 86400,
    authCodeTtl = 600,
    devInteractions = false,
    interactionsPath = '/oidc/interaction',
  } = config;

  // Create client loader for dynamic client loading from OAuth repository
  const loadClient = createClientLoader(oauthClientRepository, encryptionService);

  // Create adapter factory with dynamic client loading fallback
  const AdapterFactory = createDrizzleAdapterFactory(
    db as PostgresJsDatabase,
    async (clientId: string) => {
      const metadata = await loadClient(clientId);
      return metadata as Record<string, unknown> | undefined;
    },
  );

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
      // Resource indicators ensure all access tokens are issued as JWTs (not opaque).
      // - defaultResource: assigns the issuer as the resource when the client omits the parameter
      // - useGrantedResource: tells the token endpoint to use the granted resource even when
      //   the client doesn't re-specify it. Without this, oidc-provider issues an opaque
      //   "UserInfo token" when scope includes 'openid' and userinfo is enabled.
      resourceIndicators: {
        enabled: true,
        defaultResource: async () => issuer,
        useGrantedResource: async () => true,
        getResourceServerInfo: async (
          _ctx: KoaContextWithOIDC,
          resourceIndicator: string,
        ): Promise<ResourceServer> => {
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

      // RP-initiated logout — auto-confirm and clear fc_session cookie
      rpInitiatedLogout: {
        enabled: true,
        logoutSource: async (ctx: KoaContextWithOIDC, form: string) => {
          // Clear the platform session cookie so the user isn't auto-logged back in
          ctx.res.setHeader(
            'Set-Cookie',
            'fc_session=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax',
          );
          // Auto-submit the confirmation form for seamless logout
          ctx.body = `<!DOCTYPE html><html><head><title>Logging out...</title></head><body>${form}<script>document.forms[0].submit();</script></body></html>`;
        },
      },
    },

    // Claims configuration
    claims: {
      openid: ['sub', ...CUSTOM_CLAIMS],
      profile: ['name', 'updated_at'],
      email: ['email', 'email_verified'],
    },

    // Scopes configuration
    scopes: ['openid', 'profile', 'email', 'offline_access'],

    // Custom route paths — use /authorize instead of /auth to avoid
    // conflicts with the platform's /auth/* SPA & API routes.
    routes: {
      authorization: '/authorize',
    },

    // Use our RSA signing keys if provided
    ...(jwksConfig ? { jwks: jwksConfig } : {}),

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
      // For user tokens (AccessToken with accountId)
      const accountId = 'accountId' in token ? token.accountId : undefined;
      if (accountId) {
        const principal = await principalRepository.findById(accountId);
        if (principal) {
          const roleNames = principal.roles.map((r) => r.roleName);
          return {
            type: principal.type,
            scope: principal.scope,
            client_id: principal.clientId,
            roles: roleNames,
            applications: extractApplicationCodes(roleNames),
            clients:
              principal.scope === 'ANCHOR' ? ['*'] : principal.clientId ? [principal.clientId] : [],
          };
        }
      }

      // For client_credentials tokens (no accountId) - look up the OAuth client's
      // service account principal to add the same claims as the Java version.
      // We also set `sub` so the audit plugin can extract the principal ID.
      const tokenClientId = 'clientId' in token ? token.clientId : undefined;
      if (tokenClientId) {
        const oauthClient = await oauthClientRepository.findByClientId(tokenClientId);
        if (oauthClient?.serviceAccountPrincipalId) {
          const principal = await principalRepository.findById(oauthClient.serviceAccountPrincipalId);
          if (principal) {
            const roleNames = principal.roles.map((r) => r.roleName);
            return {
              sub: principal.id,
              type: principal.type,
              scope: principal.scope,
              client_id: principal.clientId,
              roles: roleNames,
              applications: extractApplicationCodes(roleNames),
              clients:
                principal.scope === 'ANCHOR'
                  ? ['*']
                  : principal.clientId
                    ? [principal.clientId]
                    : [],
            };
          }
        }
      }

      return undefined;
    },

    // Response types
    responseTypes: ['code'],

    // Subject types
    subjectTypes: ['public'],

    // Enable CORS
    clientBasedCORS: (_ctx: KoaContextWithOIDC, _origin: string, _client: Client): boolean => {
      // We handle CORS separately in Fastify middleware
      // Allow all origins here, actual validation happens in our CORS handler
      return true;
    },
  };

  // Create provider
  // Dynamic client loading is handled by the adapter's Client model fallback
  // (see createDrizzleAdapterFactory clientLoader parameter above)
  const provider = new Provider(issuer, providerConfig);

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
