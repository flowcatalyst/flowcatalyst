package tech.flowcatalyst.platform.client.events;

import tech.flowcatalyst.platform.common.DomainEvent;

/**
 * Sealed hierarchy for all Client domain events.
 *
 * <p>Enables compiler-enforced exhaustive handling of client/auth-config events
 * via pattern matching.
 */
public sealed interface ClientEvent extends DomainEvent
    permits AuthConfigAdditionalClientsUpdated, AuthConfigGrantedClientsUpdated {}
