/**
 * Role Definition
 *
 * Defines the structure for roles, which are collections of permissions.
 */

import type { PermissionDefinition } from './permission-definition.js';
import { permissionToString } from './permission-definition.js';

/**
 * Role definition interface.
 */
export interface RoleDefinition {
	/** Unique role code (e.g., "PLATFORM_SUPER_ADMIN") */
	readonly code: string;
	/** Human-readable name */
	readonly name: string;
	/** Description of what this role provides */
	readonly description: string;
	/** Permission strings this role grants (can include wildcards) */
	readonly permissions: readonly string[];
}

/**
 * Create a role definition from permissions.
 *
 * @param code - Role code
 * @param name - Display name
 * @param description - Role description
 * @param permissions - Array of permission definitions or patterns
 * @returns Role definition
 */
export function makeRole(
	code: string,
	name: string,
	description: string,
	permissions: readonly (PermissionDefinition | string)[],
): RoleDefinition {
	return {
		code,
		name,
		description,
		permissions: permissions.map((p) => (typeof p === 'string' ? p : permissionToString(p))),
	};
}

/**
 * Check if a role grants a specific permission.
 *
 * @param role - Role definition
 * @param permission - Permission string to check
 * @returns True if role grants the permission
 */
export function roleHasPermission(role: RoleDefinition, permission: string): boolean {
	for (const pattern of role.permissions) {
		if (permissionMatchesPattern(permission, pattern)) {
			return true;
		}
	}
	return false;
}

/**
 * Check if a permission matches a pattern (internal helper).
 */
function permissionMatchesPattern(permission: string, pattern: string): boolean {
	const permParts = permission.split(':');
	const patternParts = pattern.split(':');

	if (permParts.length !== 4 || patternParts.length !== 4) {
		return false;
	}

	for (let i = 0; i < 4; i++) {
		if (patternParts[i] !== '*' && patternParts[i] !== permParts[i]) {
			return false;
		}
	}

	return true;
}
