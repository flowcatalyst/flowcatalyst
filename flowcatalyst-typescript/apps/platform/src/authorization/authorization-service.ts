/**
 * Authorization Service
 *
 * Checks if principals have specific permissions based on their assigned roles.
 */

import type { PrincipalInfo, UserScope } from '@flowcatalyst/domain-core';
import { matchesPattern } from './permission-definition.js';
import { permissionRegistry } from './permission-registry.js';

/**
 * Principal with roles for authorization checks.
 * Uses ReadonlySet<string> for roles (matching PrincipalInfo from domain-core).
 */
export interface AuthorizablePrincipal {
	readonly id: string;
	readonly roles: ReadonlySet<string>;
	readonly scope?: UserScope;
	readonly clientId?: string | null;
}

/**
 * Check if a principal has a specific permission.
 *
 * @param principal - Principal to check
 * @param permission - Permission string to check (e.g., "platform:iam:user:create")
 * @returns True if principal has the permission
 */
export function hasPermission(principal: AuthorizablePrincipal, permission: string): boolean {
	for (const roleName of principal.roles) {
		const rolePermissions = permissionRegistry.getRolePermissions(roleName);
		for (const pattern of rolePermissions) {
			if (matchesPattern(permission, pattern)) {
				return true;
			}
		}
	}

	return false;
}

/**
 * Check if a principal has any of the specified permissions.
 *
 * @param principal - Principal to check
 * @param permissions - Permission strings to check
 * @returns True if principal has any of the permissions
 */
export function hasAnyPermission(principal: AuthorizablePrincipal, permissions: readonly string[]): boolean {
	for (const permission of permissions) {
		if (hasPermission(principal, permission)) {
			return true;
		}
	}
	return false;
}

/**
 * Check if a principal has all of the specified permissions.
 *
 * @param principal - Principal to check
 * @param permissions - Permission strings to check
 * @returns True if principal has all of the permissions
 */
export function hasAllPermissions(principal: AuthorizablePrincipal, permissions: readonly string[]): boolean {
	for (const permission of permissions) {
		if (!hasPermission(principal, permission)) {
			return false;
		}
	}
	return true;
}

/**
 * Get all effective permissions for a principal.
 *
 * @param principal - Principal to get permissions for
 * @returns Set of permission patterns the principal has
 */
export function getEffectivePermissions(principal: AuthorizablePrincipal): ReadonlySet<string> {
	const permissions = new Set<string>();

	for (const roleName of principal.roles) {
		const rolePermissions = permissionRegistry.getRolePermissions(roleName);
		for (const permission of rolePermissions) {
			permissions.add(permission);
		}
	}

	return permissions;
}

/**
 * Check if a principal can access a specific client.
 *
 * @param principal - Principal info to check (from request context)
 * @param clientId - Client ID to check access for
 * @returns True if principal can access the client
 */
export function canAccessClient(principal: PrincipalInfo, clientId: string): boolean {
	switch (principal.scope) {
		case 'ANCHOR':
			// Anchor users can access all clients
			return true;
		case 'PARTNER':
			// Partner users can access explicitly assigned clients
			// For now, check if they have the clientId in their roles context
			// This would be extended with a client access grant table
			return principal.clientId === clientId || hasClientAccess(principal, clientId);
		case 'CLIENT':
			// Client users can only access their home client
			return principal.clientId === clientId;
		default:
			return false;
	}
}

/**
 * Check if a principal has explicit client access grant.
 * This is a placeholder - actual implementation would query client access grants.
 */
function hasClientAccess(_principal: PrincipalInfo, _clientId: string): boolean {
	// TODO: Query client access grants table
	// For now, return false - explicit grants not yet implemented
	return false;
}
