/**
 * Domain Event Interface
 *
 * Base interface for all domain events. Domain events represent facts about
 * what happened in the domain (past tense). Each event has its own schema
 * and is stored in the event store.
 *
 * Events follow the CloudEvents specification structure with additional
 * fields for tracing and ordering.
 *
 * Naming convention: Events are named in past tense describing what happened.
 * - `EventTypeCreated` (not CreateEventType)
 * - `SchemaFinalised` (not FinaliseSchema)
 * - `ApplicationActivated` (not ActivateApplication)
 *
 * Implementation note: Events should be implemented as readonly objects
 * for immutability.
 */

import { generate, type EntityTypeKey } from '@flowcatalyst/tsid';
import type { ExecutionContext } from './execution-context.js';

/**
 * Base interface for all domain events.
 */
export interface DomainEvent {
	/** Unique identifier for this event (TSID Crockford Base32 string) */
	readonly eventId: string;

	/**
	 * Event type code following the format: {app}:{domain}:{aggregate}:{action}
	 * Example: "platform:control-plane:eventtype:created"
	 */
	readonly eventType: string;

	/**
	 * Schema version of this event type (e.g., "1.0").
	 * Used for event versioning and schema evolution.
	 */
	readonly specVersion: string;

	/**
	 * Source system that generated this event.
	 * Example: "platform:control-plane"
	 */
	readonly source: string;

	/**
	 * Qualified aggregate identifier.
	 * Format: {domain}.{aggregate}.{id}
	 * Example: "platform.eventtype.0HZXEQ5Y8JY5Z"
	 */
	readonly subject: string;

	/** When the event occurred */
	readonly time: Date;

	/**
	 * Execution ID for tracking a single use case execution.
	 * All events from the same use case execution share this ID.
	 */
	readonly executionId: string;

	/**
	 * Correlation ID for distributed tracing.
	 * Typically the original request ID that started the chain.
	 */
	readonly correlationId: string;

	/**
	 * ID of the event that caused this event (if any).
	 * Used to build causal chains between events.
	 */
	readonly causationId: string | null;

	/** Principal who initiated the action that produced this event */
	readonly principalId: string;

	/**
	 * Message group for ordering guarantees.
	 * Events in the same message group are processed in order.
	 * Example: "platform:eventtype:0HZXEQ5Y8JY5Z"
	 */
	readonly messageGroup: string;

	/**
	 * Serialize the event-specific data payload to JSON.
	 * This contains the domain-specific fields of the event.
	 */
	toDataJson(): string;
}

/**
 * Base fields required to construct a domain event.
 */
export interface DomainEventBase {
	eventType: string;
	specVersion: string;
	source: string;
	subject: string;
	messageGroup: string;
}

/**
 * Helper to create domain event metadata from execution context.
 */
export interface DomainEventMetadata {
	eventId: string;
	executionId: string;
	correlationId: string;
	causationId: string | null;
	principalId: string;
	time: Date;
}

/**
 * DomainEvent factory helpers.
 */
