package tech.flowcatalyst.eventtype.events;

import tech.flowcatalyst.platform.common.DomainEvent;

/**
 * Sealed hierarchy for all EventType domain events.
 *
 * <p>Enables compiler-enforced exhaustive handling of event type events
 * via pattern matching.
 */
public sealed interface EventTypeEvent extends DomainEvent
    permits EventTypeCreated, EventTypeUpdated, EventTypeArchived,
            EventTypeDeleted, SchemaAdded, SchemaDeprecated,
            SchemaFinalised, EventTypesSynced {}
