/**
 * Use Case Error Types
 *
 * Sealed error hierarchy for use case failures. Errors are categorized by type
 * to enable consistent HTTP status mapping and client-side handling.
 *
 * HTTP Status Mapping:
 * - ValidationError → 400 Bad Request
 * - NotFoundError → 404 Not Found
 * - BusinessRuleViolation → 409 Conflict
 * - ConcurrencyError → 409 Conflict
 */

/**
 * Base interface for all use case errors.
 */
export interface UseCaseErrorBase {
	readonly type: string;
	readonly code: string;
	readonly message: string;
	readonly details: Record<string, unknown>;
}

/**
 * Input validation failed (missing required fields, invalid format, etc.)
 * Maps to HTTP 400 Bad Request.
 */
export interface ValidationError extends UseCaseErrorBase {
	readonly type: 'validation';
}

/**
 * Entity not found.
 * Maps to HTTP 404 Not Found.
 */
export interface NotFoundError extends UseCaseErrorBase {
	readonly type: 'not_found';
}

/**
 * Business rule violation (entity in wrong state, constraint violated, etc.)
 * Maps to HTTP 409 Conflict.
 */
export interface BusinessRuleViolation extends UseCaseErrorBase {
	readonly type: 'business_rule';
}

/**
 * Optimistic locking conflict - entity was modified by another transaction.
 * Maps to HTTP 409 Conflict.
 */
export interface ConcurrencyError extends UseCaseErrorBase {
	readonly type: 'concurrency';
}

/**
 * Union type for all use case errors.
 */
export type UseCaseError = ValidationError | NotFoundError | BusinessRuleViolation | ConcurrencyError;

/**
 * Factory functions for creating errors.
 */
export const UseCaseError = {
	/**
	 * Create a validation error.
	 *
	 * @example
	 * ```typescript
	 * UseCaseError.validation('INVALID_EMAIL', 'Email format is invalid', { email: 'bad@' })
	 * ```
	 */
	validation(code: string, message: string, details: Record<string, unknown> = {}): ValidationError {
		return { type: 'validation', code, message, details };
	},

	/**
	 * Create a not found error.
	 *
	 * @example
	 * ```typescript
	 * UseCaseError.notFound('EVENT_TYPE_NOT_FOUND', 'Event type not found', { id: '123' })
	 * ```
	 */
	notFound(code: string, message: string, details: Record<string, unknown> = {}): NotFoundError {
		return { type: 'not_found', code, message, details };
	},

	/**
	 * Create a business rule violation error.
	 *
	 * @example
	 * ```typescript
	 * UseCaseError.businessRule('ALREADY_ARCHIVED', 'Event type is already archived', { status: 'ARCHIVED' })
	 * ```
	 */
	businessRule(code: string, message: string, details: Record<string, unknown> = {}): BusinessRuleViolation {
		return { type: 'business_rule', code, message, details };
	},

	/**
	 * Create a concurrency error.
	 *
	 * @example
	 * ```typescript
	 * UseCaseError.concurrency('VERSION_CONFLICT', 'Entity was modified by another user', { expectedVersion: 1, actualVersion: 2 })
	 * ```
	 */
	concurrency(code: string, message: string, details: Record<string, unknown> = {}): ConcurrencyError {
		return { type: 'concurrency', code, message, details };
	},

	/**
	 * Get the HTTP status code for an error.
	 */
	httpStatus(error: UseCaseError): number {
		switch (error.type) {
			case 'validation':
				return 400;
			case 'not_found':
				return 404;
			case 'business_rule':
			case 'concurrency':
				return 409;
		}
	},

	/**
	 * Check if an unknown value is a UseCaseError.
	 */
	isUseCaseError(value: unknown): value is UseCaseError {
		if (typeof value !== 'object' || value === null) return false;
		const obj = value as Record<string, unknown>;
		return (
			typeof obj['type'] === 'string' &&
			typeof obj['code'] === 'string' &&
			typeof obj['message'] === 'string' &&
			typeof obj['details'] === 'object'
		);
	},
};
