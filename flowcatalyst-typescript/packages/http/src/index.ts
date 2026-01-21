/**
 * @flowcatalyst/http
 *
 * HTTP layer utilities for FlowCatalyst platform using Fastify:
 * - Plugins for tracing, authentication, and execution context
 * - Result to HTTP response mapping
 * - OpenAPI/Zod schema utilities
 * - Structured logging with Pino (Fastify's native logger)
 *
 * @example
 * ```typescript
 * import Fastify from 'fastify';
 * import cookie from '@fastify/cookie';
 * import cors from '@fastify/cors';
 * import {
 *     tracingPlugin,
 *     auditPlugin,
 *     executionContextPlugin,
 *     errorHandlerPlugin,
 *     createStandardErrorHandlerOptions,
 *     createFastifyLoggerOptions,
 *     sendResult,
 * } from '@flowcatalyst/http';
 *
 * // Create Fastify app with logging
 * const fastify = Fastify({
 *     logger: createFastifyLoggerOptions({ serviceName: 'platform' }),
 * });
 *
 * // Register plugins
 * await fastify.register(cookie);
 * await fastify.register(cors, { origin: true, credentials: true });
 * await fastify.register(tracingPlugin);
 * await fastify.register(auditPlugin, {
 *     validateToken: async (token) => validateJwt(token),
 * });
 * await fastify.register(executionContextPlugin);
 * await fastify.register(errorHandlerPlugin, createStandardErrorHandlerOptions());
 *
 * // Route with use case
 * fastify.post('/api/users', async (request, reply) => {
 *     const ctx = request.executionContext;
 *     const body = request.body;
 *     const result = await createUserUseCase.execute(body, ctx);
 *     return sendResult(reply, result, { successStatus: 201 });
 * });
 *
 * await fastify.listen({ port: 3000, host: '0.0.0.0' });
 * ```
 */

// Types
export {
	type TracingData,
	type AuditData,
	type TracingPluginOptions,
	type AuditPluginOptions,
	type ErrorResponse,
	type FastifyRequest,
	type FastifyReply,
	type Logger,
} from './types.js';

// Plugins
export {
	tracingPlugin,
	requireTracing,
	getTracingHeaders,
	auditPlugin,
	requireAuth,
	getPrincipalId,
	isAuthenticated,
	requireAuthHook,
	requireRoleHook,
	executionContextPlugin,
	requireExecutionContext,
} from './plugins/index.js';

// Logging
export {
	createLogger,
	createRequestLogger,
	createFastifyLoggerOptions,
	type LoggingConfig,
} from './logging.js';

// Response utilities
export {
	getErrorStatus,
	toErrorResponse,
	sendResult,
	matchResult,
	jsonSuccess,
	jsonCreated,
	noContent,
	jsonError,
	notFound,
	unauthorized,
	forbidden,
	badRequest,
	type SendResultOptions,
} from './response.js';

// Error handler
export {
	errorHandlerPlugin,
	createCommonErrorMappers,
	createStandardErrorHandlerOptions,
	type ErrorHandlerConfig,
	type ErrorMapper,
} from './error-handler.js';

// OpenAPI utilities (TypeBox for native Fastify JSON Schema support)
export {
	CommonSchemas,
	ErrorResponseSchema,
	type ErrorResponseType,
	paginatedResponse,
	entitySchema,
	OpenAPIResponses,
	combineResponses,
	validateBody,
	safeValidate,
	Type,
	Value,
	type Static,
	type TSchema,
	type TObject,
} from './openapi.js';
