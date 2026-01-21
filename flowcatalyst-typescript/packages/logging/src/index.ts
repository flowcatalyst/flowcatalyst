import pino, { type Logger, type LoggerOptions } from 'pino';

export type { Logger } from 'pino';

/**
 * Log levels supported by the logger
 */
export const LogLevel = {
	TRACE: 'trace',
	DEBUG: 'debug',
	INFO: 'info',
	WARN: 'warn',
	ERROR: 'error',
	FATAL: 'fatal',
} as const;

export type LogLevel = (typeof LogLevel)[keyof typeof LogLevel];

/**
 * Logger configuration options
 */
export interface LoggerConfig {
	/** Log level */
	level: LogLevel;
	/** Service name for structured logs */
	serviceName: string;
	/** Whether to use pretty printing (dev only) */
	pretty?: boolean;
	/** Additional base context */
	base?: Record<string, unknown>;
}

/**
 * Create a configured Pino logger instance
 */
export function createLogger(config: LoggerConfig): Logger {
	const options: LoggerOptions = {
		level: config.level,
		base: {
			service: config.serviceName,
			...config.base,
		},
		timestamp: pino.stdTimeFunctions.isoTime,
		formatters: {
			level: (label) => ({ level: label }),
		},
	};

	// Use pino-pretty for development
	if (config.pretty) {
		return pino({
			...options,
			transport: {
				target: 'pino-pretty',
				options: {
					colorize: true,
					translateTime: 'SYS:standard',
					ignore: 'pid,hostname',
				},
			},
		});
	}

	return pino(options);
}

/**
 * Create a child logger with additional context
 */
export function createChildLogger(parent: Logger, bindings: Record<string, unknown>): Logger {
	return parent.child(bindings);
}

/**
 * Default logger instance (can be replaced)
 */
let defaultLogger: Logger = pino({ level: 'info' });

/**
 * Set the default logger instance
 */
export function setDefaultLogger(logger: Logger): void {
	defaultLogger = logger;
}

/**
 * Get the default logger instance
 */
export function getLogger(): Logger {
	return defaultLogger;
}

/**
 * Request context for structured logging
 */
export interface RequestContext {
	requestId: string;
	method?: string;
	path?: string;
	[key: string]: unknown;
}

/**
 * Message context for queue processing logs
 */
export interface MessageContext {
	queueMessageId: string;
	appMessageId: string;
	poolCode: string;
	messageGroupId: string;
	batchId: string;
	[key: string]: unknown;
}
