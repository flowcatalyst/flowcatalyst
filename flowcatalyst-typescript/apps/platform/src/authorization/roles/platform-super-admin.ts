/**
 * Platform Super Admin Role
 *
 * Has full access to all platform operations.
 */

import { makeRole, type RoleDefinition } from '../role-definition.js';

/**
 * Platform Super Admin role.
 * Has wildcard access to all platform operations.
 */
export const PLATFORM_SUPER_ADMIN: RoleDefinition = makeRole(
	'PLATFORM_SUPER_ADMIN',
	'Platform Super Admin',
	'Full access to all platform operations',
	[
		// Wildcard access to all platform operations
		'platform:*:*:*',
	],
);
