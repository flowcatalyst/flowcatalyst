/**
 * Fastify plugin registration — swagger, cors, cookie, tracing, audit,
 * OIDC mount, auth routes, interaction routes, federation routes, client selection.
 */

import type { FastifyInstance } from 'fastify';
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
  ErrorResponseSchema,
  MessageResponseSchema,
  SyncResponseSchema,
  BatchResponseSchema,
  BatchResultItemSchema,
  CommonSchemas,
} from '@flowcatalyst/http';
import type { UnitOfWork } from '@flowcatalyst/domain-core';
import type { EncryptionService, PasswordService } from '@flowcatalyst/platform-crypto';
import {
  mountOidcProvider,
  registerWellKnownRoutes,
  registerOAuthCompatibilityRoutes,
  registerOidcEndpointRoutes,
  registerAuthRoutes,
  registerOidcFederationRoutes,
  registerClientSelectionRoutes,
  type OidcProvider,
  type JwtKeyService,
} from '../infrastructure/oidc/index.js';
import { registerInteractionRoutes } from '../infrastructure/oidc/interaction-routes.js';
import { isOriginAllowed } from '../domain/index.js';
import { isDevelopment } from '../env.js';
import type { Env } from '../env.js';
import type { Repositories } from './repositories.js';

export interface RegisterPluginsDeps {
  env: Env;
  port: number;
  repos: Repositories;
  jwtKeyService: JwtKeyService;
  oidcProvider: OidcProvider;
  encryptionService: EncryptionService;
  passwordService: PasswordService;
  unitOfWork: UnitOfWork;
}

