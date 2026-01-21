/**
 * Platform Auth Admin Role
 *
 * Manages authentication configuration.
 */

import { makeRole, type RoleDefinition } from '../role-definition.js';
import { CLIENT_AUTH_CONFIG_PERMISSIONS, OAUTH_CLIENT_PERMISSIONS } from '../permissions/platform-auth.js';

/**
 * Platform Auth Admin role.
 * Can manage client auth configs and OAuth clients.
 */
export const PLATFORM_AUTH_ADMIN: RoleDefinition = makeRole(
	'PLATFORM_AUTH_ADMIN',
	'Platform Auth Admin',
	'Manages authentication configuration',
	[
		// Client auth config management
		CLIENT_AUTH_CONFIG_PERMISSIONS.CREATE,
		CLIENT_AUTH_CONFIG_PERMISSIONS.READ,
		CLIENT_AUTH_CONFIG_PERMISSIONS.UPDATE,
		CLIENT_AUTH_CONFIG_PERMISSIONS.DELETE,

		// OAuth client management
		OAUTH_CLIENT_PERMISSIONS.CREATE,
		OAUTH_CLIENT_PERMISSIONS.READ,
		OAUTH_CLIENT_PERMISSIONS.UPDATE,
		OAUTH_CLIENT_PERMISSIONS.DELETE,
		OAUTH_CLIENT_PERMISSIONS.REGENERATE_SECRET,
	],
);

/**
 * Platform Auth Read-Only role.
 * Can view auth configuration but not modify it.
 */
export const PLATFORM_AUTH_READONLY: RoleDefinition = makeRole(
	'PLATFORM_AUTH_READONLY',
	'Platform Auth Read-Only',
	'View-only access to authentication configuration',
	[CLIENT_AUTH_CONFIG_PERMISSIONS.READ, OAUTH_CLIENT_PERMISSIONS.READ],
);
