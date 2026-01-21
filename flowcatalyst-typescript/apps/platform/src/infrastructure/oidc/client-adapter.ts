/**
 * Client Adapter for oidc-provider
 *
 * Integrates oidc-provider with the OAuthClient repository to provide
 * OAuth client configuration for token issuance and validation.
 */

import type { ClientMetadata } from 'oidc-provider';
import type { OAuthClientRepository } from '../persistence/repositories/oauth-client-repository.js';
import type { OAuthClient } from '../../domain/oauth/oauth-client.js';
import type { EncryptionService } from '@flowcatalyst/platform-crypto';

/**
 * Maps an OAuthClient to oidc-provider ClientMetadata.
 */
function oauthClientToMetadata(
	client: OAuthClient,
	decryptedSecret: string | null,
): ClientMetadata {
	const metadata: ClientMetadata = {
		client_id: client.clientId,
		client_name: client.clientName,

		// Grant types
		grant_types: [...client.grantTypes],

		// Redirect URIs (required for authorization_code)
		redirect_uris: [...client.redirectUris],

		// Response types based on grant types
		response_types: client.grantTypes.includes('authorization_code') ? ['code'] : [],

		// Token endpoint auth method based on client type
		token_endpoint_auth_method:
			client.clientType === 'CONFIDENTIAL' ? 'client_secret_basic' : 'none',

		// PKCE requirement
		// For PUBLIC clients, always require PKCE
		// For CONFIDENTIAL clients, use the pkceRequired setting
		...(client.clientType === 'PUBLIC' || client.pkceRequired
			? {
					require_pkce: true,
					code_challenge_methods: ['S256'],
				}
			: {}),

		// Default scope
		default_max_age: 3600, // 1 hour

		// CORS origins for browser-based requests
		// oidc-provider doesn't have a direct setting for this,
		// we'll handle CORS separately in middleware
	};

	// Add secret for confidential clients
	if (client.clientType === 'CONFIDENTIAL' && decryptedSecret) {
		metadata.client_secret = decryptedSecret;
	}

	// Default scopes
	if (client.defaultScopes) {
		metadata.default_acr_values = []; // Can be extended for ACR
	}

	return metadata;
}

/**
 * Creates a client loader function for oidc-provider.
 *
 * oidc-provider can use this to dynamically load clients from the database
 * instead of requiring all clients to be pre-configured.
 */
export function createClientLoader(
	oauthClientRepository: OAuthClientRepository,
	encryptionService: EncryptionService,
): (clientId: string) => Promise<ClientMetadata | undefined> {
	return async function loadClient(clientId: string): Promise<ClientMetadata | undefined> {
		// Load client from repository
		const client = await oauthClientRepository.findByClientId(clientId);

		if (!client) {
			return undefined;
		}

		// Only return active clients
		if (!client.active) {
			return undefined;
		}

		// Decrypt client secret if present
		let decryptedSecret: string | null = null;
		if (client.clientSecretRef && encryptionService.isAvailable()) {
			const decryptResult = encryptionService.decrypt(client.clientSecretRef);
			if (decryptResult.isOk()) {
				decryptedSecret = decryptResult.value;
			}
		}

		return oauthClientToMetadata(client, decryptedSecret);
	};
}

/**
 * Creates a function to validate a client exists and is active.
 */
export function createClientValidator(
	oauthClientRepository: OAuthClientRepository,
): (clientId: string) => Promise<OAuthClient | null> {
	return async function validateClient(clientId: string): Promise<OAuthClient | null> {
		const client = await oauthClientRepository.findByClientId(clientId);

		if (!client || !client.active) {
			return null;
		}

		return client;
	};
}

/**
 * Get allowed origins for CORS from an OAuth client.
 */
export async function getClientAllowedOrigins(
	oauthClientRepository: OAuthClientRepository,
	clientId: string,
): Promise<string[]> {
	const client = await oauthClientRepository.findByClientId(clientId);

	if (!client || !client.active) {
		return [];
	}

	return [...client.allowedOrigins];
}

/**
 * Check if an origin is allowed for any active OAuth client.
 * Used for CORS preflight requests where client_id may not be known.
 */
export async function isOriginAllowedForAnyClient(
	oauthClientRepository: OAuthClientRepository,
	origin: string,
): Promise<boolean> {
	const clients = await oauthClientRepository.findActive();

	for (const client of clients) {
		if (client.allowedOrigins.includes(origin)) {
			return true;
		}
	}

	return false;
}