export async function registerPlatformPlugins(
  fastify: FastifyInstance,
  deps: RegisterPluginsDeps,
): Promise<void> {
  const { env, port, repos, jwtKeyService, oidcProvider, encryptionService, passwordService, unitOfWork } = deps;

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

  // Register shared schemas so Fastify emits $ref instead of inlining
  fastify.addSchema(ErrorResponseSchema);
  fastify.addSchema(MessageResponseSchema);
  fastify.addSchema(SyncResponseSchema);
  fastify.addSchema(BatchResponseSchema);
  fastify.addSchema(BatchResultItemSchema);
  fastify.addSchema(CommonSchemas.PaginationQuery);

  // Cookie handling (required for session tokens)
  await fastify.register(cookie);

  // CORS — enforce database-managed origins with wildcard pattern support.
  // Origins are cached and refreshed every 60 seconds to avoid per-request DB queries.
  let cachedOrigins: Set<string> = await repos.corsAllowedOriginRepository.getAllowedOrigins();
  let lastOriginRefresh = Date.now();
  const ORIGIN_CACHE_TTL_MS = 60_000;

  await fastify.register(cors, {
    credentials: true,
    origin: (origin, callback) => {
      // No origin header (e.g., server-to-server, same-origin) — allow
      if (!origin) {
        callback(null, true);
        return;
      }

      // Refresh cache if stale (non-blocking — uses current cache for this request)
      if (Date.now() - lastOriginRefresh > ORIGIN_CACHE_TTL_MS) {
        lastOriginRefresh = Date.now();
        repos.corsAllowedOriginRepository
          .getAllowedOrigins()
          .then((origins) => {
            cachedOrigins = origins;
          })
          .catch(() => {
            /* keep using stale cache */
          });
      }

      // Check origin against patterns (supports wildcards)
      if (isOriginAllowed(origin, cachedOrigins)) {
        callback(null, true);
      } else {
        fastify.log.debug({ origin }, 'CORS origin rejected');
        callback(null, false);
      }
    },
  });

  // Tracing (correlation IDs, execution IDs)
  await fastify.register(tracingPlugin);

  // Audit (authentication) - validates JWT tokens using RS256 key service
  await fastify.register(auditPlugin, {
    sessionCookieName: 'fc_session',
    validateToken: async (token: string) => {
      return jwtKeyService.validateAndGetPrincipalId(token);
    },
    loadPrincipal: async (principalId: string) => {
      // Try direct principal lookup first (user tokens have sub = principal UUID)
      let principal = await repos.principalRepository.findById(principalId);

      // For client_credentials tokens, oidc-provider sets sub = OAuth client_id
      // (e.g. "sa-inhance-php-apps"), not the principal UUID. Look up the OAuth
      // client's service account principal instead.
      if (!principal) {
        const oauthClient = await repos.oauthClientRepository.findByClientId(principalId);
        if (oauthClient?.serviceAccountPrincipalId) {
          principal = await repos.principalRepository.findById(
            oauthClient.serviceAccountPrincipalId,
          );
        }
      }

      if (!principal || !principal.active) {
        fastify.log.debug(
          { principalId, found: !!principal, active: principal?.active },
          'loadPrincipal: principal not resolved',
        );
        return null;
      }
      return {
        id: principal.id,
        type: principal.type,
        scope: principal.scope ?? (principal.type === 'SERVICE' ? 'ANCHOR' : 'CLIENT'),
        clientId: principal.clientId,
        roles: new Set(principal.roles.map((r) => r.roleName)),
      };
    },
  });

  // Execution context (combines tracing + audit for use cases)
  await fastify.register(executionContextPlugin);

  // Error handler
  await fastify.register(errorHandlerPlugin, createStandardErrorHandlerOptions());

  // Register OIDC interaction routes (before wildcard mount so parametric routes win)
  await registerInteractionRoutes(fastify, {
    provider: oidcProvider,
    validateSessionToken: (token) => jwtKeyService.validateAndGetPrincipalId(token),
    principalRepository: repos.principalRepository,
    oauthClientRepository: repos.oauthClientRepository,
    cookieName: 'fc_session',
    loginPageUrl: '/auth/login',
  });

  // Mount OIDC provider at /oidc
  await mountOidcProvider(fastify, oidcProvider, '/oidc');

  // Register well-known routes (JWKS served directly, openid-configuration redirected)
  registerWellKnownRoutes(fastify, '/oidc', jwtKeyService);

  // Register OAuth compatibility routes (/oauth/* -> /oidc/*)
  registerOAuthCompatibilityRoutes(fastify, oidcProvider, '/oidc');

  // Register root-level OIDC endpoint forwarding routes (/authorize, /token, /userinfo)
  // These forward to oidc-provider because the discovery doc advertises root-level URLs
  registerOidcEndpointRoutes(fastify, oidcProvider);

  // Register auth routes (/auth/login, /auth/logout, /auth/me, /auth/check-domain)
  await registerAuthRoutes(fastify, {
    principalRepository: repos.principalRepository,
    emailDomainMappingRepository: repos.emailDomainMappingRepository,
    identityProviderRepository: repos.identityProviderRepository,
    clientRepository: repos.clientRepository,
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
  const externalBaseUrl = env.EXTERNAL_BASE_URL ?? `http://localhost:${port}`;

  // Register OIDC federation routes (/auth/oidc/login, /auth/oidc/callback)
  await registerOidcFederationRoutes(fastify, {
    identityProviderRepository: repos.identityProviderRepository,
    emailDomainMappingRepository: repos.emailDomainMappingRepository,
    principalRepository: repos.principalRepository,
    clientRepository: repos.clientRepository,
    roleRepository: repos.roleRepository,
    idpRoleMappingRepository: repos.idpRoleMappingRepository,
    oidcLoginStateRepository: repos.oidcLoginStateRepository,
    unitOfWork,
    resolveClientSecret: async (idp) => {
      if (!idp.oidcClientSecretRef) return undefined;
      const result = encryptionService.decrypt(idp.oidcClientSecretRef);
      if (result.isOk()) {
        return result.value;
      }
      return undefined;
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
    principalRepository: repos.principalRepository,
    clientRepository: repos.clientRepository,
    emailDomainMappingRepository: repos.emailDomainMappingRepository,
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
