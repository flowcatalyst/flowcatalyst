/**
 * Auth Provider
 *
 * Authentication provider types for email domains.
 */

/**
 * Authentication provider types.
 * - INTERNAL: Password-based authentication managed by FlowCatalyst
 * - OIDC: External OpenID Connect authentication (e.g., Keycloak, Okta, Entra ID)
 */
export type AuthProvider = 'INTERNAL' | 'OIDC';

export const AuthProvider = {
	INTERNAL: 'INTERNAL' as const,
	OIDC: 'OIDC' as const,
} as const;
