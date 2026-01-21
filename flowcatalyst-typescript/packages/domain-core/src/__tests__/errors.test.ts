import { describe, it, expect } from 'vitest';
import { UseCaseError } from '../errors.js';

describe('UseCaseError', () => {
	describe('validation', () => {
		it('should create a validation error', () => {
			const error = UseCaseError.validation('INVALID_EMAIL', 'Email format is invalid', { email: 'bad@' });

			expect(error.type).toBe('validation');
			expect(error.code).toBe('INVALID_EMAIL');
			expect(error.message).toBe('Email format is invalid');
			expect(error.details).toEqual({ email: 'bad@' });
		});

		it('should default to empty details', () => {
			const error = UseCaseError.validation('REQUIRED', 'Field is required');
			expect(error.details).toEqual({});
		});
	});

	describe('notFound', () => {
		it('should create a not found error', () => {
			const error = UseCaseError.notFound('EVENT_TYPE_NOT_FOUND', 'Event type not found', { id: '123' });

			expect(error.type).toBe('not_found');
			expect(error.code).toBe('EVENT_TYPE_NOT_FOUND');
			expect(error.message).toBe('Event type not found');
			expect(error.details).toEqual({ id: '123' });
		});
	});

	describe('businessRule', () => {
		it('should create a business rule violation', () => {
			const error = UseCaseError.businessRule('ALREADY_ARCHIVED', 'Event type is already archived', {
				status: 'ARCHIVED',
			});

			expect(error.type).toBe('business_rule');
			expect(error.code).toBe('ALREADY_ARCHIVED');
			expect(error.message).toBe('Event type is already archived');
			expect(error.details).toEqual({ status: 'ARCHIVED' });
		});
	});

	describe('concurrency', () => {
		it('should create a concurrency error', () => {
			const error = UseCaseError.concurrency('VERSION_CONFLICT', 'Entity was modified by another user', {
				expectedVersion: 1,
				actualVersion: 2,
			});

			expect(error.type).toBe('concurrency');
			expect(error.code).toBe('VERSION_CONFLICT');
			expect(error.message).toBe('Entity was modified by another user');
			expect(error.details).toEqual({ expectedVersion: 1, actualVersion: 2 });
		});
	});

	describe('httpStatus', () => {
		it('should return 400 for validation errors', () => {
			const error = UseCaseError.validation('INVALID', 'Invalid');
			expect(UseCaseError.httpStatus(error)).toBe(400);
		});

		it('should return 404 for not found errors', () => {
			const error = UseCaseError.notFound('NOT_FOUND', 'Not found');
			expect(UseCaseError.httpStatus(error)).toBe(404);
		});

		it('should return 409 for business rule violations', () => {
			const error = UseCaseError.businessRule('CONFLICT', 'Conflict');
			expect(UseCaseError.httpStatus(error)).toBe(409);
		});

		it('should return 409 for concurrency errors', () => {
			const error = UseCaseError.concurrency('CONFLICT', 'Conflict');
			expect(UseCaseError.httpStatus(error)).toBe(409);
		});
	});

	describe('isUseCaseError', () => {
		it('should return true for valid use case errors', () => {
			expect(UseCaseError.isUseCaseError(UseCaseError.validation('CODE', 'msg'))).toBe(true);
			expect(UseCaseError.isUseCaseError(UseCaseError.notFound('CODE', 'msg'))).toBe(true);
			expect(UseCaseError.isUseCaseError(UseCaseError.businessRule('CODE', 'msg'))).toBe(true);
			expect(UseCaseError.isUseCaseError(UseCaseError.concurrency('CODE', 'msg'))).toBe(true);
		});

		it('should return false for non-objects', () => {
			expect(UseCaseError.isUseCaseError(null)).toBe(false);
			expect(UseCaseError.isUseCaseError(undefined)).toBe(false);
			expect(UseCaseError.isUseCaseError('string')).toBe(false);
			expect(UseCaseError.isUseCaseError(123)).toBe(false);
		});

		it('should return false for objects with missing fields', () => {
			expect(UseCaseError.isUseCaseError({ type: 'validation' })).toBe(false);
			expect(UseCaseError.isUseCaseError({ type: 'validation', code: 'X' })).toBe(false);
			expect(UseCaseError.isUseCaseError({ code: 'X', message: 'Y' })).toBe(false);
		});
	});
});
