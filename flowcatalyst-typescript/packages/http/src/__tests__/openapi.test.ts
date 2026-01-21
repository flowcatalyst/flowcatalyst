import { describe, it, expect } from 'vitest';
import { z } from 'zod';
import {
	CommonSchemas,
	ErrorResponseSchema,
	paginatedResponse,
	entitySchema,
	OpenAPIResponses,
	combineResponses,
	validateBody,
	validateQuery,
	safeValidate,
} from '../openapi.js';

describe('OpenAPI Utilities', () => {
	describe('CommonSchemas', () => {
		describe('tsid', () => {
			it('should accept valid TSID', () => {
				expect(CommonSchemas.tsid.parse('0HZXEQ5Y8JY5Z')).toBe('0HZXEQ5Y8JY5Z');
			});

			it('should reject invalid TSID', () => {
				expect(() => CommonSchemas.tsid.parse('invalid')).toThrow();
				expect(() => CommonSchemas.tsid.parse('0HZXEQ5Y8JY5Z-extra')).toThrow();
			});
		});

		describe('typedId', () => {
			it('should accept valid typed ID', () => {
				expect(CommonSchemas.typedId.parse('user_0HZXEQ5Y8JY5Z')).toBe('user_0HZXEQ5Y8JY5Z');
			});

			it('should reject invalid typed ID', () => {
				expect(() => CommonSchemas.typedId.parse('0HZXEQ5Y8JY5Z')).toThrow();
				expect(() => CommonSchemas.typedId.parse('user-0HZXEQ5Y8JY5Z')).toThrow();
			});
		});

		describe('pagination', () => {
			it('should parse pagination with defaults', () => {
				const result = CommonSchemas.pagination.parse({});
				expect(result.page).toBe(0);
				expect(result.pageSize).toBe(20);
			});

			it('should coerce string values', () => {
				const result = CommonSchemas.pagination.parse({ page: '1', pageSize: '50' });
				expect(result.page).toBe(1);
				expect(result.pageSize).toBe(50);
			});

			it('should enforce max pageSize', () => {
				expect(() => CommonSchemas.pagination.parse({ pageSize: 200 })).toThrow();
			});
		});
	});

	describe('ErrorResponseSchema', () => {
		it('should accept valid error response', () => {
			const result = ErrorResponseSchema.parse({
				message: 'Error occurred',
				code: 'ERR_CODE',
			});
			expect(result.message).toBe('Error occurred');
			expect(result.code).toBe('ERR_CODE');
		});

		it('should accept error response with details', () => {
			const result = ErrorResponseSchema.parse({
				message: 'Validation failed',
				code: 'VALIDATION',
				details: { field: 'email' },
			});
			expect(result.details).toEqual({ field: 'email' });
		});
	});

	describe('paginatedResponse', () => {
		it('should create paginated schema', () => {
			const itemSchema = z.object({ id: z.string(), name: z.string() });
			const schema = paginatedResponse(itemSchema);

			const result = schema.parse({
				items: [{ id: '1', name: 'Test' }],
				page: 0,
				pageSize: 20,
				totalItems: 1,
				totalPages: 1,
				hasNext: false,
				hasPrevious: false,
			});

			expect(result.items).toHaveLength(1);
			expect(result.totalItems).toBe(1);
		});
	});

	describe('entitySchema', () => {
		it('should create entity schema with base fields', () => {
			const schema = entitySchema({
				name: z.string(),
				active: z.boolean(),
			});

			const result = schema.parse({
				id: '0HZXEQ5Y8JY5Z',
				name: 'Test Entity',
				active: true,
				createdAt: '2024-01-01T00:00:00Z',
				updatedAt: '2024-01-01T00:00:00Z',
			});

			expect(result.id).toBe('0HZXEQ5Y8JY5Z');
			expect(result.name).toBe('Test Entity');
		});
	});

	describe('combineResponses', () => {
		it('should merge response definitions', () => {
			const responses = combineResponses(
				OpenAPIResponses.ok(z.object({ id: z.string() })),
				OpenAPIResponses.notFound(),
				OpenAPIResponses.unauthorized(),
			);

			expect(responses[200]).toBeDefined();
			expect(responses[404]).toBeDefined();
			expect(responses[401]).toBeDefined();
		});
	});

	describe('validateBody', () => {
		it('should validate and return typed body', () => {
			const schema = z.object({ email: z.string().email() });
			const body = validateBody({ email: 'test@example.com' }, schema);
			expect(body.email).toBe('test@example.com');
		});

		it('should throw on invalid body', () => {
			const schema = z.object({ email: z.string().email() });
			expect(() => validateBody({ email: 'invalid' }, schema)).toThrow();
		});
	});

	describe('validateQuery', () => {
		it('should validate query parameters', () => {
			const schema = z.object({ page: z.coerce.number() });
			const query = validateQuery({ page: '5' }, schema);
			expect(query.page).toBe(5);
		});
	});

	describe('safeValidate', () => {
		it('should return success for valid data', () => {
			const schema = z.object({ id: z.string() });
			const result = safeValidate({ id: '123' }, schema);

			expect(result.success).toBe(true);
			if (result.success) {
				expect(result.data.id).toBe('123');
			}
		});

		it('should return error for invalid data', () => {
			const schema = z.object({ id: z.string() });
			const result = safeValidate({ id: 123 }, schema);

			expect(result.success).toBe(false);
			if (!result.success) {
				expect(result.error).toBeDefined();
			}
		});
	});
});
