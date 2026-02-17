/**
 * FlowCatalyst Platform Service
 *
 * IAM and Eventing service entry point.
 */

import { existsSync } from 'node:fs';
import Fastify, { type FastifyInstance } from 'fastify';
import { createFastifyLoggerOptions } from '@flowcatalyst/http';
import { type PostCommitDispatcher } from '@flowcatalyst/persistence';
import { drizzle } from 'drizzle-orm/postgres-js';
import { createDatabase } from '@flowcatalyst/persistence';
import { getPasswordService, createEncryptionServiceFromEnv } from '@flowcatalyst/platform-crypto';
import * as platformSchema from './infrastructure/persistence/schema/drizzle-schema.js';
import { platformRelations } from './infrastructure/persistence/schema/relations.js';
import { getEnv, isDevelopment } from './env.js';
import { createOidcProvider, createJwtKeyService } from './infrastructure/oidc/index.js';
import { createPlatformConfigService } from './domain/index.js';
import { initializeAuthorization } from './authorization/index.js';
import {
  createRepositories,
  createPlatformAggregateRegistry,
  createDispatchInfrastructure,
  createUseCases,
  registerPlatformPlugins,
  registerPlatformRoutes,
} from './composition/index.js';

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
 * Result of starting the platform service.
 */
export interface PlatformResult {
  /** The running Fastify instance */
  server: FastifyInstance;
  /**
   * Set (or replace) the post-commit dispatcher at runtime.
   * Call this after the message router starts in embedded mode
   * to wire the embedded publisher.
   */
  setPostCommitDispatcher(dispatcher: PostCommitDispatcher): void;
}

// Re-export for external consumers (e.g. src/index.ts embedded publisher)
export { createPostCommitDispatcherFromPublisher } from './composition/dispatch.js';

/**
 * Start the FlowCatalyst Platform service.
 *
 * @param config - Optional overrides for port, host, database, log level
 * @returns PlatformResult with server instance and post-commit dispatcher setter
 */
export async function startPlatform(config?: PlatformConfig): Promise<PlatformResult> {
  const env = getEnv();

  const PORT = config?.port ?? env.PORT;
  const HOST = config?.host ?? env.HOST;
  const DATABASE_URL = config?.databaseUrl ?? env.DATABASE_URL;
  const LOG_LEVEL = config?.logLevel ?? env.LOG_LEVEL;

  // Initialize authorization system
  initializeAuthorization();

  // Create Fastify app with logging
  const fastify = Fastify({
    trustProxy: true,
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
  // Schema-aware db instance for repositories that use relational queries (db.query.*)
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const schemaDb: any = drizzle({ client: database.client, schema: platformSchema, relations: platformRelations } as any);

  // 1. Repositories
  const repos = createRepositories(db, schemaDb);

  // Platform config service (needed by dispatch infrastructure)
  const platformConfigService = createPlatformConfigService({
    configRepository: repos.platformConfigRepository,
    accessRepository: repos.platformConfigAccessRepository,
  });

  // 2. Aggregate registry
  const aggregateRegistry = createPlatformAggregateRegistry(repos);

  // 3. Dispatch infrastructure (event dispatch service, SQS, dispatch scheduler, UoW)
  const { uowConfig, unitOfWork, dispatchSchedulerHandle } = await createDispatchInfrastructure({
    repos,
    aggregateRegistry,
    schemaDb,
    db,
    env,
    platformConfigService,
    logger: fastify.log,
  });

  // Services
  const passwordService = getPasswordService();
  const encryptionService = createEncryptionServiceFromEnv();

  // Bootstrap: sync permissions/roles to DB + create admin user
  const { runBootstrap } = await import('./bootstrap/index.js');
  await runBootstrap({
    roleRepository: repos.roleRepository,
    permissionRepository: repos.permissionRepository,
    principalRepository: repos.principalRepository,
    applicationRepository: repos.applicationRepository,
    identityProviderRepository: repos.identityProviderRepository,
    emailDomainMappingRepository: repos.emailDomainMappingRepository,
    passwordService,
    logger: fastify.log,
  });

  // OIDC issuer URL
  const oidcIssuer = env.OIDC_ISSUER ?? env.EXTERNAL_BASE_URL ?? `http://localhost:${PORT}`;

  // JWT key service (RS256 key pair)
  const jwtKeyService = await createJwtKeyService({
    issuer: oidcIssuer,
    privateKey: env.FLOWCATALYST_JWT_PRIVATE_KEY,
    publicKey: env.FLOWCATALYST_JWT_PUBLIC_KEY,
    previousPublicKey: env.FLOWCATALYST_JWT_PREVIOUS_PUBLIC_KEY,
    keyDir: env.JWT_KEY_DIR,
    privateKeyPath: env.JWT_PRIVATE_KEY_PATH,
    publicKeyPath: env.JWT_PUBLIC_KEY_PATH,
    devKeyDir: env.JWT_DEV_KEY_DIR,
    sessionTokenTtl: env.OIDC_SESSION_TTL,
    accessTokenTtl: env.OIDC_ACCESS_TOKEN_TTL,
  });

  fastify.log.info({ keyId: jwtKeyService.getKeyId() }, 'JWT key service initialized');

  // OIDC provider
  const oidcProvider = createOidcProvider({
    issuer: oidcIssuer,
    db: db,
    principalRepository: repos.principalRepository,
    oauthClientRepository: repos.oauthClientRepository,
    encryptionService,
    cookieKeys: env.OIDC_COOKIES_KEYS,
    jwks: jwtKeyService.getSigningJwks(),
    accessTokenTtl: env.OIDC_ACCESS_TOKEN_TTL,
    idTokenTtl: env.OIDC_ID_TOKEN_TTL,
    refreshTokenTtl: env.OIDC_REFRESH_TOKEN_TTL,
    sessionTtl: env.OIDC_SESSION_TTL,
    authCodeTtl: env.OIDC_AUTH_CODE_TTL,
    devInteractions: false,
  });

  fastify.log.info({ issuer: oidcIssuer }, 'OIDC provider created');

  // 4. Use cases
  const useCases = createUseCases({
    repos,
    unitOfWork,
    passwordService,
    encryptionService,
  });

  // 5. Fastify plugins (swagger, CORS, auth, OIDC, etc.)
  await registerPlatformPlugins(fastify, {
    env,
    port: PORT,
    repos,
    jwtKeyService,
    oidcProvider,
    encryptionService,
    passwordService,
    unitOfWork,
  });

  // 6. Routes (admin, bff, sdk, batch, me, public, debug)
  await registerPlatformRoutes(fastify, {
    repos,
    useCases,
    db,
    uowConfig,
    platformConfigService,
    passwordService,
    encryptionService,
  });

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
  } else {
    // No frontend — redirect root to login
    fastify.get('/', async (request, reply) => {
      return reply.redirect('/auth/login');
    });
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

  // Register dispatch scheduler shutdown hook
  if (dispatchSchedulerHandle) {
    fastify.addHook('onClose', async () => {
      dispatchSchedulerHandle?.stop();
    });
  }

  return {
    server: fastify,
    setPostCommitDispatcher(dispatcher: PostCommitDispatcher) {
      uowConfig.postCommitDispatch = dispatcher;
    },
  };
} // end startPlatform

// Key utilities (for CLI commands like rotate-keys)
export {
  generateKeyPair,
  computeKeyId,
  loadKeyDir,
  writeKeyPair,
  removeKeyPair,
} from './infrastructure/oidc/key-utils.js';

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
