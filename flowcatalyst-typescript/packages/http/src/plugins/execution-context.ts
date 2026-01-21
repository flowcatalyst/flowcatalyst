/**
 * Execution Context Plugin
 *
 * Creates an ExecutionContext for use case execution by combining
 * tracing and audit context. This plugin should be registered after
 * tracing and audit plugins.
 */

import type { FastifyPluginAsync } from 'fastify';
import fp from 'fastify-plugin';
import { ExecutionContext } from '@flowcatalyst/domain-core';

/**
 * Execution context plugin for Fastify.
 *
 * Combines tracing context (correlation ID, causation ID) with
 * audit context (principal ID) to create an ExecutionContext for
 * use case execution.
 *
 * @example
 * ```typescript
 * import Fastify from 'fastify';
 * import { tracingPlugin, auditPlugin, executionContextPlugin } from '@flowcatalyst/http';
 *
 * const fastify = Fastify();
 *
 * // Apply in order: tracing → audit → executionContext
 * await fastify.register(tracingPlugin);
 * await fastify.register(auditPlugin, { validateToken: ... });
 * await fastify.register(executionContextPlugin);
 *
 * fastify.post('/api/users', async (request, reply) => {
 *     const ctx = request.executionContext;
 *     const result = await createUserUseCase.execute(command, ctx);
 *     // ...
 * });
 * ```
 */
const executionContextPluginAsync: FastifyPluginAsync = async (fastify) => {
	// Decorate request with execution context (using getter/setter pattern)
	fastify.decorateRequest('executionContext', {
		getter() {
			return (this as unknown as { _executionContext: ExecutionContext })._executionContext;
		},
		setter(value: ExecutionContext) {
			(this as unknown as { _executionContext: ExecutionContext })._executionContext = value;
		},
	});

	// Create execution context from tracing and audit data
	fastify.addHook('onRequest', async (request) => {
		const tracing = request.tracing;
		const audit = request.audit;

		if (!tracing) {
			throw new Error('Tracing context not available. Register tracingPlugin before executionContextPlugin.');
		}

		// Create execution context from tracing and audit data
		const executionContext = ExecutionContext.fromTracingContext(
			{
				correlationId: tracing.correlationId,
				causationId: tracing.causationId,
			},
			audit?.principalId ?? 'anonymous',
		);

		request.executionContext = executionContext;
	});
};

export const executionContextPlugin = fp(executionContextPluginAsync, {
	name: '@flowcatalyst/execution-context',
	fastify: '5.x',
	dependencies: ['@flowcatalyst/tracing'],
});

/**
 * Get execution context from Fastify request, throwing if not available.
 *
 * @param request - Fastify request
 * @returns Execution context
 * @throws Error if execution context plugin has not been registered
 */
export function requireExecutionContext(request: {
	executionContext?: ExecutionContext;
}): ExecutionContext {
	const ctx = request.executionContext;
	if (!ctx) {
		throw new Error('ExecutionContext not available. Ensure executionContextPlugin is registered.');
	}
	return ctx;
}
