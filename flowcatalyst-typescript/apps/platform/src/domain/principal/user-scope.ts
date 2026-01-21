/**
 * Defines the access scope for a user principal.
 *
 * This determines which clients the user can access:
 * - ANCHOR: Platform admin users (typically from the anchor domain). Can access all clients.
 * - PARTNER: Partner users who can access multiple explicitly assigned clients.
 * - CLIENT: Users bound to a single client (their home client).
 *
 * The scope can be:
 * 1. Derived from email domain configuration (ClientAuthConfig.clientId)
 * 2. Explicitly set on the principal for override cases
 */
export const UserScope = {
	/**
	 * Anchor/platform users - have access to all clients.
	 * Typically users from the anchor domain (e.g., flowcatalyst.local).
	 */
	ANCHOR: 'ANCHOR',

	/**
	 * Partner users - have access to multiple explicitly assigned clients.
	 * Their accessible clients are stored in client access grants.
	 */
	PARTNER: 'PARTNER',

	/**
	 * Client users - bound to a single client (their home client).
	 * Their clientId determines their access scope.
	 */
	CLIENT: 'CLIENT',
} as const;

export type UserScope = (typeof UserScope)[keyof typeof UserScope];
