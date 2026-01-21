/**
 * Type of Identity Provider used for authentication.
 */
export const IdpType = {
	/** Internal username/password authentication */
	INTERNAL: 'INTERNAL',
	/** OIDC authentication (e.g., Keycloak) */
	OIDC: 'OIDC',
} as const;

export type IdpType = (typeof IdpType)[keyof typeof IdpType];
