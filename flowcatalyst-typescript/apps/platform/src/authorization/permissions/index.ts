/**
 * Platform Permissions
 *
 * Re-exports all platform permission definitions.
 */

export * from './platform-iam.js';
export * from './platform-admin.js';
export * from './platform-auth.js';

import { IAM_PERMISSIONS } from './platform-iam.js';
import { ADMIN_PERMISSIONS } from './platform-admin.js';
import { AUTH_PERMISSIONS } from './platform-auth.js';
import type { PermissionDefinition } from '../permission-definition.js';

/**
 * All platform permissions.
 */
export const ALL_PLATFORM_PERMISSIONS: readonly PermissionDefinition[] = [
	...IAM_PERMISSIONS,
	...ADMIN_PERMISSIONS,
	...AUTH_PERMISSIONS,
];
