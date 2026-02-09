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

// Resource-level guards
export * from './resource-guard.js';

// Query scope filtering
export * from './query-scope.js';

// Permission definitions
export * from './permissions/index.js';

// Role definitions
export * from './roles/index.js';

// Allowed role filter
export { getAllowedRoleNames, filterAllowedRoles } from './allowed-role-filter.js';

// Initialization
import { permissionRegistry } from './permission-registry.js';
import { ALL_PLATFORM_PERMISSIONS } from './permissions/index.js';
import { ALL_PLATFORM_ROLES } from './roles/index.js';
import { roleCodeToDbName } from '../bootstrap/bootstrap-service.js';

/**
 * Initialize the authorization system.
 * Registers all platform permissions and roles.
 *
 * Roles are registered under both their constant code (e.g. PLATFORM_SUPER_ADMIN)
 * and their DB name (e.g. platform:super-admin) so that permission lookups work
 * regardless of which name format the principal's role assignments use.
 */
export function initializeAuthorization(): void {
  permissionRegistry.registerPermissions(ALL_PLATFORM_PERMISSIONS);
  permissionRegistry.registerRoles(ALL_PLATFORM_ROLES);

  // Also register under DB names (e.g. "platform:super-admin") since that's
  // what gets stored in principal role assignments.
  for (const role of ALL_PLATFORM_ROLES) {
    const dbName = roleCodeToDbName(role.code);
    if (dbName !== role.code) {
      permissionRegistry.registerRole({ ...role, code: dbName });
    }
  }
}
