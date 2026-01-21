/**
 * OAuth Grant Type
 *
 * Supported OAuth 2.0 grant types.
 */

/**
 * OAuth grant types.
 */
export type OAuthGrantType =
	| 'authorization_code'
	| 'client_credentials'
	| 'refresh_token'
	| 'password';

export const OAuthGrantType = {
	AUTHORIZATION_CODE: 'authorization_code' as const,
	CLIENT_CREDENTIALS: 'client_credentials' as const,
	REFRESH_TOKEN: 'refresh_token' as const,
	PASSWORD: 'password' as const,
} as const;
