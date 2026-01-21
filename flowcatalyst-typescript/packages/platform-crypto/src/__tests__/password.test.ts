import { describe, it, expect } from 'vitest';
import { PasswordService } from '../password.js';

describe('PasswordService', () => {
	const service = new PasswordService();

	describe('hash and verify', () => {
		it('should hash and verify a password', async () => {
			const password = 'SecurePass123!';
			const hashResult = await service.hash(password);

			expect(hashResult.isOk()).toBe(true);
			const hash = hashResult._unsafeUnwrap();

			// Hash should be in PHC format
			expect(hash).toMatch(/^\$argon2id\$/);

			// Verification should succeed
			const isValid = await service.verify(password, hash);
			expect(isValid).toBe(true);
		});

		it('should reject wrong password', async () => {
			const hash = (await service.hash('CorrectPassword123!'))._unsafeUnwrap();
			const isValid = await service.verify('WrongPassword123!', hash);
			expect(isValid).toBe(false);
		});

		it('should produce different hashes for same password (random salt)', async () => {
			const password = 'SamePassword123!';
			const hash1 = (await service.hash(password))._unsafeUnwrap();
			const hash2 = (await service.hash(password))._unsafeUnwrap();
			expect(hash1).not.toBe(hash2);
		});

		it('should handle unicode passwords', async () => {
			const password = 'Пароль123!世界';
			const hash = (await service.hash(password))._unsafeUnwrap();
			const isValid = await service.verify(password, hash);
			expect(isValid).toBe(true);
		});
	});

	describe('validateComplexity', () => {
		it('should accept valid password', () => {
			const result = service.validateComplexity('ValidPassword123!');
			expect(result.isOk()).toBe(true);
		});

		it('should reject password that is too short', () => {
			const result = service.validateComplexity('Short1!');
			expect(result.isErr()).toBe(true);
			expect(result._unsafeUnwrapErr().message).toContain('at least 12');
		});

		it('should reject password without uppercase', () => {
			const result = service.validateComplexity('nouppercase123!');
			expect(result.isErr()).toBe(true);
			expect(result._unsafeUnwrapErr().message).toContain('uppercase');
		});

		it('should reject password without lowercase', () => {
			const result = service.validateComplexity('NOLOWERCASE123!');
			expect(result.isErr()).toBe(true);
			expect(result._unsafeUnwrapErr().message).toContain('lowercase');
		});

		it('should reject password without digit', () => {
			const result = service.validateComplexity('NoDigitsHere!!!');
			expect(result.isErr()).toBe(true);
			expect(result._unsafeUnwrapErr().message).toContain('digit');
		});

		it('should reject password without special character', () => {
			const result = service.validateComplexity('NoSpecialChar123');
			expect(result.isErr()).toBe(true);
			expect(result._unsafeUnwrapErr().message).toContain('special');
		});

		it('should accept various special characters', () => {
			const specials = '!@#$%^&*()_+-=[]{}|;\':",./<>?`~';
			for (const special of specials) {
				const password = `ValidPassword1${special}`;
				const result = service.validateComplexity(password);
				expect(result.isOk()).toBe(true);
			}
		});
	});

	describe('validateAndHash', () => {
		it('should validate and hash valid password', async () => {
			const result = await service.validateAndHash('ValidPassword123!');
			expect(result.isOk()).toBe(true);
			expect(result._unsafeUnwrap()).toMatch(/^\$argon2id\$/);
		});

		it('should reject invalid password without hashing', async () => {
			const result = await service.validateAndHash('weak');
			expect(result.isErr()).toBe(true);
			expect(result._unsafeUnwrapErr().type).toBe('validation');
		});
	});

	describe('needsRehash', () => {
		it('should return false for fresh hash', async () => {
			const hash = (await service.hash('Password123!'))._unsafeUnwrap();
			const needsRehash = await service.needsRehash(hash);
			expect(needsRehash).toBe(false);
		});

		it('should return true for invalid hash', async () => {
			const needsRehash = await service.needsRehash('invalid-hash');
			expect(needsRehash).toBe(true);
		});
	});
});
