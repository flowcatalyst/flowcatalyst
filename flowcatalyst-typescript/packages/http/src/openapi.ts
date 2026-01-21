/**
 * OpenAPI Integration
 *
 * Utilities for OpenAPI documentation and TypeBox schema integration
 * with Fastify applications.
 *
 * TypeBox is the native choice for Fastify because:
 * - It generates JSON Schema directly (Fastify uses AJV for validation)
 * - Enables Fastify's high-speed JIT compilation
 * - No runtime conversion overhead (unlike Zod)
 * - Full TypeScript inference with schemas
 */

import { Type, type Static, type TSchema, type TObject, type TProperties } from '@sinclair/typebox';
import { Value } from '@sinclair/typebox/value';

/**
 * Common TypeBox schemas for FlowCatalyst APIs.
 */
export const CommonSchemas = {
	/**
	 * TSID - 13-character Crockford Base32 string.
	 */
	Tsid: Type.String({
		minLength: 13,
		maxLength: 13,
		pattern: '^[0-9A-HJKMNP-TV-Z]{13}$',
		description: 'TSID in Crockford Base32 format',
	}),

	/**
	 * Typed ID - entity prefix + underscore + TSID (e.g., "user_0HZXEQ5Y8JY5Z").
	 */
	TypedId: Type.String({
		pattern: '^[a-z]+_[0-9A-HJKMNP-TV-Z]{13}$',
		description: 'Typed ID (prefix_TSID format)',
	}),

	/**
	 * ISO 8601 datetime string.
	 */
	DateTime: Type.String({
		format: 'date-time',
		description: 'ISO 8601 datetime',
	}),

	/**
	 * Email address.
	 */
	Email: Type.String({
		format: 'email',
		description: 'Email address',
	}),

	/**
	 * Non-empty string.
	 */
	NonEmptyString: Type.String({
		minLength: 1,
		description: 'Non-empty string',
	}),

	/**
	 * Pagination query parameters.
	 */
	PaginationQuery: Type.Object({
		page: Type.Optional(Type.Number({ minimum: 0, default: 0 })),
		pageSize: Type.Optional(Type.Number({ minimum: 1, maximum: 100, default: 20 })),
	}),
};

/**
 * Standard error response schema.
 */
export const ErrorResponseSchema = Type.Object({
	message: Type.String({ description: 'Human-readable error message' }),
	code: Type.String({ description: 'Machine-readable error code' }),
	details: Type.Optional(Type.Record(Type.String(), Type.Unknown(), { description: 'Additional error details' })),
});

export type ErrorResponseType = Static<typeof ErrorResponseSchema>;

/**
 * Paginated response wrapper schema.
 *
 * @param itemSchema - Schema for individual items
 * @returns Schema for paginated response
 */
export function paginatedResponse<T extends TSchema>(itemSchema: T) {
	return Type.Object({
		items: Type.Array(itemSchema),
		page: Type.Number({ minimum: 0 }),
		pageSize: Type.Number({ minimum: 1 }),
		totalItems: Type.Number({ minimum: 0 }),
		totalPages: Type.Number({ minimum: 0 }),
		hasNext: Type.Boolean(),
		hasPrevious: Type.Boolean(),
	});
}

/**
 * Create a schema with common entity fields.
 *
 * @param fields - Entity-specific fields
 * @returns Schema with id, createdAt, updatedAt
 */
export function entitySchema<T extends TProperties>(fields: T) {
	return Type.Object({
		id: Type.String({ description: 'Unique entity ID' }),
		createdAt: Type.String({ format: 'date-time', description: 'When the entity was created' }),
		updatedAt: Type.String({ format: 'date-time', description: 'When the entity was last updated' }),
		...fields,
	});
}

/**
 * OpenAPI response definitions for common status codes.
 */
export const OpenAPIResponses = {
	/** 200 OK */
	ok: <T extends TSchema>(schema: T, description: string = 'Successful response') => ({
		200: {
			description,
			content: { 'application/json': { schema } },
		},
	}),

	/** 201 Created */
	created: <T extends TSchema>(schema: T, description: string = 'Resource created') => ({
		201: {
			description,
			content: { 'application/json': { schema } },
		},
	}),

	/** 204 No Content */
	noContent: (description: string = 'No content') => ({
		204: { description },
	}),

	/** 400 Bad Request */
	badRequest: (description: string = 'Invalid request') => ({
		400: {
			description,
			content: { 'application/json': { schema: ErrorResponseSchema } },
		},
	}),

	/** 401 Unauthorized */
	unauthorized: (description: string = 'Authentication required') => ({
		401: {
			description,
			content: { 'application/json': { schema: ErrorResponseSchema } },
		},
	}),

	/** 403 Forbidden */
	forbidden: (description: string = 'Access denied') => ({
		403: {
			description,
			content: { 'application/json': { schema: ErrorResponseSchema } },
		},
	}),

	/** 404 Not Found */
	notFound: (description: string = 'Resource not found') => ({
		404: {
			description,
			content: { 'application/json': { schema: ErrorResponseSchema } },
		},
	}),

	/** 409 Conflict */
	conflict: (description: string = 'Conflict') => ({
		409: {
			description,
			content: { 'application/json': { schema: ErrorResponseSchema } },
		},
	}),

	/** 500 Internal Server Error */
	serverError: (description: string = 'Internal server error') => ({
		500: {
			description,
			content: { 'application/json': { schema: ErrorResponseSchema } },
		},
	}),
};

/**
 * Combine multiple response definitions.
 *
 * @param responses - Response definitions to combine
 * @returns Combined response definitions
 *
 * @example
 * ```typescript
 * const responses = combineResponses(
 *     OpenAPIResponses.ok(UserSchema, 'User details'),
 *     OpenAPIResponses.notFound('User not found'),
 *     OpenAPIResponses.unauthorized(),
 * );
 * ```
 */
export function combineResponses(
	...responses: Array<Record<number, unknown>>
): Record<number, unknown> {
	return Object.assign({}, ...responses);
}

/**
 * Validate data against a TypeBox schema.
 * Note: Fastify validates automatically when schemas are provided in route config.
 * This function is for manual validation outside of route handlers.
 *
 * @param data - Data to validate
 * @param schema - TypeBox schema to validate against
 * @returns Validated and typed data
 * @throws Error if validation fails
 */
export function validateBody<T extends TSchema>(data: unknown, schema: T): Static<T> {
	if (!Value.Check(schema, data)) {
		const errors = [...Value.Errors(schema, data)];
		const message = errors.map((e) => `${e.path}: ${e.message}`).join(', ');
		throw new Error(`Validation failed: ${message}`);
	}
	return data as Static<T>;
}

/**
 * Safe validation that returns a Result-like object instead of throwing.
 *
 * @param data - Data to validate
 * @param schema - TypeBox schema to validate against
 * @returns Object with success flag and either data or errors
 */
export function safeValidate<T extends TSchema>(
	data: unknown,
	schema: T,
): { success: true; data: Static<T> } | { success: false; error: string } {
	if (Value.Check(schema, data)) {
		return { success: true, data: data as Static<T> };
	}
	const errors = [...Value.Errors(schema, data)];
	const message = errors.map((e) => `${e.path}: ${e.message}`).join(', ');
	return { success: false, error: message };
}

// Re-export TypeBox for convenience
export { Type, Value, type Static, type TSchema, type TObject };
