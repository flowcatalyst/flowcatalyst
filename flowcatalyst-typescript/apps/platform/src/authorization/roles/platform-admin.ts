/**
 * Platform Admin Role
 *
 * Manages clients, applications, and platform configuration.
 */

import { makeRole, type RoleDefinition } from '../role-definition.js';
import {
	CLIENT_PERMISSIONS,
	ANCHOR_DOMAIN_PERMISSIONS,
	APPLICATION_PERMISSIONS,
	AUDIT_LOG_PERMISSIONS,
} from '../permissions/platform-admin.js';

/**
 * Platform Admin role.
 * Can manage clients, applications, and anchor domains.
 */
export const PLATFORM_ADMIN: RoleDefinition = makeRole(
	'PLATFORM_ADMIN',
	'Platform Admin',
	'Manages clients, applications, and platform configuration',
	[
		// Client management
		CLIENT_PERMISSIONS.CREATE,
		CLIENT_PERMISSIONS.READ,
		CLIENT_PERMISSIONS.UPDATE,
		CLIENT_PERMISSIONS.ACTIVATE,
		CLIENT_PERMISSIONS.SUSPEND,
		CLIENT_PERMISSIONS.DEACTIVATE,

		// Anchor domain management
		ANCHOR_DOMAIN_PERMISSIONS.CREATE,
		ANCHOR_DOMAIN_PERMISSIONS.READ,
		ANCHOR_DOMAIN_PERMISSIONS.UPDATE,
		ANCHOR_DOMAIN_PERMISSIONS.DELETE,

		// Application management
		APPLICATION_PERMISSIONS.CREATE,
		APPLICATION_PERMISSIONS.READ,
		APPLICATION_PERMISSIONS.UPDATE,
		APPLICATION_PERMISSIONS.DELETE,
		APPLICATION_PERMISSIONS.ENABLE_CLIENT,
		APPLICATION_PERMISSIONS.DISABLE_CLIENT,

		// Audit log access
		AUDIT_LOG_PERMISSIONS.READ,
		AUDIT_LOG_PERMISSIONS.EXPORT,
	],
);

/**
 * Platform Admin Read-Only role.
 * Can view platform configuration but not modify it.
 */
export const PLATFORM_ADMIN_READONLY: RoleDefinition = makeRole(
	'PLATFORM_ADMIN_READONLY',
	'Platform Admin Read-Only',
	'View-only access to clients, applications, and platform configuration',
	[
		CLIENT_PERMISSIONS.READ,
		ANCHOR_DOMAIN_PERMISSIONS.READ,
		APPLICATION_PERMISSIONS.READ,
		AUDIT_LOG_PERMISSIONS.READ,
	],
);
