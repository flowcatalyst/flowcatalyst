package tech.flowcatalyst.dispatchpool.events;

import tech.flowcatalyst.platform.common.DomainEvent;

/**
 * Sealed hierarchy for all DispatchPool domain events.
 *
 * <p>Enables compiler-enforced exhaustive handling of dispatch pool events
 * via pattern matching.
 */
public sealed interface DispatchPoolEvent extends DomainEvent
    permits DispatchPoolCreated, DispatchPoolUpdated, DispatchPoolDeleted,
            DispatchPoolsSynced {}
