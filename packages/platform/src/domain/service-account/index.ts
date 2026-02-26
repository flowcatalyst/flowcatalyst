/**
 * Service Account Domain
 */

// Types
export { WebhookAuthType } from "./webhook-auth-type.js";
export { SignatureAlgorithm } from "./signature-algorithm.js";

// Domain model
export {
	type ServiceAccountData,
	createServiceAccountData,
} from "./service-account-data.js";

export { type ServiceAccount } from "./service-account.js";

// Credential generation
export {
	generateAuthToken,
	generateSigningSecret,
	generateClientSecret,
} from "./credential-generator.js";

// Events
export {
	ServiceAccountCreated,
	ServiceAccountUpdated,
	AuthTokenRegenerated,
	SigningSecretRegenerated,
	ServiceAccountDeleted,
	type ServiceAccountCreatedData,
	type ServiceAccountUpdatedData,
	type AuthTokenRegeneratedData,
	type SigningSecretRegeneratedData,
	type ServiceAccountDeletedData,
} from "./events.js";
