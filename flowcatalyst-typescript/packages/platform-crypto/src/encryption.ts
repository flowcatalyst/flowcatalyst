/**
 * AES-256-GCM Encryption Service
 *
 * Compatible with the Java EncryptedSecretProvider implementation.
 *
 * Format:
 * - Algorithm: AES-256-GCM
 * - Key: 256-bit (32 bytes), from FLOWCATALYST_APP_KEY env var (Base64)
 * - IV: 12 bytes (random per encryption)
 * - Auth tag: 128 bits (16 bytes)
 * - Ciphertext format: `encrypted:BASE64(IV || ciphertext || tag)`
 *
 * Reference formats:
 * - `encrypted:BASE64` - Already encrypted value (stored as-is)
 * - `encrypt:plaintext` - Plaintext to be encrypted on save
 */

import crypto from 'node:crypto';
import { Result, ok, err } from 'neverthrow';

const ALGORITHM = 'aes-256-gcm';
const IV_LENGTH = 12;
const TAG_LENGTH = 16;
const KEY_LENGTH = 32;
const ENCRYPTED_PREFIX = 'encrypted:';
const PLAINTEXT_PREFIX = 'encrypt:';

export type EncryptionError =
	| { type: 'not_configured'; message: string }
	| { type: 'invalid_key'; message: string }
	| { type: 'invalid_format'; message: string }
	| { type: 'encryption_failed'; message: string; cause?: Error | undefined }
	| { type: 'decryption_failed'; message: string; cause?: Error | undefined };

/**
 * AES-256-GCM Encryption Service
 */
export class EncryptionService {
	private key: Buffer | null = null;

	/**
	 * Create an encryption service with the given app key.
	 *
	 * @param appKey - Base64-encoded 256-bit key, or undefined if not configured
	 */
	constructor(appKey?: string) {
		if (appKey) {
			const keyBuffer = Buffer.from(appKey, 'base64');
			if (keyBuffer.length !== KEY_LENGTH) {
				throw new Error(
					`Invalid APP_KEY length: expected ${KEY_LENGTH} bytes (${KEY_LENGTH * 8} bits), ` +
						`got ${keyBuffer.length} bytes. Generate with: openssl rand -base64 32`,
				);
			}
			this.key = keyBuffer;
		}
	}

	/**
	 * Check if encryption is available (key is configured)
	 */
	isAvailable(): boolean {
		return this.key !== null;
	}

	/**
	 * Encrypt a plaintext string.
	 *
	 * @param plaintext - The string to encrypt
	 * @returns Result with `encrypted:BASE64` format string, or error
	 */
	encrypt(plaintext: string): Result<string, EncryptionError> {
		if (!this.key) {
			return err({
				type: 'not_configured',
				message: 'Encryption key not configured. Set FLOWCATALYST_APP_KEY environment variable.',
			});
		}

		try {
			const iv = crypto.randomBytes(IV_LENGTH);
			const cipher = crypto.createCipheriv(ALGORITHM, this.key, iv);

			const encrypted = Buffer.concat([cipher.update(plaintext, 'utf8'), cipher.final()]);

			const tag = cipher.getAuthTag();

			// Format: IV || ciphertext || tag
			const combined = Buffer.concat([iv, encrypted, tag]);

			return ok(`${ENCRYPTED_PREFIX}${combined.toString('base64')}`);
		} catch (e) {
			return err({
				type: 'encryption_failed',
				message: `Encryption failed: ${e instanceof Error ? e.message : String(e)}`,
				cause: e instanceof Error ? e : undefined,
			});
		}
	}

	/**
	 * Decrypt an encrypted reference.
	 *
	 * @param reference - The `encrypted:BASE64` format string
	 * @returns Result with decrypted plaintext, or error
	 */
	decrypt(reference: string): Result<string, EncryptionError> {
		if (!this.key) {
			return err({
				type: 'not_configured',
				message: 'Encryption key not configured. Set FLOWCATALYST_APP_KEY environment variable.',
			});
		}

		if (!reference.startsWith(ENCRYPTED_PREFIX)) {
			return err({
				type: 'invalid_format',
				message: `Invalid encrypted reference format: expected '${ENCRYPTED_PREFIX}' prefix`,
			});
		}

		try {
			const combined = Buffer.from(reference.slice(ENCRYPTED_PREFIX.length), 'base64');

			if (combined.length < IV_LENGTH + TAG_LENGTH) {
				return err({
					type: 'invalid_format',
					message: 'Encrypted data is too short',
				});
			}

			const iv = combined.subarray(0, IV_LENGTH);
			const tag = combined.subarray(-TAG_LENGTH);
			const encrypted = combined.subarray(IV_LENGTH, -TAG_LENGTH);

			const decipher = crypto.createDecipheriv(ALGORITHM, this.key, iv);
			decipher.setAuthTag(tag);

			const decrypted = Buffer.concat([decipher.update(encrypted), decipher.final()]);

			return ok(decrypted.toString('utf8'));
		} catch (e) {
			return err({
				type: 'decryption_failed',
				message: `Decryption failed: ${e instanceof Error ? e.message : String(e)}`,
				cause: e instanceof Error ? e : undefined,
			});
		}
	}

	/**
	 * Prepare a value for storage, encrypting if needed.
	 *
	 * - If value starts with `encrypt:`, encrypt the plaintext after the prefix
	 * - If value starts with `encrypted:`, return as-is
	 * - Otherwise, return as-is (not an encrypted reference)
	 *
	 * @param reference - The reference to prepare
	 * @returns Result with storage-ready value, or error
	 */
	prepareForStorage(reference: string): Result<string, EncryptionError> {
		if (reference.startsWith(PLAINTEXT_PREFIX)) {
			const plaintext = reference.slice(PLAINTEXT_PREFIX.length);
			return this.encrypt(plaintext);
		}
		return ok(reference);
	}

	/**
	 * Check if a reference is encrypted
	 */
	isEncrypted(reference: string): boolean {
		return reference.startsWith(ENCRYPTED_PREFIX);
	}

	/**
	 * Check if a reference needs encryption
	 */
	needsEncryption(reference: string): boolean {
		return reference.startsWith(PLAINTEXT_PREFIX);
	}
}

/**
 * Create an EncryptionService from environment variables.
 *
 * Looks for FLOWCATALYST_APP_KEY environment variable.
 */
export function createEncryptionServiceFromEnv(): EncryptionService {
	const appKey = process.env['FLOWCATALYST_APP_KEY'];
	return new EncryptionService(appKey);
}

/**
 * Generate a random 256-bit key suitable for FLOWCATALYST_APP_KEY.
 *
 * @returns Base64-encoded 32-byte key
 */
export function generateAppKey(): string {
	return crypto.randomBytes(KEY_LENGTH).toString('base64');
}
