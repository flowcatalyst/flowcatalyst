import { describe, it, expect } from 'vitest';
import { EncryptionService, generateAppKey } from '../encryption.js';

describe('EncryptionService', () => {
	const appKey = generateAppKey();
	const service = new EncryptionService(appKey);

	describe('encrypt/decrypt round trip', () => {
		it('should encrypt and decrypt a simple string', () => {
			const plaintext = 'my-secret-value';
			const encrypted = service.encrypt(plaintext);

			expect(encrypted.isOk()).toBe(true);
			const ciphertext = encrypted._unsafeUnwrap();
			expect(ciphertext).toMatch(/^encrypted:/);

			const decrypted = service.decrypt(ciphertext);
			expect(decrypted.isOk()).toBe(true);
			expect(decrypted._unsafeUnwrap()).toBe(plaintext);
		});

		it('should handle unicode characters', () => {
			const plaintext = 'Hello 世界 🌍 émojis';
			const encrypted = service.encrypt(plaintext);
			const decrypted = service.decrypt(encrypted._unsafeUnwrap());
			expect(decrypted._unsafeUnwrap()).toBe(plaintext);
		});

		it('should handle empty string', () => {
			const encrypted = service.encrypt('');
			const decrypted = service.decrypt(encrypted._unsafeUnwrap());
			expect(decrypted._unsafeUnwrap()).toBe('');
		});

		it('should handle long strings', () => {
			const plaintext = 'x'.repeat(10000);
			const encrypted = service.encrypt(plaintext);
			const decrypted = service.decrypt(encrypted._unsafeUnwrap());
			expect(decrypted._unsafeUnwrap()).toBe(plaintext);
		});

		it('should produce different ciphertext for same plaintext (random IV)', () => {
			const plaintext = 'test-value';
			const encrypted1 = service.encrypt(plaintext)._unsafeUnwrap();
			const encrypted2 = service.encrypt(plaintext)._unsafeUnwrap();
			expect(encrypted1).not.toBe(encrypted2);
		});
	});

	describe('decryption failures', () => {
		it('should fail with wrong key', () => {
			const encrypted = service.encrypt('secret')._unsafeUnwrap();

			const otherKey = generateAppKey();
			const otherService = new EncryptionService(otherKey);

			const result = otherService.decrypt(encrypted);
			expect(result.isErr()).toBe(true);
			expect(result._unsafeUnwrapErr().type).toBe('decryption_failed');
		});

		it('should fail with tampered ciphertext', () => {
			const encrypted = service.encrypt('secret')._unsafeUnwrap();
			// Tamper with the ciphertext
			const tampered = encrypted.slice(0, -5) + 'xxxxx';

			const result = service.decrypt(tampered);
			expect(result.isErr()).toBe(true);
		});

		it('should fail with invalid format', () => {
			const result = service.decrypt('not-encrypted:base64');
			expect(result.isErr()).toBe(true);
			expect(result._unsafeUnwrapErr().type).toBe('invalid_format');
		});

		it('should fail with too short ciphertext', () => {
			const result = service.decrypt('encrypted:YWJj');
			expect(result.isErr()).toBe(true);
		});
	});

	describe('prepareForStorage', () => {
		it('should encrypt values with encrypt: prefix', () => {
			const result = service.prepareForStorage('encrypt:my-secret');
			expect(result.isOk()).toBe(true);
			expect(result._unsafeUnwrap()).toMatch(/^encrypted:/);
		});

		it('should pass through encrypted: values', () => {
			const encrypted = service.encrypt('secret')._unsafeUnwrap();
			const result = service.prepareForStorage(encrypted);
			expect(result._unsafeUnwrap()).toBe(encrypted);
		});

		it('should pass through plain values', () => {
			const result = service.prepareForStorage('plain-value');
			expect(result._unsafeUnwrap()).toBe('plain-value');
		});
	});

	describe('helper methods', () => {
		it('isAvailable should return true when configured', () => {
			expect(service.isAvailable()).toBe(true);
		});

		it('isAvailable should return false when not configured', () => {
			const unconfigured = new EncryptionService();
			expect(unconfigured.isAvailable()).toBe(false);
		});

		it('isEncrypted should identify encrypted references', () => {
			expect(service.isEncrypted('encrypted:abc123')).toBe(true);
			expect(service.isEncrypted('plain-value')).toBe(false);
		});

		it('needsEncryption should identify encrypt: prefix', () => {
			expect(service.needsEncryption('encrypt:secret')).toBe(true);
			expect(service.needsEncryption('encrypted:abc')).toBe(false);
			expect(service.needsEncryption('plain')).toBe(false);
		});
	});
});

describe('generateAppKey', () => {
	it('should generate a 32-byte Base64 key', () => {
		const key = generateAppKey();
		const decoded = Buffer.from(key, 'base64');
		expect(decoded.length).toBe(32);
	});

	it('should generate unique keys', () => {
		const keys = new Set<string>();
		for (let i = 0; i < 100; i++) {
			keys.add(generateAppKey());
		}
		expect(keys.size).toBe(100);
	});
});

describe('key validation', () => {
	it('should reject keys that are too short', () => {
		const shortKey = Buffer.alloc(16).toString('base64');
		expect(() => new EncryptionService(shortKey)).toThrow('Invalid APP_KEY length');
	});

	it('should reject keys that are too long', () => {
		const longKey = Buffer.alloc(64).toString('base64');
		expect(() => new EncryptionService(longKey)).toThrow('Invalid APP_KEY length');
	});
});
