/**
 * Execution Context
 *
 * Context for a use case execution. Carries tracing IDs and principal information
 * through the execution of a use case. This context is used to populate domain
 * event metadata.
 *
 * The execution context enables:
 * - Distributed tracing via correlationId
 * - Causal chain tracking via causationId
 * - Process/saga tracking via executionId
 * - Audit trail via principalId
 */

import { generateRaw } from '@flowcatalyst/tsid';
import { TracingContext } from './tracing-context.js';
import type { DomainEvent } from './domain-event.js';

/**
 * Execution context data.
 */
export interface ExecutionContext {
	/** Unique ID for this execution (generated) */
	readonly executionId: string;
	/** ID for distributed tracing (usually from original request) */
	readonly correlationId: string;
	/** ID of the parent event that caused this execution (if any) */
	readonly causationId: string | null;
	/** ID of the principal performing the action */
	readonly principalId: string;
	/** When the execution was initiated */
	readonly initiatedAt: Date;
}

/**
 * Generate a new execution ID.
 */
function generateExecutionId(): string {
	return `exec-${generateRaw()}`;
}

/**
 * ExecutionContext factory functions.
 */
export const ExecutionContext = {
	/**
	 * Create a new execution context for a fresh request.
	 *
	 * The executionId is generated fresh. If a TracingContext is available
	 * (via AsyncLocalStorage), correlation/causation IDs are taken from it.
	 * Otherwise, a new correlation ID is generated.
	 *
	 * @param principalId - The principal performing the action
	 * @returns A new execution context
	 */
	create(principalId: string): ExecutionContext {
		const tracingCtx = TracingContext.current();

		if (tracingCtx) {
			return ExecutionContext.fromTracingContext(tracingCtx, principalId);
		}

		const execId = generateExecutionId();
		return {
			executionId: execId,
			correlationId: execId, // correlation starts as execution ID for fresh requests
			causationId: null, // no causation for fresh requests
			principalId,
			initiatedAt: new Date(),
		};
	},

	/**
	 * Create an execution context from TracingContext data.
	 *
	 * This is the preferred method when running within an HTTP request
	 * where TracingContext has been populated from headers by middleware.
	 *
	 * @param tracingContext - The tracing context data
	 * @param principalId - The principal performing the action
	 * @returns A new execution context with correlation/causation from tracing context
	 */
	fromTracingContext(
		tracingContext: { correlationId: string | null; causationId: string | null },
		principalId: string,
	): ExecutionContext {
		const execId = generateExecutionId();
		return {
			executionId: execId,
			correlationId: tracingContext.correlationId ?? execId,
			causationId: tracingContext.causationId,
			principalId,
			initiatedAt: new Date(),
		};
	},

	/**
	 * Create a new execution context with a specific correlation ID.
	 *
	 * Use this when you have an existing correlation ID from an
	 * upstream system or request header.
	 *
	 * @param principalId - The principal performing the action
	 * @param correlationId - The correlation ID to use
	 * @returns A new execution context
	 */
	withCorrelation(principalId: string, correlationId: string): ExecutionContext {
		return {
			executionId: generateExecutionId(),
			correlationId,
			causationId: null,
			principalId,
			initiatedAt: new Date(),
		};
	},

	/**
	 * Create a new execution context from a parent event.
	 *
	 * Use this when reacting to an event and creating a new execution.
	 * The parent event's ID becomes the causationId, and the correlationId
	 * is preserved.
	 *
	 * @param parent - The parent event that caused this execution
	 * @param principalId - The principal performing the action
	 * @returns A new execution context linked to the parent event
	 */
	fromParentEvent(parent: DomainEvent, principalId: string): ExecutionContext {
		return {
			executionId: generateExecutionId(),
			correlationId: parent.correlationId,
			causationId: parent.eventId,
			principalId,
			initiatedAt: new Date(),
		};
	},

	/**
	 * Create a child context within the same execution.
	 *
	 * Use this when an execution needs to perform sub-operations
	 * that should share the same executionId but have different causation.
	 *
	 * @param context - The parent execution context
	 * @param causingEventId - The event ID that caused this sub-operation
	 * @returns A new context with the same executionId but new causationId
	 */
	withCausation(context: ExecutionContext, causingEventId: string): ExecutionContext {
		return {
			executionId: context.executionId,
			correlationId: context.correlationId,
			causationId: causingEventId,
			principalId: context.principalId,
			initiatedAt: new Date(),
		};
	},
};
