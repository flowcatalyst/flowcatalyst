/**
 * Response Utilities
 *
 * Utilities for mapping Result types to HTTP responses and
 * handling errors consistently with Fastify.
 */

import type { FastifyReply } from 'fastify';
import { Result, type UseCaseError } from '@flowcatalyst/domain-core';
import type { ErrorResponse } from './types.js';

/**
 * HTTP status codes for use case errors.
 */
const ERROR_STATUS_MAP: Record<string, number> = {
	validation: 400,
	not_found: 404,
	business_rule: 409,
	concurrency: 409,
};

/**
 * Get HTTP status code for a use case error.
 *
 * @param error - The use case error
 * @returns HTTP status code
 */
export function getErrorStatus(error: UseCaseError): number {
	return ERROR_STATUS_MAP[error.type] ?? 500;
}

/**
 * Convert a use case error to an error response.
 *
 * @param error - The use case error
 * @returns Error response object
 */
export function toErrorResponse(error: UseCaseError): ErrorResponse {
	const hasDetails = Object.keys(error.details).length > 0;
	return {
		message: error.message,
		code: error.code,
		...(hasDetails ? { details: error.details } : {}),
	};
}

/**
 * Options for sending a result as HTTP response.
 */
export interface SendResultOptions<T, R> {
	/** Status code for success (default: 200) */
	successStatus?: number;
	/** Transform success value before sending */
	transform?: (value: T) => R;
}

/**
 * Send a Result as an HTTP response.
 *
 * On success, sends the value (optionally transformed) with the success status.
 * On failure, maps the error to an appropriate HTTP status and error response.
 *
 * @param reply - Fastify reply
 * @param result - The Result to send
 * @param options - Response options
 * @returns HTTP response
 *
 * @example
 * ```typescript
 * fastify.post('/api/users', async (request, reply) => {
 *     const ctx = request.executionContext;
 *     const result = await createUserUseCase.execute(command, ctx);
 *     return sendResult(reply, result, {
 *         successStatus: 201,
 *         transform: (event) => ({ userId: event.userId }),
 *     });
 * });
 * ```
 */
export function sendResult<T, R = T>(
	reply: FastifyReply,
	result: Result<T>,
	options: SendResultOptions<T, R> = {},
): FastifyReply {
	const { successStatus = 200, transform } = options;

	if (Result.isSuccess(result)) {
		const value = transform ? transform(result.value) : result.value;
		return reply.status(successStatus).send(value);
	}

	const status = getErrorStatus(result.error);
	const response = toErrorResponse(result.error);
	return reply.status(status).send(response);
}

/**
 * Match on a Result and return appropriate HTTP responses.
 *
 * More flexible than sendResult - allows custom handling for success and failure.
 *
 * @param reply - Fastify reply
 * @param result - The Result to match
 * @param onSuccess - Handler for success case
 * @param onFailure - Optional handler for failure case (default: toErrorResponse)
 * @returns HTTP response
 *
 * @example
 * ```typescript
 * fastify.get('/api/users/:id', async (request, reply) => {
 *     const result = await userOperations.findById(request.params.id);
 *     return matchResult(reply, result,
 *         (user) => reply.send(toUserDto(user)),
 *         (error) => {
 *             // Custom error handling
 *             if (error.code === 'USER_NOT_FOUND') {
 *                 return reply.status(404).send({ error: 'User not found' });
 *             }
 *             return reply.status(getErrorStatus(error)).send(toErrorResponse(error));
 *         }
 *     );
 * });
 * ```
 */
export function matchResult<T>(
	reply: FastifyReply,
	result: Result<T>,
	onSuccess: (value: T, reply: FastifyReply) => FastifyReply,
	onFailure?: (error: UseCaseError, reply: FastifyReply) => FastifyReply,
): FastifyReply {
	if (Result.isSuccess(result)) {
		return onSuccess(result.value, reply);
	}

	if (onFailure) {
		return onFailure(result.error, reply);
	}

	const status = getErrorStatus(result.error);
	const response = toErrorResponse(result.error);
	return reply.status(status).send(response);
}

/**
 * Create a success JSON response.
 *
 * @param reply - Fastify reply
 * @param data - Response data
 * @param status - HTTP status (default: 200)
 * @returns HTTP response
 */
export function jsonSuccess<T>(reply: FastifyReply, data: T, status: number = 200): FastifyReply {
	return reply.status(status).send(data);
}

/**
 * Create a created (201) JSON response.
 *
 * @param reply - Fastify reply
 * @param data - Response data
 * @returns HTTP response
 */
export function jsonCreated<T>(reply: FastifyReply, data: T): FastifyReply {
	return reply.status(201).send(data);
}

/**
 * Create a no content (204) response.
 *
 * @param reply - Fastify reply
 * @returns HTTP response
 */
export function noContent(reply: FastifyReply): FastifyReply {
	return reply.status(204).send();
}

/**
 * Create an error JSON response.
 *
 * @param reply - Fastify reply
 * @param status - HTTP status code
 * @param code - Error code
 * @param message - Error message
 * @param details - Optional error details
 * @returns HTTP response
 */
export function jsonError(
	reply: FastifyReply,
	status: number,
	code: string,
	message: string,
	details?: Record<string, unknown>,
): FastifyReply {
	const response: ErrorResponse = {
		code,
		message,
		...(details ? { details } : {}),
	};
	return reply.status(status).send(response);
}

/**
 * Create a not found (404) error response.
 *
 * @param reply - Fastify reply
 * @param message - Error message (default: 'Not found')
 * @returns HTTP response
 */
export function notFound(reply: FastifyReply, message: string = 'Not found'): FastifyReply {
	return jsonError(reply, 404, 'NOT_FOUND', message);
}

/**
 * Create an unauthorized (401) error response.
 *
 * @param reply - Fastify reply
 * @param message - Error message (default: 'Authentication required')
 * @returns HTTP response
 */
export function unauthorized(reply: FastifyReply, message: string = 'Authentication required'): FastifyReply {
	return jsonError(reply, 401, 'UNAUTHORIZED', message);
}

/**
 * Create a forbidden (403) error response.
 *
 * @param reply - Fastify reply
 * @param message - Error message (default: 'Access denied')
 * @returns HTTP response
 */
export function forbidden(reply: FastifyReply, message: string = 'Access denied'): FastifyReply {
	return jsonError(reply, 403, 'FORBIDDEN', message);
}

/**
 * Create a bad request (400) error response.
 *
 * @param reply - Fastify reply
 * @param message - Error message
 * @param details - Optional validation details
 * @returns HTTP response
 */
export function badRequest(reply: FastifyReply, message: string, details?: Record<string, unknown>): FastifyReply {
	return jsonError(reply, 400, 'BAD_REQUEST', message, details);
}
