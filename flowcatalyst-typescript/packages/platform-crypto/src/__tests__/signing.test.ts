import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
	signWebhook,
	createSignedRequest,
	verifyWebhookSignature,
	verifyWebhookRequest,
	extractSignatureHeaders,
	generateSigningSecret,
	SIGNATURE_HEADER,
	TIMESTAMP_HEADER,
} from '../signing.js';

describe('signWebhook', () => {
	it('should generate consistent signatures for same input', () => {
		const payload = '{"event": "test"}';
		const secret = 'test-secret';
		const timestamp = 1704067200000; // 2024-01-01T00:00:00Z

		const sig1 = signWebhook(payload, secret, timestamp);
		const sig2 = signWebhook(payload, secret, timestamp);

		expect(sig1).toBe(sig2);
	});

	it('should generate different signatures for different payloads', () => {
		const secret = 'test-secret';
		const timestamp = 1704067200000;

		const sig1 = signWebhook('{"a": 1}', secret, timestamp);
		const sig2 = signWebhook('{"a": 2}', secret, timestamp);

		expect(sig1).not.toBe(sig2);
	});

	it('should generate different signatures for different secrets', () => {
		const payload = '{"event": "test"}';
		const timestamp = 1704067200000;

		const sig1 = signWebhook(payload, 'secret-1', timestamp);
		const sig2 = signWebhook(payload, 'secret-2', timestamp);

		expect(sig1).not.toBe(sig2);
	});

	it('should generate different signatures for different timestamps', () => {
		const payload = '{"event": "test"}';
		const secret = 'test-secret';

		const sig1 = signWebhook(payload, secret, 1704067200000);
		const sig2 = signWebhook(payload, secret, 1704067200001);

		expect(sig1).not.toBe(sig2);
	});

	it('should return lowercase hex', () => {
		const sig = signWebhook('test', 'secret', Date.now());
		expect(sig).toMatch(/^[0-9a-f]{64}$/);
	});
});

describe('createSignedRequest', () => {
	it('should create a signed request with all fields', () => {
		const payload = '{"test": true}';
		const secret = 'my-secret';

		const request = createSignedRequest(payload, secret);

		expect(request.payload).toBe(payload);
		expect(request.signature).toMatch(/^[0-9a-f]{64}$/);
		expect(request.timestamp).toMatch(/^\d{4}-\d{2}-\d{2}T/);
		expect(request.bearerToken).toBeUndefined();
	});

	it('should include bearer token if provided', () => {
		const request = createSignedRequest('{}', 'secret', 'my-token');
		expect(request.bearerToken).toBe('my-token');
	});
});

