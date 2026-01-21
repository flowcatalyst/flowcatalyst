/**
 * Argon2id Password Hashing Service
 *
 * Compatible with the Java PasswordService implementation.
 *
 * Parameters (OWASP recommended for Argon2id):
 * - Memory cost: 65536 KiB (64 MiB)
 * - Time cost: 3 iterations
 * - Parallelism: 4 threads
 * - Salt length: 16 bytes
 * - Hash length: 32 bytes
 *
 * Output format: PHC string format
 * $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
 */

import argon2 from 'argon2';
import { Result, ok, err, ResultAsync } from 'neverthrow';

// Argon2id parameters matching Java implementation
const ARGON2_OPTIONS: argon2.Options = {
	type: argon2.argon2id,
	memoryCost: 65536, // 64 MiB
	timeCost: 3,
	parallelism: 4,
	hashLength: 32,
};

// Password complexity requirements
const MIN_LENGTH = 12;
const MAX_LENGTH = 128;
const SPECIAL_CHARS = "!@#$%^&*()_+-=[]{}|;':\",./<>?`~";

export type PasswordError =
	| { type: 'validation'; field: string; message: string }
	| { type: 'hashing_failed'; message: string; cause?: Error | undefined }
	| { type: 'verification_failed'; message: string; cause?: Error | undefined };

/**
 * Argon2id Password Hashing Service
 */
export class PasswordService {
	/**
	 * Hash a password using Argon2id.
	 *
	 * @param password - The plaintext password
	 * @returns Result with PHC format hash string, or error
	 */
	hash(password: string): ResultAsync<string, PasswordError> {
		return ResultAsync.fromPromise(argon2.hash(password, ARGON2_OPTIONS), (e) => ({
			type: 'hashing_failed' as const,
			message: `Password hashing failed: ${e instanceof Error ? e.message : String(e)}`,
			cause: e instanceof Error ? e : undefined,
		}));
	}

	/**
	 * Verify a password against a hash.
	 *
	 * @param password - The plaintext password to verify
	 * @param hash - The stored hash to verify against
	 * @returns true if password matches, false otherwise
	 */
	async verify(password: string, hash: string): Promise<boolean> {
		try {
			return await argon2.verify(hash, password);
		} catch {
			return false;
		}
	}

	/**
	 * Check if a hash needs to be rehashed (e.g., after parameter upgrade).
	 *
	 * @param hash - The stored hash to check
	 * @returns true if rehashing is needed
	 */
	async needsRehash(hash: string): Promise<boolean> {
		try {
			return argon2.needsRehash(hash, ARGON2_OPTIONS);
		} catch {
			return true;
		}
	}

	/**
	 * Validate password complexity requirements.
	 *
	 * Requirements:
	 * - Length: 12-128 characters
	 * - At least 1 uppercase letter
	 * - At least 1 lowercase letter
	 * - At least 1 digit
	 * - At least 1 special character
	 *
	 * @param password - The password to validate
	 * @returns Result with void on success, or validation error
	 */
	validateComplexity(password: string): Result<void, PasswordError> {
		if (password.length < MIN_LENGTH) {
			return err({
				type: 'validation',
				field: 'password',
				message: `Password must be at least ${MIN_LENGTH} characters`,
			});
		}

		if (password.length > MAX_LENGTH) {
			return err({
				type: 'validation',
				field: 'password',
				message: `Password must be at most ${MAX_LENGTH} characters`,
			});
		}

		if (!/[A-Z]/.test(password)) {
			return err({
				type: 'validation',
				field: 'password',
				message: 'Password must contain at least one uppercase letter',
			});
		}

		if (!/[a-z]/.test(password)) {
			return err({
				type: 'validation',
				field: 'password',
				message: 'Password must contain at least one lowercase letter',
			});
		}

		if (!/[0-9]/.test(password)) {
			return err({
				type: 'validation',
				field: 'password',
				message: 'Password must contain at least one digit',
			});
		}

		const hasSpecial = [...password].some((c) => SPECIAL_CHARS.includes(c));
		if (!hasSpecial) {
			return err({
				type: 'validation',
				field: 'password',
				message: 'Password must contain at least one special character',
			});
		}

		return ok(undefined);
	}

	/**
	 * Validate complexity and hash the password if valid.
	 *
	 * @param password - The plaintext password
	 * @returns Result with hash on success, or error
	 */
	validateAndHash(password: string): ResultAsync<string, PasswordError> {
		const validationResult = this.validateComplexity(password);
		if (validationResult.isErr()) {
			return ResultAsync.fromPromise(
				Promise.reject(validationResult.error),
				() => validationResult.error,
			);
		}
		return this.hash(password);
	}
}

/**
 * Singleton instance for convenience
 */
let defaultInstance: PasswordService | null = null;

/**
 * Get the default PasswordService instance
 */
export function getPasswordService(): PasswordService {
	if (!defaultInstance) {
		defaultInstance = new PasswordService();
	}
	return defaultInstance;
}
