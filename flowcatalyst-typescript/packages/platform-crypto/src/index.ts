/**
 * @flowcatalyst/platform-crypto
 *
 * Cryptographic services for the FlowCatalyst platform:
 * - AES-256-GCM encryption
 * - Argon2id password hashing
 * - HMAC-SHA256 webhook signing
 * - Multi-provider secret resolution
 */

// Encryption
export {
	EncryptionService,
	createEncryptionServiceFromEnv,
	generateAppKey,
	type EncryptionError,
} from './encryption.js';

// Password hashing
export { PasswordService, getPasswordService, type PasswordError } from './password.js';

// Webhook signing
export {
	SIGNATURE_HEADER,
	TIMESTAMP_HEADER,
	signWebhook,
	createSignedRequest,
	verifyWebhookSignature,
	verifyWebhookRequest,
	extractSignatureHeaders,
	generateSigningSecret,
	type SignedWebhookRequest,
} from './signing.js';

// Secret management
export {
	SecretService,
	EncryptedSecretProvider,
	AwsSecretsManagerProvider,
	AwsParameterStoreProvider,
	VaultSecretProvider,
	createSecretServiceFromEnv,
	type SecretProvider,
	type SecretError,
} from './secrets.js';

// PKCE (RFC 7636)
export {
	generateCodeVerifier,
	generateCodeChallenge,
	verifyCodeChallenge,
	isValidCodeVerifier,
	isValidCodeChallenge,
	type ChallengeMethod,
} from './pkce.js';
