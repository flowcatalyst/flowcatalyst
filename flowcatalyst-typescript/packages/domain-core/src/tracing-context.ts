/**
 * Tracing Context
 *
 * Request-scoped context for distributed tracing. Uses AsyncLocalStorage
 * for automatic context propagation across async boundaries.
 *
 * This context holds correlation and causation IDs for the current request.
 * It can be populated from:
 * - HTTP headers via middleware
 * - Background job context via `runWithContext`
 * - Event-driven context when processing domain events
 *
 * Standard HTTP headers:
 * - `X-Correlation-ID` - Traces a request across services
 * - `X-Causation-ID` - References the event that caused this request
 */

import { AsyncLocalStorage } from 'node:async_hooks';
import { generateRaw } from '@flowcatalyst/tsid';

/**
 * Tracing context data stored in AsyncLocalStorage.
 */
export interface TracingContextData {
	correlationId: string | null;
	causationId: string | null;
}

/**
 * AsyncLocalStorage instance for tracing context.
 */
const storage = new AsyncLocalStorage<TracingContextData>();

/**
 * Generate a trace ID.
 */
function generateTraceId(): string {
	return `trace-${generateRaw()}`;
}

/**
 * TracingContext provides distributed tracing context management.
 *
 * Uses AsyncLocalStorage for automatic propagation across async boundaries.
 */
export const TracingContext = {
	/**
	 * HTTP header names for tracing.
	 */
	CORRELATION_ID_HEADER: 'X-Correlation-ID',
	CAUSATION_ID_HEADER: 'X-Causation-ID',

	/**
	 * Get the current tracing context, or null if not in a context.
	 */
	current(): TracingContextData | null {
		return storage.getStore() ?? null;
	},

	/**
	 * Get the current tracing context, throwing if not available.
	 *
	 * Use this in code paths that require tracing context.
	 *
	 * @throws Error if no tracing context is available
	 */
	requireCurrent(): TracingContextData {
		const ctx = storage.getStore();
		if (!ctx) {
			throw new Error(
				'No TracingContext available. Ensure the request is running within ' +
					'TracingContext.runWithContext() or tracing middleware.',
			);
		}
		return ctx;
	},

	/**
	 * Get the correlation ID for the current context.
	 * If not set, generates a new one.
	 */
	getCorrelationId(): string {
		const ctx = storage.getStore();
		if (!ctx) {
			return generateTraceId();
		}
		if (!ctx.correlationId) {
			ctx.correlationId = generateTraceId();
		}
		return ctx.correlationId;
	},

	/**
	 * Get the causation ID for the current context.
	 * May be null if this is a fresh request (not caused by an event).
	 */
	getCausationId(): string | null {
		const ctx = storage.getStore();
		return ctx?.causationId ?? null;
	},

	/**
	 * Check if a correlation ID has been explicitly set.
	 */
	hasCorrelationId(): boolean {
		const ctx = storage.getStore();
		return ctx?.correlationId !== null && ctx?.correlationId !== undefined;
	},

	/**
	 * Check if a causation ID has been set.
	 */
	hasCausationId(): boolean {
		const ctx = storage.getStore();
		return ctx?.causationId !== null && ctx?.causationId !== undefined;
	},

	/**
	 * Run a function with a specific tracing context.
	 *
	 * The context is automatically propagated to all async operations
	 * within the callback.
	 *
	 * @example
	 * ```typescript
	 * await TracingContext.runWithContext(correlationId, causationId, async () => {
	 *     // TracingContext.getCorrelationId() returns correlationId here
	 *     await someAsyncOperation();
	 *     // Context is still available in nested async calls
	 * });
	 * ```
	 *
	 * @param correlationId - The correlation ID to use
	 * @param causationId - The causation ID to use (may be null)
	 * @param fn - The function to run
	 * @returns The result of the function
	 */
	runWithContext<T>(correlationId: string | null, causationId: string | null, fn: () => T): T {
		const ctx: TracingContextData = {
			correlationId,
			causationId,
		};
		return storage.run(ctx, fn);
	},

	/**
	 * Run an async function with a specific tracing context.
	 *
	 * @param correlationId - The correlation ID to use
	 * @param causationId - The causation ID to use (may be null)
	 * @param fn - The async function to run
	 * @returns A promise that resolves to the result of the function
	 */
	async runWithContextAsync<T>(
		correlationId: string | null,
		causationId: string | null,
		fn: () => Promise<T>,
	): Promise<T> {
		const ctx: TracingContextData = {
			correlationId,
			causationId,
		};
		return storage.run(ctx, fn);
	},

	/**
	 * Run a function continuing from a parent event's context.
	 *
	 * The parent event's correlationId is preserved, and its eventId
	 * becomes the causationId.
	 *
	 * @param parentCorrelationId - The parent event's correlation ID
	 * @param parentEventId - The parent event's ID (becomes causation ID)
	 * @param fn - The function to run
	 * @returns The result of the function
	 */
	runFromEvent<T>(parentCorrelationId: string, parentEventId: string, fn: () => T): T {
		return TracingContext.runWithContext(parentCorrelationId, parentEventId, fn);
	},

	/**
	 * Create a new context derived from the current one.
	 *
	 * Useful for creating child spans while preserving correlation.
	 *
	 * @param causationId - The new causation ID (e.g., from a parent event)
	 * @returns A new context data object
	 */
	deriveContext(causationId: string): TracingContextData {
		const current = storage.getStore();
		return {
			correlationId: current?.correlationId ?? generateTraceId(),
			causationId,
		};
	},

	/**
	 * Create context data from HTTP headers.
	 *
	 * @param headers - Object containing HTTP headers
	 * @returns Tracing context data
	 */
	fromHeaders(headers: Record<string, string | string[] | undefined>): TracingContextData {
		const getHeader = (name: string): string | null => {
			const value = headers[name] ?? headers[name.toLowerCase()];
			if (Array.isArray(value)) {
				return value[0] ?? null;
			}
			return value ?? null;
		};

		return {
			correlationId: getHeader(TracingContext.CORRELATION_ID_HEADER),
			causationId: getHeader(TracingContext.CAUSATION_ID_HEADER),
		};
	},

	/**
	 * Get headers to propagate the current tracing context.
	 *
	 * @returns Headers object for outgoing requests
	 */
	toHeaders(): Record<string, string> {
		const headers: Record<string, string> = {};
		const correlationId = TracingContext.getCorrelationId();
		if (correlationId) {
			headers[TracingContext.CORRELATION_ID_HEADER] = correlationId;
		}
		const causationId = TracingContext.getCausationId();
		if (causationId) {
			headers[TracingContext.CAUSATION_ID_HEADER] = causationId;
		}
		return headers;
	},
};
