import { describe, it, expect } from 'vitest';
import { Result, isSuccess, isFailure, RESULT_SUCCESS_TOKEN } from '../result.js';
import { UseCaseError } from '../errors.js';

describe('Result', () => {
	describe('success (restricted)', () => {
		it('should create a success with valid token', () => {
			const result = Result.success(RESULT_SUCCESS_TOKEN, 'value');

			expect(isSuccess(result)).toBe(true);
			expect(isFailure(result)).toBe(false);
			expect(result._tag).toBe('success');
			if (isSuccess(result)) {
				expect(result.value).toBe('value');
			}
		});

		it('should throw when called with invalid token', () => {
			expect(() => {
				Result.success('invalid' as never, 'value');
			}).toThrow('Result.success() is restricted');
		});

		it('should throw when called without token', () => {
			expect(() => {
				// @ts-expect-error Testing runtime behavior
				Result.success(undefined, 'value');
			}).toThrow('Result.success() is restricted');
		});
	});

	describe('failure', () => {
		it('should create a failure', () => {
			const error = UseCaseError.validation('INVALID', 'Invalid input');
			const result = Result.failure<string>(error);

			expect(isSuccess(result)).toBe(false);
			expect(isFailure(result)).toBe(true);
			expect(result._tag).toBe('failure');
			if (isFailure(result)) {
				expect(result.error).toBe(error);
			}
		});
	});

	describe('isSuccess / isFailure', () => {
		it('should correctly identify success', () => {
			const result = Result.success(RESULT_SUCCESS_TOKEN, 'value');
			expect(Result.isSuccess(result)).toBe(true);
			expect(Result.isFailure(result)).toBe(false);
		});

		it('should correctly identify failure', () => {
			const result = Result.failure<string>(UseCaseError.validation('X', 'Y'));
			expect(Result.isSuccess(result)).toBe(false);
			expect(Result.isFailure(result)).toBe(true);
		});
	});

	describe('map', () => {
		it('should map success values', () => {
			const result = Result.success(RESULT_SUCCESS_TOKEN, 5);
			const mapped = Result.map(result, (n) => n * 2);

			expect(isSuccess(mapped)).toBe(true);
			if (isSuccess(mapped)) {
				expect(mapped.value).toBe(10);
			}
		});

		it('should pass through failures', () => {
			const error = UseCaseError.validation('X', 'Y');
			const result = Result.failure<number>(error);
			const mapped = Result.map(result, (n) => n * 2);

			expect(isFailure(mapped)).toBe(true);
			if (isFailure(mapped)) {
				expect(mapped.error).toBe(error);
			}
		});
	});

	describe('match', () => {
		it('should call onSuccess for success', () => {
			const result = Result.success(RESULT_SUCCESS_TOKEN, 'hello');
			const matched = Result.match(
				result,
				(v) => `got: ${v}`,
				(e) => `error: ${e.code}`,
			);
			expect(matched).toBe('got: hello');
		});

		it('should call onFailure for failure', () => {
			const result = Result.failure<string>(UseCaseError.validation('CODE', 'msg'));
			const matched = Result.match(
				result,
				(v) => `got: ${v}`,
				(e) => `error: ${e.code}`,
			);
			expect(matched).toBe('error: CODE');
		});
	});

	describe('unwrap', () => {
		it('should return value for success', () => {
			const result = Result.success(RESULT_SUCCESS_TOKEN, 'value');
			expect(Result.unwrap(result)).toBe('value');
		});

		it('should throw for failure', () => {
			const result = Result.failure<string>(UseCaseError.validation('CODE', 'message'));
			expect(() => Result.unwrap(result)).toThrow('Cannot unwrap failure result: CODE - message');
		});
	});

	describe('unwrapOr', () => {
		it('should return value for success', () => {
			const result = Result.success(RESULT_SUCCESS_TOKEN, 'value');
			expect(Result.unwrapOr(result, 'default')).toBe('value');
		});

		it('should return default for failure', () => {
			const result = Result.failure<string>(UseCaseError.validation('X', 'Y'));
			expect(Result.unwrapOr(result, 'default')).toBe('default');
		});
	});
});
