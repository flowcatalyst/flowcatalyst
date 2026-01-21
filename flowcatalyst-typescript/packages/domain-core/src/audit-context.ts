/**
 * Audit Context
 *
 * Request-scoped context holding the current principal for audit logging
 * and authorization. Uses AsyncLocalStorage for automatic propagation.
 *
 * Can be populated:
 * - Automatically via middleware (for HTTP requests)
 * - Manually via `runWithPrincipal()` for tests
 * - Via `runAsSystem()` for background jobs, CLI tools, and startup tasks
 *
 * The SYSTEM principal is a special service account used for automated
 * operations that occur outside of a user request context.
 *
 * Authorization data (roles, client access) should be loaded from the database
 * (with caching), not from token claims. This ensures the platform has
 * real-time control over access and prevents stale token claims.
 */

import { AsyncLocalStorage } from 'node:async_hooks';

/**
 * User scope determining access level.
 */
export type UserScope = 'ANCHOR' | 'PARTNER' | 'CLIENT';

/**
 * Principal type.
 */
export type PrincipalType = 'USER' | 'SERVICE' | 'SYSTEM';

/**
 * Principal information stored in the audit context.
 */
export interface PrincipalInfo {
	/** Principal ID (TSID) */
	readonly id: string;
	/** Principal type */
	readonly type: PrincipalType;
	/** User scope (access level) */
	readonly scope: UserScope;
	/** Home client ID (null for ANCHOR scope) */
	readonly clientId: string | null;
	/** Role names assigned to the principal */
	readonly roles: ReadonlySet<string>;
}

/**
 * Audit context data stored in AsyncLocalStorage.
 */
export interface AuditContextData {
	principal: PrincipalInfo | null;
}

/**
 * System principal constants.
 */
export const SYSTEM_PRINCIPAL_CODE = 'SYSTEM';
export const SYSTEM_PRINCIPAL_NAME = 'System';

/**
 * AsyncLocalStorage instance for audit context.
 */
const storage = new AsyncLocalStorage<AuditContextData>();

/**
 * AuditContext provides request-scoped principal tracking.
 *
 * Uses AsyncLocalStorage for automatic propagation across async boundaries.
 */
