/**
 * OAuth Client Type
 *
 * Types of OAuth clients based on their ability to securely store secrets.
 */

/**
 * OAuth client types.
 * - PUBLIC: Cannot securely store secrets (SPAs, mobile apps)
 * - CONFIDENTIAL: Can securely store secrets (server-side apps)
 */
export type OAuthClientType = 'PUBLIC' | 'CONFIDENTIAL';

export const OAuthClientType = {
	PUBLIC: 'PUBLIC' as const,
	CONFIDENTIAL: 'CONFIDENTIAL' as const,
} as const;
