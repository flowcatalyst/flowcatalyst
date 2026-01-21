/**
 * Platform IAM Permissions
 *
 * Permissions for identity and access management operations.
 */

import { makePermission, type PermissionDefinition } from '../permission-definition.js';

const SUBDOMAIN = 'platform';
const CONTEXT = 'iam';

/**
 * User permissions.
 */
export const USER_PERMISSIONS = {
	CREATE: makePermission(SUBDOMAIN, CONTEXT, 'user', 'create', 'Create users'),
	READ: makePermission(SUBDOMAIN, CONTEXT, 'user', 'read', 'Read user details'),
	UPDATE: makePermission(SUBDOMAIN, CONTEXT, 'user', 'update', 'Update user details'),
	DELETE: makePermission(SUBDOMAIN, CONTEXT, 'user', 'delete', 'Delete users'),
	MANAGE: makePermission(SUBDOMAIN, CONTEXT, 'user', 'manage', 'Full user management'),
	ACTIVATE: makePermission(SUBDOMAIN, CONTEXT, 'user', 'activate', 'Activate users'),
	DEACTIVATE: makePermission(SUBDOMAIN, CONTEXT, 'user', 'deactivate', 'Deactivate users'),
	ASSIGN_ROLES: makePermission(SUBDOMAIN, CONTEXT, 'user', 'assign-roles', 'Assign roles to users'),
} as const;

/**
 * Role permissions.
 */
export const ROLE_PERMISSIONS = {
	CREATE: makePermission(SUBDOMAIN, CONTEXT, 'role', 'create', 'Create roles'),
	READ: makePermission(SUBDOMAIN, CONTEXT, 'role', 'read', 'Read role details'),
	UPDATE: makePermission(SUBDOMAIN, CONTEXT, 'role', 'update', 'Update role details'),
	DELETE: makePermission(SUBDOMAIN, CONTEXT, 'role', 'delete', 'Delete roles'),
	MANAGE: makePermission(SUBDOMAIN, CONTEXT, 'role', 'manage', 'Full role management'),
} as const;

/**
 * Client access permissions.
 */
export const CLIENT_ACCESS_PERMISSIONS = {
	GRANT: makePermission(SUBDOMAIN, CONTEXT, 'client-access', 'grant', 'Grant client access to users'),
	REVOKE: makePermission(SUBDOMAIN, CONTEXT, 'client-access', 'revoke', 'Revoke client access from users'),
	READ: makePermission(SUBDOMAIN, CONTEXT, 'client-access', 'read', 'Read client access grants'),
} as const;

/**
 * Permission permissions.
 */
export const PERMISSION_PERMISSIONS = {
	READ: makePermission(SUBDOMAIN, CONTEXT, 'permission', 'read', 'Read permissions'),
} as const;

/**
 * Auth config permissions.
 */
export const AUTH_CONFIG_PERMISSIONS = {
	CREATE: makePermission(SUBDOMAIN, CONTEXT, 'auth-config', 'create', 'Create auth configs'),
	READ: makePermission(SUBDOMAIN, CONTEXT, 'auth-config', 'read', 'Read auth config details'),
	UPDATE: makePermission(SUBDOMAIN, CONTEXT, 'auth-config', 'update', 'Update auth config details'),
	DELETE: makePermission(SUBDOMAIN, CONTEXT, 'auth-config', 'delete', 'Delete auth configs'),
	MANAGE: makePermission(SUBDOMAIN, CONTEXT, 'auth-config', 'manage', 'Full auth config management'),
} as const;

/**
 * All IAM permissions.
 */
export const IAM_PERMISSIONS: readonly PermissionDefinition[] = [
	...Object.values(USER_PERMISSIONS),
	...Object.values(ROLE_PERMISSIONS),
	...Object.values(CLIENT_ACCESS_PERMISSIONS),
	...Object.values(PERMISSION_PERMISSIONS),
	...Object.values(AUTH_CONFIG_PERMISSIONS),
];
