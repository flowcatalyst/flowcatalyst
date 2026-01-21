package tech.flowcatalyst.subscription;

/**
 * Binding of an event type to a subscription, including the spec version to use.
 *
 * @param eventTypeId The event type ID
 * @param eventTypeCode The event type code (denormalized for queries)
 * @param specVersion The spec version of the event type to bind to
 */
public record EventTypeBinding(
    String eventTypeId,
    String eventTypeCode,
    String specVersion
) {}