describe('verifyWebhookSignature', () => {
	const payload = '{"event": "test"}';
	const secret = 'test-secret';

	beforeEach(() => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2024-01-01T00:05:00Z'));
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('should verify valid signature with numeric timestamp', () => {
		const timestamp = Date.now();
		const signature = signWebhook(payload, secret, timestamp);

		const isValid = verifyWebhookSignature(payload, signature, secret, timestamp);
		expect(isValid).toBe(true);
	});

	it('should verify valid signature with ISO timestamp', () => {
		const timestamp = Date.now();
		const isoTimestamp = new Date(timestamp).toISOString();
		const signature = signWebhook(payload, secret, timestamp);

		const isValid = verifyWebhookSignature(payload, signature, secret, isoTimestamp);
		expect(isValid).toBe(true);
	});

	it('should reject signature with wrong secret', () => {
		const timestamp = Date.now();
		const signature = signWebhook(payload, secret, timestamp);

		const isValid = verifyWebhookSignature(payload, signature, 'wrong-secret', timestamp);
		expect(isValid).toBe(false);
	});

	it('should reject expired timestamp', () => {
		// Create signature from 10 minutes ago (beyond 5 minute tolerance)
		const oldTimestamp = Date.now() - 10 * 60 * 1000;
		const signature = signWebhook(payload, secret, oldTimestamp);

		const isValid = verifyWebhookSignature(payload, signature, secret, oldTimestamp);
		expect(isValid).toBe(false);
	});

	it('should accept timestamp within tolerance', () => {
		// Create signature from 4 minutes ago (within 5 minute tolerance)
		const recentTimestamp = Date.now() - 4 * 60 * 1000;
		const signature = signWebhook(payload, secret, recentTimestamp);

		const isValid = verifyWebhookSignature(payload, signature, secret, recentTimestamp);
		expect(isValid).toBe(true);
	});

	it('should reject invalid timestamp format', () => {
		const signature = signWebhook(payload, secret, Date.now());
		const isValid = verifyWebhookSignature(payload, signature, secret, 'not-a-timestamp');
		expect(isValid).toBe(false);
	});

	it('should reject tampered signature', () => {
		const timestamp = Date.now();
		const signature = signWebhook(payload, secret, timestamp);
		const tampered = signature.replace(/[0-9a-f]/, 'x');

		const isValid = verifyWebhookSignature(payload, tampered, secret, timestamp);
		expect(isValid).toBe(false);
	});
});

describe('extractSignatureHeaders', () => {
	it('should extract from Headers object', () => {
		const headers = new Headers();
		headers.set(SIGNATURE_HEADER, 'abc123');
		headers.set(TIMESTAMP_HEADER, '2024-01-01T00:00:00Z');

		const result = extractSignatureHeaders(headers);
		expect(result).toEqual({
			signature: 'abc123',
			timestamp: '2024-01-01T00:00:00Z',
		});
	});

	it('should extract from Map', () => {
		const headers = new Map<string, string>();
		headers.set(SIGNATURE_HEADER, 'abc123');
		headers.set(TIMESTAMP_HEADER, '2024-01-01T00:00:00Z');

		const result = extractSignatureHeaders(headers);
		expect(result).toEqual({
			signature: 'abc123',
			timestamp: '2024-01-01T00:00:00Z',
		});
	});

	it('should extract from plain object', () => {
		const headers = {
			[SIGNATURE_HEADER]: 'abc123',
			[TIMESTAMP_HEADER]: '2024-01-01T00:00:00Z',
		};

		const result = extractSignatureHeaders(headers);
		expect(result).toEqual({
			signature: 'abc123',
			timestamp: '2024-01-01T00:00:00Z',
		});
	});

	it('should return null if signature missing', () => {
		const headers = { [TIMESTAMP_HEADER]: '2024-01-01T00:00:00Z' };
		const result = extractSignatureHeaders(headers);
		expect(result).toBeNull();
	});

	it('should return null if timestamp missing', () => {
		const headers = { [SIGNATURE_HEADER]: 'abc123' };
		const result = extractSignatureHeaders(headers);
		expect(result).toBeNull();
	});
});

describe('verifyWebhookRequest', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2024-01-01T00:00:00Z'));
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('should verify valid request', () => {
		const payload = '{"test": true}';
		const secret = 'test-secret';
		const request = createSignedRequest(payload, secret);

		const headers = {
			[SIGNATURE_HEADER]: request.signature,
			[TIMESTAMP_HEADER]: request.timestamp,
		};

		const isValid = verifyWebhookRequest(payload, headers, secret);
		expect(isValid).toBe(true);
	});

	it('should reject request with missing headers', () => {
		const isValid = verifyWebhookRequest('{}', {}, 'secret');
		expect(isValid).toBe(false);
	});
});

describe('generateSigningSecret', () => {
	it('should generate 32-byte secret by default', () => {
		const secret = generateSigningSecret();
		const decoded = Buffer.from(secret, 'base64');
		expect(decoded.length).toBe(32);
	});

	it('should generate custom length secret', () => {
		const secret = generateSigningSecret(64);
		const decoded = Buffer.from(secret, 'base64');
		expect(decoded.length).toBe(64);
	});

	it('should generate unique secrets', () => {
		const secrets = new Set<string>();
		for (let i = 0; i < 100; i++) {
			secrets.add(generateSigningSecret());
		}
		expect(secrets.size).toBe(100);
	});
});
