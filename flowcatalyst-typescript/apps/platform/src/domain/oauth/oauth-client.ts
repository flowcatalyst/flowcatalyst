/**
 * OAuth Client Entity
 *
 * Represents an OAuth 2.0 client application that can request tokens.
 */

import { generate } from '@flowcatalyst/tsid';
import type { OAuthClientType } from './oauth-client-type.js';
import type { OAuthGrantType } from './oauth-grant-type.js';

/**
 * OAuth Client entity.
 */
export interface OAuthClient {
	/** TSID primary key */
	readonly id: string;

	/** Unique client identifier (e.g., "my-app-client") */
	readonly clientId: string;

	/** Display name for the client */
	readonly clientName: string;

	/** Client type: PUBLIC or CONFIDENTIAL */
	readonly clientType: OAuthClientType;

	/** Reference to the client secret (NOT the secret itself) */
	readonly clientSecretRef: string | null;

	/** Allowed redirect URIs for authorization code flow */
	readonly redirectUris: readonly string[];

	/** Allowed CORS origins for browser-based requests */
	readonly allowedOrigins: readonly string[];

	/** Allowed OAuth grant types */
	readonly grantTypes: readonly OAuthGrantType[];

	/** Default scopes (space-separated) */
	readonly defaultScopes: string | null;

	/** Whether PKCE is required for authorization code flow */
	readonly pkceRequired: boolean;

	/** Application IDs this client can access */
	readonly applicationIds: readonly string[];

	/** Service account principal ID for machine-to-machine auth */
	readonly serviceAccountPrincipalId: string | null;

	/** Whether the client is active */
	readonly active: boolean;

	/** When the client was created */
	readonly createdAt: Date;

	/** When the client was last updated */
	readonly updatedAt: Date;
}

/**
 * Input for creating a new OAuth client.
 */
export type NewOAuthClient = Omit<OAuthClient, 'createdAt' | 'updatedAt'> & {
	createdAt?: Date;
	updatedAt?: Date;
};

/**
 * Input for creating an OAuth client.
 */
export interface CreateOAuthClientInput {
	clientId: string;
	clientName: string;
	clientType: OAuthClientType;
	clientSecretRef?: string | null | undefined;
	redirectUris?: readonly string[] | undefined;
	allowedOrigins?: readonly string[] | undefined;
	grantTypes?: readonly OAuthGrantType[] | undefined;
	defaultScopes?: string | null | undefined;
	pkceRequired?: boolean | undefined;
	applicationIds?: readonly string[] | undefined;
}

/**
 * Create a new OAuth client.
 */
export function createOAuthClient(input: CreateOAuthClientInput): NewOAuthClient {
	return {
		id: generate('OAUTH_CLIENT'),
		clientId: input.clientId,
		clientName: input.clientName,
		clientType: input.clientType,
		clientSecretRef: input.clientSecretRef ?? null,
		redirectUris: input.redirectUris ?? [],
		allowedOrigins: input.allowedOrigins ?? [],
		grantTypes: input.grantTypes ?? ['authorization_code', 'refresh_token'],
		defaultScopes: input.defaultScopes ?? null,
		pkceRequired: input.pkceRequired ?? true,
		applicationIds: input.applicationIds ?? [],
		serviceAccountPrincipalId: null,
		active: true,
	};
}

/**
 * Validate an OAuth client configuration.
 */
export function validateOAuthClient(client: OAuthClient): string | null {
	// CONFIDENTIAL clients must have a secret
	if (client.clientType === 'CONFIDENTIAL' && !client.clientSecretRef) {
		return 'Confidential clients must have a client secret';
	}

	// Authorization code flow requires redirect URIs
	if (client.grantTypes.includes('authorization_code') && client.redirectUris.length === 0) {
		return 'Authorization code grant requires at least one redirect URI';
	}

	return null;
}