export const DomainEvent = {
	/**
	 * Generate a new event ID.
	 */
	generateId(): string {
		return generate('EVENT');
	},

	/**
	 * Create event metadata from an execution context.
	 *
	 * @param ctx - The execution context
	 * @returns Event metadata
	 */
	metadataFrom(ctx: ExecutionContext): DomainEventMetadata {
		return {
			eventId: generate('EVENT'),
			executionId: ctx.executionId,
			correlationId: ctx.correlationId,
			causationId: ctx.causationId,
			principalId: ctx.principalId,
			time: new Date(),
		};
	},

	/**
	 * Create a subject string for an aggregate.
	 *
	 * @param domain - The domain name (e.g., "platform")
	 * @param aggregate - The aggregate type (e.g., "eventtype")
	 * @param id - The aggregate ID
	 * @returns The subject string (e.g., "platform.eventtype.0HZXEQ5Y8JY5Z")
	 */
	subject(domain: string, aggregate: string, id: string): string {
		return `${domain}.${aggregate}.${id}`;
	},

	/**
	 * Create a message group string for an aggregate.
	 *
	 * @param domain - The domain name (e.g., "platform")
	 * @param aggregate - The aggregate type (e.g., "eventtype")
	 * @param id - The aggregate ID
	 * @returns The message group string (e.g., "platform:eventtype:0HZXEQ5Y8JY5Z")
	 */
	messageGroup(domain: string, aggregate: string, id: string): string {
		return `${domain}:${aggregate}:${id}`;
	},

	/**
	 * Create an event type code.
	 *
	 * @param app - The application name (e.g., "platform")
	 * @param domain - The domain name (e.g., "control-plane")
	 * @param aggregate - The aggregate type (e.g., "eventtype")
	 * @param action - The action in past tense (e.g., "created")
	 * @returns The event type code (e.g., "platform:control-plane:eventtype:created")
	 */
	eventType(app: string, domain: string, aggregate: string, action: string): string {
		return `${app}:${domain}:${aggregate}:${action}`;
	},

	/**
	 * Extract the aggregate type from a subject string.
	 *
	 * Subject format: "platform.eventtype.123456789"
	 * Returns: "Eventtype" (capitalized)
	 */
	extractAggregateType(subject: string): string {
		if (!subject) {
			return 'Unknown';
		}
		const parts = subject.split('.');
		if (parts.length >= 2) {
			const aggregateType = parts[1]!;
			return aggregateType.charAt(0).toUpperCase() + aggregateType.slice(1).replace(/-/g, '');
		}
		return 'Unknown';
	},

	/**
	 * Extract the entity ID from a subject string.
	 *
	 * Subject format: "platform.eventtype.123456789"
	 * Returns: "123456789"
	 */
	extractEntityId(subject: string): string | null {
		if (!subject) {
			return null;
		}
		const parts = subject.split('.');
		if (parts.length >= 3) {
			return parts[2]!;
		}
		return null;
	},
};

/**
 * Abstract base class for domain events that provides common functionality.
 *
 * Extend this class to create concrete domain events:
 *
 * @example
 * ```typescript
 * interface EventTypeCreatedData {
 *     eventTypeId: string;
 *     code: string;
 *     name: string;
 * }
 *
 * class EventTypeCreated extends BaseDomainEvent<EventTypeCreatedData> {
 *     constructor(ctx: ExecutionContext, data: EventTypeCreatedData) {
 *         super({
 *             eventType: 'platform:control-plane:eventtype:created',
 *             specVersion: '1.0',
 *             source: 'platform:control-plane',
 *             subject: DomainEvent.subject('platform', 'eventtype', data.eventTypeId),
 *             messageGroup: DomainEvent.messageGroup('platform', 'eventtype', data.eventTypeId),
 *         }, ctx, data);
 *     }
 * }
 * ```
 */
export abstract class BaseDomainEvent<TData extends Record<string, unknown>> implements DomainEvent {
	readonly eventId: string;
	readonly eventType: string;
	readonly specVersion: string;
	readonly source: string;
	readonly subject: string;
	readonly time: Date;
	readonly executionId: string;
	readonly correlationId: string;
	readonly causationId: string | null;
	readonly principalId: string;
	readonly messageGroup: string;

	protected readonly data: TData;

	constructor(base: DomainEventBase, ctx: ExecutionContext, data: TData) {
		const metadata = DomainEvent.metadataFrom(ctx);

		this.eventId = metadata.eventId;
		this.eventType = base.eventType;
		this.specVersion = base.specVersion;
		this.source = base.source;
		this.subject = base.subject;
		this.time = metadata.time;
		this.executionId = metadata.executionId;
		this.correlationId = metadata.correlationId;
		this.causationId = metadata.causationId;
		this.principalId = metadata.principalId;
		this.messageGroup = base.messageGroup;
		this.data = data;
	}

	toDataJson(): string {
		return JSON.stringify(this.data);
	}

	/**
	 * Get the event-specific data payload.
	 */
	getData(): TData {
		return this.data;
	}
}
