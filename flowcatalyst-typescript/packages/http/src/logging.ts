/**
 * Structured Logging
 *
 * Pino-based structured logging with automatic tracing context injection.
 * Creates request-scoped loggers that include correlation and execution IDs.
 *
 * Fastify uses Pino natively, so this module provides utilities for
 * configuring logging and creating child loggers with tracing context.
 */

import pino, { type Logger, type LoggerOptions } from 'pino';
import type { TracingData } from './types.js';

/**
 * Configuration for logging.
 */
export interface LoggingConfig {
	/** Log level (default: 'info') */
	readonly level?: string;
	/** Service name for log context */
	readonly serviceName?: string;
	/** Paths to skip logging (e.g., /health) */
	readonly skipPaths?: string[];
	/** Additional base context to include in all logs */
	readonly baseContext?: Record<string, unknown>;
	/** Custom Pino options */
	readonly pinoOptions?: LoggerOptions;
}

/**
 * Create the base logger instance.
 *
 * @param config - Logging configuration
 * @returns Pino logger instance
 *
 * @example
 * ```typescript
 * const logger = createLogger({
 *     level: 'info',
 *     serviceName: 'platform',
 * });
 *
 * logger.info({ userId: '123' }, 'User created');
 * ```
 */
export function createLogger(config: LoggingConfig = {}): Logger {
	const { level = 'info', serviceName, baseContext = {}, pinoOptions = {} } = config;

	const options: LoggerOptions = {
		level,
		...pinoOptions,
		base: {
			...(serviceName ? { service: serviceName } : {}),
			...baseContext,
			...pinoOptions.base,
		},
	};

	return pino(options);
}

/**
 * Create a child logger with tracing context.
 *
 * @param baseLogger - The base Pino logger
 * @param tracing - Tracing data from request context
 * @returns Child logger with tracing fields
 */
export function createRequestLogger(baseLogger: Logger, tracing: TracingData): Logger {
	return baseLogger.child({
		correlationId: tracing.correlationId,
		executionId: tracing.executionId,
		...(tracing.causationId ? { causationId: tracing.causationId } : {}),
	});
}

/**
 * Create Fastify logger options.
 *
 * Fastify uses Pino internally, so this creates compatible logger options.
 *
 * @param config - Logging configuration
 * @returns Pino logger options for Fastify
 *
 * @example
 * ```typescript
 * import Fastify from 'fastify';
 * import { createFastifyLoggerOptions } from '@flowcatalyst/http';
 *
 * const fastify = Fastify({
 *     logger: createFastifyLoggerOptions({
 *         level: 'info',
 *         serviceName: 'platform',
 *     }),
 * });
 * ```
 */
export function createFastifyLoggerOptions(config: LoggingConfig = {}): LoggerOptions | boolean {
	const { level = 'info', serviceName, baseContext = {}, pinoOptions = {} } = config;

	return {
		level,
		...pinoOptions,
		base: {
			...(serviceName ? { service: serviceName } : {}),
			...baseContext,
			...pinoOptions.base,
		},
	};
}

export type { Logger, LoggerOptions };
