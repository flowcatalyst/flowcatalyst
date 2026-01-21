/**
 * Tracing Plugin
 *
 * Fastify plugin for distributed tracing. Extracts correlation and causation IDs
 * from request headers and propagates them to response headers.
 *
 * This plugin integrates with TracingContext from @flowcatalyst/domain-core
 * for consistent tracing across HTTP and background processing.
 */

import type { FastifyPluginAsync } from 'fastify';
import fp from 'fastify-plugin';
import { generateRaw } from '@flowcatalyst/tsid';
import type { TracingPluginOptions, TracingData } from '../types.js';

/** Default header names */
const DEFAULT_CORRELATION_ID_HEADER = 'x-correlation-id';
const DEFAULT_REQUEST_ID_HEADER = 'x-request-id';
const DEFAULT_CAUSATION_ID_HEADER = 'x-causation-id';

/**
 * Tracing plugin for Fastify.
 *
 * Extracts correlation ID from request headers (X-Correlation-ID or X-Request-ID),
 * generates one if not present, and adds it to response headers.
 *
 * @example
 * ```typescript
 * import Fastify from 'fastify';
 * import { tracingPlugin } from '@flowcatalyst/http';
 *
 * const fastify = Fastify();
 * await fastify.register(tracingPlugin);
 *
 * fastify.get('/api/users', (request, reply) => {
 *     const { correlationId, executionId } = request.tracing;
 *     console.log(`Request ${executionId} in trace ${correlationId}`);
 *     return { users: [] };
 * });
 * ```
 */
const tracingPluginAsync: FastifyPluginAsync<TracingPluginOptions> = async (fastify, opts) => {
	const {
		correlationIdHeader = DEFAULT_CORRELATION_ID_HEADER,
		requestIdHeader = DEFAULT_REQUEST_ID_HEADER,
		causationIdHeader = DEFAULT_CAUSATION_ID_HEADER,
		propagateToResponse = true,
	} = opts;

	// Decorate request with tracing data (using getter/setter pattern)
	fastify.decorateRequest('tracing', {
		getter() {
			return (this as unknown as { _tracing: TracingData })._tracing;
		},
		setter(value: TracingData) {
			(this as unknown as { _tracing: TracingData })._tracing = value;
		},
	});

	// Add tracing data to each request
	fastify.addHook('onRequest', async (request, reply) => {
		// Extract correlation ID from headers (case-insensitive), or generate one
		let correlationId =
			(request.headers[correlationIdHeader] as string | undefined) ??
			(request.headers[requestIdHeader] as string | undefined);

		if (!correlationId) {
			correlationId = `trace-${generateRaw()}`;
		}

		// Extract causation ID from headers (may be null)
		const causationId = (request.headers[causationIdHeader] as string | undefined) ?? null;

		// Generate unique execution ID for this request
		const executionId = `exec-${generateRaw()}`;

		// Store tracing data in request
		const tracingData: TracingData = {
			correlationId,
			causationId,
			executionId,
			startTime: Date.now(),
		};
		request.tracing = tracingData;

		// Add correlation ID to response headers
		if (propagateToResponse) {
			reply.header(correlationIdHeader, correlationId);
		}
	});
};

export const tracingPlugin = fp(tracingPluginAsync, {
	name: '@flowcatalyst/tracing',
	fastify: '5.x',
});

/**
 * Get tracing data from request, throwing if not available.
 *
 * @param request - Fastify request
 * @returns Tracing data
 * @throws Error if tracing plugin has not been applied
 */
export function requireTracing(request: { tracing?: TracingData }): TracingData {
	const tracing = request.tracing;
	if (!tracing) {
		throw new Error('Tracing context not available. Ensure tracingPlugin is registered.');
	}
	return tracing;
}

/**
 * Get tracing headers for outgoing requests.
 * Use this when making HTTP calls to other services to propagate tracing.
 *
 * @param tracing - Tracing data from current request
 * @returns Headers object for outgoing request
 */
export function getTracingHeaders(tracing: TracingData): Record<string, string> {
	const headers: Record<string, string> = {
		[DEFAULT_CORRELATION_ID_HEADER]: tracing.correlationId,
	};

	// For outgoing requests, the current execution becomes the causation
	headers[DEFAULT_CAUSATION_ID_HEADER] = tracing.executionId;

	return headers;
}
