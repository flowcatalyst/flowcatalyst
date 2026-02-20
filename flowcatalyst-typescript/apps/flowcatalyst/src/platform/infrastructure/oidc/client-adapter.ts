/**
 * Client Adapter for oidc-provider
 *
 * Integrates oidc-provider with the OAuthClient repository to provide
 * OAuth client configuration for token issuance and validation.
 */

import type { ClientMetadata } from "oidc-provider";
import type { OAuthClientRepository } from "../persistence/repositories/oauth-client-repository.js";
import type { OAuthClient } from "../../domain/oauth/oauth-client.js";
import type { EncryptionService } from "@flowcatalyst/platform-crypto";

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
		response_types: client.grantTypes.includes("authorization_code")
			? ["code"]
			: [],

		// Token endpoint auth method based on client type
		// Use client_secret_post for compatibility with SDKs that send credentials as form params
		token_endpoint_auth_method:
			client.clientType === "CONFIDENTIAL" ? "client_secret_post" : "none",

		// PKCE requirement
		// For PUBLIC clients, always require PKCE
		// For CONFIDENTIAL clients, use the pkceRequired setting
		...(client.clientType === "PUBLIC" || client.pkceRequired
			? {
					require_pkce: true,
					code_challenge_methods: ["S256"],
				}
			: {}),

		// Default scope
		default_max_age: 3600, // 1 hour

		// Post-logout redirect URIs — derive from redirect URI origins + allowed origins.
		// This enables RP-initiated logout (the SPA redirects to /session/end with
		// post_logout_redirect_uri and oidc-provider validates it against this list).
		post_logout_redirect_uris: derivePostLogoutUris(client),
	};

	// Add secret for confidential clients
	if (client.clientType === "CONFIDENTIAL" && decryptedSecret) {
		metadata.client_secret = decryptedSecret;
	}

	// Default scopes
	if (client.defaultScopes) {
		metadata.default_acr_values = []; // Can be extended for ACR
	}

	return metadata;
}

/**
 * Derive post-logout redirect URIs from redirect URIs and allowed origins.
 * SPA logout typically redirects to the app origin (e.g., http://localhost:3000).
 * Wildcard patterns (e.g., https://qa-*.example.com/callback) are skipped since
 * oidc-provider only supports exact post-logout URIs — the wildcard redirect URIs
 * are handled separately by the custom redirectUriAllowed override.
 */
function derivePostLogoutUris(client: OAuthClient): string[] {
	const uris = new Set<string>();

	// Extract origins from redirect URIs (skip wildcard patterns)
	for (const uri of client.redirectUris) {
		if (uri.includes("*")) continue;
		try {
			const url = new URL(uri);
			uris.add(url.origin);
		} catch {
			// Skip invalid URIs
		}
	}

	// Add allowed origins directly (skip wildcard patterns)
	for (const origin of client.allowedOrigins) {
		if (origin.includes("*")) continue;
		uris.add(origin);
	}

	return [...uris];
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
	return async function loadClient(
		clientId: string,
	): Promise<ClientMetadata | undefined> {
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
	return async function validateClient(
		clientId: string,
	): Promise<OAuthClient | null> {
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
