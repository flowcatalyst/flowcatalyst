package tech.flowcatalyst.event.operations;

import tech.flowcatalyst.event.ContextData;

import java.time.Instant;
import java.util.List;

/**
 * Operation to create a new Event.
 *
 * @param specVersion     The version that corresponds to the event type spec version
 * @param type            The event type code (e.g., "operant:execution:trip:started")
 * @param source          The source URI (typically app identifier)
 * @param subject         Qualified aggregate ID (e.g., "operant.execution.trip.1234")
 * @param time            When the event occurred (defaults to now if null)
 * @param data            String representation of the event data
 * @param correlationId   Optional correlation ID for tracing
 * @param causationId     Optional ID of the causing event
 * @param deduplicationId Optional idempotency key
 * @param messageGroup    Message group for ordering
 * @param contextData     Key-value pairs for search
 */
public record CreateEvent(
    String specVersion,
    String type,
    String source,
    String subject,
    Instant time,
    String data,
    String correlationId,
    String causationId,
    String deduplicationId,
    String messageGroup,
    List<ContextData> contextData
) {
}
