/**
 * AuthRole Entity
 *
 * Represents a role definition stored in the database.
 *
 * Roles can come from three sources:
 * - CODE: Defined in code, synced to DB at startup
 * - DATABASE: Created by administrators through the UI
 * - SDK: Registered by external applications via the SDK API
 *
 * The role name is prefixed with the application code (e.g., "platform:tenant-admin").
 * SDK roles are auto-prefixed when registered.
 */

import { generate } from '@flowcatalyst/tsid';

/**
 * Source of a role definition.
 */
export type RoleSource = 'CODE' | 'DATABASE' | 'SDK';

export const RoleSource = {
	/** Defined in code, synced to DB at startup */
	CODE: 'CODE' as const,
	/** Created by administrators through the UI */
	DATABASE: 'DATABASE' as const,
	/** Registered by external applications via the SDK API */
	SDK: 'SDK' as const,
} as const;

/**
 * AuthRole entity.
 */
export interface AuthRole {
	readonly id: string;

	/** The application this role belongs to (ID reference) */
	readonly applicationId: string | null;

	/** The application code (denormalized for queries) */
	readonly applicationCode: string | null;

	/** Full role name with application prefix (e.g., "platform:tenant-admin") */
	readonly name: string;

	/** Human-readable display name (e.g., "Tenant Administrator") */
	readonly displayName: string;

	/** Description of what this role grants access to */
	readonly description: string | null;

	/** Set of permission strings granted by this role */
	readonly permissions: readonly string[];

	/** Source of this role definition */
	readonly source: RoleSource;

	/** If true, this role syncs to IDPs configured for client-managed roles */
	readonly clientManaged: boolean;

	readonly createdAt: Date;
	readonly updatedAt: Date;
}

/**
 * Input for creating a new AuthRole.
 */
export interface CreateAuthRoleInput {
	/** The application this role belongs to */
	readonly applicationId?: string | null;
	/** The application code */
	readonly applicationCode?: string | null;
	/** Short role name (will be prefixed with applicationCode if provided) */
	readonly shortName: string;
	/** Human-readable display name */
	readonly displayName: string;
	readonly description?: string | null;
	readonly permissions?: readonly string[];
	readonly source?: RoleSource;
	readonly clientManaged?: boolean;
}

/**
 * Create a new AuthRole entity.
 */
export function createAuthRole(input: CreateAuthRoleInput): AuthRole {
	const now = new Date();
	// Build full name with prefix if application code is provided
	const name = input.applicationCode
		? `${input.applicationCode}:${input.shortName.toLowerCase()}`
		: input.shortName.toLowerCase();

	return {
		id: generate('ROLE'),
		applicationId: input.applicationId ?? null,
		applicationCode: input.applicationCode ?? null,
		name,
		displayName: input.displayName,
		description: input.description ?? null,
		permissions: input.permissions ?? [],
		source: input.source ?? RoleSource.DATABASE,
		clientManaged: input.clientManaged ?? false,
		createdAt: now,
		updatedAt: now,
	};
}

/**
 * Update an AuthRole entity.
 */
export function updateAuthRole(
	role: AuthRole,
	updates: {
		displayName?: string;
		description?: string | null;
		permissions?: readonly string[];
		clientManaged?: boolean;
	},
): AuthRole {
	return {
		...role,
		displayName: updates.displayName ?? role.displayName,
		description: updates.description !== undefined ? updates.description : role.description,
		permissions: updates.permissions ?? role.permissions,
		clientManaged: updates.clientManaged ?? role.clientManaged,
		updatedAt: new Date(),
	};
}

/**
 * Extract the role name without the application prefix.
 */
export function getShortName(role: AuthRole): string {
	if (role.name.includes(':')) {
		return role.name.substring(role.name.indexOf(':') + 1);
	}
	return role.name;
}
