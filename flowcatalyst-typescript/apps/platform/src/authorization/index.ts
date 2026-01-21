/**
 * Authorization Module
 *
 * Provides permission-based authorization for platform operations.
 */

// Core types and utilities
export * from './permission-definition.js';
export * from './role-definition.js';

// Registry
export * from './permission-registry.js';

// Authorization service
export * from './authorization-service.js';

// Fastify hooks
export * from './require-permission.js';

// Permission definitions
export * from './permissions/index.js';

// Role definitions
export * from './roles/index.js';

// Initialization
import { permissionRegistry } from './permission-registry.js';
import { ALL_PLATFORM_PERMISSIONS } from './permissions/index.js';
import { ALL_PLATFORM_ROLES } from './roles/index.js';

/**
 * Initialize the authorization system.
 * Registers all platform permissions and roles.
 */
export function initializeAuthorization(): void {
	permissionRegistry.registerPermissions(ALL_PLATFORM_PERMISSIONS);
	permissionRegistry.registerRoles(ALL_PLATFORM_ROLES);
}
