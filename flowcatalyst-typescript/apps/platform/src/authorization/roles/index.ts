/**
 * Platform Roles
 *
 * Re-exports all platform role definitions.
 */

export * from './platform-super-admin.js';
export * from './platform-iam-admin.js';
export * from './platform-admin.js';
export * from './platform-auth-admin.js';

import { PLATFORM_SUPER_ADMIN } from './platform-super-admin.js';
import { PLATFORM_IAM_ADMIN, PLATFORM_IAM_READONLY } from './platform-iam-admin.js';
import { PLATFORM_ADMIN, PLATFORM_ADMIN_READONLY } from './platform-admin.js';
import { PLATFORM_AUTH_ADMIN, PLATFORM_AUTH_READONLY } from './platform-auth-admin.js';
import type { RoleDefinition } from '../role-definition.js';

/**
 * All platform roles.
 */
export const ALL_PLATFORM_ROLES: readonly RoleDefinition[] = [
	PLATFORM_SUPER_ADMIN,
	PLATFORM_IAM_ADMIN,
	PLATFORM_IAM_READONLY,
	PLATFORM_ADMIN,
	PLATFORM_ADMIN_READONLY,
	PLATFORM_AUTH_ADMIN,
	PLATFORM_AUTH_READONLY,
];
