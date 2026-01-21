/**
 * Auth Config Type
 *
 * Type of authentication configuration, determining user scope and access.
 */

/**
 * Auth config types - determines user access scope.
 * - ANCHOR: Platform-wide access, users get ANCHOR scope (all clients)
 * - PARTNER: Partner access, users get PARTNER scope (granted clients only)
 * - CLIENT: Client-specific access, users get CLIENT scope (primary + additional clients)
 */
export type AuthConfigType = 'ANCHOR' | 'PARTNER' | 'CLIENT';

export const AuthConfigType = {
	ANCHOR: 'ANCHOR' as const,
	PARTNER: 'PARTNER' as const,
	CLIENT: 'CLIENT' as const,
} as const;
