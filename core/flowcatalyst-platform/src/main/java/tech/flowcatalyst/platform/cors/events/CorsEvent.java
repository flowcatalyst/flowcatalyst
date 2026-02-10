package tech.flowcatalyst.platform.cors.events;

import tech.flowcatalyst.platform.common.DomainEvent;

/**
 * Sealed hierarchy for all CORS domain events.
 *
 * <p>Enables compiler-enforced exhaustive handling of CORS events
 * via pattern matching.
 */
public sealed interface CorsEvent extends DomainEvent
    permits CorsOriginAdded, CorsOriginDeleted {}
