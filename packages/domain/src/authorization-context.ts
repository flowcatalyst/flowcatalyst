/**
 * Authorization Context
 *
 * Carries authorization information about the executing principal,
 * including what applications and clients they can access.
 *
 * This context enables resource-level authorization checks in use cases:
 * - Application access — which applications can the principal access?
 * - Client access scope — which clients can the principal access?
 * - Role and permission information for action-level checks
 *
 * A null AuthorizationContext (on ExecutionContext) means the call is
 * system/internal and bypasses resource-level authorization.
 */

/**
 * Authorization context data for the current principal.
 */
export interface AuthorizationContext {
	/** The authenticated principal's ID */
	readonly principalId: string;
	/** Roles assigned to the principal */
	readonly roles: ReadonlySet<string>;
	/** Permissions derived from roles */
	readonly permissions: ReadonlySet<string>;
	/** Application IDs this principal can access (null = all) */
	readonly accessibleApplicationIds: ReadonlySet<string> | null;
	/** Application codes this principal can access (null = all) */
	readonly accessibleApplicationCodes: ReadonlySet<string> | null;
	/** Client IDs this principal can access (null = all) */
	readonly accessibleClientIds: ReadonlySet<string> | null;
	/** Whether this principal can access all clients (ANCHOR scope) */
	readonly canAccessAllClients: boolean;
}

/**
 * AuthorizationContext helper functions.
 */
export const AuthorizationContext = {
	/**
	 * Check if the principal can access a specific application by ID.
	 */
	canAccessApplication(
		authz: AuthorizationContext,
		applicationId: string,
	): boolean {
		return (
			authz.accessibleApplicationIds !== null &&
			authz.accessibleApplicationIds.has(applicationId)
		);
	},

	/**
	 * Check if the principal can access a specific application by code.
	 */
	canAccessApplicationByCode(
		authz: AuthorizationContext,
		applicationCode: string,
	): boolean {
		return (
			authz.accessibleApplicationCodes !== null &&
			authz.accessibleApplicationCodes.has(applicationCode)
		);
	},

	/**
	 * Check if the principal can access a resource with the given code prefix.
	 *
	 * Resource codes follow the pattern "{applicationCode}:{resourceName}".
	 * For example, "tms:shipment-event" belongs to the "tms" application.
	 */
	canAccessResourceWithPrefix(
		authz: AuthorizationContext,
		resourceCode: string,
	): boolean {
		if (!resourceCode) return false;
		if (authz.accessibleApplicationCodes === null) return false;
		for (const code of authz.accessibleApplicationCodes) {
			if (resourceCode.startsWith(`${code}:`)) return true;
		}
		return false;
	},

	/**
	 * Check if the principal can access a specific client.
	 */
	canAccessClient(authz: AuthorizationContext, clientId: string): boolean {
		if (authz.canAccessAllClients) return true;
		return (
			authz.accessibleClientIds !== null &&
			authz.accessibleClientIds.has(clientId)
		);
	},

	/**
	 * Check if the principal is a platform administrator.
	 */
	isPlatformAdmin(authz: AuthorizationContext): boolean {
		return (
			authz.roles.has("platform:super-admin") ||
			authz.roles.has("platform:platform-admin")
		);
	},

	/**
	 * Check if the principal has a specific role.
	 */
	hasRole(authz: AuthorizationContext, roleName: string): boolean {
		return authz.roles.has(roleName);
	},

	/**
	 * Check if the principal has a specific permission.
	 */
	hasPermission(authz: AuthorizationContext, permissionName: string): boolean {
		return authz.permissions.has(permissionName);
	},
};
