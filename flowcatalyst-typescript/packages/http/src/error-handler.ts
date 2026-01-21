/**
 * Error Handler
 *
 * Global error handler plugin for Fastify applications.
 * Catches exceptions and maps them to appropriate HTTP responses.
 */

import type { FastifyPluginAsync, FastifyError } from 'fastify';
import fp from 'fastify-plugin';
import type { ErrorResponse } from './types.js';

/**
 * Configuration for the error handler.
 */
export interface ErrorHandlerConfig {
	/** Whether to include stack traces in responses (default: false) */
	readonly includeStack?: boolean;
	/** Custom error mappers */
	readonly mappers?: ErrorMapper[];
}

/**
 * Custom error mapper function.
 */
export interface ErrorMapper {
	/** Check if this mapper handles the error */
	canHandle: (error: Error) => boolean;
	/** Map the error to an HTTP response */
	toResponse: (error: Error) => { status: number; body: ErrorResponse };
}

/**
 * Create an error handler plugin for Fastify.
 *
 * Handles:
 * - Fastify errors (statusCode property)
 * - Custom errors (via registered mappers)
 * - Unknown errors (returns 500 Internal Server Error)
 *
 * @param config - Error handler configuration
 * @returns Fastify plugin
 *
 * @example
 * ```typescript
 * import Fastify from 'fastify';
 * import { errorHandlerPlugin } from '@flowcatalyst/http';
 *
 * const fastify = Fastify({ logger: true });
 *
 * await fastify.register(errorHandlerPlugin, {
 *     mappers: [
 *         {
 *             canHandle: (e) => e.name === 'ValidationError',
 *             toResponse: (e) => ({
 *                 status: 400,
 *                 body: { code: 'VALIDATION_ERROR', message: e.message },
 *             }),
 *         },
 *     ],
 * });
 * ```
 */
const errorHandlerPluginAsync: FastifyPluginAsync<ErrorHandlerConfig> = async (fastify, opts) => {
	const { includeStack = false, mappers = [] } = opts;

	fastify.setErrorHandler((error: FastifyError, request, reply) => {
		const log = request.log;
		const tracing = request.tracing;

		// Get status code from error if available
		const statusCode = error.statusCode ?? 500;

		// Handle Fastify errors (with statusCode < 500)
		if (statusCode < 500) {
			const body: ErrorResponse = {
				code: `HTTP_${statusCode}`,
				message: error.message || 'An error occurred',
			};
			return reply.status(statusCode).send(body);
		}

		// Try custom mappers
		for (const mapper of mappers) {
			if (mapper.canHandle(error)) {
				const { status, body } = mapper.toResponse(error);

				if (status >= 500) {
					log.error(
						{
							error: error.name,
							message: error.message,
							status,
							...(tracing ? { correlationId: tracing.correlationId } : {}),
						},
						'Mapped error',
					);
				}

				return reply.status(status).send(body);
			}
		}

		// Log unexpected errors
		log.error(
			{
				error: error.name,
				message: error.message,
				stack: error.stack,
				...(tracing ? { correlationId: tracing.correlationId } : {}),
			},
			'Unhandled error',
		);

		// Return generic error response
		const body: ErrorResponse = {
			code: 'INTERNAL_ERROR',
			message: 'An unexpected error occurred',
			...(includeStack && error.stack ? { details: { stack: error.stack } } : {}),
		};

		return reply.status(500).send(body);
	});
};

export const errorHandlerPlugin = fp(errorHandlerPluginAsync, {
	name: '@flowcatalyst/error-handler',
	fastify: '5.x',
});

/**
 * Create common error mappers.
 *
 * @returns Array of common error mappers
 */
export function createCommonErrorMappers(): ErrorMapper[] {
	return [
		// TypeBox validation errors (from Fastify's AJV integration)
		{
			canHandle: (e) => e.name === 'FST_ERR_VALIDATION' || (e as FastifyError).code === 'FST_ERR_VALIDATION',
			toResponse: (e) => {
				const fastifyError = e as FastifyError & { validation?: unknown[] };
				return {
					status: 400,
					body: {
						code: 'VALIDATION_ERROR',
						message: 'Request validation failed',
						...(fastifyError.validation ? { details: { errors: fastifyError.validation } } : {}),
					},
				};
			},
		},
		// JSON parse errors
		{
			canHandle: (e) => e instanceof SyntaxError && e.message.includes('JSON'),
			toResponse: () => ({
				status: 400,
				body: {
					code: 'INVALID_JSON',
					message: 'Invalid JSON in request body',
				},
			}),
		},
	];
}

/**
 * Create the standard error handler plugin with common mappers.
 *
 * @returns Error handler plugin options
 */
export function createStandardErrorHandlerOptions(): ErrorHandlerConfig {
	return {
		mappers: createCommonErrorMappers(),
	};
}
