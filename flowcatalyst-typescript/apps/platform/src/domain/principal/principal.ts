/**
 * Principal Aggregate
 *
 * Unified identity model for both users and service accounts.
 * This is the main aggregate for identity management.
 */

import { generate } from '@flowcatalyst/tsid';
import type { PrincipalType } from './principal-type.js';
import type { UserScope } from './user-scope.js';
import type { UserIdentity } from './user-identity.js';
import type { RoleAssignment } from './role-assignment.js';

/**
 * Principal entity - represents a user or service account.
 */
export interface Principal {
	/** TSID primary key */
	readonly id: string;

	/** Type of principal (USER or SERVICE) */
	readonly type: PrincipalType;

	/**
	 * Access scope for user principals.
	 * Determines which clients this user can access.
	 * - ANCHOR: Can access all clients (platform admin)
	 * - PARTNER: Can access explicitly assigned clients
	 * - CLIENT: Can only access their home client
	 *
	 * For SERVICE principals, this may be null.
	 */
	readonly scope: UserScope | null;

	/**
	 * Client this principal belongs to (home client).
	 * - For CLIENT scope: Required, determines their access
	 * - For PARTNER scope: Optional, may have a home client
	 * - For ANCHOR scope: Should be null
	 * - For SERVICE type with client scope: The client the service account belongs to
	 */
	readonly clientId: string | null;

	/**
	 * Application this service account belongs to (for SERVICE type).
	 * When set, this service account was auto-created for the application
	 * and can manage resources prefixed with the application's code.
	 * Null for standalone service accounts or USER type principals.
	 */
	readonly applicationId: string | null;

	/** Display name */
	readonly name: string;

	/** Whether the principal is active */
	readonly active: boolean;

	/** When the principal was created */
	readonly createdAt: Date;

	/** When the principal was last updated */
	readonly updatedAt: Date;

	/** Embedded user identity (for USER type) */
	readonly userIdentity: UserIdentity | null;

	/** Embedded role assignments (denormalized for fast lookup) */
	readonly roles: readonly RoleAssignment[];
}

/**
 * Input for creating a new Principal.
 */
export type NewPrincipal = Omit<Principal, 'createdAt' | 'updatedAt'> & {
	createdAt?: Date;
	updatedAt?: Date;
};

/**
 * Create a new user principal.
 */
export function createUserPrincipal(params: {
	name: string;
	scope: UserScope;
	clientId: string | null;
	userIdentity: UserIdentity;
}): NewPrincipal {
	return {
		id: generate('PRINCIPAL'),
		type: 'USER',
		scope: params.scope,
		clientId: params.clientId,
		applicationId: null,
		name: params.name,
		active: true,
		userIdentity: params.userIdentity,
		roles: [],
	};
}

/**
 * Get role names from a principal as a Set.
 */
export function getRoleNames(principal: Principal): ReadonlySet<string> {
	return new Set(principal.roles.map((r) => r.roleName));
}

/**
 * Check if a principal has a specific role.
 */
export function hasRole(principal: Principal, roleName: string): boolean {
	return principal.roles.some((r) => r.roleName === roleName);
}

/**
 * Update a principal with new values.
 */
export function updatePrincipal(
	principal: Principal,
	updates: Partial<Pick<Principal, 'name' | 'active' | 'scope' | 'clientId' | 'userIdentity' | 'roles'>>,
): Principal {
	return {
		...principal,
		...updates,
		updatedAt: new Date(),
	};
}

/**
 * Assign roles to a principal, replacing existing roles.
 */
export function assignRoles(principal: Principal, roleAssignments: readonly RoleAssignment[]): Principal {
	return {
		...principal,
		roles: roleAssignments,
		updatedAt: new Date(),
	};
}
