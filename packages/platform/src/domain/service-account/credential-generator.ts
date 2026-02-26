/**
 * Credential Generator
 *
 * Generates secure random credentials for service accounts.
 */

import { randomBytes } from "node:crypto";

const AUTH_TOKEN_PREFIX = "fc_";
const AUTH_TOKEN_LENGTH = 24;
const SIGNING_SECRET_BYTES = 32;
const CLIENT_SECRET_LENGTH = 48;

const ALPHANUMERIC =
	"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";

/**
 * Generate a random alphanumeric string of the given length.
 */
function randomAlphanumeric(length: number): string {
	const bytes = randomBytes(length);
	let result = "";
	for (let i = 0; i < length; i++) {
		result += ALPHANUMERIC[bytes[i]! % ALPHANUMERIC.length];
	}
	return result;
}

/**
 * Generate a webhook auth token.
 * Format: "fc_" + 24 random alphanumeric characters.
 */
export function generateAuthToken(): string {
	return AUTH_TOKEN_PREFIX + randomAlphanumeric(AUTH_TOKEN_LENGTH);
}

/**
 * Generate a webhook signing secret.
 * Format: 32 random bytes, hex-encoded (64 characters).
 */
export function generateSigningSecret(): string {
	return randomBytes(SIGNING_SECRET_BYTES).toString("hex");
}

/**
 * Generate an OAuth client secret.
 * Format: 48 random alphanumeric characters.
 */
export function generateClientSecret(): string {
	return randomAlphanumeric(CLIENT_SECRET_LENGTH);
}
