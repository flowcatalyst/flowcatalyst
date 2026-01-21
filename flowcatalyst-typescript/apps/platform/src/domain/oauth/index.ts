/**
 * OAuth Domain
 *
 * Domain models for OAuth client management.
 */

export { type OAuthClientType, OAuthClientType as OAuthClientTypeValues } from './oauth-client-type.js';
export { type OAuthGrantType, OAuthGrantType as OAuthGrantTypeValues } from './oauth-grant-type.js';
export {
	type OAuthClient,
	type NewOAuthClient,
	type CreateOAuthClientInput,
	createOAuthClient,
	validateOAuthClient,
} from './oauth-client.js';
export {
	type OAuthClientCreatedData,
	OAuthClientCreated,
	type OAuthClientUpdatedData,
	OAuthClientUpdated,
	type OAuthClientSecretRegeneratedData,
	OAuthClientSecretRegenerated,
	type OAuthClientDeletedData,
	OAuthClientDeleted,
} from './events.js';
