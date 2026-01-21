/**
 * Platform Admin Permissions
 *
 * Permissions for platform administration operations.
 */

import { makePermission, type PermissionDefinition } from '../permission-definition.js';

const SUBDOMAIN = 'platform';
const CONTEXT = 'admin';

/**
 * Client permissions.
 */
export const CLIENT_PERMISSIONS = {
	CREATE: makePermission(SUBDOMAIN, CONTEXT, 'client', 'create', 'Create clients'),
	READ: makePermission(SUBDOMAIN, CONTEXT, 'client', 'read', 'Read client details'),
	UPDATE: makePermission(SUBDOMAIN, CONTEXT, 'client', 'update', 'Update client details'),
	DELETE: makePermission(SUBDOMAIN, CONTEXT, 'client', 'delete', 'Delete clients'),
	MANAGE: makePermission(SUBDOMAIN, CONTEXT, 'client', 'manage', 'Full client management'),
	ACTIVATE: makePermission(SUBDOMAIN, CONTEXT, 'client', 'activate', 'Activate clients'),
	SUSPEND: makePermission(SUBDOMAIN, CONTEXT, 'client', 'suspend', 'Suspend clients'),
	DEACTIVATE: makePermission(SUBDOMAIN, CONTEXT, 'client', 'deactivate', 'Deactivate clients'),
} as const;

/**
 * Anchor domain permissions.
 */
export const ANCHOR_DOMAIN_PERMISSIONS = {
	CREATE: makePermission(SUBDOMAIN, CONTEXT, 'anchor-domain', 'create', 'Create anchor domains'),
	READ: makePermission(SUBDOMAIN, CONTEXT, 'anchor-domain', 'read', 'Read anchor domains'),
	UPDATE: makePermission(SUBDOMAIN, CONTEXT, 'anchor-domain', 'update', 'Update anchor domains'),
	DELETE: makePermission(SUBDOMAIN, CONTEXT, 'anchor-domain', 'delete', 'Delete anchor domains'),
	MANAGE: makePermission(SUBDOMAIN, CONTEXT, 'anchor-domain', 'manage', 'Full anchor domain management'),
} as const;

/**
 * Application permissions.
 */
export const APPLICATION_PERMISSIONS = {
	CREATE: makePermission(SUBDOMAIN, CONTEXT, 'application', 'create', 'Create applications'),
	READ: makePermission(SUBDOMAIN, CONTEXT, 'application', 'read', 'Read application details'),
	UPDATE: makePermission(SUBDOMAIN, CONTEXT, 'application', 'update', 'Update application details'),
	DELETE: makePermission(SUBDOMAIN, CONTEXT, 'application', 'delete', 'Delete applications'),
	MANAGE: makePermission(SUBDOMAIN, CONTEXT, 'application', 'manage', 'Full application management'),
	ACTIVATE: makePermission(SUBDOMAIN, CONTEXT, 'application', 'activate', 'Activate applications'),
	DEACTIVATE: makePermission(SUBDOMAIN, CONTEXT, 'application', 'deactivate', 'Deactivate applications'),
	ENABLE_CLIENT: makePermission(SUBDOMAIN, CONTEXT, 'application', 'enable-client', 'Enable application for client'),
	DISABLE_CLIENT: makePermission(
		SUBDOMAIN,
		CONTEXT,
		'application',
		'disable-client',
		'Disable application for client',
	),
} as const;

/**
 * Audit log permissions.
 */
export const AUDIT_LOG_PERMISSIONS = {
	READ: makePermission(SUBDOMAIN, CONTEXT, 'audit-log', 'read', 'Read audit logs'),
	EXPORT: makePermission(SUBDOMAIN, CONTEXT, 'audit-log', 'export', 'Export audit logs'),
} as const;

/**
 * All admin permissions.
 */
export const ADMIN_PERMISSIONS: readonly PermissionDefinition[] = [
	...Object.values(CLIENT_PERMISSIONS),
	...Object.values(ANCHOR_DOMAIN_PERMISSIONS),
	...Object.values(APPLICATION_PERMISSIONS),
	...Object.values(AUDIT_LOG_PERMISSIONS),
];
