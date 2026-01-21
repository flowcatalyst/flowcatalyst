/**
 * Audit Plugin
 *
 * Fastify plugin for authentication and audit context. Extracts principal ID
 * from session cookie or Bearer token and populates the audit context.
 *
 * This plugin integrates with AuditContext from @flowcatalyst/domain-core
 * for consistent principal tracking across the application.
 */

import type { FastifyPluginAsync, preHandlerHookHandler } from 'fastify';
import fp from 'fastify-plugin';
import type { AuditPluginOptions, AuditData } from '../types.js';

// Declare the cookies property from @fastify/cookie
declare module 'fastify' {
	interface FastifyRequest {
		cookies: Record<string, string | undefined>;
	}
}

/**
 * Audit plugin for Fastify.
 *
 * Attempts to authenticate the request using:
 * 1. Session cookie (preferred for browser clients)
 * 2. Bearer token in Authorization header (for API clients)
 *
 * @example
 * ```typescript
 * import Fastify from 'fastify';
 * import cookie from '@fastify/cookie';
 * import { auditPlugin } from '@flowcatalyst/http';
 *
 * const fastify = Fastify();
 * await fastify.register(cookie);
 * await fastify.register(auditPlugin, {
 *     sessionCookieName: 'session',
 *     skipPaths: ['/health', '/metrics'],
 *     validateToken: async (token) => {
 *         // Validate JWT and return principal ID
 *         const claims = await verifyJwt(token);
 *         return claims?.sub ?? null;
 *     },
 * });
 *
 * fastify.get('/api/me', (request, reply) => {
 *     const { principalId } = request.audit;
 *     if (!principalId) {
 *         return reply.status(401).send({ error: 'Not authenticated' });
 *     }
 *     return { principalId };
 * });
 * ```
 */
const auditPluginAsync: FastifyPluginAsync<AuditPluginOptions> = async (fastify, opts) => {
	const { sessionCookieName = 'session', skipPaths = [], validateToken, loadPrincipal } = opts;

	// Decorate request with audit data (using getter/setter pattern)
	fastify.decorateRequest('audit', {
		getter() {
			return (this as unknown as { _audit: AuditData })._audit;
		},
		setter(value: AuditData) {
			(this as unknown as { _audit: AuditData })._audit = value;
		},
	});

	// Add audit data to each request
	fastify.addHook('onRequest', async (request) => {
		const path = new URL(request.url, `http://${request.hostname}`).pathname;

		// Skip authentication for specified paths
		for (const skipPath of skipPaths) {
			if (path.startsWith(skipPath)) {
				request.audit = { principalId: null, principal: null };
				return;
			}
		}

		let principalId: string | null = null;

		// Try session cookie first (requires @fastify/cookie)
		const cookies = request.cookies;
		const sessionToken = cookies?.[sessionCookieName];
		if (sessionToken) {
			principalId = await validateToken(sessionToken);
		}

		// Fall back to Bearer token
		if (!principalId) {
			const authHeader = request.headers.authorization;
			if (authHeader?.startsWith('Bearer ')) {
				const token = authHeader.substring('Bearer '.length);
				principalId = await validateToken(token);
			}
		}

		// Load full principal if configured and authenticated
		let principal = null;
		if (principalId && loadPrincipal) {
			principal = await loadPrincipal(principalId);
		}

		// Store audit data in request
		const auditData: AuditData = {
			principalId,
			principal,
		};
		request.audit = auditData;
	});
};

export const auditPlugin = fp(auditPluginAsync, {
	name: '@flowcatalyst/audit',
	fastify: '5.x',
});

/**
 * Require authentication - returns 401 if not authenticated.
 *
 * @param request - Fastify request
 * @returns Principal ID
 * @throws Object with statusCode for Fastify error handling
 */
export function requireAuth(request: { audit?: AuditData }): string {
	const audit = request.audit;
	if (!audit?.principalId) {
		throw { statusCode: 401, message: 'Authentication required' };
	}
	return audit.principalId;
}

/**
 * Get principal ID if authenticated, null otherwise.
 *
 * @param request - Fastify request
 * @returns Principal ID or null
 */
export function getPrincipalId(request: { audit?: AuditData }): string | null {
	return request.audit?.principalId ?? null;
}

/**
 * Check if request is authenticated.
 *
 * @param request - Fastify request
 * @returns True if authenticated
 */
export function isAuthenticated(request: { audit?: AuditData }): boolean {
	return request.audit?.principalId != null;
}

/**
 * Create an authentication preHandler hook.
 *
 * @returns preHandler hook that throws 401 if not authenticated
 *
 * @example
 * ```typescript
 * fastify.get('/protected', { preHandler: requireAuthHook() }, (request, reply) => {
 *     // Only authenticated users can access
 * });
 * ```
 */
export function requireAuthHook(): preHandlerHookHandler {
	return async (request, reply) => {
		const audit = request.audit;
		if (!audit?.principalId) {
			return reply.status(401).send({
				code: 'UNAUTHORIZED',
				message: 'Authentication required',
			});
		}
	};
}

/**
 * Create a role authorization preHandler hook.
 *
 * @param roleName - Required role name
 * @returns preHandler hook that throws 403 if role not present
 *
 * @example
 * ```typescript
 * fastify.get('/admin/users', { preHandler: requireRoleHook('admin') }, (request, reply) => {
 *     // Only admins can access
 * });
 * ```
 */
export function requireRoleHook(roleName: string): preHandlerHookHandler {
	return async (request, reply) => {
		const audit = request.audit;
		if (!audit?.principal) {
			return reply.status(401).send({
				code: 'UNAUTHORIZED',
				message: 'Authentication required',
			});
		}

		if (!audit.principal.roles.has(roleName)) {
			return reply.status(403).send({
				code: 'FORBIDDEN',
				message: 'Insufficient permissions',
			});
		}
	};
}
