/**
 * Fastify Adapter for oidc-provider
 *
 * Mounts the oidc-provider (which uses Koa internally) onto Fastify
 * using a Node.js http.RequestListener adapter.
 */

import type { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import type { OidcProvider } from './provider.js';
import type { JwtKeyService } from './jwt-key-service.js';

/**
 * Mount the OIDC provider on a Fastify instance.
 *
 * The provider is mounted at a base path (default: /oidc) and handles
 * all OAuth 2.0 / OIDC endpoints:
 *
 * - GET  /oidc/.well-known/openid-configuration  - Discovery document
 * - GET  /oidc/.well-known/jwks.json             - JSON Web Key Set
 * - GET  /oidc/auth                              - Authorization endpoint
 * - POST /oidc/token                             - Token endpoint
 * - POST /oidc/token/introspection               - Token introspection
 * - POST /oidc/token/revocation                  - Token revocation
 * - GET  /oidc/userinfo                          - UserInfo endpoint
 * - POST /oidc/userinfo                          - UserInfo endpoint
 * - GET  /oidc/session/end                       - Logout endpoint
 * - GET  /oidc/interaction/:uid                  - Interaction pages
 * - POST /oidc/interaction/:uid                  - Interaction submission
 *
 * @param fastify - Fastify instance
 * @param provider - OIDC provider instance
 * @param basePath - Base path for OIDC endpoints (default: /oidc)
 */
export async function mountOidcProvider(
  fastify: FastifyInstance,
  provider: OidcProvider,
  basePath = '/oidc',
): Promise<void> {
  // Get the provider's HTTP callback
  const callback = provider.callback();

  // Create a wildcard route that forwards to oidc-provider
  // oidc-provider expects to handle the full path internally
  fastify.all(`${basePath}/*`, async (request: FastifyRequest, reply: FastifyReply) => {
    // oidc-provider expects the path relative to its mount point
    const originalUrl = request.raw.url ?? '';
    const oidcPath = originalUrl.replace(basePath, '') || '/';

    // Create a modified request object with the adjusted URL
    const req = request.raw;
    const res = reply.raw;

    // Store original URL and modify for oidc-provider
    const storedUrl = req.url;
    req.url = oidcPath;

    // Let oidc-provider handle the request
    await new Promise<void>((resolve, reject) => {
      // oidc-provider's callback returns a promise
      callback(req, res)
        .then(() => resolve())
        .catch((err: Error) => reject(err))
        .finally(() => {
          // Restore original URL
          req.url = storedUrl;
        });
    });

    // Mark reply as sent (oidc-provider handles the response directly)
    reply.hijack();
  });

  // Also handle the exact base path (for discovery)
  fastify.all(basePath, async (request: FastifyRequest, reply: FastifyReply) => {
    const req = request.raw;
    const res = reply.raw;

    const storedUrl = req.url;
    req.url = '/';

    await new Promise<void>((resolve, reject) => {
      callback(req, res)
        .then(() => resolve())
        .catch((err: Error) => reject(err))
        .finally(() => {
          req.url = storedUrl;
        });
    });

    reply.hijack();
  });

  fastify.log.info({ path: basePath }, 'OIDC provider mounted');
}

/**
 * Register well-known routes for OIDC discovery and JWKS.
 *
 * - /.well-known/openid-configuration -> redirect to oidc-provider
 * - /.well-known/jwks.json -> served directly from JwtKeyService
 *
 * JWKS is served directly (not via redirect) so that token consumers
 * (message router, SDKs) can verify session tokens without following redirects.
 */
export function registerWellKnownRoutes(
  fastify: FastifyInstance,
  basePath = '/oidc',
  jwtKeyService?: JwtKeyService,
): void {
  // OpenID Configuration discovery - redirect to oidc-provider
  fastify.get('/.well-known/openid-configuration', async (request, reply) => {
    return reply.redirect(`${basePath}/.well-known/openid-configuration`);
  });

  // JWKS endpoint - serve directly from our key service
  fastify.get('/.well-known/jwks.json', async (request, reply) => {
    if (jwtKeyService) {
      return reply.send(jwtKeyService.getJwks());
    }
    // Fallback to redirect if no key service provided
    return reply.redirect(`${basePath}/.well-known/jwks.json`);
  });
}

/**
 * Register OAuth compatibility routes for Java API parity.
 *
 * The Java version uses /oauth/authorize and /oauth/token,
 * while oidc-provider uses /oidc/auth and /oidc/token.
 *
 * This creates proxies to maintain backwards compatibility:
 * - /oauth/authorize -> /oidc/auth
 * - /oauth/token -> /oidc/token
 * - /oauth/jwks -> /oidc/jwks
 * - /oauth/introspect -> /oidc/token/introspection
 * - /oauth/revoke -> /oidc/token/revocation
 */
export function registerOAuthCompatibilityRoutes(
  fastify: FastifyInstance,
  provider: OidcProvider,
  basePath = '/oidc',
): void {
  const callback = provider.callback();

  // Helper to forward requests to oidc-provider with path rewrite
  const forwardToOidc = async (
    request: FastifyRequest,
    reply: FastifyReply,
    oidcPath: string,
  ): Promise<void> => {
    const req = request.raw;
    const res = reply.raw;

    const storedUrl = req.url;
    req.url = oidcPath;

    await new Promise<void>((resolve, reject) => {
      callback(req, res)
        .then(() => resolve())
        .catch((err: Error) => reject(err))
        .finally(() => {
          req.url = storedUrl;
        });
    });

    reply.hijack();
  };

  // /oauth/authorize -> /oidc/auth
  fastify.get('/oauth/authorize', async (request, reply) => {
    // Preserve query string
    const queryString = request.raw.url?.split('?')[1] ?? '';
    const oidcPath = queryString ? `/auth?${queryString}` : '/auth';
    await forwardToOidc(request, reply, oidcPath);
  });

  // /oauth/token -> /oidc/token
  fastify.post('/oauth/token', async (request, reply) => {
    await forwardToOidc(request, reply, '/token');
  });

  // /oauth/jwks -> /oidc/jwks
  fastify.get('/oauth/jwks', async (request, reply) => {
    await forwardToOidc(request, reply, '/jwks');
  });

  // /oauth/introspect -> /oidc/token/introspection
  fastify.post('/oauth/introspect', async (request, reply) => {
    await forwardToOidc(request, reply, '/token/introspection');
  });

  // /oauth/revoke -> /oidc/token/revocation
  fastify.post('/oauth/revoke', async (request, reply) => {
    await forwardToOidc(request, reply, '/token/revocation');
  });

  fastify.log.info('OAuth compatibility routes registered (/oauth/* -> /oidc/*)');
}
