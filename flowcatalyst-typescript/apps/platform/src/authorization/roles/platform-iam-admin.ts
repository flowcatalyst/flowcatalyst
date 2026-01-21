/**
 * Platform IAM Admin Role
 *
 * Manages users, roles, and access control.
 */

import { makeRole, type RoleDefinition } from '../role-definition.js';
import { USER_PERMISSIONS, ROLE_PERMISSIONS, CLIENT_ACCESS_PERMISSIONS } from '../permissions/platform-iam.js';

/**
 * Platform IAM Admin role.
 * Can manage users, roles, and client access.
 */
export const PLATFORM_IAM_ADMIN: RoleDefinition = makeRole(
	'PLATFORM_IAM_ADMIN',
	'Platform IAM Admin',
	'Manages users, roles, and access control',
	[
		// User management
		USER_PERMISSIONS.CREATE,
		USER_PERMISSIONS.READ,
		USER_PERMISSIONS.UPDATE,
		USER_PERMISSIONS.DELETE,
		USER_PERMISSIONS.ACTIVATE,
		USER_PERMISSIONS.DEACTIVATE,
		USER_PERMISSIONS.ASSIGN_ROLES,

		// Role management
		ROLE_PERMISSIONS.CREATE,
		ROLE_PERMISSIONS.READ,
		ROLE_PERMISSIONS.UPDATE,
		ROLE_PERMISSIONS.DELETE,

		// Client access management
		CLIENT_ACCESS_PERMISSIONS.GRANT,
		CLIENT_ACCESS_PERMISSIONS.REVOKE,
		CLIENT_ACCESS_PERMISSIONS.READ,
	],
);

/**
 * Platform IAM Read-Only role.
 * Can view users and roles but not modify them.
 */
export const PLATFORM_IAM_READONLY: RoleDefinition = makeRole(
	'PLATFORM_IAM_READONLY',
	'Platform IAM Read-Only',
	'View-only access to users and roles',
	[USER_PERMISSIONS.READ, ROLE_PERMISSIONS.READ, CLIENT_ACCESS_PERMISSIONS.READ],
);
