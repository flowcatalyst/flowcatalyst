/**
 * PKCE (Proof Key for Code Exchange) Service
 *
 * Implements RFC 7636 PKCE for OAuth 2.0 public clients.
 *
 * @see https://datatracker.ietf.org/doc/html/rfc7636
 */

import crypto from 'node:crypto';

/**
 * PKCE challenge method.
 * - S256: SHA-256 hash of verifier (recommended)
 * - plain: Verifier sent as-is (for clients that can't do SHA-256)
 */
export type ChallengeMethod = 'S256' | 'plain';

/**
 * Regex for valid code verifier/challenge characters (unreserved URI characters).
 * RFC 7636 Section 4.1: ALPHA / DIGIT / "-" / "." / "_" / "~"
 */
const UNRESERVED_URI_REGEX = /^[A-Za-z0-9\-._~]+$/;

/**
 * Code verifier/challenge must be between 43 and 128 characters.
 * RFC 7636 Section 4.1
 */
const MIN_LENGTH = 43;
const MAX_LENGTH = 128;

/**
 * Number of random bytes for verifier generation.
 * 48 bytes -> 64 base64url characters (after padding removal)
 */
const VERIFIER_BYTES = 48;

/**
 * Convert a Buffer to base64url encoding (no padding).
 */
function toBase64Url(buffer: Buffer): string {
	return buffer.toString('base64url');
}

/**
 * Generate a cryptographically random code verifier.
 *
 * Creates a 48-byte random value encoded as base64url,
 * resulting in a 64-character string.
 *
 * @returns A 64-character base64url-encoded code verifier
 */
export function generateCodeVerifier(): string {
	const bytes = crypto.randomBytes(VERIFIER_BYTES);
	return toBase64Url(bytes);
}

/**
 * Generate a code challenge from a code verifier.
 *
 * Uses SHA-256 hash of the verifier, encoded as base64url.
 *
 * @param verifier - The code verifier string
 * @returns The code challenge (43 characters for SHA-256)
 */
export function generateCodeChallenge(verifier: string): string {
	const hash = crypto.createHash('sha256').update(verifier, 'ascii').digest();
	return toBase64Url(hash);
}

/**
 * Verify a code challenge against a code verifier.
 *
 * Uses constant-time comparison to prevent timing attacks.
 *
 * @param verifier - The code verifier from the token request
 * @param challenge - The code challenge from the authorization request
 * @param method - The challenge method (default: 'S256')
 * @returns true if the verifier matches the challenge
 */
export function verifyCodeChallenge(verifier: string, challenge: string, method: ChallengeMethod = 'S256'): boolean {
	if (!verifier || !challenge) {
		return false;
	}

	let computedChallenge: string;

	if (method === 'S256') {
		computedChallenge = generateCodeChallenge(verifier);
	} else if (method === 'plain') {
		computedChallenge = verifier;
	} else {
		// Unknown method
		return false;
	}

	// Constant-time comparison to prevent timing attacks
	return constantTimeEqual(computedChallenge, challenge);
}

/**
 * Constant-time string comparison using XOR.
 *
 * Prevents timing attacks by always comparing all bytes
 * regardless of where a mismatch occurs.
 */
function constantTimeEqual(a: string, b: string): boolean {
	if (a.length !== b.length) {
		// Still perform comparison to maintain constant time for same-length strings
		// but result will be false
		const longer = a.length > b.length ? a : b;
		const shorter = a.length > b.length ? b : a;

		let result = 0;
		for (let i = 0; i < longer.length; i++) {
			const charA = longer.charCodeAt(i);
			const charB = i < shorter.length ? shorter.charCodeAt(i) : 0;
			result |= charA ^ charB;
		}
		// Different lengths always means not equal
		return false;
	}

	let result = 0;
	for (let i = 0; i < a.length; i++) {
		result |= a.charCodeAt(i) ^ b.charCodeAt(i);
	}
	return result === 0;
}

/**
 * Check if a string is a valid code verifier.
 *
 * Valid verifiers must:
 * - Be between 43 and 128 characters long
 * - Contain only unreserved URI characters: [A-Za-z0-9\-._~]
 *
 * @param verifier - The string to validate
 * @returns true if the verifier is valid
 */
export function isValidCodeVerifier(verifier: string): boolean {
	if (!verifier) {
		return false;
	}

	if (verifier.length < MIN_LENGTH || verifier.length > MAX_LENGTH) {
		return false;
	}

	return UNRESERVED_URI_REGEX.test(verifier);
}

/**
 * Check if a string is a valid code challenge.
 *
 * Valid challenges must:
 * - Be between 43 and 128 characters long
 * - Contain only base64url characters (subset of unreserved URI chars)
 *
 * @param challenge - The string to validate
 * @returns true if the challenge is valid
 */
export function isValidCodeChallenge(challenge: string): boolean {
	if (!challenge) {
		return false;
	}

	if (challenge.length < MIN_LENGTH || challenge.length > MAX_LENGTH) {
		return false;
	}

	// Base64url uses A-Za-z0-9-_ which is a subset of unreserved URI chars
	return UNRESERVED_URI_REGEX.test(challenge);
}
