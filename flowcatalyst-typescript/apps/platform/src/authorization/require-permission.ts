/**
 * Require Permission Hook
 *
 * Fastify preHandler hook that enforces permission requirements on routes.
 */

import type { preHandlerHookHandler } from 'fastify';
import type { PermissionDefinition } from './permission-definition.js';
import { permissionToString } from './permission-definition.js';
import { hasPermission, hasAnyPermission, hasAllPermissions } from './authorization-service.js';

/**
 * Create a preHandler hook that requires a specific permission.
 *
 * @param permission - Permission required (definition or string)
 * @returns Fastify preHandler hook
 *
 * @example
 * fastify.get('/users', {
 *   preHandler: requirePermission(IAM_PERMISSIONS.USER_READ),
 * }, handler);
 */
export function requirePermission(permission: PermissionDefinition | string): preHandlerHookHandler {
	const permissionString = typeof permission === 'string' ? permission : permissionToString(permission);

	return async (request, reply) => {
		const principal = request.audit?.principal;

		if (!principal) {
			return reply.status(401).send({
				error: 'UNAUTHORIZED',
				message: 'Authentication required',
			});
		}

		if (!hasPermission(principal, permissionString)) {
			return reply.status(403).send({
				error: 'FORBIDDEN',
				message: 'Insufficient permissions',
				required: permissionString,
			});
		}
	};
}

/**
 * Create a preHandler hook that requires any of the specified permissions.
 *
 * @param permissions - Permissions required (at least one)
 * @returns Fastify preHandler hook
 *
 * @example
 * fastify.get('/resource', {
 *   preHandler: requireAnyPermission([
 *     IAM_PERMISSIONS.RESOURCE_READ,
 *     IAM_PERMISSIONS.RESOURCE_MANAGE,
 *   ]),
 * }, handler);
 */
export function requireAnyPermission(
	permissions: readonly (PermissionDefinition | string)[],
): preHandlerHookHandler {
	const permissionStrings = permissions.map((p) => (typeof p === 'string' ? p : permissionToString(p)));

	return async (request, reply) => {
		const principal = request.audit?.principal;

		if (!principal) {
			return reply.status(401).send({
				error: 'UNAUTHORIZED',
				message: 'Authentication required',
			});
		}

		if (!hasAnyPermission(principal, permissionStrings)) {
			return reply.status(403).send({
				error: 'FORBIDDEN',
				message: 'Insufficient permissions',
				required: permissionStrings,
				mode: 'any',
			});
		}
	};
}

/**
 * Create a preHandler hook that requires all of the specified permissions.
 *
 * @param permissions - Permissions required (all must be present)
 * @returns Fastify preHandler hook
 *
 * @example
 * fastify.post('/sensitive', {
 *   preHandler: requireAllPermissions([
 *     IAM_PERMISSIONS.SENSITIVE_READ,
 *     IAM_PERMISSIONS.SENSITIVE_WRITE,
 *   ]),
 * }, handler);
 */
export function requireAllPermissions(
	permissions: readonly (PermissionDefinition | string)[],
): preHandlerHookHandler {
	const permissionStrings = permissions.map((p) => (typeof p === 'string' ? p : permissionToString(p)));

	return async (request, reply) => {
		const principal = request.audit?.principal;

		if (!principal) {
			return reply.status(401).send({
				error: 'UNAUTHORIZED',
				message: 'Authentication required',
			});
		}

		if (!hasAllPermissions(principal, permissionStrings)) {
			return reply.status(403).send({
				error: 'FORBIDDEN',
				message: 'Insufficient permissions',
				required: permissionStrings,
				mode: 'all',
			});
		}
	};
}

/**
 * Create a preHandler hook that requires authentication but no specific permission.
 *
 * @returns Fastify preHandler hook
 */
export function requireAuthentication(): preHandlerHookHandler {
	return async (request, reply) => {
		const principal = request.audit?.principal;

		if (!principal) {
			return reply.status(401).send({
				error: 'UNAUTHORIZED',
				message: 'Authentication required',
			});
		}

		// Note: If the principal exists in request context, it means
		// authentication was successful and the account is active.
		// Inactive accounts should fail authentication at the token validation stage.
	};
}

/**
 * Create a preHandler hook that requires the user to have access to a specific client.
 *
 * @param getClientId - Function to extract client ID from request
 * @returns Fastify preHandler hook
 */
export function requireClientAccess(
	getClientId: (request: Parameters<preHandlerHookHandler>[0]) => string | null,
): preHandlerHookHandler {
	return async (request, reply) => {
		const principal = request.audit?.principal;

		if (!principal) {
			return reply.status(401).send({
				error: 'UNAUTHORIZED',
				message: 'Authentication required',
			});
		}

		const clientId = getClientId(request);
		if (!clientId) {
			return reply.status(400).send({
				error: 'BAD_REQUEST',
				message: 'Client ID required',
			});
		}

		// Import dynamically to avoid circular dependency
		const { canAccessClient } = await import('./authorization-service.js');

		if (!canAccessClient(principal, clientId)) {
			return reply.status(403).send({
				error: 'FORBIDDEN',
				message: 'Access to client denied',
				clientId,
			});
		}
	};
}
