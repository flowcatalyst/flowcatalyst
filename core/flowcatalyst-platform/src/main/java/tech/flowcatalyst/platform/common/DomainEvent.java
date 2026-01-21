package tech.flowcatalyst.platform.common;

import java.time.Instant;

/**
 * Base interface for all domain events.
 *
 * <p>Domain events represent facts about what happened in the domain (past tense).
 * Each event has its own schema and is stored in the event store.
 *
 * <p>Events follow the CloudEvents specification structure with additional
 * fields for tracing and ordering.
 *
 * <p>Naming convention: Events are named in past tense describing what happened.
 * <ul>
 *   <li>{@code EventTypeCreated} (not CreateEventType)</li>
 *   <li>{@code SchemaFinalised} (not FinaliseSchema)</li>
 *   <li>{@code ApplicationActivated} (not ActivateApplication)</li>
 * </ul>
 *
 * <p>Implementation note: Events should be implemented as Java records for
 * immutability and automatic equals/hashCode/toString.
 *
 * @see tech.flowcatalyst.event.Event The MongoDB event entity
 */
public interface DomainEvent {

    /**
     * Unique identifier for this event (TSID Crockford Base32 string).
     */
    String eventId();

    /**
     * Event type code following the format: {app}:{domain}:{aggregate}:{action}
     * <p>Example: "platform:control-plane:eventtype:created"
     */
    String eventType();

    /**
     * Schema version of this event type (e.g., "1.0").
     * <p>Used for event versioning and schema evolution.
     */
    String specVersion();

    /**
     * Source system that generated this event.
     * <p>Example: "platform:control-plane"
     */
    String source();

    /**
     * Qualified aggregate identifier.
     * <p>Format: {domain}.{aggregate}.{id}
     * <p>Example: "platform.eventtype.123456789"
     */
    String subject();

    /**
     * When the event occurred.
     */
    Instant time();

    /**
     * Execution ID for tracking a single use case execution.
     * <p>All events from the same use case execution share this ID.
     */
    String executionId();

    /**
     * Correlation ID for distributed tracing.
     * <p>Typically the original request ID that started the chain.
     */
    String correlationId();

    /**
     * ID of the event that caused this event (if any).
     * <p>Used to build causal chains between events.
     */
    String causationId();

    /**
     * Principal who initiated the action that produced this event.
     */
    String principalId();

    /**
     * Message group for ordering guarantees.
     * <p>Events in the same message group are processed in order.
     * <p>Example: "platform:eventtype:123456789"
     */
    String messageGroup();

    /**
     * Serialize the event-specific data payload to JSON.
     * <p>This contains the domain-specific fields of the event.
     */
    String toDataJson();
}
