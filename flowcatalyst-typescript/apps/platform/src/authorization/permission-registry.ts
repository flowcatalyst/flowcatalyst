/**
 * Permission Registry
 *
 * In-memory registry for permissions and roles.
 * Provides fast lookups for authorization checks.
 */

import type { PermissionDefinition } from './permission-definition.js';
import type { RoleDefinition } from './role-definition.js';

/**
 * Registry for permissions and roles.
 */
export class PermissionRegistry {
	private readonly permissions = new Map<string, PermissionDefinition>();
	private readonly roles = new Map<string, RoleDefinition>();

	/**
	 * Register a permission definition.
	 */
	registerPermission(permission: PermissionDefinition): void {
		const key = `${permission.subdomain}:${permission.context}:${permission.aggregate}:${permission.action}`;
		this.permissions.set(key, permission);
	}

	/**
	 * Register multiple permissions.
	 */
	registerPermissions(permissions: readonly PermissionDefinition[]): void {
		for (const permission of permissions) {
			this.registerPermission(permission);
		}
	}

	/**
	 * Register a role definition.
	 */
	registerRole(role: RoleDefinition): void {
		this.roles.set(role.code, role);
	}

	/**
	 * Register multiple roles.
	 */
	registerRoles(roles: readonly RoleDefinition[]): void {
		for (const role of roles) {
			this.registerRole(role);
		}
	}

	/**
	 * Get a permission by its string representation.
	 */
	getPermission(permissionString: string): PermissionDefinition | undefined {
		return this.permissions.get(permissionString);
	}

	/**
	 * Get a role by its code.
	 */
	getRole(code: string): RoleDefinition | undefined {
		return this.roles.get(code);
	}

	/**
	 * Get all registered permissions.
	 */
	getAllPermissions(): readonly PermissionDefinition[] {
		return Array.from(this.permissions.values());
	}

	/**
	 * Get all registered roles.
	 */
	getAllRoles(): readonly RoleDefinition[] {
		return Array.from(this.roles.values());
	}

	/**
	 * Get permissions for a role.
	 */
	getRolePermissions(roleCode: string): readonly string[] {
		const role = this.roles.get(roleCode);
		return role?.permissions ?? [];
	}

	/**
	 * Check if a permission string is registered.
	 */
	hasPermission(permissionString: string): boolean {
		return this.permissions.has(permissionString);
	}

	/**
	 * Check if a role code is registered.
	 */
	hasRole(code: string): boolean {
		return this.roles.has(code);
	}

	/**
	 * Clear all registered permissions and roles.
	 */
	clear(): void {
		this.permissions.clear();
		this.roles.clear();
	}
}

/**
 * Global permission registry instance.
 */
export const permissionRegistry = new PermissionRegistry();
