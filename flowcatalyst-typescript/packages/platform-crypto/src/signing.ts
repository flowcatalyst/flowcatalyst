/**
 * Webhook Signing Service (HMAC-SHA256)
 *
 * Compatible with the Java WebhookSigner implementation.
 *
 * Signature format:
 * - Algorithm: HMAC-SHA256
 * - Payload: `{timestamp}{body}` (concatenated, no separator)
 * - Output: Lowercase hex-encoded digest
 *
 * Headers:
 * - X-FLOWCATALYST-SIGNATURE: The HMAC-SHA256 signature
 * - X-FLOWCATALYST-TIMESTAMP: ISO 8601 timestamp (millisecond precision)
 */

import crypto from 'node:crypto';

export const SIGNATURE_HEADER = 'X-FLOWCATALYST-SIGNATURE';
export const TIMESTAMP_HEADER = 'X-FLOWCATALYST-TIMESTAMP';

// Default tolerance for timestamp validation (5 minutes)
const DEFAULT_TOLERANCE_MS = 5 * 60 * 1000;

/**
 * Signed webhook request containing all signature information
 */
export interface SignedWebhookRequest {
	/** The request payload (body) */
	payload: string;
	/** The HMAC-SHA256 signature (hex) */
	signature: string;
	/** The ISO 8601 timestamp */
	timestamp: string;
	/** Optional bearer token for authentication */
	bearerToken?: string | undefined;
}

/**
 * Generate an HMAC-SHA256 signature for a webhook payload.
 *
 * @param payload - The request body to sign
 * @param secret - The signing secret
 * @param timestamp - Unix timestamp in milliseconds
 * @returns Lowercase hex-encoded HMAC-SHA256 signature
 */
export function signWebhook(payload: string, secret: string, timestamp: number): string {
	const signaturePayload = `${timestamp}${payload}`;
	return crypto.createHmac('sha256', secret).update(signaturePayload).digest('hex');
}

/**
 * Generate a signed webhook request with all required headers.
 *
 * @param payload - The request body to sign
 * @param secret - The signing secret
 * @param bearerToken - Optional bearer token for authentication
 * @returns SignedWebhookRequest with signature and timestamp
 */
export function createSignedRequest(payload: string, secret: string, bearerToken?: string): SignedWebhookRequest {
	const timestamp = Date.now();
	const isoTimestamp = new Date(timestamp).toISOString();
	const signature = signWebhook(payload, secret, timestamp);

	return {
		payload,
		signature,
		timestamp: isoTimestamp,
		bearerToken,
	};
}

/**
 * Verify a webhook signature.
 *
 * @param payload - The request body that was signed
 * @param signature - The signature to verify
 * @param secret - The signing secret
 * @param timestamp - The timestamp used in signing (ISO 8601 or Unix ms)
 * @param toleranceMs - Maximum age of the request in milliseconds (default: 5 minutes)
 * @returns true if signature is valid and timestamp is within tolerance
 */
export function verifyWebhookSignature(
	payload: string,
	signature: string,
	secret: string,
	timestamp: string | number,
	toleranceMs: number = DEFAULT_TOLERANCE_MS,
): boolean {
	// Parse timestamp
	let timestampMs: number;
	if (typeof timestamp === 'number') {
		timestampMs = timestamp;
	} else {
		const parsed = Date.parse(timestamp);
		if (Number.isNaN(parsed)) {
			return false;
		}
		timestampMs = parsed;
	}

	// Check timestamp freshness
	const now = Date.now();
	if (Math.abs(now - timestampMs) > toleranceMs) {
		return false;
	}

	// Compute expected signature
	const expectedSignature = signWebhook(payload, secret, timestampMs);

	// Constant-time comparison to prevent timing attacks
	try {
		return crypto.timingSafeEqual(Buffer.from(signature, 'hex'), Buffer.from(expectedSignature, 'hex'));
	} catch {
		// If buffers have different lengths, timingSafeEqual throws
		return false;
	}
}

/**
 * Extract signature and timestamp from request headers.
 *
 * @param headers - Headers object or Map-like with get() method
 * @returns Object with signature and timestamp, or null if missing
 */
export function extractSignatureHeaders(
	headers: Headers | Map<string, string> | Record<string, string | undefined>,
): { signature: string; timestamp: string } | null {
	let signature: string | undefined;
	let timestamp: string | undefined;

	if (headers instanceof Headers) {
		signature = headers.get(SIGNATURE_HEADER) ?? undefined;
		timestamp = headers.get(TIMESTAMP_HEADER) ?? undefined;
	} else if (headers instanceof Map) {
		signature = headers.get(SIGNATURE_HEADER) ?? undefined;
		timestamp = headers.get(TIMESTAMP_HEADER) ?? undefined;
	} else {
		// Plain object - check both cases
		const headerObj = headers as Record<string, string | undefined>;
		signature = headerObj[SIGNATURE_HEADER] ?? headerObj[SIGNATURE_HEADER.toLowerCase()];
		timestamp = headerObj[TIMESTAMP_HEADER] ?? headerObj[TIMESTAMP_HEADER.toLowerCase()];
	}

	if (!signature || !timestamp) {
		return null;
	}

	return { signature, timestamp };
}

/**
 * Verify a webhook request using headers.
 *
 * @param payload - The request body
 * @param headers - The request headers
 * @param secret - The signing secret
 * @param toleranceMs - Maximum age of the request in milliseconds
 * @returns true if valid
 */
export function verifyWebhookRequest(
	payload: string,
	headers: Headers | Map<string, string> | Record<string, string | undefined>,
	secret: string,
	toleranceMs: number = DEFAULT_TOLERANCE_MS,
): boolean {
	const extracted = extractSignatureHeaders(headers);
	if (!extracted) {
		return false;
	}

	return verifyWebhookSignature(payload, extracted.signature, secret, extracted.timestamp, toleranceMs);
}

/**
 * Generate a cryptographically secure random signing secret.
 *
 * @param bytes - Number of bytes (default: 32 for 256 bits)
 * @returns Base64-encoded secret
 */
export function generateSigningSecret(bytes: number = 32): string {
	return crypto.randomBytes(bytes).toString('base64');
}
