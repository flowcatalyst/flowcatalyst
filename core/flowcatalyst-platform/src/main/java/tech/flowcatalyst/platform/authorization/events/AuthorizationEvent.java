package tech.flowcatalyst.platform.authorization.events;

import tech.flowcatalyst.platform.common.DomainEvent;

/**
 * Sealed hierarchy for all Authorization domain events.
 *
 * <p>Enables compiler-enforced exhaustive handling of authorization/role events
 * via pattern matching.
 */
public sealed interface AuthorizationEvent extends DomainEvent
    permits RoleCreated, RoleUpdated, RoleDeleted, RolesSynced {}
