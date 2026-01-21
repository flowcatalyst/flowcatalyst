/**
 * Platform Auth Permissions
 *
 * Permissions for authentication configuration operations.
 */

import { makePermission, type PermissionDefinition } from '../permission-definition.js';

const SUBDOMAIN = 'platform';
const CONTEXT = 'auth';

/**
 * Client auth config permissions.
 */
export const CLIENT_AUTH_CONFIG_PERMISSIONS = {
	CREATE: makePermission(SUBDOMAIN, CONTEXT, 'client-auth-config', 'create', 'Create client auth configs'),
	READ: makePermission(SUBDOMAIN, CONTEXT, 'client-auth-config', 'read', 'Read client auth configs'),
	UPDATE: makePermission(SUBDOMAIN, CONTEXT, 'client-auth-config', 'update', 'Update client auth configs'),
	DELETE: makePermission(SUBDOMAIN, CONTEXT, 'client-auth-config', 'delete', 'Delete client auth configs'),
	MANAGE: makePermission(SUBDOMAIN, CONTEXT, 'client-auth-config', 'manage', 'Full client auth config management'),
} as const;

/**
 * OAuth client permissions.
 */
export const OAUTH_CLIENT_PERMISSIONS = {
	CREATE: makePermission(SUBDOMAIN, CONTEXT, 'oauth-client', 'create', 'Create OAuth clients'),
	READ: makePermission(SUBDOMAIN, CONTEXT, 'oauth-client', 'read', 'Read OAuth client details'),
	UPDATE: makePermission(SUBDOMAIN, CONTEXT, 'oauth-client', 'update', 'Update OAuth client details'),
	DELETE: makePermission(SUBDOMAIN, CONTEXT, 'oauth-client', 'delete', 'Delete OAuth clients'),
	MANAGE: makePermission(SUBDOMAIN, CONTEXT, 'oauth-client', 'manage', 'Full OAuth client management'),
	REGENERATE_SECRET: makePermission(
		SUBDOMAIN,
		CONTEXT,
		'oauth-client',
		'regenerate-secret',
		'Regenerate OAuth client secret',
	),
} as const;

/**
 * All auth permissions.
 */
export const AUTH_PERMISSIONS: readonly PermissionDefinition[] = [
	...Object.values(CLIENT_AUTH_CONFIG_PERMISSIONS),
	...Object.values(OAUTH_CLIENT_PERMISSIONS),
];
