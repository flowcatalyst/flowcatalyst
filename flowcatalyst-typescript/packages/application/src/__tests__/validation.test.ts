import { describe, it, expect } from 'vitest';
import { Result } from '@flowcatalyst/domain-core';
import {
	validateRequired,
	validateFormat,
	validateMaxLength,
	validateMinLength,
	validateRange,
	validateOneOf,
	validateEmail,
	validateAll,
} from '../validation.js';

describe('Validation', () => {
	describe('validateRequired', () => {
		it('should pass for non-empty string', () => {
			const result = validateRequired('hello', 'name', 'NAME_REQUIRED');
			expect(Result.isSuccess(result)).toBe(true);
			if (Result.isSuccess(result)) {
				expect(result.value).toBe('hello');
			}
		});

		it('should fail for null', () => {
			const result = validateRequired(null, 'name', 'NAME_REQUIRED');
			expect(Result.isFailure(result)).toBe(true);
			if (Result.isFailure(result)) {
				expect(result.error.code).toBe('NAME_REQUIRED');
				expect(result.error.details['field']).toBe('name');
			}
		});

		it('should fail for undefined', () => {
			const result = validateRequired(undefined, 'name', 'NAME_REQUIRED');
			expect(Result.isFailure(result)).toBe(true);
		});

		it('should fail for empty string', () => {
			const result = validateRequired('', 'name', 'NAME_REQUIRED');
			expect(Result.isFailure(result)).toBe(true);
		});

		it('should fail for whitespace-only string', () => {
			const result = validateRequired('   ', 'name', 'NAME_REQUIRED');
			expect(Result.isFailure(result)).toBe(true);
		});

		it('should pass for non-string values', () => {
			const numResult = validateRequired(0, 'count', 'COUNT_REQUIRED');
			expect(Result.isSuccess(numResult)).toBe(true);

			const objResult = validateRequired({ a: 1 }, 'data', 'DATA_REQUIRED');
			expect(Result.isSuccess(objResult)).toBe(true);
		});

		it('should use custom error message', () => {
			const result = validateRequired(null, 'name', 'NAME_REQUIRED', 'Please provide a name');
			if (Result.isFailure(result)) {
				expect(result.error.message).toBe('Please provide a name');
			}
		});
	});

	describe('validateFormat', () => {
		it('should pass for matching pattern', () => {
			const result = validateFormat('abc-123', /^[a-z]+-\d+$/, 'code', 'INVALID_CODE');
			expect(Result.isSuccess(result)).toBe(true);
		});

		it('should fail for non-matching pattern', () => {
			const result = validateFormat('ABC_123', /^[a-z]+-\d+$/, 'code', 'INVALID_CODE');
			expect(Result.isFailure(result)).toBe(true);
			if (Result.isFailure(result)) {
				expect(result.error.code).toBe('INVALID_CODE');
				expect(result.error.details['field']).toBe('code');
			}
		});
	});

	describe('validateMaxLength', () => {
		it('should pass for string within limit', () => {
			const result = validateMaxLength('hello', 10, 'name', 'NAME_TOO_LONG');
			expect(Result.isSuccess(result)).toBe(true);
		});

		it('should pass for string at limit', () => {
			const result = validateMaxLength('hello', 5, 'name', 'NAME_TOO_LONG');
			expect(Result.isSuccess(result)).toBe(true);
		});

		it('should fail for string over limit', () => {
			const result = validateMaxLength('hello world', 5, 'name', 'NAME_TOO_LONG');
			expect(Result.isFailure(result)).toBe(true);
			if (Result.isFailure(result)) {
				expect(result.error.details['length']).toBe(11);
				expect(result.error.details['maxLength']).toBe(5);
			}
		});
	});

	describe('validateMinLength', () => {
		it('should pass for string at or above minimum', () => {
			const result = validateMinLength('hello', 3, 'password', 'PASSWORD_TOO_SHORT');
			expect(Result.isSuccess(result)).toBe(true);
		});

		it('should fail for string below minimum', () => {
			const result = validateMinLength('ab', 3, 'password', 'PASSWORD_TOO_SHORT');
			expect(Result.isFailure(result)).toBe(true);
			if (Result.isFailure(result)) {
				expect(result.error.details['length']).toBe(2);
				expect(result.error.details['minLength']).toBe(3);
			}
		});
	});

	describe('validateRange', () => {
		it('should pass for value in range', () => {
			const result = validateRange(50, 1, 100, 'score', 'SCORE_OUT_OF_RANGE');
			expect(Result.isSuccess(result)).toBe(true);
		});

		it('should pass for value at boundaries', () => {
			expect(Result.isSuccess(validateRange(1, 1, 100, 'score', 'ERR'))).toBe(true);
			expect(Result.isSuccess(validateRange(100, 1, 100, 'score', 'ERR'))).toBe(true);
		});

		it('should fail for value below range', () => {
			const result = validateRange(0, 1, 100, 'score', 'SCORE_OUT_OF_RANGE');
			expect(Result.isFailure(result)).toBe(true);
		});

		it('should fail for value above range', () => {
			const result = validateRange(101, 1, 100, 'score', 'SCORE_OUT_OF_RANGE');
			expect(Result.isFailure(result)).toBe(true);
		});
	});

	describe('validateOneOf', () => {
		it('should pass for value in list', () => {
			const result = validateOneOf('active', ['active', 'inactive', 'pending'] as const, 'status', 'INVALID_STATUS');
			expect(Result.isSuccess(result)).toBe(true);
		});

		it('should fail for value not in list', () => {
			const result = validateOneOf('unknown', ['active', 'inactive'] as const, 'status', 'INVALID_STATUS');
			expect(Result.isFailure(result)).toBe(true);
			if (Result.isFailure(result)) {
				expect(result.error.details['value']).toBe('unknown');
				expect(result.error.details['allowedValues']).toEqual(['active', 'inactive']);
			}
		});
	});

	describe('validateEmail', () => {
		it('should pass for valid email', () => {
			const result = validateEmail('user@example.com');
			expect(Result.isSuccess(result)).toBe(true);
		});

		it('should fail for email without @', () => {
			const result = validateEmail('userexample.com');
			expect(Result.isFailure(result)).toBe(true);
		});

		it('should fail for email without domain', () => {
			const result = validateEmail('user@');
			expect(Result.isFailure(result)).toBe(true);
		});

		it('should use default field name and error code', () => {
			const result = validateEmail('invalid');
			if (Result.isFailure(result)) {
				expect(result.error.code).toBe('INVALID_EMAIL');
				expect(result.error.details['field']).toBe('email');
			}
		});
	});

	describe('validateAll', () => {
		it('should pass when all validations pass', () => {
			const result = validateAll(
				() => validateRequired('value', 'field', 'REQUIRED'),
				() => validateMaxLength('value', 10, 'field', 'TOO_LONG'),
			);
			expect(Result.isSuccess(result)).toBe(true);
		});

		it('should fail on first validation failure', () => {
			const result = validateAll(
				() => validateRequired('value', 'field', 'REQUIRED'),
				() => validateMaxLength('value', 3, 'field', 'TOO_LONG'),
				() => validateRequired(null, 'other', 'OTHER_REQUIRED'),
			);
			expect(Result.isFailure(result)).toBe(true);
			if (Result.isFailure(result)) {
				// Should fail on maxLength (second validation), not null check (third)
				expect(result.error.code).toBe('TOO_LONG');
			}
		});

		it('should return void on success', () => {
			const result = validateAll(() => validateRequired('x', 'f', 'R'));
			if (Result.isSuccess(result)) {
				expect(result.value).toBeUndefined();
			}
		});
	});
});