export const AuditContext = {
	/**
	 * Get the current audit context, or null if not in a context.
	 */
	current(): AuditContextData | null {
		return storage.getStore() ?? null;
	},

	/**
	 * Get the current principal, or null if not authenticated.
	 */
	getPrincipal(): PrincipalInfo | null {
		const ctx = storage.getStore();
		return ctx?.principal ?? null;
	},

	/**
	 * Get the current principal ID, or null if not authenticated.
	 */
	getPrincipalId(): string | null {
		return AuditContext.getPrincipal()?.id ?? null;
	},

	/**
	 * Get the current principal ID, throwing if not authenticated.
	 *
	 * @throws Error if not authenticated
	 */
	requirePrincipalId(): string {
		const principalId = AuditContext.getPrincipalId();
		if (!principalId) {
			throw new Error('Authentication required');
		}
		return principalId;
	},

	/**
	 * Get the current principal, throwing if not authenticated.
	 *
	 * @throws Error if not authenticated
	 */
	requirePrincipal(): PrincipalInfo {
		const principal = AuditContext.getPrincipal();
		if (!principal) {
			throw new Error('Authentication required');
		}
		return principal;
	},

	/**
	 * Check if the context has an authenticated principal.
	 */
	isAuthenticated(): boolean {
		return AuditContext.getPrincipal() !== null;
	},

	/**
	 * Check if the current principal is the system principal.
	 */
	isSystemPrincipal(): boolean {
		const principal = AuditContext.getPrincipal();
		return principal?.type === 'SYSTEM';
	},

	/**
	 * Check if the current principal has access to all clients (ANCHOR scope).
	 */
	hasAccessToAllClients(): boolean {
		const principal = AuditContext.getPrincipal();
		return principal?.scope === 'ANCHOR';
	},

	/**
	 * Check if the current principal has access to a specific client.
	 *
	 * @param clientId - The client ID to check
	 */
	hasAccessToClient(clientId: string): boolean {
		const principal = AuditContext.getPrincipal();
		if (!principal) {
			return false;
		}

		// ANCHOR scope has access to all clients
		if (principal.scope === 'ANCHOR') {
			return true;
		}

		// CLIENT scope only has access to their home client
		if (principal.scope === 'CLIENT') {
			return clientId === principal.clientId;
		}

		// PARTNER scope - check home client (partner_client_access would need to be checked separately)
		if (principal.scope === 'PARTNER') {
			return clientId === principal.clientId;
		}

		return false;
	},

	/**
	 * Get the roles assigned to the current principal.
	 */
	getRoles(): ReadonlySet<string> {
		const principal = AuditContext.getPrincipal();
		return principal?.roles ?? new Set();
	},

	/**
	 * Check if the current principal has a specific role.
	 *
	 * @param roleName - The role name to check
	 */
	hasRole(roleName: string): boolean {
		return AuditContext.getRoles().has(roleName);
	},

	/**
	 * Get the home client ID for the current principal.
	 */
	getHomeClientId(): string | null {
		return AuditContext.getPrincipal()?.clientId ?? null;
	},

	/**
	 * Run a function with a specific principal.
	 *
	 * @param principal - The principal information
	 * @param fn - The function to run
	 * @returns The result of the function
	 */
	runWithPrincipal<T>(principal: PrincipalInfo, fn: () => T): T {
		const ctx: AuditContextData = { principal };
		return storage.run(ctx, fn);
	},

	/**
	 * Run an async function with a specific principal.
	 *
	 * @param principal - The principal information
	 * @param fn - The async function to run
	 * @returns A promise that resolves to the result of the function
	 */
	async runWithPrincipalAsync<T>(principal: PrincipalInfo, fn: () => Promise<T>): Promise<T> {
		const ctx: AuditContextData = { principal };
		return storage.run(ctx, fn);
	},

	/**
	 * Run a function as the system principal.
	 *
	 * Use this for background jobs, startup tasks, and CLI tools.
	 *
	 * @param systemPrincipalId - The ID of the system principal (must be created first)
	 * @param fn - The function to run
	 * @returns The result of the function
	 */
	runAsSystem<T>(systemPrincipalId: string, fn: () => T): T {
		const systemPrincipal: PrincipalInfo = {
			id: systemPrincipalId,
			type: 'SYSTEM',
			scope: 'ANCHOR', // System has full access
			clientId: null,
			roles: new Set(['SYSTEM']),
		};
		return AuditContext.runWithPrincipal(systemPrincipal, fn);
	},

	/**
	 * Run an async function as the system principal.
	 *
	 * @param systemPrincipalId - The ID of the system principal
	 * @param fn - The async function to run
	 * @returns A promise that resolves to the result of the function
	 */
	async runAsSystemAsync<T>(systemPrincipalId: string, fn: () => Promise<T>): Promise<T> {
		const systemPrincipal: PrincipalInfo = {
			id: systemPrincipalId,
			type: 'SYSTEM',
			scope: 'ANCHOR',
			clientId: null,
			roles: new Set(['SYSTEM']),
		};
		return AuditContext.runWithPrincipalAsync(systemPrincipal, fn);
	},

	/**
	 * Create a principal info object.
	 *
	 * @param id - Principal ID
	 * @param type - Principal type
	 * @param scope - User scope
	 * @param clientId - Home client ID (null for ANCHOR)
	 * @param roles - Role names
	 * @returns Principal info
	 */
	createPrincipal(
		id: string,
		type: PrincipalType,
		scope: UserScope,
		clientId: string | null,
		roles: string[] = [],
	): PrincipalInfo {
		return {
			id,
			type,
			scope,
			clientId,
			roles: new Set(roles),
		};
	},
};
